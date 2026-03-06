package daemon

import (
	"context"

	"coldmic/internal/domain"
)

// SessionService is the control surface required by daemon transports.
type SessionService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) (domain.StopResult, error)
	Abort() error
	Status() domain.Status
	LastTranscript() (domain.LatestTranscript, error)
}
