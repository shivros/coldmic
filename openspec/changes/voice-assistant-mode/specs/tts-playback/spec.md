# Spec: TTS Playback

## Purpose

Add text-to-speech synthesis and audio playback so assistant responses are spoken aloud through the system audio output.

## ADDED Requirements

### Requirement: TextToSpeech Port

`internal/ports/tts.go` defines the TTS interface:

```go
type TextToSpeech interface {
    Synthesize(ctx context.Context, text string) ([]byte, error)
    Play(ctx context.Context, text string) error
}
```

- `Synthesize` returns raw audio bytes (format depends on provider)
- `Play` synthesizes and plays through system audio output in one call
- Implementations must handle long text by chunking if necessary

### Requirement: Edge-TTS Provider

`internal/providers/edge_tts.go` implements `TextToSpeech` using the `edge-tts` CLI tool.

Configuration:
- `COLDMIC_TTS_ENGINE` — provider name, default `edge-tts`
- `COLDMIC_TTS_VOICE` — voice name, default `en-US-AriaNeural`
- `COLDMIC_TTS_RATE` — speech rate adjustment, default `+0%`
- `COLDMIC_TTS_VOLUME` — volume adjustment, default `+0%`

The provider:
- Spawns `edge-tts` as a subprocess
- Pipes text via stdin or command argument
- Receives mp3 audio on stdout
- For `Play`, pipes mp3 to `ffplay -nodisp -autoexit -loglevel quiet` or `paplay` (configurable)

### Requirement: Playback Command

`COLDMIC_TTS_PLAYBACK_CMD` selects the audio player:
- `ffplay` (default) — universal, uses ffmpeg
- `paplay` — PulseAudio native
- `aplay` — ALSA native

The playback command must:
- Block until playback completes (so the conversation controller knows when speaking is done)
- Be killed if the context is cancelled (interrupt current speech)

### Requirement: TTS Provider Registration

Bootstrap selects TTS provider based on `COLDMIC_TTS_ENGINE`. Unknown engines fail fast at startup.

#### Scenario: Synthesize and play a response

Given `COLDMIC_TTS_ENGINE=edge-tts` and `edge-tts` is in PATH
When `Play(ctx, "The weather in Denver is sunny and 72 degrees")` is called
Then `edge-tts` is spawned with the text and configured voice
Then the resulting mp3 is piped to `ffplay`
Then the method returns after playback completes

#### Scenario: Context cancellation during playback

Given TTS is playing a long response
When the context is cancelled
Then the `edge-tts` subprocess is killed
Then the `ffplay` subprocess is killed
Then the method returns context.Canceled

#### Scenario: edge-tts not found

Given `edge-tts` is not in PATH
When `Play` or `Synthesize` is called
Then an error is returned indicating the tool is missing
