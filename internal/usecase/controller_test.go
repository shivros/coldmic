package usecase

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

func TestSessionControllerStartStopSuccess(t *testing.T) {
	t.Parallel()

	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	streamSession := newFakeStreamingSession()
	streamSession.events <- domain.TranscriptEvent{Kind: domain.TranscriptKindPartial, Text: "hello"}
	streamSession.events <- domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "hello world"}
	provider := &fakeProvider{sessions: []ports.StreamingSession{streamSession}}
	rules := &fakeRules{transform: "HELLO WORLD"}
	clipboard := &fakeClipboard{}
	events := &fakeEventSink{}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		provider,
		rules,
		clipboard,
		events,
		Config{ChunkSize: 512, StreamingGrace: 0},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	result, err := controller.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if result.RawTranscript != "hello world" {
		t.Fatalf("unexpected raw transcript: %q", result.RawTranscript)
	}
	if result.FinalTranscript != "HELLO WORLD" {
		t.Fatalf("unexpected final transcript: %q", result.FinalTranscript)
	}
	if !result.Copied {
		t.Fatalf("expected copied=true")
	}

	if clipboard.lastText != "HELLO WORLD" {
		t.Fatalf("clipboard did not receive transformed transcript")
	}

	if len(events.partials) == 0 || events.partials[0] != "hello" {
		t.Fatalf("expected partial transcript event")
	}
	if len(events.finals) == 0 || events.finals[0].transformed != "HELLO WORLD" {
		t.Fatalf("expected final transcript event")
	}

	states := events.snapshotStates()
	if len(states) < 3 {
		t.Fatalf("expected at least 3 state transitions, got %d", len(states))
	}
	if states[0].reason != domain.SessionReasonRecordingStarted {
		t.Fatalf("unexpected first reason: %s", states[0].reason)
	}
	if states[1].reason != domain.SessionReasonTranscribing {
		t.Fatalf("unexpected second reason: %s", states[1].reason)
	}
	if states[len(states)-1].reason != domain.SessionReasonTranscriptCopied {
		t.Fatalf("unexpected final reason: %s", states[len(states)-1].reason)
	}
}

func TestSessionControllerStopWithoutActiveSession(t *testing.T) {
	t.Parallel()

	controller := NewSessionController(
		&fakeAudioCapture{},
		&fakeProvider{},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)

	_, err := controller.Stop(context.Background())
	if !errors.Is(err, ErrNoActiveSession) {
		t.Fatalf("expected ErrNoActiveSession, got %v", err)
	}
}

func TestSessionControllerAbortLifecycle(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	events := &fakeEventSink{}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{},
		&fakeClipboard{},
		events,
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := controller.Abort(); err != nil {
		t.Fatalf("abort failed: %v", err)
	}

	states := events.snapshotStates()
	if states[len(states)-1].reason != domain.SessionReasonRecordingDiscarded {
		t.Fatalf("expected discarded reason, got %s", states[len(states)-1].reason)
	}
}

func TestSessionControllerStopClipboardFailureIsNonFatal(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	streamSession.events <- domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "text"}
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	events := &fakeEventSink{}
	clipboard := &fakeClipboard{err: errors.New("clipboard down")}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{transform: "text"},
		clipboard,
		events,
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	result, err := controller.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if result.Copied {
		t.Fatalf("expected copied=false when clipboard fails")
	}

	states := events.snapshotStates()
	if states[len(states)-1].reason != domain.SessionReasonTranscriptReadyClipboardFailed {
		t.Fatalf("unexpected final reason: %s", states[len(states)-1].reason)
	}

	errorsGot := events.snapshotErrors()
	if len(errorsGot) == 0 || errorsGot[len(errorsGot)-1].code != domain.ErrorCodeClipboard {
		t.Fatalf("expected clipboard error event")
	}
}

func TestSessionControllerStopRulesFailure(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	streamSession.events <- domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "text"}
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	events := &fakeEventSink{}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{err: errors.New("bad rules")},
		&fakeClipboard{},
		events,
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	_, err := controller.Stop(context.Background())
	if err == nil {
		t.Fatalf("expected rules error")
	}

	states := events.snapshotStates()
	if states[len(states)-1].reason != domain.SessionReasonRulesFailed {
		t.Fatalf("expected rules_failed, got %s", states[len(states)-1].reason)
	}
}

func TestSessionControllerStopNoTranscriptWithStreamError(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	streamSession.waitErr = errors.New("stream failed")
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	events := &fakeEventSink{}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{},
		&fakeClipboard{},
		events,
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	_, err := controller.Stop(context.Background())
	if err == nil || err.Error() != "stream failed" {
		t.Fatalf("expected stream failure, got %v", err)
	}

	states := events.snapshotStates()
	if states[len(states)-1].reason != domain.SessionReasonTranscriptionFailed {
		t.Fatalf("expected transcription_failed, got %s", states[len(states)-1].reason)
	}
}

