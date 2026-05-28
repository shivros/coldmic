# Proposal: Local Wake-Word STT

## Why

Currently, every speech segment detected by the VAD is streamed to Deepgram for transcription **before** wake phrase matching. This means ambient speech, TV audio, and background noise all incur Deepgram API costs — even though the vast majority of utterances won't contain a wake phrase.

For an always-on desktop assistant, this cost profile is unsustainable. The system should be able to detect wake phrases entirely locally, only reaching out to cloud STT when the user is actually engaging in conversation.

Additionally, a local STT capability has broader value beyond wake-word detection. Once integrated, it enables:
- Fully offline push-to-talk transcription
- Low-latency local transcription for scenarios where cloud STT is unavailable or too slow
- A fallback when Deepgram is unreachable

## What Changes

ColdMic gains a **local STT layer** using whisper.cpp that sits between the VAD and Deepgram in the continuous listening pipeline. When a VAD speech segment ends, audio goes to local whisper first. Only if a wake phrase is detected does the system engage Deepgram for high-quality transcription and conversation.

This requires three new capabilities:

1. **Local STT provider** — whisper.cpp subprocess-based transcription provider that processes buffered audio locally, with auto-download of model binary and weights
2. **Two-phase continuous listening** — modify `ContinuousListener` to support local-first transcription with optional cloud fallback for wake-confirmed speech
3. **Configuration & bootstrap wiring** — new env vars and bootstrap integration for the local STT provider

## Capabilities

- `local-stt` — Local STT provider (whisper.cpp subprocess) with auto-download, batch transcription, and provider interface
- `two-phase-listening` — ContinuousListener two-phase transcription: local STT for wake check → cloud STT for conversation
- `local-stt-config` — Configuration, bootstrap wiring, and integration tests

## Impact

- **Architecture**: New `LocalSTT` port (simpler than `TranscriptionProvider` — batch, not streaming), new whisper.cpp provider, modified `ContinuousListener` with two-phase flow
- **Dependencies**: whisper.cpp binary (statically compiled, auto-downloaded), `ggml-tiny.en` model (~75MB, auto-downloaded to `~/.cache/coldmic/`)
- **Config**: New env vars: `COLDMIC_LOCAL_STT`, `COLDMIC_LOCAL_STT_MODEL`
- **Backward compatibility**: Fully additive — if `COLDMIC_LOCAL_STT` is unset or empty, behavior is unchanged (Deepgram for everything)
- **Linear parent**: COD-298
