package usecase

import (
	"context"
	"io"
	"testing"
	"time"

	"coldmic/internal/audio"
	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

func TestContinuousListenerStartStop(t *testing.T) {
	t.Parallel()

	// Fake audio that returns EOF immediately (silence).
	audioCap := &fakeAudioCapture{
		sessions: []ports.AudioSession{
			&fakeEOFSession{},
		},
	}
	vad := audio.NewEnergyVAD(500)
	provider := &fakeProvider{
		sessions: []ports.StreamingSession{newFakeStreamingSession()},
	}
	events := &fakeEventSink{}

	cfg := ContinuousListenerConfig{
		WakePhrases:  []string{"hey alice", "alice"},
		VADThreshold: 500,
		SilenceMs:    200,
		FrameMs:      30,
		Audio: ports.AudioConfig{
			SampleRate: 16000,
			Channels:   1,
		},
		Streaming: ports.StreamingConfig{
			SampleRate:     16000,
			Channels:       1,
			Encoding:       "linear16",
			InterimResults: true,
		},
		ChunkSize: 4096,
	}

	listener := NewContinuousListener(audioCap, vad, provider, events, cfg)

	err := listener.Start(context.Background())
	if err != nil {
		t.Fatalf("expected clean exit on EOF, got %v", err)
	}

	if listener.Running() {
		t.Fatalf("expected listener to not be running after EOF")
	}
}

func TestContinuousListenerCtxCancel(t *testing.T) {
	t.Parallel()

	// Fake audio that blocks until context is cancelled.
	audioCap := &fakeAudioCapture{
		sessions: []ports.AudioSession{
			&fakeBlockingSession{done: make(chan struct{})},
		},
	}
	vad := audio.NewEnergyVAD(500)
	provider := &fakeProvider{
		sessions: []ports.StreamingSession{newFakeStreamingSession()},
	}
	events := &fakeEventSink{}

	cfg := ContinuousListenerConfig{
		WakePhrases:  []string{"hey alice"},
		VADThreshold: 500,
		SilenceMs:    200,
		FrameMs:      30,
		Audio:        ports.AudioConfig{SampleRate: 16000, Channels: 1},
		Streaming:    ports.StreamingConfig{SampleRate: 16000, Channels: 1, Encoding: "linear16"},
		ChunkSize:    4096,
	}

	listener := NewContinuousListener(audioCap, vad, provider, events, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- listener.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	if !listener.Running() {
		t.Fatalf("expected listener to be running")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("listener did not stop in time")
	}

	if listener.Running() {
		t.Fatalf("expected listener to not be running after stop")
	}
}

func TestContinuousListenerCannotStartTwice(t *testing.T) {
	t.Parallel()

	audioCap := &fakeAudioCapture{
		sessions: []ports.AudioSession{
			&fakeBlockingSession{done: make(chan struct{})},
		},
	}
	vad := audio.NewEnergyVAD(500)
	provider := &fakeProvider{
		sessions: []ports.StreamingSession{newFakeStreamingSession()},
	}
	events := &fakeEventSink{}

	cfg := ContinuousListenerConfig{
		WakePhrases:  []string{"hey alice"},
		VADThreshold: 500,
		SilenceMs:    200,
		FrameMs:      30,
		Audio:        ports.AudioConfig{SampleRate: 16000, Channels: 1},
		Streaming:    ports.StreamingConfig{SampleRate: 16000, Channels: 1, Encoding: "linear16"},
		ChunkSize:    4096,
	}

	listener := NewContinuousListener(audioCap, vad, provider, events, cfg)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	errCh := make(chan error, 1)
	go func() {
		errCh <- listener.Start(ctx1)
	}()

	time.Sleep(50 * time.Millisecond)

	err := listener.Start(context.Background())
	if err != domain.ErrContinuousActive {
		t.Fatalf("expected ErrContinuousActive, got %v", err)
	}

	cancel1()
	<-errCh
}

func TestContinuousListenerWakePhraseMatching(t *testing.T) {
	t.Parallel()

	cfg := ContinuousListenerConfig{
		WakePhrases: []string{"hey alice", "alice"},
	}
	events := &fakeEventSink{}
	listener := NewContinuousListener(nil, nil, nil, events, cfg)

	text, ok := listener.matchWakePhrase("Hey Alice, what's the weather?")
	if !ok {
		t.Fatalf("expected wake phrase match")
	}
	if text != "what's the weather?" {
		t.Fatalf("unexpected remaining text: %q", text)
	}

	text, ok = listener.matchWakePhrase("alice set a timer")
	if !ok {
		t.Fatalf("expected wake phrase match for 'alice'")
	}
	if text != "set a timer" {
		t.Fatalf("unexpected remaining text: %q", text)
	}

	_, ok = listener.matchWakePhrase("I need to grab coffee")
	if ok {
		t.Fatalf("expected no wake phrase match")
	}
}

func TestContinuousListenerWakePhraseEmptyPhrases(t *testing.T) {
	t.Parallel()

	cfg := ContinuousListenerConfig{
		WakePhrases: []string{},
	}
	events := &fakeEventSink{}
	listener := NewContinuousListener(nil, nil, nil, events, cfg)

	_, ok := listener.matchWakePhrase("Hey Alice, what's the weather?")
	if ok {
		t.Fatalf("expected no match with empty wake phrases")
	}
}

func TestContinuousListenerEventsReturnsChannel(t *testing.T) {
	t.Parallel()

	cfg := ContinuousListenerConfig{
		WakePhrases: []string{"hey alice"},
	}
	events := &fakeEventSink{}
	listener := NewContinuousListener(nil, nil, nil, events, cfg)

	ch := listener.Events()
	if ch == nil {
		t.Fatal("expected non-nil events channel")
	}

	// Should be the same channel as the internal outCh.
	if ch != listener.Events() {
		t.Fatal("expected Events() to return the same channel")
	}
}

func TestContinuousListenerEmitSendsToChannel(t *testing.T) {
	t.Parallel()

	cfg := ContinuousListenerConfig{
		WakePhrases: []string{"hey alice"},
	}
	events := &fakeEventSink{}
	listener := NewContinuousListener(nil, nil, nil, events, cfg)

	evt := ListenerEvent{
		Kind:      ListenerEventWakePhrase,
		Text:      "test",
		SessionID: "session-1",
	}
	listener.emit(evt)

	select {
	case got := <-listener.Events():
		if got.Kind != ListenerEventWakePhrase || got.Text != "test" || got.SessionID != "session-1" {
			t.Fatalf("unexpected event: %+v", got)
		}
	default:
		t.Fatal("expected event on channel")
	}
}

func TestContinuousListenerEmitDropsWhenFull(t *testing.T) {
	t.Parallel()

	cfg := ContinuousListenerConfig{
		WakePhrases: []string{"hey alice"},
	}
	events := &fakeEventSink{}
	listener := NewContinuousListener(nil, nil, nil, events, cfg)

	// Fill the channel (capacity is 32).
	for i := 0; i < 32; i++ {
		listener.emit(ListenerEvent{Kind: ListenerEventTranscript, Text: "fill"})
	}

	// This emit should be dropped (channel full).
	listener.emit(ListenerEvent{Kind: ListenerEventWakePhrase, Text: "dropped"})

	// Drain and verify all events are "fill".
	count := 0
	for {
		select {
		case <-listener.Events():
			count++
		default:
			goto done
		}
	}
done:
	if count != 32 {
		t.Fatalf("expected 32 events, got %d", count)
	}
}

// fakeEOFSession immediately returns EOF on Read.
type fakeEOFSession struct{}

func (f *fakeEOFSession) Read(_ []byte) (int, error) { return 0, io.EOF }
func (f *fakeEOFSession) Close() error               { return nil }
func (f *fakeEOFSession) Stop() error                { return nil }

// fakeBlockingSession blocks on Read until done is closed.
type fakeBlockingSession struct {
	done chan struct{}
}

func (f *fakeBlockingSession) Read(_ []byte) (int, error) {
	<-f.done
	return 0, io.EOF
}
func (f *fakeBlockingSession) Close() error { return nil }
func (f *fakeBlockingSession) Stop() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}
