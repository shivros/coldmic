# Spec: VAD-Gated Continuous Listening

## Purpose

Extend ColdMic with an always-on listening mode that uses Voice Activity Detection to gate audio capture, streams speech to Deepgram for transcription, and detects wake phrases to trigger the conversation loop.

## ADDED Requirements

### Requirement: Voice Activity Detection Module

`internal/audio/vad.go` wraps Silero VAD for speech onset/offset detection.

- Loads the Silero VAD ONNX model at initialization
- Processes 16kHz mono PCM audio in 30ms frames
- Exposes a streaming API: feed audio chunks, get VAD events (speech start, speech end)
- Configurable sensitivity: `COLDMIC_VAD_THRESHOLD` — speech probability threshold, default `0.5`
- Configurable silence duration: `COLDMIC_VAD_SILENCE_MS` — ms of silence before declaring speech end, default `800`

```go
type VADEvent int
const (
    VADSpeechStart VADEvent = iota
    VADSpeechEnd
)

type VAD interface {
    Process(frame []byte) (probability float64, err error)
    Reset()
}
```

### Requirement: Continuous Listener Use Case

`internal/usecase/continuous_listener.go` orchestrates the continuous listening loop:

1. Capture mic audio continuously (reuse `AudioCapture` port)
2. Feed audio frames through VAD
3. On `VADSpeechStart` — begin streaming to `TranscriptionProvider` (Deepgram)
4. On `VADSpeechEnd` — close the Deepgram stream, collect final transcript
5. Check final transcript for wake phrase match
6. If wake phrase matched → strip wake phrase, emit transcript to output channel
7. If no wake phrase → discard transcript, continue listening

The listener emits events via a channel:
```go
type ListenerEvent struct {
    Kind      ListenerEventKind
    Text      string
    SessionID string
    Timestamp time.Time
}
```

### Requirement: Wake Phrase Detection

Wake phrases are matched case-insensitively against the **start** of each transcript segment.

Configuration:
- `COLDMIC_WAKE_PHRASES` — comma-separated list, default `hey alice,alice`
- Phrases are stripped from the transcript before forwarding
- Partial/fuzzy matching is NOT required for v1 (exact prefix match after case normalization)

#### Scenario: Wake phrase at start of utterance

Given `COLDMIC_WAKE_PHRASES=hey alice,alice`
When the final transcript is "Hey Alice, what's the weather in Denver?"
Then "what's the weather in Denver?" is forwarded to the output channel
And the wake phrase "hey alice" is stripped

#### Scenario: No wake phrase

Given `COLDMIC_WAKE_PHRASES=hey alice,alice`
When the final transcript is "I need to grab coffee"
Then no event is emitted to the output channel
And the listener continues listening

### Requirement: Continuous Mode HTTP API

New daemon endpoints:
- `POST /v1/conversation/listen/start` — begin continuous listening
- `POST /v1/conversation/listen/stop` — stop continuous listening
- Extends `GET /v1/session/status` to return `continuous` state

### Requirement: Continuous Mode Session State

Extend `internal/domain/types.go`:
- Add `SessionStateContinuous SessionState = "continuous"`
- The continuous listener runs alongside (not instead of) the existing push-to-talk mode
- Only one continuous session can be active at a time

#### Scenario: Start continuous listening while idle

Given the daemon is idle
When `POST /v1/conversation/listen/start` is called
Then mic capture begins with VAD gating
And status returns `{"state": "continuous", "active": true}`

#### Scenario: Stop continuous listening

Given continuous listening is active
When `POST /v1/conversation/listen/stop` is called
Then mic capture stops
And status returns `{"state": "idle", "active": false}`

#### Scenario: VAD detects speech and transcribes

Given continuous listening is active
When the user says "Hey Alice, set a timer for 5 minutes"
Then VAD fires `VADSpeechStart`, audio streams to Deepgram
Then VAD fires `VADSpeechEnd` after 800ms silence
Then "set a timer for 5 minutes" is emitted on the listener event channel
