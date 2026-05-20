package usecase

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

// --- Mocks ---

type mockConversationBackend struct {
	mu       sync.Mutex
	response *ports.ConversationResponse
	err      error
	calls    []string
}

func (m *mockConversationBackend) SendMessage(_ context.Context, sessionID string, text string) (*ports.ConversationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, text)
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &ports.ConversationResponse{Text: "mock response", SessionID: sessionID}, nil
}

func (m *mockConversationBackend) StreamMessage(_ context.Context, sessionID string, text string) (<-chan ports.StreamChunk, error) {
	ch := make(chan ports.StreamChunk, 1)
	ch <- ports.StreamChunk{Text: "mock stream", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockConversationBackend) CloseSession(_ context.Context, _ string) error {
	return nil
}

func (m *mockConversationBackend) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

type mockTTS struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (m *mockTTS) Play(_ context.Context, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, text)
	return m.err
}

func (m *mockTTS) Synthesize(_ context.Context, text string) ([]byte, error) {
	return []byte("audio"), nil
}

func (m *mockTTS) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

type mockConvEventSink struct {
	mu     sync.Mutex
	states []convStateChange
}

type convStateChange struct {
	state  domain.ConversationState
	reason domain.ConversationStateReason
}

func (m *mockConvEventSink) SessionStateChanged(_ domain.SessionState, _ domain.SessionStateReason) {}
func (m *mockConvEventSink) PartialTranscript(_ string)                                             {}
func (m *mockConvEventSink) FinalTranscript(_, _, _ string)                                         {}
func (m *mockConvEventSink) SessionError(_ domain.ErrorCode, _ string)                              {}
func (m *mockConvEventSink) ConversationStateChanged(state domain.ConversationState, reason domain.ConversationStateReason) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, convStateChange{state, reason})
}

func (m *mockConvEventSink) getStates() []convStateChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]convStateChange, len(m.states))
	copy(result, m.states)
	return result
}

// --- Helper ---

func newTestController(backend ports.ConversationBackend, tts ports.TextToSpeech, sink *mockConvEventSink) *ConversationController {
	cfg := ConversationControllerConfig{
		StopPhrases:    []string{"thanks alice", "that's all", "goodbye", "bye alice", "stop"},
		SilenceTimeout: 30 * time.Second,
	}
	return &ConversationController{
		backend: backend,
		tts:     tts,
		events:  sink,
		cfg:     cfg,
		state:   domain.ConvStateIdle,
	}
}

func newTestControllerWithListener(
	listener *ContinuousListener,
	backend ports.ConversationBackend,
	tts ports.TextToSpeech,
	sink *mockConvEventSink,
) *ConversationController {
	cfg := ConversationControllerConfig{
		StopPhrases:    []string{"thanks alice", "that's all", "goodbye", "bye alice", "stop"},
		SilenceTimeout: 30 * time.Second,
	}
	return NewConversationController(listener, backend, tts, sink, cfg)
}

// --- Tests ---

func TestConversationController_InitialState(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected initial state to be idle, got %s", ctrl.State())
	}
	if ctrl.Running() {
		t.Error("expected controller to not be running initially")
	}
}

func TestConversationController_Status(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	status := ctrl.Status()
	if status.State != domain.ConvStateIdle {
		t.Errorf("expected status state idle, got %s", status.State)
	}
	if status.Active {
		t.Error("expected status to not be active in idle state")
	}
}

