package ports

import "context"

// LocalSTT provides batch speech-to-text transcription using a local engine.
// Unlike TranscriptionProvider (which is streaming), LocalSTT takes a complete
// audio buffer and returns the transcript text.
type LocalSTT interface {
	// Init prepares the provider (downloads model/binary if needed).
	Init() error
	// Transcribe converts a PCM audio buffer to text.
	// audioPCM is 16-bit little-endian mono at the configured sample rate.
	// Returns empty string for silent/unintelligible audio (no error).
	Transcribe(ctx context.Context, audioPCM []byte) (string, error)
}
