# Development

## Prerequisites

Use Go 1.23+, Node.js/npm, ffmpeg, `lefthook`, and `gitleaks`. Wails desktop builds also need their platform-native dependencies.

## Commands

- `make test`: Go test suite.
- `make quality-go`: Go formatting, vet, and staticcheck.
- `make ci-quality-frontend`: install frontend dependencies and run lint/build.
- `make ci-test-go`: race-enabled Go tests and the coverage gate.
- `make ci-test-frontend`: frontend coverage tests.
- `make ci`: complete local CI gate.
- `make ci-build-cli`: build CLI and daemon binaries.
- `make verify-ci-contract`: verify Makefile/CI contract alignment.

When Go commands fail because `frontend/dist` is missing, build the frontend (`cd frontend && npm ci && npm run build`) or use the same minimal dist stub CI uses when the frontend itself is outside the change.

Run the narrowest relevant tests while iterating, then run `make ci` before requesting review. Do not hand-edit generated frontend build output.
