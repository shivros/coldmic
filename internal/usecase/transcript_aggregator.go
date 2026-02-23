package usecase

import (
	"strings"
	"sync"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

type transcriptAggregator struct {
	mu         sync.Mutex
	finals     []string
	lastSpoken string
}

func newTranscriptAggregator() *transcriptAggregator {
	return &transcriptAggregator{}
}

func (a *transcriptAggregator) Add(event domain.TranscriptEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	text := strings.TrimSpace(event.Text)
	if text == "" {
		return
	}
	a.lastSpoken = text
	if event.Kind == domain.TranscriptKindFinal {
		a.finals = append(a.finals, text)
	}
}

func (a *transcriptAggregator) Raw() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	joined := strings.TrimSpace(strings.Join(a.finals, " "))
	if joined == "" {
		return a.lastSpoken
	}

	if a.lastSpoken == "" {
		return joined
	}

	if strings.HasSuffix(joined, a.lastSpoken) {
		return joined
	}

	if len(a.lastSpoken) > len(joined) {
		return strings.TrimSpace(joined + " " + a.lastSpoken)
	}

	return joined
}

func consumeTranscriptionEvents(
	session ports.StreamingSession,
	aggregator *transcriptAggregator,
	events ports.EventSink,
	done chan struct{},
) {
	defer close(done)

	for event := range session.Events() {
		text := strings.TrimSpace(event.Text)
		if text == "" {
			continue
		}
		aggregator.Add(event)
		if event.Kind == domain.TranscriptKindPartial {
			events.PartialTranscript(text)
		}
	}
}
