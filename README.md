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
- `lefthook` (for tracked git hooks; see https://lefthook.dev/installation/)
- `gitleaks` (for staged secret scanning; see https://github.com/gitleaks/gitleaks?tab=readme-ov-file#installing)

## Configuration

ColdMIC loads configuration in this precedence order:

1. CLI flags (where a command exposes them, such as `--daemon-url`)
2. Environment variables
3. `~/.config/coldmic/config.yaml`
4. Built-in defaults

Create a documented YAML template:

```bash
go run ./cmd/coldmic config init
```

Inspect the resolved config after YAML and environment overrides. Secret values are redacted
in command output:

```bash
go run ./cmd/coldmic config show
# or machine-readable output
go run ./cmd/coldmic config show --json
```

Existing `.envrc`-style setup still works: every previous environment variable
continues to override the YAML file. Set `COLDMIC_CONFIG_FILE` to load a config
from a non-default path.

Primary environment variables:

- `DEEPGRAM_API_KEY` (required)
- `DEEPGRAM_API_BASE` (default: `https://api.deepgram.com/v1`)
- `DEEPGRAM_MODEL` (default: `nova-2`)
- `DEEPGRAM_LANGUAGE` (optional)
- `DEEPGRAM_SMART_FORMAT` (default: `true`)
- `COLDMIC_AUDIO_INPUT_FORMAT` (default: `pulse`)
- `COLDMIC_AUDIO_INPUT_DEVICE` (default: `default`)
- `COLDMIC_FFMPEG_COMMAND` (default: `ffmpeg`)
- `COLDMIC_RULES_FILE` (optional custom substitutions path)
- `COLDMIC_DEBUG` (optional, enables verbose daemon telemetry when `true`/`1`)
- `COLDMIC_DAEMON_ADDR` (daemon bind address, default: `127.0.0.1:4317`)
- `COLDMIC_DAEMON_URL` (CLI daemon URL, default: `http://127.0.0.1:4317`)

Conversation mode:

- `COLDMIC_CONVERSATION_BACKEND` (default: `openai`)
- `COLDMIC_BACKEND_BASE_URL` (default: `https://api.openai.com/v1`)
- `COLDMIC_BACKEND_API_KEY` (required for conversation mode)
- `COLDMIC_BACKEND_MODEL` (default: `gpt-4o`)
- `COLDMIC_BACKEND_SYSTEM_PROMPT` (default: `You are a helpful voice assistant.`)
- `COLDMIC_BACKEND_STREAM` (default: `true`)
- `COLDMIC_BACKEND_TIMEOUT` (default: `30s`)
- `COLDMIC_BACKEND_MAX_HISTORY` (default: `20`)
- `COLDMIC_STOP_PHRASES` (comma-separated, default: `thanks alice,that's all,goodbye,bye alice,stop`)
- `COLDMIC_CONVERSATION_TIMEOUT` (silence timeout, default: `30s`)
- `COLDMIC_WAKE_PHRASES` (comma-separated, default: `hey alice,alice`)
- `COLDMIC_TTS_ENGINE` (default: `edge-tts`)
- `COLDMIC_TTS_VOICE` (default: `en-US-AriaNeural`)
- `COLDMIC_TTS_RATE` (default: `+0%`)
- `COLDMIC_TTS_VOLUME` (default: `+0%`)
- `COLDMIC_TTS_PLAYBACK_CMD` (default: `ffplay`)

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

Before your first commit in a fresh clone, install the tracked git hooks:

```bash
make install-hooks
```

Then verify the hook blocks a generated test key:

```bash
make verify-hooks
```

This enables staged secret scanning via `gitleaks protect --staged` on every commit.

```bash
make dev
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
go run ./cmd/coldmic abort
go run ./cmd/coldmic transcript
```

JSON output is supported on each command:

```bash
go run ./cmd/coldmic status --json
```

Script-friendly status check (no stdout payload):

```bash
go run ./cmd/coldmic status --check
# exit 0 when active, exit 1 when idle
```

Optional no-arg toggle compatibility mode:

```bash
export COLDMIC_TOGGLE_COMPAT=true
go run ./cmd/coldmic
```

When enabled, no-arg CLI invocation checks current status and toggles:

- idle -> `start`
- active -> `stop`

When `COLDMIC_TOGGLE_COMPAT` is unset or not `true`, no-arg invocation keeps strict behavior and prints usage with an error.

### Conversation Mode

Start a voice assistant conversation (requires `COLDMIC_BACKEND_API_KEY` and a running daemon):

```bash
go run ./cmd/coldmic conversation start
go run ./cmd/coldmic conversation status
go run ./cmd/coldmic conversation stop
```

JSON output:

```bash
go run ./cmd/coldmic conversation status --json
```

Conversation mode uses VAD-gated continuous listening with wake phrase detection, an OpenAI-compatible backend for responses, and edge-tts for speech playback. Say a wake phrase (default: "Hey Alice") to trigger a conversation cycle, and a stop phrase (default: "Thanks Alice", "goodbye", etc.) or silence timeout to end.

Daemon HTTP API:

- `POST /v1/session/start`
- `POST /v1/session/stop`
- `POST /v1/session/abort`
- `GET /v1/session/status`
- `GET /v1/session/transcript/latest`

## Build

Build everything reproducibly:

```bash
make build
```

This produces:

- `build/bin/coldmic-desktop` - the Wails desktop app
- `build/bin/coldmic` - the CLI client
- `build/bin/coldmicd` - the headless daemon

Install the CLI tools into `$(go env GOPATH)/bin`:

```bash
make install-cli
```

Important:

- Do not use `go run .` or `go install .` for the desktop app at the repo root. Wails desktop apps must be built with the Wails CLI so the correct build tags are applied.
- If you only want the command-line tools, use `go install ./cmd/coldmic ./cmd/coldmicd` or `make install-cli`.

Build only the desktop app:

```bash
wails build
```

or:

```bash
make build-app
```

Build only the CLI tools:

```bash
make build-cli
```

Run the desktop app in development:

```bash
make dev
```

Run tests:

```bash
make test
```

Run the same quality gates used by CI:

```bash
make lint-go
make lint-frontend
make test-go-race
make test-go-coverage
make test-frontend
make test-frontend-coverage
make quality-go
make quality-frontend
make ci
```

Run CI-oriented interface targets directly:

```bash
make ci-quality-frontend
make ci-test-frontend
make ci-test-go
make ci-build-cli
make ci-build-desktop
make ci-build-desktop-linux
make verify-ci-contract
```

## CI

GitHub Actions workflow: `.github/workflows/ci.yml`

What runs:

- on every pull request
- on every push to `main`
- Go quality checks (`gofmt`, `go vet`, `staticcheck`)
- frontend lint/build checks
- Go tests (race enabled) + Go coverage gate (default minimum: 74%)
- frontend tests (Vitest) with coverage thresholds (lines/statements 85%, functions 80%, branches 60%)
- build matrix for `ubuntu-latest`, `macos-latest`, and `windows-latest`
- workflow linting (`actionlint`)
- CI contract verification (`scripts/ci/verify_ci_contract.sh`)

CI contract (workflow -> Make targets):

- frontend quality job -> `make ci-quality-frontend`
- frontend test job -> `make ci-test-frontend`
- matrix CLI build -> `make ci-build-cli`
- matrix desktop build (Linux) -> `make ci-build-desktop-linux`
- matrix desktop build (macOS/Windows) -> `make ci-build-desktop`

The old root-package path will fail with:

```text
Error: Wails applications will not build without the correct build tags.
```

That message means the desktop app entrypoint was invoked with plain Go tooling instead of `wails build` or `wails dev`.

Legacy direct Wails command:

```bash
wails build
```
