# Tasks: Local Wake-Word STT

## Phase 1: Port & Provider

- [x] Define `LocalSTT` interface in `internal/ports/local_stt.go` (`Init`, `Transcribe`)
- [x] Add `LocalSTT` config fields to `internal/config/config.go` (`COLDMIC_LOCAL_STT`, `COLDMIC_LOCAL_STT_MODEL`)
- [x] Create `internal/providers/whispercpp/` package
  - [x] `whisper.go` — subprocess provider implementing `LocalSTT`
  - [x] Auto-download model weights to `~/.cache/coldmic/` (binary must be on PATH)
  - [x] WAV header construction for PCM audio piped to stdin
  - [x] stdout parsing for transcript text
  - [x] Context cancellation kills entire process group (`Setpgid` + custom `Cancel`)
- [x] Unit tests for whisper.cpp provider (mocked subprocess)

## Phase 2: Two-Phase Continuous Listening

- [x] Modify `ContinuousListener` to accept optional `localSTT` provider
  - [x] New field: `localSTT ports.LocalSTT` (nil when disabled)
  - [x] When VAD speech segment ends and `localSTT != nil`:
    - [x] Buffer all speech audio during VAD-gated segment
    - [x] Feed buffered audio to `localSTT.Transcribe()`
    - [x] Check wake phrase on local transcript
    - [x] On match: start Deepgram streaming session, replay buffered audio
    - [x] On no match: discard buffer, stay listening (no Deepgram call)
  - [x] When `localSTT == nil`: existing behavior (stream to Deepgram during speech)
  - [x] Fallback on local STT failure: start cloud session with buffered audio
- [x] Unit tests pass (existing tests updated for new constructor signature)

## Phase 3: Bootstrap & Config

- [x] Wire `LocalSTT` in `internal/bootstrap/wire.go`
  - [x] Conditional creation based on `COLDMIC_LOCAL_STT` config value
  - [x] Pass to `NewContinuousListener` when enabled
  - [x] Fallback: nil `LocalSTT` when disabled (preserves existing behavior)
- [x] Update `ContinuousListenerConfig` to include `LocalSTT` field (via constructor)
- [x] Verify existing tests still pass (no regressions when LocalSTT is nil)

## Phase 4: Testing & Verification

- [x] Existing tests pass with race detector (edge_tts flake is pre-existing)
- [x] `go vet` clean
- [x] `go build ./...` clean
- [x] `gofmt` applied to all changed files
- [ ] Integration test: local STT enabled → wake phrase detected locally → cloud session starts
- [ ] Integration test: local STT disabled → existing Deepgram-only behavior unchanged
- [ ] Verify `make ci` passes (coverage gate — may need additional test coverage)
