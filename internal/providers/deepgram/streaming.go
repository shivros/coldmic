package deepgram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"coldmic/internal/debuglog"
	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

// Config controls Deepgram websocket settings.
type Config struct {
	APIKey      string
	APIBaseURL  string
	Model       string
	Language    string
	SmartFormat bool
}

// Provider implements ports.TranscriptionProvider for Deepgram.
type Provider struct {
	cfg Config
}

func NewProvider(cfg Config) *Provider {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.deepgram.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "nova-2"
	}
	return &Provider{cfg: cfg}
}

func (p *Provider) StartStreaming(ctx context.Context, cfg ports.StreamingConfig) (ports.StreamingSession, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, errors.New("DEEPGRAM_API_KEY is not configured")
	}

	wsURL, err := buildListenURL(p.cfg, cfg)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Authorization", "Token "+p.cfg.APIKey)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Deepgram websocket: %w", err)
	}
	debuglog.Printf("deepgram connected url=%s", wsURL)

	session := &streamingSession{
		conn:   conn,
		events: make(chan domain.TranscriptEvent, 64),
		audio:  make(chan []byte, 32),
		done:   make(chan struct{}),
	}

	session.wg.Add(2)
	go session.readLoop()
	go session.writeLoop()
	go func() {
		session.wg.Wait()
		close(session.events)
		close(session.done)
		_ = conn.Close()
	}()

	go func() {
		<-ctx.Done()
		_ = session.Close()
	}()

	return session, nil
}

type streamingSession struct {
	conn *websocket.Conn

	events chan domain.TranscriptEvent
	audio  chan []byte
	done   chan struct{}

	wg sync.WaitGroup

	errMu sync.Mutex
	err   error

	closeSendOnce sync.Once
	closeOnce     sync.Once
	sendMu        sync.RWMutex
	sendClosed    bool
}

func (s *streamingSession) SendAudio(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}

	s.sendMu.RLock()
	closed := s.sendClosed
	s.sendMu.RUnlock()
	if closed {
		return errors.New("audio stream is already closed")
	}

	copied := append([]byte(nil), chunk...)
	select {
	case s.audio <- copied:
		return nil
	case <-s.done:
		if err := s.waitErr(); err != nil {
			return err
		}
		return errors.New("session closed")
	}
}

func (s *streamingSession) CloseSend() error {
	s.closeSendOnce.Do(func() {
		s.sendMu.Lock()
		s.sendClosed = true
		close(s.audio)
		s.sendMu.Unlock()
	})
	return nil
}

func (s *streamingSession) Events() <-chan domain.TranscriptEvent {
	return s.events
}

func (s *streamingSession) Wait() error {
	<-s.done
	return s.waitErr()
}

func (s *streamingSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.CloseSend()
		_ = s.conn.Close()
	})
	<-s.done
	return s.waitErr()
}

func (s *streamingSession) waitErr() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *streamingSession) setErr(err error) {
	if err == nil {
		return
	}
	if isExpectedShutdownErr(err) {
		return
	}

	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func isExpectedShutdownErr(err error) bool {
	if errors.Is(err, net.ErrClosed) || errors.Is(err, websocket.ErrCloseSent) {
		return true
	}

	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return false
	}

	switch closeErr.Code {
	case websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived:
		return true
	default:
		return false
	}
}

