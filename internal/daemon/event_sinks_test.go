package daemon

import "testing"

func TestLoggingEventSinkMethods(t *testing.T) {
	var sink LoggingEventSink
	sink.SessionStateChanged("idle", "recording_started")
	sink.PartialTranscript("partial")
	sink.FinalTranscript("raw", "transformed", "session-1")
	sink.SessionError("transcription", "boom")
	sink.ConversationStateChanged("idle", "wake_detected")
}
