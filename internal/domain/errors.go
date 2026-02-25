package domain

import "errors"

var (
	ErrNoActiveSession       = errors.New("no active recording session")
	ErrNoTranscriptAvailable = errors.New("no transcript available")
)
