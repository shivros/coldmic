package daemon

import "coldmic/internal/domain"

// NoopEventSink drops events in headless daemon mode.
type NoopEventSink struct{}

func (NoopEventSink) SessionStateChanged(_ domain.SessionState, _ domain.SessionStateReason) {}
func (NoopEventSink) PartialTranscript(_ string)                                             {}
func (NoopEventSink) FinalTranscript(_, _, _ string)                                         {}
func (NoopEventSink) SessionError(_ domain.ErrorCode, _ string)                              {}
