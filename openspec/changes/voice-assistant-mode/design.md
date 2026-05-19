# Design: Voice Assistant Mode

## Architecture Overview

The voice assistant mode adds a new subsystem to the existing ColdMic daemon. It does not modify the push-to-talk flow — it's an entirely separate mode that coexists.

```
                          ┌─────────────────────────┐
                          │    ColdMic Daemon        │
                          │                          │
  ┌──────────┐            │  ┌────────────────────┐  │
  │ Microphone├───────────┼─▶│ ContinuousListener  │  │
  └──────────┘            │  │  (VAD + Deepgram)   │  │
                          │  └────────┬───────────┘  │
                          │           │ events        │
                          │           ▼               │
                          │  ┌────────────────────┐  │
                          │  │ Conversation        │  │
                          │  │ Controller          │  │
                          │  │ (state machine)     │  │
                          │  └───┬──────────┬─────┘  │
                          │      │          │         │
                          │      ▼          ▼         │
                          │  ┌───────┐  ┌─────────┐  │
                          │  │Backend│  │   TTS   │  │
                          │  │Bridge │  │Playback │  │
                          │  └───┬───┘  └─────────┘  │
                          └──────┼────────────────────┘
                                 │
                          ┌──────▼──────┐
                          │  OpenAI API │
                          │  (or compat)│
                          └─────────────┘
```

## Key Design Decisions

### D1: Bridge pattern for backends (not adapter)

Each backend implements the `ConversationBackend` interface directly. There's no intermediate adapter or translation layer. This keeps the interface clean — if a backend can't support streaming, it returns a single-element channel.

Registration happens in bootstrap via a simple switch on the config value. No plugin system, no reflection. Adding a backend = write the struct + add a case to the switch.

### D2: VAD-gated, not wake-word-model-gated

Using Silero VAD + Deepgram streaming instead of a dedicated wake-word model (like Porcupine or openWakeUp):

- **Pro**: No additional model to train, package, or configure. Deepgram already handles speech → text. We just pattern-match on the result.
- **Pro**: Supports arbitrary wake phrases without retraining. User changes `COLDMIC_WAKE_PHRASES` and it works immediately.
- **Con**: Higher power usage than a dedicated wake-word model (streaming to Deepgram on every utterance). Acceptable for desktop; will need optimization for mobile.
- **Con**: Slight latency on first detection (must wait for Deepgram to return). Acceptable for v1.

### D3: edge-tts as first TTS engine

edge-tts is free, fast (~100-200ms synthesis), and produces natural-sounding speech. It requires no local model or GPU. The subprocess approach means adding new engines is trivial — just implement the interface.

For v2+ considerations: Piper (local, ONNX-based, sub-50ms) or streaming TTS APIs for lower latency.

### D4: In-memory conversation history

Session history is kept in memory, bounded by `COLDMIC_BACKEND_MAX_HISTORY`. No persistence across daemon restarts. This is acceptable because:
- Voice conversations are typically short (seconds to minutes)
- Daemon restarts are infrequent
- Adding persistence later is a non-breaking change

### D5: State machine is the sole coordinator

The `ConversationController` owns the state machine. It's the only component that decides when to listen, process, speak, or stop. The listener, backend, and TTS are passive — they do work when asked and return results.

This prevents race conditions: there's one goroutine driving the state machine, receiving events via channels.

## Dependency Flow

```
bootstrap/wire.go
    ├─→ AudioCapture (existing)
    ├─→ TranscriptionProvider (existing, Deepgram)
    ├─→ VAD (new, Silero)
    ├─→ ContinuousListener(AudioCapture, VAD, TranscriptionProvider)
    ├─→ ConversationBackend (new, OpenAI bridge)
    ├─→ TextToSpeech (new, edge-tts)
    ├─→ ConversationController(ContinuousListener, ConversationBackend, TextToSpeech, EventSink)
    └─→ HTTP handlers registered on daemon router
```

## File Layout

```
internal/
├── ports/
│   ├── conversation.go          # ConversationBackend interface
│   └── tts.go                   # TextToSpeech interface
├── providers/
│   ├── openai_conversation.go   # OpenAI-compatible bridge
│   └── edge_tts.go              # edge-tts provider
├── audio/
│   └── vad.go                   # Silero VAD wrapper
├── usecase/
│   ├── conversation_controller.go  # State machine
│   └── continuous_listener.go      # VAD + STT loop
├── domain/
│   └── types.go                 # Extended with conversation states
├── daemon/
│   └── httpapi.go               # Extended with /v1/conversation/* routes
├── bootstrap/
│   └── wire.go                  # Extended with conversation wiring
└── cli/
    └── client.go                # Extended with conversation commands
```

## Linear Reference

Parent: COD-276
Sub-tickets: COD-277 (backend), COD-278 (VAD), COD-279 (TTS), COD-280 (controller), COD-281 (integration)
