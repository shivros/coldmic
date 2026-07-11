# Spec: Local STT Provider

## Purpose

Provides local, offline speech-to-text transcription using whisper.cpp. Used by the continuous listener for wake phrase detection without cloud API calls. Designed as a batch transcription interface — takes complete audio buffers, returns text.

## ADDED Requirements

### Requirement: LocalSTT interface

The `LocalSTT` interface in `internal/ports/local_stt.go` provides batch transcription:

```go
type LocalSTT interface {
    // Init prepares the provider (downloads model if needed).
    Init() error
    // Transcribe converts a PCM audio buffer to text.
    // audioPCM is 16-bit little-endian mono at the configured sample rate.
    Transcribe(ctx context.Context, audioPCM []byte) (string, error)
}
```

#### Scenario: Successful transcription

- Given a valid PCM audio buffer containing speech
- When `Transcribe` is called
- Then the whisper.cpp subprocess processes the audio
- And the transcript text is returned
- And the subprocess exits cleanly

#### Scenario: Empty or silent audio

- Given a PCM audio buffer with no detectable speech
- When `Transcribe` is called
- Then an empty string is returned (no error)

#### Scenario: Subprocess failure

- Given the whisper.cpp binary fails to start or crashes
- When `Transcribe` is called
- Then an error is returned wrapping the underlying failure
- And the caller can fall back to Deepgram

### Requirement: whisper.cpp subprocess provider

The `whispercpp` provider in `internal/providers/whispercpp/` runs whisper.cpp as a subprocess.

#### Scenario: First-run auto-download

- Given no whisper binary or model exists in `~/.cache/coldmic/`
- When `Init` is called
- Then the whisper.cpp binary is downloaded for the current OS/arch
- And the `ggml-tiny.en` model (~75MB) is downloaded
- And both are stored in `~/.cache/coldmic/`
- And subsequent calls skip download if files exist

#### Scenario: Audio piped via stdin

- Given the provider is initialized
- When `Transcribe` is called with PCM audio
- Then a 16-bit PCM WAV header is prepended to the audio data
- And the combined data is piped to `whisper-cpp --model <path> --language en --output-text -`
- And stdout is parsed to extract the transcript line

#### Scenario: Context cancellation

- Given a transcription is in progress
- When the context is cancelled
- Then the subprocess is killed immediately
- And a context.Canceled error is returned

### Requirement: Config integration

The local STT provider is configured via environment variables:

- `COLDMIC_LOCAL_STT`: Provider name (`whispercpp` or empty for disabled). Default: empty (disabled).
- `COLDMIC_LOCAL_STT_MODEL`: Model name to use. Default: `tiny.en`.

#### Scenario: Provider disabled (default)

- Given `COLDMIC_LOCAL_STT` is unset or empty
- Then no `LocalSTT` provider is created
- And the `ContinuousListener` uses single-phase Deepgram transcription

#### Scenario: Provider enabled

- Given `COLDMIC_LOCAL_STT=whispercpp`
- Then the whisper.cpp provider is created and initialized in bootstrap
- And it's passed to the `ContinuousListener` for wake phrase detection
