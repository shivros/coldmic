package domain

import "time"

// SessionState models the push-to-talk and continuous listening lifecycle.
type SessionState string

const (
	SessionStateIdle       SessionState = "idle"
	SessionStateRecording  SessionState = "recording"
	SessionStateStopping   SessionState = "stopping"
	SessionStateError      SessionState = "error"
	SessionStateContinuous SessionState = "continuous"
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
	SessionID       string `json:"sessionId,omitempty"`
}

// LatestTranscript captures the most recent successful stop output.
type LatestTranscript struct {
	Result     StopResult `json:"result"`
	CapturedAt time.Time  `json:"capturedAt"`
}

// ConversationState models the voice assistant conversation lifecycle.
type ConversationState string

const (
	ConvStateIdle       ConversationState = "idle"
	ConvStateListening  ConversationState = "listening"
	ConvStateProcessing ConversationState = "processing"
	ConvStateSpeaking   ConversationState = "speaking"
)

// ConversationStateReason provides a structured reason for conversation state transitions.
type ConversationStateReason string

const (
	ConvReasonWakeDetected    ConversationStateReason = "wake_detected"
	ConvReasonSpeechReceived  ConversationStateReason = "speech_received"
	ConvReasonBackendResponse ConversationStateReason = "backend_response"
	ConvReasonPlaybackDone    ConversationStateReason = "playback_done"
	ConvReasonStopPhrase      ConversationStateReason = "stop_phrase"
	ConvReasonSilenceTimeout  ConversationStateReason = "silence_timeout"
	ConvReasonManualStop      ConversationStateReason = "manual_stop"
	ConvReasonError           ConversationStateReason = "error"
)

// ConversationStatus summarizes the current conversation controller status.
type ConversationStatus struct {
	State     ConversationState `json:"state"`
	SessionID string            `json:"sessionId,omitempty"`
	Active    bool              `json:"active"`
}

// Status summarizes the current runtime status.
type Status struct {
	State   SessionState `json:"state"`
	Active  bool         `json:"active"`
	Message string       `json:"message,omitempty"`
	Mode    string       `json:"mode,omitempty"` // "ptt" (push-to-talk) or "continuous"
}