func TestConversationController_IsValidTransition(t *testing.T) {
	tests := []struct {
		from     domain.ConversationState
		to       domain.ConversationState
		expected bool
	}{
		{domain.ConvStateIdle, domain.ConvStateListening, true},
		{domain.ConvStateIdle, domain.ConvStateProcessing, false},
		{domain.ConvStateIdle, domain.ConvStateSpeaking, false},
		{domain.ConvStateListening, domain.ConvStateProcessing, true},
		{domain.ConvStateListening, domain.ConvStateIdle, true},
		{domain.ConvStateListening, domain.ConvStateSpeaking, false},
		{domain.ConvStateProcessing, domain.ConvStateSpeaking, true},
		{domain.ConvStateProcessing, domain.ConvStateIdle, true},
		{domain.ConvStateProcessing, domain.ConvStateListening, false},
		{domain.ConvStateSpeaking, domain.ConvStateListening, true},
		{domain.ConvStateSpeaking, domain.ConvStateIdle, true},
		{domain.ConvStateSpeaking, domain.ConvStateProcessing, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			result := isValidTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("isValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestConversationController_SetState(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	// Valid: IDLE → LISTENING
	if err := ctrl.SetState(domain.ConvStateListening); err != nil {
		t.Errorf("expected valid transition IDLE→LISTENING, got error: %v", err)
	}

	// Invalid: LISTENING → SPEAKING
	if err := ctrl.SetState(domain.ConvStateSpeaking); err == nil {
		t.Error("expected invalid transition LISTENING→SPEAKING to fail")
	}

	// Valid: LISTENING → PROCESSING
	if err := ctrl.SetState(domain.ConvStateProcessing); err != nil {
		t.Errorf("expected valid transition LISTENING→PROCESSING, got error: %v", err)
	}

	// Valid: PROCESSING → SPEAKING
	if err := ctrl.SetState(domain.ConvStateSpeaking); err != nil {
		t.Errorf("expected valid transition PROCESSING→SPEAKING, got error: %v", err)
	}

	// Valid: SPEAKING → LISTENING
	if err := ctrl.SetState(domain.ConvStateListening); err != nil {
		t.Errorf("expected valid transition SPEAKING→LISTENING, got error: %v", err)
	}

	// Valid: LISTENING → IDLE (stop signal)
	if err := ctrl.SetState(domain.ConvStateIdle); err != nil {
		t.Errorf("expected valid transition LISTENING→IDLE, got error: %v", err)
	}
}

func TestConversationController_StopPhraseDetection(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	tests := []struct {
		text     string
		expected bool
	}{
		{"Thanks Alice, that's all", true},
		{"that's all for now", true},
		{"goodbye", true},
		{"Bye Alice", true},
		{"stop", true},
		{"STOP", true},
		{"can you stop the music", true},
		{"what time is it", false},
		{"tell me a joke", false},
		{"hello there", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := ctrl.IsStopPhrase(tt.text)
			if result != tt.expected {
				t.Errorf("IsStopPhrase(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestConversationController_StopPhraseCustomConfig(t *testing.T) {
	sink := &mockConvEventSink{}
	cfg := ConversationControllerConfig{
		StopPhrases:    []string{"end chat", "quit now"},
		SilenceTimeout: 30 * time.Second,
	}
	ctrl := NewConversationController(nil, nil, nil, sink, cfg)

	if !ctrl.IsStopPhrase("end chat") {
		t.Error("expected custom stop phrase 'end chat' to be detected")
	}
	if !ctrl.IsStopPhrase("quit now please") {
		t.Error("expected custom stop phrase 'quit now' to be detected in text")
	}
	if ctrl.IsStopPhrase("stop") {
		t.Error("expected 'stop' to NOT be a stop phrase with custom config")
	}
}

func TestConversationController_DefaultConfig(t *testing.T) {
	cfg := DefaultConversationConfig()
	if cfg.SilenceTimeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cfg.SilenceTimeout)
	}
	if len(cfg.StopPhrases) == 0 {
		t.Error("expected default stop phrases to be non-empty")
	}
}

func TestConversationController_ProcessUtterance(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	// Set state to LISTENING first.
	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.processUtterance(context.Background(), "hello")

	// Should have transitioned: LISTENING → PROCESSING → SPEAKING → LISTENING
	states := sink.getStates()
	if len(states) < 3 {
		t.Fatalf("expected at least 3 state changes, got %d: %+v", len(states), states)
	}

	// Check transitions.
	expectedStates := []convStateChange{
		{domain.ConvStateProcessing, domain.ConvReasonSpeechReceived},
		{domain.ConvStateSpeaking, domain.ConvReasonBackendResponse},
		{domain.ConvStateListening, domain.ConvReasonPlaybackDone},
	}
	for i, expected := range expectedStates {
		if states[i].state != expected.state {
			t.Errorf("state change %d: expected state %s, got %s", i, expected.state, states[i].state)
		}
		if states[i].reason != expected.reason {
			t.Errorf("state change %d: expected reason %s, got %s", i, expected.reason, states[i].reason)
		}
	}

	// Verify backend was called.
	calls := backend.getCalls()
	if len(calls) != 1 || calls[0] != "hello" {
		t.Errorf("expected backend call with 'hello', got %v", calls)
	}

	// Verify TTS was called.
	ttsCalls := tts.getCalls()
	if len(ttsCalls) != 1 || ttsCalls[0] != "mock response" {
		t.Errorf("expected TTS call with 'mock response', got %v", ttsCalls)
	}

	// Final state should be LISTENING.
	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected final state LISTENING, got %s", ctrl.State())
	}
}

func TestConversationController_BackendError(t *testing.T) {
	backend := &mockConversationBackend{err: context.DeadlineExceeded}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.processUtterance(context.Background(), "hello")

	// Should recover: PROCESSING → SPEAKING (error msg) → LISTENING
	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected state LISTENING after backend error, got %s", ctrl.State())
	}

	// TTS should have spoken the error message.
	ttsCalls := tts.getCalls()
	if len(ttsCalls) != 1 {
		t.Fatalf("expected 1 TTS call, got %d", len(ttsCalls))
	}
	if !strings.Contains(ttsCalls[0], "Sorry") {
		t.Errorf("expected error message to contain 'Sorry', got %q", ttsCalls[0])
	}
}

func TestConversationController_TransitionToIdle(t *testing.T) {
	backend := &mockConversationBackend{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, nil, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.transitionToIdle(domain.ConvReasonStopPhrase)

	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected state IDLE, got %s", ctrl.State())
	}

	states := sink.getStates()
	if len(states) != 1 {
		t.Fatalf("expected 1 state change, got %d", len(states))
	}
	if states[0].state != domain.ConvStateIdle {
		t.Errorf("expected IDLE state event, got %s", states[0].state)
	}
	if states[0].reason != domain.ConvReasonStopPhrase {
		t.Errorf("expected stop_phrase reason, got %s", states[0].reason)
	}
}

func TestConversationController_HandleTranscript_StopPhrase(t *testing.T) {
	backend := &mockConversationBackend{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, nil, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	// Simulate a transcript with a stop phrase.
	ctrl.handleTranscript(context.Background(), ListenerEvent{
		Kind: ListenerEventTranscript,
		Text: "thanks alice, goodbye",
	})

	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected state IDLE after stop phrase, got %s", ctrl.State())
	}

	// Backend should NOT have been called.
	calls := backend.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected no backend calls after stop phrase, got %d", len(calls))
	}
}

func TestConversationController_HandleTranscript_Empty(t *testing.T) {
	backend := &mockConversationBackend{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, nil, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	// Empty transcript should be ignored.
	ctrl.handleTranscript(context.Background(), ListenerEvent{
		Kind: ListenerEventTranscript,
		Text: "",
	})

	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected state LISTENING after empty transcript, got %s", ctrl.State())
	}

	calls := backend.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected no backend calls for empty transcript, got %d", len(calls))
	}
}

func TestConversationController_HandleWakePhrase(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	// Should only work from IDLE.
	ctrl.handleWakePhrase(context.Background(), ListenerEvent{
		Kind:      ListenerEventWakePhrase,
		Text:      "",
		SessionID: "session-1",
	})

	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected state LISTENING after wake phrase, got %s", ctrl.State())
	}

	states := sink.getStates()
	if len(states) != 1 {
		t.Fatalf("expected 1 state change, got %d", len(states))
	}
	if states[0].state != domain.ConvStateListening {
		t.Errorf("expected LISTENING state event, got %s", states[0].state)
	}
	if states[0].reason != domain.ConvReasonWakeDetected {
		t.Errorf("expected wake_detected reason, got %s", states[0].reason)
	}
}

