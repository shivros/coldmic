package domain

import "errors"

var (
	ErrNoActiveSession       = errors.New("no active recording session")
	ErrNoTranscriptAvailable = errors.New("no transcript available")
	ErrContinuousActive      = errors.New("continuous listening session is already active")
	ErrNoContinuousSession   = errors.New("no active continuous listening session")

	// Conversation controller errors.
	ErrConversationActive    = errors.New("conversation is already active")
	ErrNoConversationActive  = errors.New("no active conversation")
	ErrInvalidTransition     = errors.New("invalid state transition")
)