func (s *streamingSession) writeLoop() {
	defer s.wg.Done()

	for chunk := range s.audio {
		if err := s.conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
			debuglog.Printf("deepgram audio send failed: %v", err)
			s.setErr(fmt.Errorf("failed to send audio: %w", err))
			return
		}
	}

	if err := s.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"CloseStream"}`)); err != nil {
		debuglog.Printf("deepgram close stream failed: %v", err)
		s.setErr(fmt.Errorf("failed to close stream: %w", err))
		return
	}
	debuglog.Printf("deepgram sent CloseStream")
}

func (s *streamingSession) readLoop() {
	defer s.wg.Done()

	for {
		_, payload, err := s.conn.ReadMessage()
		if err != nil {
			debuglog.Printf("deepgram read failed: %v", err)
			s.setErr(fmt.Errorf("failed to read provider event: %w", err))
			return
		}

		var response deepgramResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			debuglog.Printf("deepgram ignored non-json payload bytes=%d", len(payload))
			continue
		}
		if response.Type != "" {
			debuglog.Printf("deepgram event type=%s is_final=%t speech_final=%t", response.Type, response.IsFinal, response.SpeechFinal)
		}

		if strings.EqualFold(response.Type, "Error") {
			message := strings.TrimSpace(response.Message)
			if message == "" {
				message = "deepgram returned an unknown error"
			}
			debuglog.Printf("deepgram error event message=%q", message)
			s.emit(domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "", IsSpeechFinal: true})
			s.setErr(errors.New(message))
			return
		}

		transcript := extractTranscript(response)
		if transcript == "" {
			continue
		}

		event := domain.TranscriptEvent{Text: transcript, IsSpeechFinal: response.SpeechFinal}
		if response.IsFinal || response.SpeechFinal {
			event.Kind = domain.TranscriptKindFinal
		} else {
			event.Kind = domain.TranscriptKindPartial
		}
		debuglog.Printf("deepgram transcript kind=%s speech_final=%t text=%q", event.Kind, event.IsSpeechFinal, truncateForLog(transcript, 160))
		s.emit(event)
	}
}

func (s *streamingSession) emit(event domain.TranscriptEvent) {
	select {
	case s.events <- event:
	case <-s.done:
	default:
	}
}

type deepgramResponse struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	IsFinal     bool   `json:"is_final"`
	SpeechFinal bool   `json:"speech_final"`

	Channel struct {
		Alternatives []struct {
			Transcript string `json:"transcript"`
		} `json:"alternatives"`
	} `json:"channel"`

	Results struct {
		Channels []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"channels"`
	} `json:"results"`
}

func extractTranscript(response deepgramResponse) string {
	if len(response.Channel.Alternatives) > 0 {
		if text := strings.TrimSpace(response.Channel.Alternatives[0].Transcript); text != "" {
			return text
		}
	}
	if len(response.Results.Channels) > 0 && len(response.Results.Channels[0].Alternatives) > 0 {
		return strings.TrimSpace(response.Results.Channels[0].Alternatives[0].Transcript)
	}
	return ""
}

func truncateForLog(input string, max int) string {
	if max <= 0 || len(input) <= max {
		return input
	}
	return input[:max] + "..."
}

func buildListenURL(providerCfg Config, streamCfg ports.StreamingConfig) (string, error) {
	base := providerCfg.APIBaseURL
	if base == "" {
		base = "https://api.deepgram.com/v1"
	}
	base = strings.TrimSpace(base)

	if strings.HasPrefix(base, "https://") {
		base = "wss://" + strings.TrimPrefix(base, "https://")
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	base = strings.TrimRight(base, "/")

	listenURL, err := url.Parse(base + "/listen")
	if err != nil {
		return "", fmt.Errorf("invalid Deepgram API base URL: %w", err)
	}

	query := listenURL.Query()
	if streamCfg.Encoding == "" {
		streamCfg.Encoding = "linear16"
	}
	if streamCfg.SampleRate <= 0 {
		streamCfg.SampleRate = 16000
	}
	if streamCfg.Channels <= 0 {
		streamCfg.Channels = 1
	}
	query.Set("model", providerCfg.Model)
	query.Set("encoding", streamCfg.Encoding)
	query.Set("sample_rate", fmt.Sprintf("%d", streamCfg.SampleRate))
	query.Set("channels", fmt.Sprintf("%d", streamCfg.Channels))
	query.Set("interim_results", fmt.Sprintf("%t", streamCfg.InterimResults))
	query.Set("smart_format", fmt.Sprintf("%t", providerCfg.SmartFormat))
	if providerCfg.Language != "" {
		query.Set("language", providerCfg.Language)
	}
	listenURL.RawQuery = query.Encode()
	return listenURL.String(), nil
}
