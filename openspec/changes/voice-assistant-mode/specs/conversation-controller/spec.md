# Spec: Conversation Controller

## Purpose

Orchestrate the full voice assistant conversation loop with a state machine that coordinates the continuous listener, conversation backend, and TTS playback.

## ADDED Requirements

### Requirement: Conversation State Machine

`internal/usecase/conversation_controller.go` implements a four-state machine:

```
IDLE ──(wake phrase)──→ LISTENING
LISTENING ──(speech end)──→ PROCESSING
PROCESSING ──(backend response)──→ SPEAKING
SPEAKING ──(playback done)──→ LISTENING
LISTENING ──(stop signal / timeout)──→ IDLE
```

States:
- `ConvStateIdle` — no active conversation, waiting for wake phrase from listener
- `ConvStateListening` — actively listening for user speech
- `ConvStateProcessing` — waiting for backend response
- `ConvStateSpeaking` — TTS is playing the response

### Requirement: Conversation Loop

The controller subscribes to the `ContinuousListener` event channel and drives the loop:

1. **IDLE → LISTENING**: When a `ListenerEvent` with a matched wake phrase arrives
2. **LISTENING → PROCESSING**: When a `ListenerEvent` with user speech arrives (or the wake phrase text itself if it contains a command)
3. **PROCESSING → SPEAKING**: When the `ConversationBackend` returns a response, pass it to TTS
4. **SPEAKING → LISTENING**: When TTS playback completes, resume listening
5. **LISTENING → IDLE**: On stop signal or silence timeout

### Requirement: Stop Signal Detection

The controller detects stop signals in two ways:

**Explicit stop phrases** — matched case-insensitively in user speech:
- `COLDMIC_STOP_PHRASES` — comma-separated list, default `thanks alice,that's all,goodbye,bye alice,stop`
- If a stop phrase is detected in the transcript, the conversation ends immediately

**Silence timeout** — if no speech is detected for the configured duration:
- `COLDMIC_CONVERSATION_TIMEOUT` — default `30s`
- Timer resets on each speech event
- On timeout, conversation returns to IDLE

### Requirement: State Change Events

The controller emits state changes to the `EventSink` (existing port):

```go
// New event methods on EventSink:
ConversationStateChanged(state ConversationState, reason ConversationStateReason)
```

Reasons include: `wake_detected`, `speech_received`, `backend_response`, `playback_done`, `stop_phrase`, `silence_timeout`, `manual_stop`, `error`.

State changes are also available via the HTTP API for UI integration.

### Requirement: Error Handling

- Backend errors: log and speak an error message ("Sorry, I had trouble with that. Could you try again?"), return to LISTENING
- TTS errors: log and skip playback, return to LISTENING
- Fatal errors (mic lost, daemon shutting down): return to IDLE with error event

#### Scenario: Full conversation loop

Given conversation mode is active and state is IDLE
When user says "Hey Alice, what time is it?"
Then state transitions: IDLE → LISTENING → PROCESSING
When backend responds "It's 3:42 PM."
Then state transitions: PROCESSING → SPEAKING
When TTS playback completes
Then state transitions: SPEAKING → LISTENING (awaits next utterance)

#### Scenario: Stop phrase ends conversation

Given state is LISTENING (in a conversation)
When user says "Thanks Alice, that's all"
Then the stop phrase "thanks alice" is detected
Then state transitions: LISTENING → IDLE

#### Scenario: Silence timeout ends conversation

Given state is LISTENING and `COLDMIC_CONVERSATION_TIMEOUT=30s`
When 30 seconds pass with no speech detected
Then state transitions: LISTENING → IDLE

#### Scenario: Backend error recovery

Given state is PROCESSING
When the backend returns an error
Then TTS speaks "Sorry, I had trouble with that. Could you try again?"
Then state transitions: SPEAKING → LISTENING (conversation continues)