func TestSessionControllerStartRestartStopsPreviousSession(t *testing.T) {
	t.Parallel()

	firstStream := newFakeStreamingSession()
	secondStream := newFakeStreamingSession()
	firstAudio := &fakeAudioSession{chunks: [][]byte{[]byte("a")}}
	secondAudio := &fakeAudioSession{chunks: [][]byte{[]byte("b")}}
	events := &fakeEventSink{}

	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{firstAudio, secondAudio}},
		&fakeProvider{sessions: []ports.StreamingSession{firstStream, secondStream}},
		&fakeRules{},
		&fakeClipboard{},
		events,
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("second start failed: %v", err)
	}

	if firstAudio.stopCalls == 0 {
		t.Fatalf("expected first session audio to be stopped on restart")
	}
	if firstStream.closeCalls == 0 {
		t.Fatalf("expected first stream to be closed on restart")
	}

	states := events.snapshotStates()
	if states[len(states)-1].reason != domain.SessionReasonRecordingRestarted {
		t.Fatalf("expected recording_restarted reason")
	}
}

func TestSessionControllerStatusActive(t *testing.T) {
	t.Parallel()

	streamSession := newFakeStreamingSession()
	audioSession := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	controller := NewSessionController(
		&fakeAudioCapture{sessions: []ports.AudioSession{audioSession}},
		&fakeProvider{sessions: []ports.StreamingSession{streamSession}},
		&fakeRules{},
		&fakeClipboard{},
		&fakeEventSink{},
		Config{},
	)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	status := controller.Status()
	if status.State != domain.SessionStateRecording || !status.Active {
		t.Fatalf("unexpected status: %+v", status)
	}
}

type fakeAudioCapture struct {
	sessions []ports.AudioSession
	err      error
	calls    int
}

func (f *fakeAudioCapture) Start(_ context.Context, _ ports.AudioConfig) (ports.AudioSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.sessions) {
		return nil, errors.New("no audio session configured")
	}
	session := f.sessions[f.calls]
	f.calls++
	return session, nil
}

type fakeAudioSession struct {
	mu        sync.Mutex
	chunks    [][]byte
	index     int
	stopCalls int
	stopErr   error
}

func (f *fakeAudioSession) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.index >= len(f.chunks) {
		return 0, io.EOF
	}
	n := copy(p, f.chunks[f.index])
	f.index++
	return n, nil
}

func (f *fakeAudioSession) Close() error { return nil }

func (f *fakeAudioSession) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	return f.stopErr
}

type fakeProvider struct {
	sessions []ports.StreamingSession
	err      error
	calls    int
}

func (f *fakeProvider) StartStreaming(_ context.Context, _ ports.StreamingConfig) (ports.StreamingSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.sessions) {
		return nil, errors.New("no stream session configured")
	}
	session := f.sessions[f.calls]
	f.calls++
	return session, nil
}

type fakeStreamingSession struct {
	events     chan domain.TranscriptEvent
	waitErr    error
	closeSend  int
	closeCalls int
	closed     bool
	mu         sync.Mutex
}

func newFakeStreamingSession() *fakeStreamingSession {
	return &fakeStreamingSession{events: make(chan domain.TranscriptEvent, 16)}
}

func (f *fakeStreamingSession) SendAudio(_ []byte) error { return nil }

func (f *fakeStreamingSession) CloseSend() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeSend++
	if !f.closed {
		close(f.events)
		f.closed = true
	}
	return nil
}

func (f *fakeStreamingSession) Events() <-chan domain.TranscriptEvent { return f.events }

func (f *fakeStreamingSession) Wait() error {
	time.Sleep(5 * time.Millisecond)
	return f.waitErr
}

func (f *fakeStreamingSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	if !f.closed {
		close(f.events)
		f.closed = true
	}
	return nil
}

type fakeRules struct {
	transform string
	err       error
}

func (f *fakeRules) Apply(text string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.transform != "" {
		return f.transform, nil
	}
	return text, nil
}

type fakeClipboard struct {
	lastText string
	err      error
}

func (f *fakeClipboard) SetText(_ context.Context, text string) error {
	f.lastText = text
	return f.err
}

type fakeEventSink struct {
	mu sync.Mutex

	states   []stateEvent
	finals   []finalEvent
	partials []string
	errors   []errEvent
}

type stateEvent struct {
	state  domain.SessionState
	reason domain.SessionStateReason
}

type finalEvent struct {
	raw         string
	transformed string
}

type errEvent struct {
	code   domain.ErrorCode
	detail string
}

func (f *fakeEventSink) SessionStateChanged(state domain.SessionState, reason domain.SessionStateReason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = append(f.states, stateEvent{state: state, reason: reason})
}

func (f *fakeEventSink) PartialTranscript(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.partials = append(f.partials, text)
}

func (f *fakeEventSink) FinalTranscript(raw string, transformed string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finals = append(f.finals, finalEvent{raw: raw, transformed: transformed})
}

func (f *fakeEventSink) SessionError(code domain.ErrorCode, detail string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = append(f.errors, errEvent{code: code, detail: detail})
}

func (f *fakeEventSink) snapshotStates() []stateEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]stateEvent, len(f.states))
	copy(out, f.states)
	return out
}

func (f *fakeEventSink) snapshotErrors() []errEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]errEvent, len(f.errors))
	copy(out, f.errors)
	return out
}
