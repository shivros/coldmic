# Tasks: Voice Assistant Mode

## Phase 1: Ports & Providers (COD-277, COD-279)

- [x] Define `ConversationBackend` interface in `internal/ports/conversation.go` (SendMessage, StreamMessage, CloseSession)
- [x] Define `TextToSpeech` interface in `internal/ports/tts.go` (Synthesize, Play)
- [x] Define domain types: `ConversationResponse`, `StreamChunk`, `ConversationState`, conversation-related errors in `internal/domain/types.go`
- [x] Implement OpenAI-compatible bridge in `internal/providers/openai/conversation.go`
  - [x] Non-streaming SendMessage with chat completions
  - [x] Streaming StreamMessage with SSE parsing
  - [x] In-memory session history with max-turn eviction
  - [x] Retry logic for transient errors (exclude 4xx auth)
  - [x] Markdown stripping for voice output
- [x] Implement edge-tts provider in `internal/providers/edge_tts.go`
  - [x] Synthesize via subprocess
  - [x] Play via ffplay/paplay subprocess
  - [x] Context cancellation kills subprocesses
- [x] Unit tests for OpenAI bridge (mocked HTTP)
- [x] Unit tests for edge-tts provider (mocked subprocess)

## Phase 2: VAD & Continuous Listening (COD-278)

- [x] Implement Silero VAD wrapper in `internal/audio/vad.go`
  - [x] Load ONNX model
  - [x] Process 30ms frames, return speech probability
  - [x] Speech onset/offset detection with configurable threshold and silence duration
- [x] Implement `ContinuousListener` in `internal/usecase/continuous_listener.go`
  - [x] Continuous mic capture loop
  - [x] VAD-gated streaming to Deepgram
  - [x] Wake phrase matching on final transcripts
  - [x] Event channel output
- [x] Add `SessionStateContinuous` to domain types
- [x] Unit tests for VAD wrapper
- [x] Unit tests for continuous listener with mocked AudioCapture and TranscriptionProvider

## Phase 3: Conversation Controller (COD-280)

- [x] Implement `ConversationController` state machine in `internal/usecase/conversation_controller.go`
  - [x] Four states: Idle, Listening, Processing, Speaking
  - [x] State transitions driven by listener events
  - [x] Stop phrase detection in transcripts
  - [x] Silence timeout with configurable duration
  - [x] Error recovery (backend errors → speak apology → resume listening)
  - [x] State change events to EventSink
- [x] Extend `EventSink` interface with `ConversationStateChanged`
- [x] Unit tests for all state transitions
- [x] Unit tests for stop phrase and timeout scenarios

## Phase 4: Daemon Integration (COD-281)

- [x] Add HTTP API endpoints in `internal/daemon/httpapi.go`
  - [x] `POST /v1/conversation/start`
  - [x] `POST /v1/conversation/stop`
  - [x] `GET /v1/conversation/status`
- [x] Add CLI commands in `internal/cli/client.go`
  - [x] `coldmic conversation start`
  - [x] `coldmic conversation stop`
  - [x] `coldmic conversation status`
- [x] Extend `internal/bootstrap/wire.go` with conversation subsystem wiring
  - [x] Backend provider selection from config
  - [x] TTS provider selection from config
  - [x] Construct ContinuousListener, ConversationController
  - [x] Register HTTP handlers
- [x] Add all configuration variables with defaults
- [x] Update README.md with Conversation Mode section
- [ ] Integration test: start daemon → start conversation → verify status endpoint

## Phase 5: End-to-End Verification

- [ ] Manual smoke test: "Hey Alice" → response → TTS playback → continue listening
- [ ] Manual smoke test: stop phrase ends conversation
- [ ] Manual smoke test: silence timeout ends conversation
- [ ] Manual smoke test: backend error recovery
- [ ] Verify push-to-talk mode still works independently
