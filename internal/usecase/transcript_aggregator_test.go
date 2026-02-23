package usecase

import (
	"testing"

	"coldmic/internal/domain"
)

func TestTranscriptAggregatorUsesFinalsAndLastSpokenFallback(t *testing.T) {
	t.Parallel()

	agg := newTranscriptAggregator()
	agg.Add(domain.TranscriptEvent{Kind: domain.TranscriptKindPartial, Text: "hello"})
	agg.Add(domain.TranscriptEvent{Kind: domain.TranscriptKindFinal, Text: "hello world"})
	agg.Add(domain.TranscriptEvent{Kind: domain.TranscriptKindPartial, Text: "hello world again"})

	got := agg.Raw()
	if got != "hello world hello world again" {
		t.Fatalf("unexpected transcript: %q", got)
	}
}

func TestTranscriptAggregatorIgnoresEmpty(t *testing.T) {
	t.Parallel()

	agg := newTranscriptAggregator()
	agg.Add(domain.TranscriptEvent{Kind: domain.TranscriptKindPartial, Text: "   "})
	if got := agg.Raw(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
