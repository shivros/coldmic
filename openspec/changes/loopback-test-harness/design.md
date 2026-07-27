# Design: Loopback Test Harness

## Problem Analysis

### Why PulseAudio compat layer doesn't work

PipeWire's PulseAudio compatibility layer (`module-null-sink` loaded via `pactl`) creates a virtual sink and monitor source, but audio played via `pacat -d <sink_name>` doesn't reliably appear on `<sink_name>.monitor` when a *second* process also captures from that monitor. The routing is inconsistent — sometimes the audio arrives, sometimes it doesn't.

### Why native PipeWire works

Native PipeWire clients (`pw-cat`) use the graph connection system directly. When `pw-cat -p --target coldmic_loopback` plays audio, it creates a definitive port-to-port link in the PipeWire graph. Similarly, `pw-cat -r --target coldmic_loopback.monitor` (or ColdMic's ffmpeg via `pipewire-pulse`) creates a definitive capture link. The graph links are visible via `pw-link -l` and can be manually wired if auto-linking fails.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  loopback-test.sh                    │
│                                                     │
│  ┌──────────┐    pw-cat -p     ┌──────────────┐     │
│  │ KittenTTS│ ──────────────> │ coldmic_loop- │     │
│  │ .wav gen │    --target     │  back (sink)  │     │
│  └──────────┘                 └──────┬───────┘     │
│                                      │ monitor     │
│                                      ▼             │
│  ┌──────────────┐    ffmpeg    ┌──────────────┐     │
│  │  coldmicd    │ <────────── │  .monitor     │     │
│  │  (daemon)    │  -f pulse   │  (source)     │     │
│  │              │  -i monitor └──────────────┘     │
│  │  VAD ────────│                                     │
│  │  Deepgram ───│                                     │
│  │  Wake match──│                                     │
│  │  Conv state──│                                     │
│  └──────────────┘                                     │
│         │                                            │
│         ▼                                            │
│  ┌──────────────┐                                    │
│  │ Verification │  Reads daemon log + HTTP API      │
│  │ (assertions) │  Confirms each stage fired         │
│  └──────────────┘                                    │
└─────────────────────────────────────────────────────┘
```

## Key Design Decisions

### 1. Audio injection via pw-cat, not pacat

`pacat` (PulseAudio compat) doesn't reliably route to null-sink monitors when multiple consumers exist. `pw-cat -p --target <node>` creates a direct PipeWire graph link that always works.

**Implication**: The harness must check if `pw-cat` is available and fail with a clear message if not.

### 2. ColdMic capture stays on PulseAudio format

ColdMic's ffmpeg already uses `-f pulse -i <device>.monitor`. This works fine on the *capture* side — PipeWire's pulse compat layer properly delivers monitor audio to ffmpeg. The routing issue is only on the *playback* side (getting audio INTO the sink).

### 3. Virtual device lifecycle

The harness creates the null-sink at startup and can optionally clean it up at exit. It should NOT assume the sink already exists — it creates it fresh. But it should also handle the case where a previous run left it behind.

### 4. Verification via daemon logs + HTTP API

The harness can't directly observe VAD internals. Instead it:
- Tails the daemon's debug log (`COLDMIC_DEBUG=1`) for expected log lines
- Polls the HTTP API (`/v1/conversation/status`, `/v1/session/transcript/latest`)
- Asserts that specific pipeline stages produced output within a timeout

### 5. TTS audio must contain the wake phrase

The synthetic speech must say "Alice" (or "Hey Alice") to trigger the wake phrase matcher. Without this, VAD will detect speech but the conversation won't start.

### 6. VAD threshold must be set correctly for the test

The default config ships `vad_threshold: 500` (fixed to `0.5` in the bug fix). For the loopback test, where TTS audio may have different characteristics than a physical microphone, `COLDMIC_VAD_THRESHOLD=0.3` gives more sensitivity.

## Edge Cases

- **PipeWire not running**: Harness checks with `pw-cli info 0` and exits with a clear error
- **pw-cat not installed**: Harness checks `command -v pw-cat` and falls back to `pacat` with a warning that routing may not work
- **Port 4317 in use**: Harness checks if a daemon is already running and reuses it, or kills and restarts
- **Deepgram API key missing**: Harness checks `DEEPGRAM_API_KEY` and exits with a clear error — this is a real pipeline test, not a mock
- **Conversation timeout**: Harness sets `COLDMIC_CONVERSATION_TIMEOUT=120s` to give the pipeline time to process
