package main

import (
	"context"
	"errors"
	"testing"

	"coldmic/internal/domain"
)

func TestSessionReasonMessage(t *testing.T) {
	t.Parallel()

	cases := map[domain.SessionStateReason]string{
		domain.SessionReasonMicCold:                        "Mic cold",
		domain.SessionReasonRecordingStarted:               "Recording started",
		domain.SessionReasonRecordingRestarted:             "Recording restarted; previous capture discarded",
		domain.SessionReasonTranscribing:                   "Recording stopped. Transcribing...",
		domain.SessionReasonTranscriptCopied:               "Transcript copied to clipboard",
		domain.SessionReasonTranscriptReadyClipboardFailed: "Transcript ready (clipboard write failed)",
		domain.SessionReasonRecordingDiscarded:             "Recording discarded",
		domain.SessionReasonNoTranscript:                   "No transcript captured",
		domain.SessionReasonTranscriptionFailed:            "Transcription failed",
		domain.SessionReasonRulesFailed:                    "Rules processing failed",
	}

	for reason, want := range cases {
		reason := reason
		want := want
		t.Run(string(reason), func(t *testing.T) {
			t.Parallel()
			if got := sessionReasonMessage(reason); got != want {
				t.Fatalf("unexpected message: %q", got)
			}
		})
	}

	if got := sessionReasonMessage("unknown"); got != "" {
		t.Fatalf("expected empty unknown reason message, got %q", got)
	}
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	cases := map[domain.ErrorCode]string{
		domain.ErrorCodeStartup:       "Startup failed",
		domain.ErrorCodeAudioStop:     "Audio stop issue",
		domain.ErrorCodeAudioStream:   "Audio streaming issue",
		domain.ErrorCodeClipboard:     "Clipboard write failed",
		domain.ErrorCodeRules:         "Rules processing failed",
		domain.ErrorCodeTranscription: "Transcription error",
	}
	for code, want := range cases {
		code := code
		want := want
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			if got := errorMessage(code, "ignored"); got != want {
				t.Fatalf("unexpected message: %q", got)
			}
		})
	}

	if got := errorMessage("unknown", "detail"); got != "detail" {
		t.Fatalf("expected detail fallback, got %q", got)
	}
	if got := errorMessage("unknown", ""); got != "Unknown error" {
		t.Fatalf("expected unknown fallback, got %q", got)
	}
}

func TestRequireReady(t *testing.T) {
	t.Parallel()

	app := &App{}
	if err := app.requireReady(); err == nil {
		t.Fatalf("expected uninitialized error")
	}

	bootErr := errors.New("boot")
	app.bootErr = bootErr
	if err := app.requireReady(); !errors.Is(err, bootErr) {
		t.Fatalf("expected boot error, got %v", err)
	}
}

func TestGetStatusWhenNotInitialized(t *testing.T) {
	t.Parallel()

	app := &App{}
	status := app.GetStatus()
	if status.State != domain.SessionStateIdle || status.Active {
		t.Fatalf("unexpected status: %+v", status)
	}

	app.bootErr = errors.New("boot")
	status = app.GetStatus()
	if status.State != domain.SessionStateError || status.Active != false || status.Message != "boot" {
		t.Fatalf("unexpected boot status: %+v", status)
	}
}

func TestAppEventEmittersIncludeSessionID(t *testing.T) {
	app := &App{ctx: context.Background()}
	events := captureEvents(t)

	app.SessionStateChanged(domain.SessionStateIdle, domain.SessionReasonMicCold)
	app.PartialTranscript("partial")
	app.FinalTranscript("raw", "final", "session-1")
	app.SessionError(domain.ErrorCodeTranscription, "detail")

	if len(*events) != 4 {
		t.Fatalf("expected 4 emitted events, got %d", len(*events))
	}

	if (*events)[0].name != eventSession {
		t.Fatalf("expected first event %q, got %q", eventSession, (*events)[0].name)
	}
	if (*events)[0].payload["state"] != string(domain.SessionStateIdle) {
		t.Fatalf("unexpected session state payload: %+v", (*events)[0].payload)
	}
	if (*events)[1].name != eventPartial || (*events)[1].payload["text"] != "partial" {
		t.Fatalf("unexpected partial event payload: %+v", (*events)[1])
	}
	if (*events)[2].name != eventFinal {
		t.Fatalf("expected final event name %q, got %q", eventFinal, (*events)[2].name)
	}
	if (*events)[2].payload["sessionId"] != "session-1" {
		t.Fatalf("expected sessionId in final payload, got %+v", (*events)[2].payload)
	}
	if (*events)[3].name != eventError || (*events)[3].payload["code"] != string(domain.ErrorCodeTranscription) {
		t.Fatalf("unexpected error event payload: %+v", (*events)[3])
	}
}

func TestAppEventEmittersNoopWithoutContext(t *testing.T) {
	app := &App{}
	events := captureEvents(t)

	app.SessionStateChanged(domain.SessionStateIdle, domain.SessionReasonMicCold)
	app.PartialTranscript("partial")
	app.FinalTranscript("raw", "final", "session-2")
	app.SessionError(domain.ErrorCodeTranscription, "detail")

	if len(*events) != 0 {
		t.Fatalf("expected no events when app context is nil, got %d", len(*events))
	}
}

type emittedEvent struct {
	name    string
	payload map[string]string
}

func captureEvents(t *testing.T) *[]emittedEvent {
	t.Helper()

	events := []emittedEvent{}
	original := eventsEmit
	eventsEmit = func(_ context.Context, eventName string, optionalData ...interface{}) {
		payload := map[string]string{}
		if len(optionalData) > 0 {
			if data, ok := optionalData[0].(map[string]string); ok {
				for key, value := range data {
					payload[key] = value
				}
			}
		}
		events = append(events, emittedEvent{name: eventName, payload: payload})
	}

	t.Cleanup(func() {
		eventsEmit = original
	})
	return &events
}
