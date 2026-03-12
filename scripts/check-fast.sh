#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

./scripts/check-format.sh
./scripts/check-workflows.sh
go test -count=1 ./...
golangci-lint run
