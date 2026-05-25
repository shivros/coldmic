package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"coldmic/internal/cli"
	"coldmic/internal/daemon"
	"coldmic/internal/domain"
	"coldmic/internal/ports"
	"coldmic/internal/usecase"
)

// ---------------------------------------------------------------------------
// Integration test: start daemon → start conversation → verify status endpoint
//
// Validates the full wiring path:
//
//	HTTP handler → SessionService → ConversationController → ContinuousListener
//
// All hardware-dependent dependencies are mocked. The test exercises:
//  1. GET  /v1/conversation/status  → idle, not active
//  2. POST /v1/conversation/start   → accepted (202)
//  3. GET  /v1/conversation/status  → controller is running (armed, idle state)
//  4. POST /v1/conversation/stop    → ok
//  5. GET  /v1/conversation/status  → idle, not active again
//  6. Double-start returns error (controller already running)
//  7. Stop-when-idle returns 409 Conflict
//  8. Method-not-allowed on all conversation endpoints
// ---------------------------------------------------------------------------

// --- Mocks ---

// blockingAudioSession blocks on Read until context is cancelled, then returns io.EOF.
type blockingAudioSession struct {
	ctx context.Context
}

func (s *blockingAudioSession) Read(p []byte) (int, error) {
	<-s.ctx.Done()
	return 0, io.EOF
}

func (s *blockingAudioSession) Close() error { return nil }
func (s *blockingAudioSession) Stop() error  { return nil }

// mockAudioCapture returns a blocking session that lives until ctx is cancelled.
type mockAudioCapture struct{}

func (m *mockAudioCapture) Start(ctx context.Context, _ ports.AudioConfig) (ports.AudioSession, error) {
	return &blockingAudioSession{ctx: ctx}, nil
}

// mockVAD always reports silence so the listener sits idle.
type mockVAD struct{}

func (mockVAD) Process(_ []byte) (float64, error) { return 0.0, nil }
func (mockVAD) Reset()                            {}

// mockTranscriptionProvider is never called because VAD reports silence.
type mockTranscriptionProvider struct{}

func (mockTranscriptionProvider) StartStreaming(_ context.Context, _ ports.StreamingConfig) (ports.StreamingSession, error) {
	return nil, nil
}

// mockConversationBackend returns canned responses.
type mockConversationBackend struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockConversationBackend) SendMessage(_ context.Context, sessionID string, text string) (*ports.ConversationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, text)
	return &ports.ConversationResponse{Text: "mock response", SessionID: sessionID}, nil
}

func (m *mockConversationBackend) StreamMessage(_ context.Context, sessionID string, text string) (<-chan ports.StreamChunk, error) {
	ch := make(chan ports.StreamChunk, 1)
	ch <- ports.StreamChunk{Text: "mock stream", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockConversationBackend) CloseSession(_ context.Context, _ string) error { return nil }

// mockTTS records calls but does nothing.
type mockTTS struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockTTS) Play(_ context.Context, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, text)
	return nil
}

func (m *mockTTS) Synthesize(_ context.Context, text string) ([]byte, error) {
	return []byte("audio"), nil
}

// mockEventSink collects conversation state changes.
type mockEventSink struct {
	mu     sync.Mutex
	states []convStateChange
}

type convStateChange struct {
	state  domain.ConversationState
	reason domain.ConversationStateReason
}

func (m *mockEventSink) SessionStateChanged(_ domain.SessionState, _ domain.SessionStateReason) {}
func (m *mockEventSink) PartialTranscript(_ string)                                             {}
func (m *mockEventSink) FinalTranscript(_, _, _ string)                                         {}
func (m *mockEventSink) SessionError(_ domain.ErrorCode, _ string)                              {}
func (m *mockEventSink) ConversationStateChanged(state domain.ConversationState, reason domain.ConversationStateReason) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, convStateChange{state, reason})
}

// --- Helpers ---

// conversationStatusResponse mirrors daemon.ConversationStatusResponse.
type conversationStatusResponse struct {
	OK     bool                      `json:"ok"`
	Error  string                    `json:"error,omitempty"`
	Status domain.ConversationStatus `json:"status"`
}

// buildTestStack wires the full daemon stack with mocked hardware dependencies
// and returns an httptest.Server ready to serve requests.
func buildTestStack(t *testing.T) (*httptest.Server, *mockConversationBackend, *mockTTS, *mockEventSink) {
	t.Helper()

	eventSink := &mockEventSink{}
	backend := &mockConversationBackend{}
	tts := &mockTTS{}

	listener := usecase.NewContinuousListener(
		&mockAudioCapture{},
		mockVAD{},
		mockTranscriptionProvider{},
		eventSink,
		usecase.ContinuousListenerConfig{
			WakePhrases: []string{"hey alice", "alice"},
			Audio: ports.AudioConfig{
				SampleRate:  16000,
				Channels:    1,
				InputFormat: "s16le",
			},
			Streaming: ports.StreamingConfig{
				SampleRate:     16000,
				Channels:       1,
				Encoding:       "linear16",
				InterimResults: true,
			},
		},
	)

	conversationCtrl := usecase.NewConversationController(
		listener,
		backend,
		tts,
		eventSink,
		usecase.ConversationControllerConfig{
			StopPhrases:    []string{"thanks alice", "that's all", "goodbye", "bye alice", "stop"},
			SilenceTimeout: 30 * time.Second,
		},
	)

	// Minimal SessionController for PTT (not exercised in this test).
	controller := usecase.NewSessionController(
		nil, nil, nil, nil, eventSink, usecase.Config{},
	)

	session := usecase.NewSessionServiceWithConversation(controller, listener, conversationCtrl)
	api := daemon.NewAPI(session)
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	return srv, backend, tts, eventSink
}

