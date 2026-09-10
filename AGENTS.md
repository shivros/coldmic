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

---

# Development

## Prerequisites

Use Go 1.23+, Node.js/npm, ffmpeg, `lefthook`, and `gitleaks`. Wails desktop builds also need their platform-native dependencies.

## Commands

- `make test`: Go test suite.
- `make quality-go`: Go formatting, vet, and staticcheck.
- `make ci-quality-frontend`: install frontend dependencies and run lint/build.
- `make ci-test-go`: race-enabled Go tests and the coverage gate.
- `make ci-test-frontend`: frontend coverage tests.
- `make ci`: complete local CI gate.
- `make ci-build-cli`: build CLI and daemon binaries.
- `make verify-ci-contract`: verify Makefile/CI contract alignment.

When Go commands fail because `frontend/dist` is missing, build the frontend (`cd frontend && npm ci && npm run build`) or use the same minimal dist stub CI uses when the frontend itself is outside the change.

Run the narrowest relevant tests while iterating, then run `make ci` before requesting review. Do not hand-edit generated frontend build output.

---

# Workflow

1. Read the relevant port, provider, use case, and existing tests before changing behavior.
2. Keep domain/use-case logic independent of external providers; add or adjust ports before wiring providers in `internal/bootstrap/`.
3. Keep configuration compatible with the documented precedence order and redact secrets from diagnostics.
4. Add focused tests for behavior changes. Preserve the race and coverage expectations in the CI contract.
5. Run the applicable Make targets, then `make ci` for a complete change.
6. Before committing, inspect `git diff --check`. Lefthook runs staged gitleaks protection; never bypass it.

GitHub Actions is the source of truth for the full OS build matrix. Local Linux desktop builds can require GTK/WebKit dependencies that are absent on a development machine.
