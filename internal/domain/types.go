package domain

// SessionState models the push-to-talk lifecycle.
type SessionState string

const (
	SessionStateIdle      SessionState = "idle"
	SessionStateRecording SessionState = "recording"
	SessionStateStopping  SessionState = "stopping"
	SessionStateError     SessionState = "error"
)

// SessionStateReason provides a structured reason for state transitions.
type SessionStateReason string

const (
	SessionReasonMicCold                        SessionStateReason = "mic_cold"
	SessionReasonRecordingStarted               SessionStateReason = "recording_started"
	SessionReasonRecordingRestarted             SessionStateReason = "recording_restarted"
	SessionReasonTranscribing                   SessionStateReason = "transcribing"
	SessionReasonTranscriptCopied               SessionStateReason = "transcript_copied"
	SessionReasonTranscriptReadyClipboardFailed SessionStateReason = "transcript_clipboard_failed"
	SessionReasonRecordingDiscarded             SessionStateReason = "recording_discarded"
	SessionReasonNoTranscript                   SessionStateReason = "no_transcript"
	SessionReasonTranscriptionFailed            SessionStateReason = "transcription_failed"
	SessionReasonRulesFailed                    SessionStateReason = "rules_failed"
)

// ErrorCode identifies non-fatal and fatal backend errors.
type ErrorCode string

const (
	ErrorCodeStartup       ErrorCode = "startup"
	ErrorCodeAudioStop     ErrorCode = "audio_stop"
	ErrorCodeAudioStream   ErrorCode = "audio_stream"
	ErrorCodeTranscription ErrorCode = "transcription"
	ErrorCodeRules         ErrorCode = "rules"
	ErrorCodeClipboard     ErrorCode = "clipboard"
)

// TranscriptKind identifies whether a stream event is partial or final text.
type TranscriptKind string

const (
	TranscriptKindPartial TranscriptKind = "partial"
	TranscriptKindFinal   TranscriptKind = "final"
)

// TranscriptEvent represents incremental transcription output from a provider.
type TranscriptEvent struct {
	Kind          TranscriptKind `json:"kind"`
	Text          string         `json:"text"`
	IsSpeechFinal bool           `json:"isSpeechFinal"`
}

// StopResult is returned once recording is stopped and transcription is processed.
type StopResult struct {
	RawTranscript   string `json:"rawTranscript"`
	FinalTranscript string `json:"finalTranscript"`
	Copied          bool   `json:"copied"`
}

// Status summarizes the current runtime status.
type Status struct {
	State   SessionState `json:"state"`
	Active  bool         `json:"active"`
	Message string       `json:"message,omitempty"`
}
