#!/usr/bin/env bash
set -euo pipefail

required_targets=(
  ci-quality-frontend
  ci-test-frontend
  ci-build-cli
  ci-build-desktop
  ci-build-desktop-linux
  ci-test-go
)

for target in "${required_targets[@]}"; do
  echo "Validating Make target: ${target}"
  make -n "${target}" >/dev/null

done

echo "CI contract check passed."
