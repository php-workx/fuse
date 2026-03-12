#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

if ! command -v actionlint >/dev/null 2>&1; then
  echo "actionlint is required for workflow lint checks. Run: make install-dev" >&2
  exit 1
fi

actionlint
