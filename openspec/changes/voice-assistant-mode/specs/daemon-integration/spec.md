# Spec: Daemon Integration

## Purpose

Wire all voice assistant components into the daemon's CLI, HTTP API, configuration, and bootstrap layer so the full conversation mode is accessible to users.

## ADDED Requirements

### Requirement: CLI Commands

New subcommands on the `coldmic` CLI:

```
coldmic conversation start    # start Hey Alice mode
coldmic conversation stop     # stop Hey Alice mode
coldmic conversation status   # print current conversation state
```

- `start` begins continuous listening + conversation controller
- `stop` ends the conversation loop and continuous listening
- `status` prints current state (JSON with `--json`, human-readable otherwise)
- All commands communicate with the daemon via HTTP API

### Requirement: HTTP API Endpoints

New daemon endpoints under `/v1/conversation/`:

- `POST /v1/conversation/start` — start conversation mode
- `POST /v1/conversation/stop` — stop conversation mode
- `GET /v1/conversation/status` — current state

Response format:
```json
{
  "state": "listening",
  "active": true,
  "sessionID": "conv-abc123",
  "duration": "00:02:15"
}
```

### Requirement: Bootstrap Wiring

`internal/bootstrap/wire.go` constructs the conversation subsystem:

1. Read `COLDMIC_CONVERSATION_BACKEND` → instantiate matching `ConversationBackend`
2. Read `COLDMIC_TTS_ENGINE` → instantiate matching `TextToSpeech`
3. Instantiate `ContinuousListener` with `AudioCapture` + `VAD` + `TranscriptionProvider`
4. Instantiate `ConversationController` with listener + backend + TTS
5. Register conversation HTTP handlers on the daemon router
6. Register CLI commands

All dependencies are wired via constructor injection. No package-level globals.

### Requirement: Configuration Schema

All conversation-mode config variables:

```
# Backend provider
COLDMIC_CONVERSATION_BACKEND    = openai
COLDMIC_BACKEND_BASE_URL        = https://api.openai.com/v1
COLDMIC_BACKEND_API_KEY         = (required when backend=openai)
COLDMIC_BACKEND_MODEL           = gpt-4o
COLDMIC_BACKEND_SYSTEM_PROMPT   = "You are a helpful voice assistant."
COLDMIC_BACKEND_STREAM          = true
COLDMIC_BACKEND_TIMEOUT         = 30s
COLDMIC_BACKEND_MAX_HISTORY     = 20

# Listening
COLDMIC_WAKE_PHRASES            = hey alice,alice
COLDMIC_VAD_THRESHOLD           = 0.5
COLDMIC_VAD_SILENCE_MS          = 800
COLDMIC_CONVERSATION_TIMEOUT    = 30s
COLDMIC_STOP_PHRASES            = thanks alice,that's all,goodbye,bye alice,stop

# TTS
COLDMIC_TTS_ENGINE              = edge-tts
COLDMIC_TTS_VOICE               = en-US-AriaNeural
COLDMIC_TTS_RATE                = +0%
COLDMIC_TTS_VOLUME              = +0%
COLDMIC_TTS_PLAYBACK_CMD        = ffplay
```

All variables have sensible defaults. Only `COLDMIC_BACKEND_API_KEY` is required to start a conversation.

### Requirement: README Documentation

Update `README.md` with a "Conversation Mode" section covering:
- Prerequisites (edge-tts, Silero VAD model)
- Quick start commands
- Configuration reference
- Architecture overview (link to spec)

#### Scenario: Start conversation from CLI

Given the daemon is running
When `coldmic conversation start` is called
Then the daemon begins continuous listening
And `coldmic conversation status` returns `{"state": "idle", "active": true}` (waiting for wake phrase)

#### Scenario: End-to-end voice assistant session

Given conversation mode is active
When the user says "Hey Alice, what's the capital of France?"
Then the wake phrase is detected
Then the transcript is sent to the backend
Then the response is spoken via TTS
Then the controller returns to LISTENING state

#### Scenario: Missing backend API key

Given `COLDMIC_BACKEND_API_KEY` is not set
When `coldmic conversation start` is called
Then the daemon returns an error: "backend API key required"
And conversation mode does not start
