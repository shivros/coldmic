# Tasks: Local Wake-Word STT

## Phase 1: Port & Provider

- [ ] Define `LocalSTT` interface in `internal/ports/local_stt.go` (`Init`, `Transcribe`)
- [ ] Add `LocalSTT` config fields to `internal/config/config.go` (`COLDMIC_LOCAL_STT`, `COLDMIC_LOCAL_STT_MODEL`)
- [ ] Create `internal/providers/whispercpp/` package
  - [ ] `whisper.go` — subprocess provider implementing `LocalSTT`
  - [ ] `download.go` — auto-download binary + model to `~/.cache/coldmic/`
  - [ ] WAV header construction for PCM audio piped to stdin
  - [ ] stdout parsing for transcript text
  - [ ] Context cancellation kills subprocess
- [ ] Unit tests for whisper.cpp provider (mocked subprocess)

## Phase 2: Two-Phase Continuous Listening

- [ ] Modify `ContinuousListener` to accept optional `LocalSTT` provider
  - [ ] New field: `localSTT ports.LocalSTT` (nil when disabled)
  - [ ] When VAD speech segment ends and `localSTT != nil`:
    - [ ] Buffer all speech audio during VAD-gated segment
    - [ ] Feed buffered audio to `localSTT.Transcribe()`
    - [ ] Check wake phrase on local transcript
    - [ ] On match: create Deepgram streaming session, replay buffered audio, continue live
    - [ ] On no match: discard buffer, stay listening (no Deepgram call)
  - [ ] When `localSTT == nil`: existing behavior (stream to Deepgram during speech)
- [ ] Unit tests for two-phase flow with mocked LocalSTT

## Phase 3: Bootstrap & Config

- [ ] Wire `LocalSTT` in `internal/bootstrap/wire.go`
  - [ ] Conditional creation based on `COLDMIC_LOCAL_STT` config value
  - [ ] Pass to `NewContinuousListener` when enabled
  - [ ] Fallback: nil `LocalSTT` when disabled (preserves existing behavior)
- [ ] Update `ContinuousListenerConfig` to include `LocalSTT` field
- [ ] Verify existing tests still pass (no regressions when LocalSTT is nil)

## Phase 4: Testing & Verification

- [ ] Integration test: local STT enabled → wake phrase detected locally → Deepgram session starts
- [ ] Integration test: local STT enabled → no wake phrase → no Deepgram call
- [ ] Integration test: local STT disabled → existing Deepgram-only behavior unchanged
- [ ] Verify `make test` passes with race detector
- [ ] Verify `make ci` passes (gofmt, staticcheck, coverage gate)
