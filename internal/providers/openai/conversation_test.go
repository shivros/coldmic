package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		BaseURL:      "https://api.example.com/v1",
		APIKey:       "test-key",
		Model:        "gpt-4o",
		SystemPrompt: "You are a test assistant.",
		Stream:       true,
		Timeout:      10 * time.Second,
		MaxHistory:   20,
	}
}

func TestNewProvider_Defaults(t *testing.T) {
	p := NewProvider(Config{})
	if p.cfg.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", p.cfg.Timeout)
	}
	if p.cfg.MaxHistory != 20 {
		t.Errorf("expected default max history 20, got %d", p.cfg.MaxHistory)
	}
	if p.cfg.Model != "gpt-4o" {
		t.Errorf("expected default model gpt-4o, got %s", p.cfg.Model)
	}
	if p.cfg.SystemPrompt == "" {
		t.Error("expected non-empty default system prompt")
	}
}

func TestSendMessage_MissingAPIKey(t *testing.T) {
	cfg := testConfig()
	cfg.APIKey = ""
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-1", "hello")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "COLDMIC_BACKEND_API_KEY") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendMessage_NonStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer token, got %s", r.Header.Get("Authorization"))
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Model != "gpt-4o" {
			t.Errorf("expected model gpt-4o, got %s", req.Model)
		}
		if len(req.Messages) < 2 {
			t.Errorf("expected at least 2 messages (system + user), got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message role=system, got %s", req.Messages[0].Role)
		}
		if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
			t.Errorf("expected user message 'hello', got role=%s content=%s", req.Messages[1].Role, req.Messages[1].Content)
		}

		resp := chatResponse{
			ID:    "chatcmpl-test",
			Model: "gpt-4o",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "Hello! How can I help you today?"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	resp, err := p.SendMessage(context.Background(), "session-1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Text == "" {
		t.Error("expected non-empty response text")
	}
	if resp.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", resp.SessionID)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", resp.Model)
	}
}

func TestSendMessage_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-1", "hello")
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendMessage_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal server error"}}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-1", "hello")
	if err == nil {
		t.Fatal("expected server error")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendMessage_MaintainsHistory(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Second call should have 4 messages: system + user1 + assistant1 + user2
		if count == 2 {
			if len(req.Messages) != 4 {
				t.Errorf("expected 4 messages on second call, got %d", len(req.Messages))
			}
			if req.Messages[1].Role != "user" || req.Messages[1].Content != "first message" {
				t.Errorf("expected first user message, got role=%s content=%s", req.Messages[1].Role, req.Messages[1].Content)
			}
			if req.Messages[2].Role != "assistant" {
				t.Errorf("expected assistant message at index 2, got %s", req.Messages[2].Role)
			}
		}

		resp := chatResponse{
			ID:    fmt.Sprintf("chatcmpl-%d", count),
			Model: "gpt-4o",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: fmt.Sprintf("response %d", count)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-1", "first message")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	_, err = p.SendMessage(context.Background(), "session-1", "second message")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
}

func TestSendMessage_TruncatesHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Model: "gpt-4o",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "ok"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	cfg.MaxHistory = 2 // keep only 2 turns (4 messages: user+assistant pairs)
	p := NewProvider(cfg)

	// Send 5 messages to exceed max history.
	for i := 0; i < 5; i++ {
		_, err := p.SendMessage(context.Background(), "session-1", fmt.Sprintf("message %d", i))
		if err != nil {
			t.Fatalf("message %d failed: %v", i, err)
		}
	}

	// Verify history was truncated.
	p.mu.Lock()
	hist := p.sessions["session-1"]
	msgCount := len(hist.messages)
	p.mu.Unlock()

	if msgCount > cfg.MaxHistory*2 {
		t.Errorf("expected max %d messages, got %d", cfg.MaxHistory*2, msgCount)
	}
}

func TestStreamMessage_Streaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)

		if !req.Stream {
			t.Error("expected stream=true in request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)

		chunks := []string{"Hello", " there", "!"}
		for _, chunk := range chunks {
			data := streamChunk{
				Choices: []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				}{
					{Delta: struct {
						Content string `json:"content"`
					}{Content: chunk}},
				},
			}
			payload, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		// Send finish.
		finishReason := "stop"
		doneChunk := streamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{
				{FinishReason: &finishReason},
			},
		}
		payload, _ := json.Marshal(doneChunk)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	ch, err := p.StreamMessage(context.Background(), "session-1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var texts []string
	var gotDone bool
	for chunk := range ch {
		if chunk.Done {
			gotDone = true
			break
		}
		texts = append(texts, chunk.Text)
	}

	if !gotDone {
		t.Error("expected done chunk")
	}
	combined := strings.Join(texts, "")
	if combined != "Hello there!" {
		t.Errorf("expected 'Hello there!', got %q", combined)
	}
}

func TestStreamMessage_MissingAPIKey(t *testing.T) {
	cfg := testConfig()
	cfg.APIKey = ""
	p := NewProvider(cfg)

	_, err := p.StreamMessage(context.Background(), "session-1", "hello")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestStreamMessage_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.StreamMessage(context.Background(), "session-1", "hello")
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCloseSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Model: "gpt-4o",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "ok"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = p.CloseSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.mu.Lock()
	_, exists := p.sessions["session-1"]
	p.mu.Unlock()

	if exists {
		t.Error("expected session to be deleted after close")
	}
}

func TestStreamMessage_SSE_DoneMarker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Send one chunk then data: [DONE]
		data := streamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{
				{Delta: struct {
					Content string `json:"content"`
				}{Content: "Hi"}},
			},
		}
		payload, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	ch, err := p.StreamMessage(context.Background(), "session-1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var texts []string
	var gotDone bool
	for chunk := range ch {
		if chunk.Done {
			gotDone = true
			break
		}
		texts = append(texts, chunk.Text)
	}

	if !gotDone {
		t.Error("expected done chunk")
	}
	if len(texts) != 1 || texts[0] != "Hi" {
		t.Errorf("expected single chunk 'Hi', got %v", texts)
	}
}

