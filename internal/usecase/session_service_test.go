package usecase

import (
	"context"
	"errors"
	"testing"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

func TestSessionServiceCachesLastTranscript(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	streamSession.events <- domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "text"}
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{transform: "TEXT"},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)

	service := NewSessionService(controller)
	if _, err := service.LastTranscript(); !errors.Is(err, domain.ErrNoTranscriptAvailable) {
		t.Fatalf("expected ErrNoTranscriptAvailable, got %v", err)
	}

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	result, err := service.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if result.SessionID == "" {
		t.Fatalf("expected non-empty session id on stop result")
	}

	latest, err := service.LastTranscript()
	if err != nil {
		t.Fatalf("last transcript failed: %v", err)
	}

	if latest.Result.FinalTranscript != "TEXT" {
		t.Fatalf("unexpected transcript: %+v", latest.Result)
	}
	if latest.Result.SessionID != result.SessionID {
		t.Fatalf("expected session id %q to persist in latest transcript, got %q", result.SessionID, latest.Result.SessionID)
	}
	if latest.CapturedAt.IsZero() {
		t.Fatalf("expected capture timestamp")
	}
}

func TestSessionServiceStatusAndAbort(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{transform: "TEXT"},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	service := NewSessionService(controller)

	idle := service.Status()
	if idle.State != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %+v", idle)
	}

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	recording := service.Status()
	if recording.State != domain.SessionStateRecording {
		t.Fatalf("expected recording, got %+v", recording)
	}

	if err := service.Abort(); err != nil {
		t.Fatalf("abort failed: %v", err)
	}
	afterAbort := service.Status()
	if afterAbort.State != domain.SessionStateIdle {
		t.Fatalf("expected idle after abort, got %+v", afterAbort)
	}
}

func TestSessionServiceConversationNotConfigured(t *testing.T) {
	t.Parallel()

	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	service := NewSessionService(controller)

	if err := service.StartConversation(context.Background()); err == nil {
		t.Fatal("expected error when conversation not configured")
	}
	if err := service.StopConversation(); err == nil {
		t.Fatal("expected error when conversation not configured")
	}
	status := service.ConversationStatus()
	if status.State != domain.ConvStateIdle {
		t.Fatalf("expected idle conv status, got %v", status.State)
	}
}

func TestSessionServiceContinuousNotConfigured(t *testing.T) {
	t.Parallel()

	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	service := NewSessionService(controller)

	if err := service.StartContinuous(context.Background()); err == nil {
		t.Fatal("expected error when continuous not configured")
	}
	if err := service.StopContinuous(); err == nil {
		t.Fatal("expected error when continuous not configured")
	}
}

func TestNewSessionServiceWithConversation(t *testing.T) {
	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	listener := NewContinuousListener(
		&fakeAudioCapture{},
		&fakeVAD{},
		&fakeProvider{},
		&fakeEventSink{},
		ContinuousListenerConfig{},
	)
	convCtrl := NewConversationController(
		listener,
		&fakeConversationBackend{},
		&fakeTTSProvider{},
		&fakeEventSink{},
		ConversationControllerConfig{},
	)

	service := NewSessionServiceWithConversation(controller, listener, convCtrl)
	if service == nil {
		t.Fatal("expected non-nil service")
	}

	// ConversationStatus should return idle since nothing is running.
	status := service.ConversationStatus()
	if status.State != domain.ConvStateIdle {
		t.Fatalf("expected idle, got %v", status.State)
	}

	// StopConversation should fail since nothing is running.
	if err := service.StopConversation(); err == nil {
		t.Fatal("expected error stopping non-running conversation")
	}

	// StopContinuous should fail since listener isn't running.
	if err := service.StopContinuous(); err == nil {
		t.Fatal("expected error stopping non-running continuous")
	}
}

func TestNewSessionServiceWithContinuousStatus(t *testing.T) {
	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	listener := NewContinuousListener(
		&fakeAudioCapture{},
		&fakeVAD{},
		&fakeProvider{},
		&fakeEventSink{},
		ContinuousListenerConfig{},
	)
	service := NewSessionServiceWithContinuous(controller, listener)
	if service == nil {
		t.Fatal("expected non-nil service")
	}
	status := service.Status()
	if status.Mode != "ptt" || status.State != domain.SessionStateIdle {
		t.Fatalf("unexpected status: %+v", status)
	}

	listener.running = true
	status = service.Status()
	if status.Mode != "continuous" || status.State != domain.SessionStateContinuous || !status.Active {
		t.Fatalf("expected continuous active status, got %+v", status)
	}

	listener.cancel = func() {}
	if err := service.StopContinuous(); err != nil {
		t.Fatalf("expected StopContinuous success, got %v", err)
	}
}

func TestSessionServiceStartConversationGuardContinuousActive(t *testing.T) {
	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)
	listener := NewContinuousListener(
		&fakeAudioCapture{},
		&fakeVAD{},
		&fakeProvider{},
		&fakeEventSink{},
		ContinuousListenerConfig{},
	)
	listener.running = true
	convCtrl := NewConversationController(
		listener,
		&fakeConversationBackend{},
		&fakeTTSProvider{},
		&fakeEventSink{},
		ConversationControllerConfig{},
	)
	service := NewSessionServiceWithConversation(controller, listener, convCtrl)
	if err := service.StartConversation(context.Background()); !errors.Is(err, domain.ErrContinuousActive) {
		t.Fatalf("expected ErrContinuousActive, got %v", err)
	}
}

// Minimal fakes for session_service tests.

type fakeVAD struct{}

func (fakeVAD) Process(_ []byte) (float64, error) { return 0, nil }
func (fakeVAD) Reset()                            {}

type fakeConversationBackend struct{}

func (fakeConversationBackend) SendMessage(_ context.Context, _, _ string) (*ports.ConversationResponse, error) {
	return &ports.ConversationResponse{Text: "ok"}, nil
}
func (fakeConversationBackend) StreamMessage(_ context.Context, _, _ string) (<-chan ports.StreamChunk, error) {
	ch := make(chan ports.StreamChunk, 1)
	ch <- ports.StreamChunk{Text: "ok", Done: true}
	close(ch)
	return ch, nil
}
func (fakeConversationBackend) CloseSession(_ context.Context, _ string) error { return nil }

type fakeTTSProvider struct{}

func (fakeTTSProvider) Synthesize(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (fakeTTSProvider) Play(_ context.Context, _ string) error                 { return nil }
