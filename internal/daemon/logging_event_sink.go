package daemon

import (
	"log"

	"coldmic/internal/domain"
)

// LoggingEventSink writes backend lifecycle events to the daemon log.
type LoggingEventSink struct{}

func (LoggingEventSink) SessionStateChanged(state domain.SessionState, reason domain.SessionStateReason) {
	log.Printf("session state=%s reason=%s", state, reason)
}

func (LoggingEventSink) PartialTranscript(text string) {
	log.Printf("partial transcript=%q", text)
}

func (LoggingEventSink) FinalTranscript(raw string, transformed string, sessionID string) {
	log.Printf("final transcript session_id=%s raw=%q transformed=%q", sessionID, raw, transformed)
}

func (LoggingEventSink) SessionError(code domain.ErrorCode, detail string) {
	log.Printf("session error code=%s detail=%q", code, detail)
}
