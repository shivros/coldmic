#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

command -v lefthook >/dev/null || { echo "lefthook is required" >&2; exit 1; }
command -v gitleaks >/dev/null || { echo "gitleaks is required" >&2; exit 1; }
command -v ssh-keygen >/dev/null || { echo "ssh-keygen is required" >&2; exit 1; }

if [ ! -x .git/hooks/pre-commit ]; then
  echo "pre-commit hook is not installed; run 'make install-hooks' first" >&2
  exit 1
fi

scratch_dir="$(mktemp -d)"
test_file=".gitleaks-hook-verification.key"
cleanup() {
  rm -f "$test_file"
  git restore --staged "$test_file" >/dev/null 2>&1 || true
  rm -rf "$scratch_dir"
}
trap cleanup EXIT

ssh-keygen -q -t ed25519 -N '' -f "$scratch_dir/test-key" >/dev/null
cp "$scratch_dir/test-key" "$test_file"
git add "$test_file"

if lefthook run pre-commit >/dev/null 2>&1; then
  echo "expected pre-commit hook to fail on a staged private key, but it passed" >&2
  exit 1
fi

echo "secret hook verification passed"
