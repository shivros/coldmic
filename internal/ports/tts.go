package ports

import "context"

// TextToSpeech synthesizes audio from text and plays it through the system output.
type TextToSpeech interface {
	// Synthesize generates raw audio bytes from text.
	// The format depends on the provider (e.g. mp3 for edge-tts).
	Synthesize(ctx context.Context, text string) ([]byte, error)

	// Play synthesizes and plays audio through the system audio output.
	// Blocks until playback completes or the context is cancelled.
	Play(ctx context.Context, text string) error
}
