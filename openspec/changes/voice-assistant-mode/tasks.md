# Tasks: Voice Assistant Mode

## Phase 1: Ports & Providers (COD-277, COD-279)

- [ ] Define `ConversationBackend` interface in `internal/ports/conversation.go` (SendMessage, StreamMessage, CloseSession)
- [ ] Define `TextToSpeech` interface in `internal/ports/tts.go` (Synthesize, Play)
- [ ] Define domain types: `ConversationResponse`, `StreamChunk`, `ConversationState`, conversation-related errors in `internal/domain/types.go`
- [ ] Implement OpenAI-compatible bridge in `internal/providers/openai_conversation.go`
  - [ ] Non-streaming SendMessage with chat completions
  - [ ] Streaming StreamMessage with SSE parsing
  - [ ] In-memory session history with max-turn eviction
  - [ ] Retry logic for transient errors (exclude 4xx auth)
  - [ ] Markdown stripping for voice output
- [ ] Implement edge-tts provider in `internal/providers/edge_tts.go`
  - [ ] Synthesize via subprocess
  - [ ] Play via ffplay/paplay subprocess
  - [ ] Context cancellation kills subprocesses
- [ ] Unit tests for OpenAI bridge (mocked HTTP)
- [ ] Unit tests for edge-tts provider (mocked subprocess)

## Phase 2: VAD & Continuous Listening (COD-278)

- [ ] Implement Silero VAD wrapper in `internal/audio/vad.go`
  - [ ] Load ONNX model
  - [ ] Process 30ms frames, return speech probability
  - [ ] Speech onset/offset detection with configurable threshold and silence duration
- [ ] Implement `ContinuousListener` in `internal/usecase/continuous_listener.go`
  - [ ] Continuous mic capture loop
  - [ ] VAD-gated streaming to Deepgram
  - [ ] Wake phrase matching on final transcripts
  - [ ] Event channel output
- [ ] Add `SessionStateContinuous` to domain types
- [ ] Unit tests for VAD wrapper
- [ ] Unit tests for continuous listener with mocked AudioCapture and TranscriptionProvider

## Phase 3: Conversation Controller (COD-280)

- [ ] Implement `ConversationController` state machine in `internal/usecase/conversation_controller.go`
  - [ ] Four states: Idle, Listening, Processing, Speaking
  - [ ] State transitions driven by listener events
  - [ ] Stop phrase detection in transcripts
  - [ ] Silence timeout with configurable duration
  - [ ] Error recovery (backend errors → speak apology → resume listening)
  - [ ] State change events to EventSink
- [ ] Extend `EventSink` interface with `ConversationStateChanged`
- [ ] Unit tests for all state transitions
- [ ] Unit tests for stop phrase and timeout scenarios

## Phase 4: Daemon Integration (COD-281)

- [ ] Add HTTP API endpoints in `internal/daemon/httpapi.go`
  - [ ] `POST /v1/conversation/start`
  - [ ] `POST /v1/conversation/stop`
  - [ ] `GET /v1/conversation/status`
- [ ] Add CLI commands in `internal/cli/client.go`
  - [ ] `coldmic conversation start`
  - [ ] `coldmic conversation stop`
  - [ ] `coldmic conversation status`
- [ ] Extend `internal/bootstrap/wire.go` with conversation subsystem wiring
  - [ ] Backend provider selection from config
  - [ ] TTS provider selection from config
  - [ ] Construct ContinuousListener, ConversationController
  - [ ] Register HTTP handlers
- [ ] Add all configuration variables with defaults
- [ ] Update README.md with Conversation Mode section
- [ ] Integration test: start daemon → start conversation → verify status endpoint

## Phase 5: End-to-End Verification

- [ ] Manual smoke test: "Hey Alice" → response → TTS playback → continue listening
- [ ] Manual smoke test: stop phrase ends conversation
- [ ] Manual smoke test: silence timeout ends conversation
- [ ] Manual smoke test: backend error recovery
- [ ] Verify push-to-talk mode still works independently
