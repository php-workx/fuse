#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

bash ./scripts/check-format.sh
bash ./scripts/check-workflows.sh

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required for fast checks. Run: make install-dev" >&2
  exit 1
fi

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint is required for fast checks. Run: make install-dev" >&2
  exit 1
fi

go test -count=1 ./...
golangci-lint run
