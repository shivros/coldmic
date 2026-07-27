# Tasks: Loopback Test Harness (COD-389)

## Prerequisites

- [x] Create OpenSpec change (this file)
- [x] Create Linear ticket COD-389
- [x] Initial investigation: proved audio flows through pacat/parec loopback
- [x] Identified two bugs: hardcoded VAD threshold, wrong default config value

## Phase 1: Bug Fixes

- [ ] Commit fix: `continuous_listener.go` — use `l.cfg.VADThreshold` instead of hardcoded `0.5` (line 404)
- [ ] Commit fix: `config.go` — default `vad_threshold: 0.5` instead of `500` (line 943)
- [ ] Verify `cargo test` equivalent: `go test ./internal/usecase/ ./internal/config/` passes

## Phase 2: Harness Implementation

- [ ] Write `scripts/loopback-test.sh` with:
  - [ ] PipeWire null-sink creation (`pactl load-module module-null-sink`)
  - [ ] TTS audio generation (KittenTTS with wake phrase "Alice")
  - [ ] Audio injection via `pw-cat -p --target coldmic_loopback`
  - [ ] ColdMic daemon startup with correct env vars
  - [ ] Conversation mode start
  - [ ] Audio playback after listener is ready
  - [ ] Verification: tail daemon log for "speech detected"
  - [ ] Verification: check transcript endpoint for wake phrase match
  - [ ] Cleanup: kill daemon, unload virtual sink

## Phase 3: Run It — THIS IS THE DELIVERABLE

- [ ] Execute `scripts/loopback-test.sh` on endver
- [ ] Observe daemon log showing `continuous listener: speech detected`
- [ ] Observe daemon log showing Deepgram connection and transcript
- [ ] Observe daemon log showing `wake phrase matched`
- [ ] If any stage fails: debug, fix, and re-run until it works
- [ ] Capture the successful run output as evidence

## Phase 4: Cleanup & Archive

- [ ] Archive the OpenSpec change
- [ ] Update COD-389 with results
- [ ] Update coldmic skill with harness documentation
