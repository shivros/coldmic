# ColdMic

ColdMic is a desktop application for push-to-talk speech transcription and voice assistant interaction. Built with Go and Wails, it captures audio from your microphone, transcribes it in real time via Deepgram streaming, and copies the final transcript to your clipboard.

## Features

- **Push-to-talk transcription** — hold to record, release to transcribe
- **Real-time streaming** — live partial transcripts via Deepgram nova-2
- **Substitution rules** — deterministic text replacements (literals and regex)
- **Headless daemon** — run without a GUI, control via CLI or HTTP API
- **Voice assistant mode** — VAD-gated continuous listening with wake phrase detection, OpenAI-compatible backend for conversation, and text-to-speech playback
- **Local wake-word detection** — optional whisper.cpp integration to avoid Deepgram costs until a wake phrase is detected
- **YAML configuration** — config file with environment variable overrides

## Architecture

ColdMic uses a clean ports-and-providers architecture:

```
internal/
├── ports/          # Interfaces (AudioCapture, TranscriptionProvider, LocalSTT, RulesEngine, Clipboard, etc.)
├── providers/      # Interface implementations (deepgram/, openai/, edge_tts/, whispercpp/)
├── usecase/        # Business logic (SessionService, Controller, ConversationController, ContinuousListener)
├── domain/         # Types, errors, state enums
├── audio/          # Audio capture adapters + Silero VAD wrapper
├── rules/          # Substitution rules engine
├── daemon/         # HTTP API
├── bootstrap/      # Dependency wiring
├── cli/            # CLI client
├── config/         # Configuration loading (YAML + env)
└── debuglog/       # Structured logging
```

New providers implement an interface and register in bootstrap — zero core changes required.

## Prerequisites

