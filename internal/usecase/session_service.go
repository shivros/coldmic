package usecase

import (
	"context"
	"sync"
	"time"

	"coldmic/internal/domain"
)

// SessionService provides an application-level API for session lifecycle control.
type SessionService struct {
	controller *SessionController

	mu     sync.RWMutex
	latest *domain.LatestTranscript
}

func NewSessionService(controller *SessionController) *SessionService {
	return &SessionService{controller: controller}
}

func (s *SessionService) Start(ctx context.Context) error {
	return s.controller.Start(ctx)
}

func (s *SessionService) Stop(ctx context.Context) (domain.StopResult, error) {
	result, err := s.controller.Stop(ctx)
	if err != nil {
		return domain.StopResult{}, err
	}

	s.mu.Lock()
	s.latest = &domain.LatestTranscript{
		Result:     result,
		CapturedAt: time.Now().UTC(),
	}
	s.mu.Unlock()

	return result, nil
}

func (s *SessionService) Abort() error {
	return s.controller.Abort()
}

func (s *SessionService) Status() domain.Status {
	return s.controller.Status()
}

func (s *SessionService) LastTranscript() (domain.LatestTranscript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return domain.LatestTranscript{}, domain.ErrNoTranscriptAvailable
	}
	return *s.latest, nil
}
