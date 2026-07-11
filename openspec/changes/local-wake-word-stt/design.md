# Design: Local Wake-Word STT

## Architecture Overview

The local STT layer inserts whisper.cpp between the VAD and Deepgram in the continuous listening pipeline. It doesn't replace Deepgram — it acts as a cheap gatekeeper that filters out non-wake-phrase audio before any cloud API calls.

```
                          ┌──────────────────────────────────────────┐
                          │            ColdMic Daemon                │
                          │                                          │
  ┌──────────┐            │  ┌────────────────────────────────────┐  │
  │ Microphone├───────────┼─▶│ ContinuousListener                  │  │
  └──────────┘            │  │                                      │  │
                          │  │  ┌──────────┐                       │  │
                          │  │  │ SileroVAD │ speech segments      │  │
                          │  │  └─────┬────┘                       │  │
                          │  │        │                            │  │
                          │  │        ▼                            │  │
                          │  │  ┌──────────────────┐  wake match? │  │
                          │  │  │ whisper.cpp tiny  │──── NO ──────┤  │
                          │  │  │ (local, ~300ms)   │              │  │
                          │  │  └────────┬─────────┘              │  │
                          │  │           │ YES                     │  │
                          │  │           ▼                         │  │
                          │  │  ┌──────────────────┐              │  │
                          │  │  │ Deepgram STT      │              │  │
                          │  │  │ (cloud, high-qual)│              │  │
                          │  │  └────────┬─────────┘              │  │
                          │  └───────────┼────────────────────────┘  │
                          │              │ events                     │
                          │              ▼                            │
                          │     ConversationController               │
                          │     (unchanged)                           │
                          └──────────────────────────────────────────┘
```

## Key Design Decisions

### D1: New `LocalSTT` port — not reusing `TranscriptionProvider`

whisper.cpp is a **batch** transcription engine: it takes a complete audio buffer and returns text. It does not support streaming. The existing `TranscriptionProvider` interface is designed around streaming (`StartStreaming` → `StreamingSession` with `SendAudio`/`Events`).

Instead of forcing whisper.cpp into a streaming abstraction, we define a simpler `LocalSTT` interface:

```go
type LocalSTT interface {
    Transcribe(ctx context.Context, audioPCM []byte) (string, error)
    Init() error
}
```

This keeps the streaming contract clean for cloud providers and gives local engines a natural batch API.

**Future value**: `LocalSTT` can be reused for offline push-to-talk mode — the user could choose `COLDMIC_LOCAL_STT=whispercpp` for the main transcription path too, bypassing Deepgram entirely.

### D2: whisper.cpp as a subprocess

whisper.cpp is invoked as a statically compiled binary via `exec.Command`. This avoids CGo dependencies, shared library issues, and cross-compilation headaches.

The binary and model are auto-downloaded on first use to `~/.cache/coldmic/`:
- Binary: `~/.cache/coldmic/whisper-cpp` (or `whisper-cpp.exe` on Windows)
- Model: `~/.cache/coldmic/ggml-tiny.en.bin` (~75MB)

Audio is piped to stdin as 16-bit PCM WAV, transcript is read from stdout. This matches the ffmpeg subprocess pattern already used throughout ColdMic.

### D3: Buffered audio replay for Deepgram

When whisper.cpp detects a wake phrase, the `ContinuousListener` needs to send the *same* audio to Deepgram for high-quality transcription. The speech segment is already buffered in memory during VAD processing, so this is a simple replay — no re-capture needed.

The flow becomes:
1. VAD detects speech → buffer audio chunks
2. Speech ends → feed buffered audio to whisper.cpp
3. Wake phrase match → create Deepgram streaming session → replay buffered audio → continue streaming live audio
4. No match → discard buffer, stay listening

This adds ~300ms latency to wake detection (whisper.cpp processing time) but saves the cost and latency of a Deepgram session for every non-wake utterance.

### D4: Config-gated, zero-change default behavior

The local STT layer is entirely opt-in via `COLDMIC_LOCAL_STT`. When unset or empty, the `ContinuousListener` uses the original single-phase flow (VAD → Deepgram directly). This ensures:
- Existing deployments work unchanged
- The feature can be tested incrementally
- Fallback to Deepgram-only is trivial (unset the env var)

## Dependency Flow

```
bootstrap/wire.go
    ├─→ AudioCapture (existing)
    ├─→ TranscriptionProvider (existing, Deepgram)
    ├─→ VAD (existing, Silero)
    ├─→ LocalSTT (new, whisper.cpp)  ← conditional on COLDMIC_LOCAL_STT
    ├─→ ContinuousListener(AudioCapture, VAD, LocalSTT?, TranscriptionProvider)
    ├─→ ConversationBackend (existing)
    ├─→ TextToSpeech (existing)
    └─→ ConversationController (existing, unchanged)
```

## File Layout

```
internal/
├── ports/
│   └── local_stt.go              # LocalSTT interface
├── providers/
│   └── whispercpp/
│       ├── whisper.go             # whisper.cpp subprocess provider
│       ├── download.go            # Auto-download binary + model
│       └── whisper_test.go        # Unit tests
├── usecase/
│   └── continuous_listener.go     # Modified: two-phase transcription
├── config/
│   └── config.go                  # Extended: LocalSTT config
└── bootstrap/
    └── wire.go                    # Extended: wire LocalSTT conditionally
```

## Linear Reference

Parent: COD-298
