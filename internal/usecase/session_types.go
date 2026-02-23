package usecase

import (
	"sync"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

type activeSession struct {
	cancel func()
	audio  ports.AudioSession
	stream ports.StreamingSession

	stateMu sync.Mutex
	state   domain.SessionState

	aggregator *transcriptAggregator
	eventsDone chan struct{}
	audioDone  chan struct{}
}

func (s *activeSession) setState(state domain.SessionState) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = state
}

func (s *activeSession) getState() domain.SessionState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}