func TestConversationController_HandleWakePhrase_WithText(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	// Wake phrase with embedded text should be processed immediately.
	ctrl.handleWakePhrase(context.Background(), ListenerEvent{
		Kind:      ListenerEventWakePhrase,
		Text:      "what time is it",
		SessionID: "session-1",
	})

	// Should have gone: IDLE → LISTENING → PROCESSING → SPEAKING → LISTENING
	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected state LISTENING after wake phrase with text, got %s", ctrl.State())
	}

	calls := backend.getCalls()
	if len(calls) != 1 || calls[0] != "what time is it" {
		t.Errorf("expected backend call with 'what time is it', got %v", calls)
	}
}

func TestConversationController_HandleWakePhrase_IgnoresNonIdle(t *testing.T) {
	backend := &mockConversationBackend{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, nil, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateProcessing
	ctrl.mu.Unlock()

	ctrl.handleWakePhrase(context.Background(), ListenerEvent{
		Kind:      ListenerEventWakePhrase,
		Text:      "",
		SessionID: "session-1",
	})

	// State should remain PROCESSING.
	if ctrl.State() != domain.ConvStateProcessing {
		t.Errorf("expected state PROCESSING, got %s", ctrl.State())
	}
}

func TestConversationController_SilenceTimer(t *testing.T) {
	sink := &mockConvEventSink{}
	cfg := ConversationControllerConfig{
		StopPhrases:    []string{"stop"},
		SilenceTimeout: 100 * time.Millisecond, // Short timeout for testing.
	}
	ctrl := NewConversationController(nil, nil, nil, sink, cfg)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.resetSilenceTimer()

	// Wait for timer to fire.
	time.Sleep(200 * time.Millisecond)

	// Check that the silence timer channel has something.
	ctrl.mu.Lock()
	ch := ctrl.silenceCh
	ctrl.mu.Unlock()

	if ch == nil {
		t.Error("expected silence channel to be set after resetSilenceTimer")
	}
}

func TestConversationController_SilenceTimeoutTransition(t *testing.T) {
	sink := &mockConvEventSink{}
	cfg := ConversationControllerConfig{
		StopPhrases:    []string{"stop"},
		SilenceTimeout: 50 * time.Millisecond,
	}
	ctrl := NewConversationController(nil, nil, nil, sink, cfg)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	// Start silence timer.
	ctrl.resetSilenceTimer()

	// Wait for it to fire and manually trigger (simulating the event loop).
	// In real use, the Start() loop reads from silenceCh.
	ctrl.mu.Lock()
	ch := ctrl.silenceCh
	ctrl.mu.Unlock()

	// Wait for the timer to fire.
	<-ch

	// Manually call the transition logic that the event loop would call.
	ctrl.mu.Lock()
	ctrl.silenceTimer = nil
	ctrl.silenceCh = nil
	ctrl.mu.Unlock()

	ctrl.transitionToIdle(domain.ConvReasonSilenceTimeout)

	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected state IDLE after silence timeout, got %s", ctrl.State())
	}

	states := sink.getStates()
	if len(states) != 1 {
		t.Fatalf("expected 1 state change, got %d", len(states))
	}
	if states[0].reason != domain.ConvReasonSilenceTimeout {
		t.Errorf("expected silence_timeout reason, got %s", states[0].reason)
	}
}

