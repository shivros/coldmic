package daemon

import (
	"time"

	"coldmic/internal/domain"
)

type ErrorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type StatusResponse struct {
	OK     bool          `json:"ok"`
	Status domain.Status `json:"status"`
}

type StopResponse struct {
	OK     bool              `json:"ok"`
	Status domain.Status     `json:"status"`
	Result domain.StopResult `json:"result"`
}

type LatestTranscriptResponse struct {
	OK       bool              `json:"ok"`
	Captured time.Time         `json:"captured"`
	Result   domain.StopResult `json:"result"`
}