- **Go** 1.23+
- **Node.js** / npm (for the Wails frontend)
- **ffmpeg** in PATH (audio capture)
- **Deepgram API key** (for transcription)
- `lefthook` (git hooks; see [installation](https://lefthook.dev/installation/))
- `gitleaks` (secret scanning; see [installation](https://github.com/gitleaks/gitleaks#installing))

## Quick Start

### Build

```bash
git clone https://github.com/shivros/coldmic.git
cd coldmic
make build
```

This produces:

| Binary | Description |
|--------|-------------|
| `build/bin/coldmic-desktop` | Wails desktop app with GUI |
| `build/bin/coldmic` | CLI client |
| `build/bin/coldmicd` | Headless daemon |

### Install CLI tools

```bash
make install-cli
```

### Run

Start the daemon:

```bash
coldmicd --addr 127.0.0.1:4317
```

Transcribe:

```bash
coldmic start      # begin recording
coldmic status     # check recording state
coldmic stop       # stop, transcribe, copy to clipboard
coldmic transcript # show last transcript
```

Check version:

```bash
coldmic version    # or: coldmic --version / coldmic -v
```

### Development

```bash
make install-hooks   # first clone only — install lefthook + gitleaks
make verify-hooks    # verify hooks block test secrets
make dev             # desktop app in dev mode
make test            # Go tests
make ci              # full CI gate (quality + coverage + builds)
```

## Configuration

ColdMic loads configuration in this precedence order:

1. CLI flags (where a command exposes them, e.g. `--daemon-url`)
2. Environment variables
3. `~/.config/coldmic/config.yaml`
4. Built-in defaults

Generate a config template:

```bash
coldmic config init
```

Inspect the resolved config (secrets redacted):

```bash
coldmic config show
coldmic config show --json
```

### Key Environment Variables

**Transcription:**

| Variable | Default | Description |
|----------|---------|-------------|
| `DEEPGRAM_API_KEY` | *(required)* | Deepgram API key |
| `DEEPGRAM_MODEL` | `nova-2` | Deepgram model |
| `COLDMIC_AUDIO_INPUT_FORMAT` | `pulse` | ffmpeg audio format |
| `COLDMIC_AUDIO_INPUT_DEVICE` | `default` | Audio input device |
| `COLDMIC_RULES_FILE` | *(optional)* | Custom substitutions file path |
| `COLDMIC_DEBUG` | *(off)* | Verbose daemon telemetry when `true`/`1` |

**Daemon:**

| Variable | Default | Description |
|----------|---------|-------------|
| `COLDMIC_DAEMON_ADDR` | `127.0.0.1:4317` | Daemon bind address |
| `COLDMIC_DAEMON_URL` | `http://127.0.0.1:4317` | CLI daemon URL |
| `COLDMIC_TOGGLE_COMPAT` | *(off)* | No-arg toggle mode when `true` |

**Conversation mode:**

| Variable | Default | Description |
|----------|---------|-------------|
| `COLDMIC_BACKEND_BASE_URL` | `https://api.openai.com/v1` | LLM backend URL |
| `COLDMIC_BACKEND_API_KEY` | *(required for convo)* | Backend API key |
| `COLDMIC_BACKEND_MODEL` | `gpt-4o` | Backend model |
| `COLDMIC_BACKEND_SYSTEM_PROMPT` | `You are a helpful voice assistant.` | System prompt |
| `COLDMIC_BACKEND_STREAM` | `true` | Stream backend responses |
| `COLDMIC_WAKE_PHRASES` | `hey alice,alice` | Comma-separated wake phrases |
| `COLDMIC_STOP_PHRASES` | `thanks alice,that's all,goodbye,bye alice,stop` | Comma-separated stop phrases |
| `COLDMIC_VAD_THRESHOLD` | `0.5` | Voice activity detection threshold |
| `COLDMIC_VAD_SILENCE_MS` | `800` | Silence duration to end speech segment |
| `COLDMIC_CONVERSATION_TIMEOUT` | `30s` | Silence timeout to end conversation |

**Text-to-speech:**

| Variable | Default | Description |
|----------|---------|-------------|
| `COLDMIC_TTS_ENGINE` | `edge-tts` | TTS engine |
| `COLDMIC_TTS_VOICE` | `en-US-AriaNeural` | TTS voice |
| `COLDMIC_TTS_RATE` | `+0%` | TTS speech rate |
| `COLDMIC_TTS_VOLUME` | `+0%` | TTS volume |
| `COLDMIC_TTS_PLAYBACK_CMD` | `ffplay` | Audio playback command |

**Local wake-word STT:**

| Variable | Default | Description |
|----------|---------|-------------|
| `COLDMIC_LOCAL_STT` | *(disabled)* | Set to `whispercpp` to enable local wake detection |
| `COLDMIC_LOCAL_STT_MODEL` | `tiny.en` | whisper.cpp model name |

Rules file fallback order: `COLDMIC_RULES_FILE` → `~/.config/coldmic/substitutions.rules` → `~/.config/hypr/whisper-substitutions.rules`

### Rules Format

```
# literal replacement
wront => wrong

# regex replacement (case-insensitive by default)
s/teh/the/g
```

## Voice Assistant Mode

ColdMic can operate as a hands-free voice assistant:

```bash
coldmic conversation start    # begin listening for wake phrases
coldmic conversation status   # show conversation state
coldmic conversation stop     # end conversation
```

The pipeline:

```
Mic → Silero VAD → [optional: whisper.cpp local wake check]
  → wake phrase match?
    NO → discard, keep listening (zero Deepgram cost)
    YES → Deepgram streaming → LLM backend → TTS playback
```

The backend is any OpenAI-compatible API. Point it at OpenAI, OpenRouter, a local llama.cpp, or your own server.

## Daemon HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/session/start` | Begin recording |
| `POST` | `/v1/session/stop` | Stop recording, return transcript |
| `POST` | `/v1/session/abort` | Discard recording |
| `GET` | `/v1/session/status` | Current state |
| `GET` | `/v1/session/transcript/latest` | Last transcript |

JSON output is available on all CLI commands:

```bash
coldmic status --json
coldmic stop --json
```

Script-friendly exit codes:

```bash
coldmic status --check   # exit 0 = active, exit 1 = idle
```

## Toggle Mode

For quick push-to-talk bound to a hotkey:

```bash
export COLDMIC_TOGGLE_COMPAT=true
coldmic   # idle → start recording
coldmic   # active → stop, transcribe
```

## CI

GitHub Actions runs on every PR and push to `main`:

- Go: gofmt, vet, staticcheck, race-enabled tests, 74% coverage gate
- Frontend: ESLint, Vitest with coverage thresholds
- Build matrix: Ubuntu, macOS, Windows

## License

MIT
