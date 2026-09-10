# ColdMic Context

ColdMic is a Go 1.23+ desktop and daemon application for push-to-talk transcription and voice-assistant interaction. It uses Wails for the desktop UI, Deepgram for streaming transcription, and a clean ports-and-providers architecture.

## Architecture

- `internal/ports/`: interfaces for audio capture, transcription, local STT, rules, and clipboard.
- `internal/providers/`: provider implementations such as Deepgram, OpenAI-compatible backends, Edge TTS, and whisper.cpp.
- `internal/usecase/`: application behavior including sessions, controllers, and continuous listening.
- `internal/domain/`: domain types, errors, and state enums.
- `internal/bootstrap/`: dependency wiring.
- `internal/daemon/` and `internal/cli/`: HTTP daemon API and CLI client.
- `internal/config/`: YAML and environment-backed configuration.
- `frontend/`: Wails frontend. Its built output is embedded by Go, so `frontend/dist` must exist for Go commands that load the desktop package.

New providers should implement a port and be registered in bootstrap; avoid coupling providers into core use cases.

## Configuration and secrets

Configuration precedence is CLI flags, environment variables, `~/.config/coldmic/config.yaml`, then defaults. Never commit API keys or generated runtime artifacts. The main sensitive variables are `DEEPGRAM_API_KEY` and `COLDMIC_BACKEND_API_KEY`.
