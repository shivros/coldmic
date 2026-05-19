# Proposal: Voice Assistant Mode

## Why

ColdMic currently provides push-to-talk transcription only. Users must physically trigger recording, wait for transcription, and manually paste results. This limits ColdMic to a transcription tool when it could be a full voice assistant interface.

Users (starting with the author) want a "Hey Alice" experience: speak a wake phrase, have a natural back-and-forth conversation with an AI assistant, and hear responses spoken aloud — all hands-free.

The transport must be fast out of the gate. Users won't adopt inconvenient tools. An OpenAI-compatible HTTP bridge ensures low latency and broad provider support (Hermes gateway, OpenAI, Ollama, LiteLLM, etc.) from day one.

## What Changes

ColdMic gains a new **conversation mode** — an always-on listening mode orchestrated by a state machine that cycles through listening → processing → speaking → listening until a stop signal.

This requires five new capabilities:

1. **ConversationBackend port + OpenAI-compatible bridge** — pluggable backend interface with an OpenAI-compatible HTTP provider as the first implementation
2. **VAD-gated continuous listening** — always-on mic capture gated by Voice Activity Detection, with wake phrase matching on transcript segments
3. **TTS playback** — text-to-speech synthesis and audio playback for assistant responses
4. **Conversation controller** — state machine orchestrating the full listen → process → speak loop
5. **Daemon integration** — wiring everything into the CLI, HTTP API, and bootstrap

## Capabilities

- `conversation-backend` — Pluggable AI backend interface with OpenAI-compatible bridge
- `vad-continuous-listening` — VAD-gated microphone capture with wake phrase detection
- `tts-playback` — Text-to-speech synthesis and audio output
- `conversation-controller` — State machine for the conversation loop
- `daemon-integration` — CLI, HTTP API, config, and bootstrap wiring

## Impact

- **Architecture**: New ports (`ConversationBackend`, `TextToSpeech`), new use-case layer (`ConversationController`, `ContinuousListener`), extended daemon API surface
- **Dependencies**: Silero VAD (ONNX model), edge-tts (subprocess), OpenAI-compatible HTTP endpoint
- **Config**: New environment variables for backend, listening, and TTS settings
- **Backward compatibility**: Fully additive — existing push-to-talk mode is untouched
- **Linear parent**: COD-276
