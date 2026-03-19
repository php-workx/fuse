#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

./scripts/check-fast.sh
go test -race -count=1 ./...
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.24.12}" govulncheck ./...
FUSE_RELEASE_CHECK=1 go test ./internal/releasecheck -count=1 -run 'TestReleaseCheckShellWrapperCompatibility|TestReleaseCheckLocaleInvariantClassification' -v
make build-all