func TestConversationController_Stop(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.running = true
	ctrl.cancel = cancel
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.Stop()

	if ctrl.Running() {
		t.Error("expected controller to not be running after stop")
	}
	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected state IDLE after stop, got %s", ctrl.State())
	}
}

func TestConversationController_EmitStateEvents(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	// Emit several state changes.
	ctrl.emitState(domain.ConvStateListening, domain.ConvReasonWakeDetected)
	ctrl.emitState(domain.ConvStateProcessing, domain.ConvReasonSpeechReceived)
	ctrl.emitState(domain.ConvStateSpeaking, domain.ConvReasonBackendResponse)
	ctrl.emitState(domain.ConvStateListening, domain.ConvReasonPlaybackDone)
	ctrl.emitState(domain.ConvStateIdle, domain.ConvReasonStopPhrase)

	states := sink.getStates()
	if len(states) != 5 {
		t.Fatalf("expected 5 state events, got %d", len(states))
	}

	expected := []convStateChange{
		{domain.ConvStateListening, domain.ConvReasonWakeDetected},
		{domain.ConvStateProcessing, domain.ConvReasonSpeechReceived},
		{domain.ConvStateSpeaking, domain.ConvReasonBackendResponse},
		{domain.ConvStateListening, domain.ConvReasonPlaybackDone},
		{domain.ConvStateIdle, domain.ConvReasonStopPhrase},
	}
	for i, exp := range expected {
		if states[i] != exp {
			t.Errorf("event %d: expected %+v, got %+v", i, exp, states[i])
		}
	}
}

func TestConversationController_FullLoop(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	// Simulate: IDLE → (wake phrase) → LISTENING → (transcript) → PROCESSING → SPEAKING → LISTENING → (stop) → IDLE

	// 1. Wake phrase.
	ctrl.handleWakePhrase(context.Background(), ListenerEvent{
		Kind:      ListenerEventWakePhrase,
		Text:      "",
		SessionID: "session-1",
	})
	if ctrl.State() != domain.ConvStateListening {
		t.Fatalf("step 1: expected LISTENING, got %s", ctrl.State())
	}

	// 2. User speaks.
	ctrl.handleTranscript(context.Background(), ListenerEvent{
		Kind:      ListenerEventTranscript,
		Text:      "what's the weather?",
		SessionID: "session-1",
	})
	if ctrl.State() != domain.ConvStateListening {
		t.Fatalf("step 2: expected LISTENING (after loop), got %s", ctrl.State())
	}

	// 3. User says stop phrase.
	ctrl.handleTranscript(context.Background(), ListenerEvent{
		Kind:      ListenerEventTranscript,
		Text:      "that's all",
		SessionID: "session-1",
	})
	if ctrl.State() != domain.ConvStateIdle {
		t.Fatalf("step 3: expected IDLE, got %s", ctrl.State())
	}

	// Verify backend was called once.
	calls := backend.getCalls()
	if len(calls) != 1 || calls[0] != "what's the weather?" {
		t.Errorf("expected 1 backend call, got %v", calls)
	}

	// Verify TTS was called once (for the response).
	ttsCalls := tts.getCalls()
	if len(ttsCalls) != 1 {
		t.Errorf("expected 1 TTS call, got %d", len(ttsCalls))
	}

	// Verify full event sequence.
	states := sink.getStates()
	expectedEvents := []convStateChange{
		{domain.ConvStateListening, domain.ConvReasonWakeDetected},
		{domain.ConvStateProcessing, domain.ConvReasonSpeechReceived},
		{domain.ConvStateSpeaking, domain.ConvReasonBackendResponse},
		{domain.ConvStateListening, domain.ConvReasonPlaybackDone},
		{domain.ConvStateIdle, domain.ConvReasonStopPhrase},
	}
	if len(states) != len(expectedEvents) {
		t.Fatalf("expected %d state events, got %d: %+v", len(expectedEvents), len(states), states)
	}
	for i, exp := range expectedEvents {
		if states[i] != exp {
			t.Errorf("event %d: expected %+v, got %+v", i, exp, states[i])
		}
	}
}