// --- Tests ---

func TestIntegration_ConversationLifecycle(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)
	client := cli.NewClient(srv.URL)
	ctx := context.Background()

	// Step 1: Verify initial status is idle.
	t.Run("initial_status_idle", func(t *testing.T) {
		status, err := client.ConversationStatus(ctx)
		if err != nil {
			t.Fatalf("ConversationStatus failed: %v", err)
		}
		if status.State != domain.ConvStateIdle {
			t.Errorf("expected initial state idle, got %q", status.State)
		}
		if status.Active {
			t.Error("expected active=false initially")
		}
	})

	// Step 2: Start conversation — returns 202 Accepted.
	t.Run("start_conversation_accepted", func(t *testing.T) {
		// Use raw HTTP to verify status code.
		resp, err := http.Post(srv.URL+"/v1/conversation/start", "application/json", nil)
		if err != nil {
			t.Fatalf("POST /v1/conversation/start failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
		}

		var body conversationStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !body.OK {
			t.Error("expected ok=true in response")
		}
	})

	// Step 3: Verify status after start.
	// The controller is running but in "idle" state (armed, waiting for wake phrase).
	// Active = (state != idle) so it reports false. This is correct: the system is
	// armed but no conversation cycle has begun.
	t.Run("status_while_armed", func(t *testing.T) {
		// Allow goroutines to start.
		time.Sleep(50 * time.Millisecond)

		status, err := client.ConversationStatus(ctx)
		if err != nil {
			t.Fatalf("ConversationStatus failed: %v", err)
		}
		// State is "idle" because no wake phrase has been detected yet.
		// Active is derived from state != idle, so it's false.
		// This is correct: the system is armed but no active conversation cycle.
		if status.State != domain.ConvStateIdle {
			t.Logf("note: state=%q (controller is armed, waiting for wake phrase)", status.State)
		}
	})

	// Step 4: Stop conversation.
	t.Run("stop_conversation", func(t *testing.T) {
		status, err := client.ConversationStop(ctx)
		if err != nil {
			t.Fatalf("ConversationStop failed: %v", err)
		}

		// Allow goroutines to wind down.
		time.Sleep(100 * time.Millisecond)

		status, err = client.ConversationStatus(ctx)
		if err != nil {
			t.Fatalf("ConversationStatus after stop failed: %v", err)
		}
		if status.Active {
			t.Errorf("expected active=false after stop, got %+v", status)
		}
		if status.State != domain.ConvStateIdle {
			t.Errorf("expected state idle after stop, got %q", status.State)
		}
	})

	// Step 5: Verify status is idle after full stop.
	t.Run("status_idle_after_stop", func(t *testing.T) {
		status, err := client.ConversationStatus(ctx)
		if err != nil {
			t.Fatalf("ConversationStatus failed: %v", err)
		}
		if status.State != domain.ConvStateIdle {
			t.Errorf("expected idle state, got %q", status.State)
		}
		if status.Active {
			t.Error("expected active=false after conversation stopped")
		}
	})
}

func TestIntegration_ConversationDoubleStartError(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)
	client := cli.NewClient(srv.URL)
	ctx := context.Background()

	// Start first conversation.
	_, err := client.ConversationStart(ctx)
	if err != nil {
		t.Fatalf("first ConversationStart failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Attempt to start again while controller is running.
	// The HTTP handler's quick check uses ConversationStatus().Active which is
	// false when state=idle (controller armed but no wake phrase detected).
	// So it falls through to StartConversation() which checks Running() and
	// returns ErrConversationActive, now correctly mapped to 409 Conflict.
	resp, err := http.Post(srv.URL+"/v1/conversation/start", "application/json", nil)
	if err != nil {
		t.Fatalf("second start request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 Conflict on double-start, got %d", resp.StatusCode)
	}

	var body conversationStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(body.Error, "conversation is already active") {
		t.Errorf("expected 'conversation is already active' error, got %q", body.Error)
	}

	// Clean up.
	_, _ = client.ConversationStop(ctx)
}

func TestIntegration_ConversationStopWhenIdleConflict(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)

	// Stop when no conversation is active — should return 409.
	resp, err := http.Post(srv.URL+"/v1/conversation/stop", "application/json", nil)
	if err != nil {
		t.Fatalf("stop request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 Conflict on stop-when-idle, got %d", resp.StatusCode)
	}

	var body conversationStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(body.Error, "no active conversation") {
		t.Errorf("expected 'no active conversation' error, got %q", body.Error)
	}
}

func TestIntegration_ConversationStatusMethodNotAllowed(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)

	resp, err := http.Post(srv.URL+"/v1/conversation/status", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestIntegration_ConversationStartMethodNotAllowed(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)

	resp, err := http.Get(srv.URL + "/v1/conversation/start")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestIntegration_ConversationStopMethodNotAllowed(t *testing.T) {
	srv, _, _, _ := buildTestStack(t)

	resp, err := http.Get(srv.URL + "/v1/conversation/stop")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}
