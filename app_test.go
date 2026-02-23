package main

import (
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
