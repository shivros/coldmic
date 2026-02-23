package ports

import (
	"context"
	"io"

	"coldmic/internal/domain"
)

// AudioConfig describes how the microphone should be captured.
type AudioConfig struct {
	SampleRate  int
	Channels    int
	InputFormat string
	InputDevice string
}

// AudioSession is a live capture session.
type AudioSession interface {
	io.ReadCloser
	Stop() error
}

// AudioCapture creates microphone capture sessions.
type AudioCapture interface {
	Start(ctx context.Context, cfg AudioConfig) (AudioSession, error)
}

// StreamingConfig describes provider-agnostic streaming settings.
type StreamingConfig struct {
	SampleRate     int
	Channels       int
	Encoding       string
	InterimResults bool
}

// StreamingSession is an active provider websocket session.
type StreamingSession interface {
	SendAudio(chunk []byte) error
	CloseSend() error
	Events() <-chan domain.TranscriptEvent
	Wait() error
	Close() error
}

// TranscriptionProvider starts streaming transcription sessions.
type TranscriptionProvider interface {
	StartStreaming(ctx context.Context, cfg StreamingConfig) (StreamingSession, error)
}

// RulesEngine transforms transcripts using deterministic rules.
type RulesEngine interface {
	Apply(text string) (string, error)
}

// Clipboard writes text into the system clipboard.
type Clipboard interface {
	SetText(ctx context.Context, text string) error
}

// EventSink emits backend state/events to the UI.
type EventSink interface {
	SessionStateChanged(state domain.SessionState, reason domain.SessionStateReason)
	PartialTranscript(text string)
	FinalTranscript(raw string, transformed string)
	SessionError(code domain.ErrorCode, detail string)
}
