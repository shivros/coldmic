package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"coldmic/internal/debuglog"
	"coldmic/internal/ports"
)

// Precompiled regexes for stripMarkdown (avoid recompilation on every call).
var (
	reBoldItalic = regexp.MustCompile(`\*{1,3}([^*]+)\*{1,3}`)
	reHeaders    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reCodeBlocks = regexp.MustCompile("```[\\s\\S]*?```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reLinks      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

// maxRetries is the number of retry attempts for transient errors.
const maxRetries = 3

// Config controls the OpenAI-compatible conversation backend.
type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
	Stream       bool
	Timeout      time.Duration
	MaxHistory   int
}

// Provider implements ports.ConversationBackend for OpenAI-compatible APIs.
type Provider struct {
	cfg    Config
	client *http.Client

	mu       sync.Mutex
	sessions map[string]*conversationHistory
}

type conversationHistory struct {
	messages []chatMessage
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the payload sent to the OpenAI Chat Completions API.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// chatResponse is the non-streaming response from the API.
type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// streamChunk is a single SSE data line from the streaming API.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// NewProvider creates a new OpenAI-compatible conversation backend.
func NewProvider(cfg Config) *Provider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 20
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "You are a helpful voice assistant."
	}
	return &Provider{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.Timeout},
		sessions: make(map[string]*conversationHistory),
	}
}

// SendMessage sends a single user message and returns the assistant response.
func (p *Provider) SendMessage(ctx context.Context, sessionID string, text string) (*ports.ConversationResponse, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, errors.New("COLDMIC_BACKEND_API_KEY is not configured")
	}

	p.mu.Lock()
	hist := p.getOrCreateHistory(sessionID)
	hist.messages = append(hist.messages, chatMessage{Role: "user", Content: text})
	p.mu.Unlock()

	messages := p.buildMessages(hist)

	body, err := json.Marshal(chatRequest{
		Model:       p.cfg.Model,
		Messages:    messages,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := p.doRequest(ctx, body, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check COLDMIC_BACKEND_API_KEY", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("backend returned no choices")
	}

	assistantText := stripMarkdown(chatResp.Choices[0].Message.Content)

	p.mu.Lock()
	hist.messages = append(hist.messages, chatMessage{Role: "assistant", Content: assistantText})
	p.truncateHistory(hist)
	p.mu.Unlock()

	debuglog.Printf("conversation-backend send session=%s response_len=%d", sessionID, len(assistantText))

	return &ports.ConversationResponse{
		Text:      assistantText,
		SessionID: sessionID,
		Model:     chatResp.Model,
	}, nil
}

// StreamMessage sends a user message and returns a channel of response chunks.
func (p *Provider) StreamMessage(ctx context.Context, sessionID string, text string) (<-chan ports.StreamChunk, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, errors.New("COLDMIC_BACKEND_API_KEY is not configured")
	}

	p.mu.Lock()
	hist := p.getOrCreateHistory(sessionID)
	hist.messages = append(hist.messages, chatMessage{Role: "user", Content: text})
	messages := p.buildMessages(hist)
	p.mu.Unlock()

	body, err := json.Marshal(chatRequest{
		Model:       p.cfg.Model,
		Messages:    messages,
		Stream:      true,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := p.doRequest(ctx, body, true)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, fmt.Errorf("authentication failed (HTTP %d): check COLDMIC_BACKEND_API_KEY", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan ports.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		var fullText strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- ports.StreamChunk{Text: "", Done: true}
				break
			}

			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				debuglog.Printf("conversation-backend stream unmarshal error: %v", err)
				continue
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					fullText.WriteString(content)
					ch <- ports.StreamChunk{Text: content, Done: false}
				}
				if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "stop" {
					ch <- ports.StreamChunk{Text: "", Done: true}
					break
				}
			}
		}

		// Append the full assistant response to history.
		assistantText := stripMarkdown(fullText.String())
		p.mu.Lock()
		hist.messages = append(hist.messages, chatMessage{Role: "assistant", Content: assistantText})
		p.truncateHistory(hist)
		p.mu.Unlock()

		debuglog.Printf("conversation-backend stream session=%s response_len=%d", sessionID, len(assistantText))
	}()

	return ch, nil
}

// CloseSession releases in-memory history for a session.
func (p *Provider) CloseSession(_ context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
	debuglog.Printf("conversation-backend close session=%s", sessionID)
	return nil
}

func (p *Provider) getOrCreateHistory(sessionID string) *conversationHistory {
	hist, ok := p.sessions[sessionID]
	if !ok {
		hist = &conversationHistory{}
		p.sessions[sessionID] = hist
	}
	return hist
}

func (p *Provider) buildMessages(hist *conversationHistory) []chatMessage {
	msgs := make([]chatMessage, 0, len(hist.messages)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: p.cfg.SystemPrompt})
	msgs = append(msgs, hist.messages...)
	return msgs
}

func (p *Provider) truncateHistory(hist *conversationHistory) {
	limit := p.cfg.MaxHistory * 2 // user + assistant pairs
	if len(hist.messages) > limit {
		hist.messages = hist.messages[len(hist.messages)-limit:]
	}
}

func (p *Provider) doRequest(ctx context.Context, body []byte, stream bool) (*http.Response, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	debuglog.Printf("conversation-backend request endpoint=%s model=%s stream=%t", endpoint, p.cfg.Model, stream)

	return doRequestWithRetry(ctx, p.client, req, maxRetries)
}

// stripMarkdown removes common markdown formatting from text for voice output.
func stripMarkdown(text string) string {
	text = reBoldItalic.ReplaceAllString(text, "$1")
	text = reHeaders.ReplaceAllString(text, "")
	text = reCodeBlocks.ReplaceAllString(text, "")
	text = reInlineCode.ReplaceAllString(text, "$1")
	text = reLinks.ReplaceAllString(text, "$1")
	return strings.TrimSpace(text)
}

// RetryableHTTPDoer is a minimal interface for test doubles.
type RetryableHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// retryable checks if an HTTP status code indicates a transient error.
func retryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode >= 500
}

// isAuthError checks if an HTTP status code indicates an auth failure.
func isAuthError(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// doRequestWithRetry performs a request with exponential backoff on transient errors.
// Auth errors are NOT retried.
func doRequestWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			debuglog.Printf("conversation-backend retry attempt=%d delay=%v", attempt, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Reset body for retry.
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("failed to reset request body: %w", err)
			}
			req.Body = body
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if isAuthError(resp.StatusCode) {
			return resp, nil // caller handles auth errors
		}

		if retryable(resp.StatusCode) && attempt < maxRetries {
			resp.Body.Close()
			lastErr = fmt.Errorf("transient HTTP %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
