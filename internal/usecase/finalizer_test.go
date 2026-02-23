package usecase

import (
	"context"
	"errors"
	"testing"

	"coldmic/internal/domain"
)

func TestTranscriptFinalizerRulesFailure(t *testing.T) {
	t.Parallel()

	events := &fakeEventSink{}
	f := newTranscriptFinalizer(&fakeRules{err: errors.New("rules")}, &fakeClipboard{}, events)

	_, reason, err := f.Finalize(context.Background(), "raw")
	if err == nil {
		t.Fatalf("expected rules error")
	}
	if reason != domain.SessionReasonRulesFailed {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestTranscriptFinalizerClipboardFailure(t *testing.T) {
	t.Parallel()

	events := &fakeEventSink{}
	clipboard := &fakeClipboard{err: errors.New("clipboard")}
	f := newTranscriptFinalizer(&fakeRules{transform: "final"}, clipboard, events)

	result, reason, err := f.Finalize(context.Background(), "raw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Copied {
		t.Fatalf("expected copied=false")
	}
	if reason != domain.SessionReasonTranscriptReadyClipboardFailed {
		t.Fatalf("unexpected reason: %s", reason)
	}
}