func TestStreamMessage_HistoryAppended(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		data := streamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{
				{Delta: struct {
					Content string `json:"content"`
				}{Content: "World"}},
			},
		}
		payload, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()

		finishReason := "stop"
		doneChunk := streamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{
				{FinishReason: &finishReason},
			},
		}
		payload2, _ := json.Marshal(doneChunk)
		fmt.Fprintf(w, "data: %s\n\n", payload2)
		flusher.Flush()
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	ch, err := p.StreamMessage(context.Background(), "session-1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
		// drain
	}

	p.mu.Lock()
	hist := p.sessions["session-1"]
	p.mu.Unlock()

	if len(hist.messages) != 2 {
		t.Fatalf("expected 2 messages (user+assistant), got %d", len(hist.messages))
	}
	if hist.messages[0].Role != "user" {
		t.Errorf("expected first message role=user, got %s", hist.messages[0].Role)
	}
	if hist.messages[1].Role != "assistant" {
		t.Errorf("expected second message role=assistant, got %s", hist.messages[1].Role)
	}
	if hist.messages[1].Content != "World" {
		t.Errorf("expected assistant content 'World', got %q", hist.messages[1].Content)
	}
}

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{"**bold**", "bold"},
		{"*italic*", "italic"},
		{"***both***", "both"},
		{"# Heading", "Heading"},
		{"## Sub", "Sub"},
		{"`code`", "code"},
		{"[link](https://example.com)", "link"},
		{"  hello  ", "hello"},
		{"Hello **world** and *friends*", "Hello world and friends"},
	}

	for _, tt := range tests {
		got := stripMarkdown(tt.input)
		if got != tt.output {
			t.Errorf("stripMarkdown(%q) = %q, want %q", tt.input, got, tt.output)
		}
	}
}

func TestContext_Cancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	cfg.Timeout = 10 * time.Second
	p := NewProvider(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.SendMessage(ctx, "session-1", "hello")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestSendMessage_SeparateSessionsIsolated(t *testing.T) {
	var lastSystemPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		if len(req.Messages) > 0 {
			lastSystemPrompt = req.Messages[0].Content
		}

		resp := chatResponse{
			Model: "gpt-4o",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "ok"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.BaseURL = server.URL
	p := NewProvider(cfg)

	_, err := p.SendMessage(context.Background(), "session-a", "hello A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = p.SendMessage(context.Background(), "session-b", "hello B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sessions are separate.
	p.mu.Lock()
	histA := p.sessions["session-a"]
	histB := p.sessions["session-b"]
	p.mu.Unlock()

	if len(histA.messages) != 2 {
		t.Errorf("session-a: expected 2 messages, got %d", len(histA.messages))
	}
	if len(histB.messages) != 2 {
		t.Errorf("session-b: expected 2 messages, got %d", len(histB.messages))
	}
	if histA.messages[0].Content != "hello A" {
		t.Errorf("session-a: expected first user msg 'hello A', got %q", histA.messages[0].Content)
	}
	if histB.messages[0].Content != "hello B" {
		t.Errorf("session-b: expected first user msg 'hello B', got %q", histB.messages[0].Content)
	}

	_ = lastSystemPrompt
}