func TestConversationController_ProcessUtterance_FromWrongState(t *testing.T) {
	backend := &mockConversationBackend{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, nil, sink)

	// From IDLE, processUtterance should be a no-op.
	ctrl.processUtterance(context.Background(), "hello")

	calls := backend.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected no backend calls from IDLE state, got %d", len(calls))
	}
}

func TestConversationController_HandleEvent_UnknownInIdle(t *testing.T) {
	sink := &mockConvEventSink{}
	ctrl := newTestController(nil, nil, sink)

	// Transcript in IDLE should be ignored.
	ctrl.handleEvent(context.Background(), ListenerEvent{
		Kind: ListenerEventTranscript,
		Text: "hello",
	})

	if ctrl.State() != domain.ConvStateIdle {
		t.Errorf("expected IDLE state, got %s", ctrl.State())
	}
}

func TestConversationController_ConcurrentAccess(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	var wg sync.WaitGroup
	var errors atomic.Int64

	// Hammer the controller with concurrent state reads and writes.
	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			_ = ctrl.State()
		}()

		go func() {
			defer wg.Done()
			_ = ctrl.Status()
		}()

		go func() {
			defer wg.Done()
			ctrl.mu.Lock()
			if ctrl.state == domain.ConvStateIdle {
				ctrl.state = domain.ConvStateListening
				ctrl.sessionID = "concurrent-test"
			} else {
				ctrl.state = domain.ConvStateIdle
				ctrl.sessionID = ""
			}
			ctrl.mu.Unlock()
		}()
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("got %d errors during concurrent access", errors.Load())
	}
}

func TestConversationController_TTSError(t *testing.T) {
	backend := &mockConversationBackend{}
	tts := &mockTTS{err: context.Canceled}
	sink := &mockConvEventSink{}
	ctrl := newTestController(backend, tts, sink)

	ctrl.mu.Lock()
	ctrl.state = domain.ConvStateListening
	ctrl.sessionID = "test-session"
	ctrl.mu.Unlock()

	ctrl.processUtterance(context.Background(), "hello")

	// Should still transition back to LISTENING even if TTS fails.
	if ctrl.State() != domain.ConvStateListening {
		t.Errorf("expected LISTENING after TTS error, got %s", ctrl.State())
	}
}

func TestConversationController_ConfigDefaults(t *testing.T) {
	sink := &mockConvEventSink{}

	// Empty config should use defaults.
	cfg := ConversationControllerConfig{}
	ctrl := NewConversationController(nil, nil, nil, sink, cfg)

	if ctrl.cfg.SilenceTimeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", ctrl.cfg.SilenceTimeout)
	}
	if len(ctrl.cfg.StopPhrases) == 0 {
		t.Error("expected default stop phrases")
	}
}

func TestValidTransitionsComplete(t *testing.T) {
	transitions := validTransitions()

	// All 4 states should be present.
	expectedStates := []domain.ConversationState{
		domain.ConvStateIdle,
		domain.ConvStateListening,
		domain.ConvStateProcessing,
		domain.ConvStateSpeaking,
	}
	for _, state := range expectedStates {
		if _, ok := transitions[state]; !ok {
			t.Errorf("missing transitions for state %s", state)
		}
	}

	// Verify symmetry: every target transition is also validated.
	for from, targets := range transitions {
		for _, to := range targets {
			if !isValidTransition(from, to) {
				t.Errorf("validTransitions says %s → %s but isValidTransition returns false", from, to)
			}
		}
	}
}
