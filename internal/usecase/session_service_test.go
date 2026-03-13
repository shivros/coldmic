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
