# Proposal: Loopback Test Harness

## Why

ColdMic's conversation mode (VAD → wake phrase → Deepgram → LLM → TTS) has never been tested end-to-end. The implementation is code-complete (Phases 1–4 merged), but Phase 5 — manual smoke testing — has been blocked for weeks because it requires a human physically speaking into a microphone. Shiv is the bottleneck; she always has higher priorities.

Meanwhile, real bugs are hiding in the untested pipeline. During initial investigation of the harness, two bugs were found:
1. VAD threshold hardcoded to `> 0.5` instead of using the configured `l.cfg.VADThreshold` — meaning the config setting was decorative
2. Default config `vad_threshold: 500` paired with Silero VAD engine — which expects a speech probability of 0.0–1.0, making speech detection impossible at defaults

A loopback test harness eliminates the human dependency. Synthetic TTS audio is routed through a virtual audio device back into ColdMic's capture pipeline, enabling fully automated testing of the conversation flow.

## What Changes

A new test harness script (`scripts/loopback-test.sh`) and supporting changes:

1. **Virtual audio device creation** — spin up a PipeWire null-sink + monitor source pair that ColdMic's ffmpeg can capture from
2. **Synthetic speech injection** — generate TTS audio containing the wake phrase and play it into the virtual sink via native PipeWire (pw-cat), bypassing the PulseAudio compat layer's routing quirks
3. **Pipeline verification** — start ColdMic daemon, start conversation mode, inject audio, and verify each stage produces expected output (VAD speech detected → Deepgram transcript → wake phrase match → conversation state transition)
4. **Bug fixes** — VAD threshold config wiring and default value correction (already patched locally)

## Capabilities

- `loopback-audio-routing` — Virtual PipeWire device creation and synthetic audio injection
- `pipeline-verification` — Assertions that each conversation-mode stage fires correctly

## Impact

- **New file**: `scripts/loopback-test.sh` — the harness entry point
- **Bug fix**: `internal/usecase/continuous_listener.go` — use `l.cfg.VADThreshold` instead of hardcoded `0.5`
- **Bug fix**: `internal/config/config.go` — default `vad_threshold: 0.5` (Silero scale) instead of `500` (EnergyVAD scale)
- **Dependencies**: PipeWire (pw-cat, pw-link, pw-cli), KittenTTS or fallback sine wave
- **Test environment**: Must run on a machine with PipeWire and audio subsystem (endver or desk)
- **Linear**: COD-389

## Critical Constraint

**The harness must be run and verified by the implementer.** Writing the script is not sufficient. The implementer must execute it, observe the daemon logs showing VAD speech detection and Deepgram transcription, and confirm the wake phrase match fires. A script that exists but doesn't work is a failure. Using it is the point.
