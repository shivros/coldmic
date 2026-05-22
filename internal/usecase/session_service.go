package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"coldmic/internal/domain"
)

// SessionService provides an application-level API for session lifecycle control.
type SessionService struct {
	controller             *SessionController
	continuousListener     *ContinuousListener
	conversationController *ConversationController

	mu     sync.RWMutex
	latest *domain.LatestTranscript
}

func NewSessionService(controller *SessionController) *SessionService {
	return &SessionService{controller: controller}
}

// NewSessionServiceWithContinuous creates a SessionService with continuous listening support.
func NewSessionServiceWithContinuous(controller *SessionController, listener *ContinuousListener) *SessionService {
	return &SessionService{
		controller:         controller,
		continuousListener: listener,
	}
}

// NewSessionServiceWithConversation creates a SessionService with full conversation support.
func NewSessionServiceWithConversation(
	controller *SessionController,
	listener *ContinuousListener,
	conversationCtrl *ConversationController,
) *SessionService {
	return &SessionService{
		controller:             controller,
		continuousListener:     listener,
		conversationController: conversationCtrl,
	}
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
	// Check continuous listener first.
	if s.continuousListener != nil && s.continuousListener.Running() {
		return domain.Status{
			State:  domain.SessionStateContinuous,
			Active: true,
			Mode:   "continuous",
		}
	}

	status := s.controller.Status()
	status.Mode = "ptt"
	return status
}

func (s *SessionService) LastTranscript() (domain.LatestTranscript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return domain.LatestTranscript{}, domain.ErrNoTranscriptAvailable
	}
	return *s.latest, nil
}

// StartContinuous starts VAD-gated continuous listening. The listener runs in a
// background goroutine and emits events on its Events() channel.
func (s *SessionService) StartContinuous(ctx context.Context) error {
	if s.continuousListener == nil {
		return errors.New("continuous listening not configured")
	}
	// Start synchronously checks running state under its own mutex, preventing
	// the double-start race that existed with the goroutine+Running() pattern.
	// It blocks for the duration of the listening session (until ctx cancelled).
	return s.continuousListener.Start(ctx)
}

// StopContinuous stops an active continuous listening session.
func (s *SessionService) StopContinuous() error {
	if s.continuousListener == nil {
		return errors.New("continuous listening not configured")
	}
	if !s.continuousListener.Running() {
		return domain.ErrNoContinuousSession
	}
	s.continuousListener.Stop()
	return nil
}

// StartConversation starts the conversation controller loop. This runs in a
// background goroutine and coordinates with the ContinuousListener, backend,
// and TTS subsystems.
func (s *SessionService) StartConversation(ctx context.Context) error {
	if s.conversationController == nil {
		return errors.New("conversation mode not configured")
	}
	if s.continuousListener == nil {
		return errors.New("continuous listening not configured")
	}
	// Start the continuous listener first (it feeds events to the controller).
	go func() {
		_ = s.continuousListener.Start(context.WithoutCancel(ctx))
	}()
	// Start the conversation controller (subscribes to listener events).
	go func() {
		_ = s.conversationController.Start(context.WithoutCancel(ctx))
	}()
	return nil
}

// StopConversation stops an active conversation session.
func (s *SessionService) StopConversation() error {
	if s.conversationController == nil {
		return errors.New("conversation mode not configured")
	}
	if !s.conversationController.Running() {
		return domain.ErrNoConversationActive
	}
	// Stop the conversation controller first (it will drain listener events).
	s.conversationController.Stop()
	// Then stop the continuous listener.
	if s.continuousListener != nil && s.continuousListener.Running() {
		s.continuousListener.Stop()
	}
	return nil
}

// ConversationStatus returns the current conversation controller status.
func (s *SessionService) ConversationStatus() domain.ConversationStatus {
	if s.conversationController == nil {
		return domain.ConversationStatus{State: domain.ConvStateIdle}
	}
	return s.conversationController.Status()
}
