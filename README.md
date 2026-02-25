# ColdMic (Tracer Bullet)

ColdMic is a Wails + Go desktop app for intentional push-to-talk transcription.

This tracer-bullet implementation provides an end-to-end path:

- hold-to-talk recording
- Deepgram low-latency streaming transcription
- live partial transcript updates
- deterministic substitution rules
- final transcript copied to clipboard

## Current Scope

This is the initial functional slice, not the full product.

- provider: Deepgram websocket streaming
- recorder: `ffmpeg` microphone capture adapter (currently configured for Linux PulseAudio defaults)
- frontend: in-app hold button and `Space` key hold behavior

## Prerequisites

- Go 1.23+
- Node/npm
- `ffmpeg` available in PATH
- Deepgram API key

## Configuration

Environment variables:

- `DEEPGRAM_API_KEY` (required)
- `DEEPGRAM_API_BASE` (default: `https://api.deepgram.com/v1`)
- `DEEPGRAM_MODEL` (default: `nova-2`)
- `DEEPGRAM_LANGUAGE` (optional)
- `DEEPGRAM_SMART_FORMAT` (default: `true`)
- `COLDMIC_AUDIO_INPUT_FORMAT` (default: `pulse`)
- `COLDMIC_AUDIO_INPUT_DEVICE` (default: `default`)
- `COLDMIC_FFMPEG_COMMAND` (default: `ffmpeg`)
- `COLDMIC_RULES_FILE` (optional custom substitutions path)
- `COLDMIC_DAEMON_ADDR` (daemon bind address, default: `127.0.0.1:4317`)
- `COLDMIC_DAEMON_URL` (CLI daemon URL, default: `http://127.0.0.1:4317`)

Rules-file fallback order:

1. `COLDMIC_RULES_FILE`
2. `~/.config/coldmic/substitutions.rules`
3. `~/.config/hypr/whisper-substitutions.rules`

## Rules Format

Rules support two line types:

- literal replacement: `FROM => TO`
- regex replacement: `s/regex/replacement/flags`

Case-insensitive matching is enabled by default for regex rules unless explicitly set.

## Development

```bash
wails dev
```

## CLI + Daemon

Run the local daemon (headless, no UI):

```bash
go run ./cmd/coldmicd --addr 127.0.0.1:4317
```

Control ColdMic from CLI:

```bash
go run ./cmd/coldmic start
go run ./cmd/coldmic status
go run ./cmd/coldmic stop
go run ./cmd/coldmic transcript
```

JSON output is supported on each command:

```bash
go run ./cmd/coldmic status --json
```

Daemon HTTP API:

- `POST /v1/session/start`
- `POST /v1/session/stop`
- `GET /v1/session/status`
- `GET /v1/session/transcript/latest`

## Build

```bash
wails build
```
