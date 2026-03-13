#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cd "$ROOT_DIR"

echo "== Current toolchain =="
go version
go test -count=1 ./...

echo
echo "== Go version compatibility =="
set +e
GOTOOLCHAIN=go1.24.0 go test -count=1 ./...
go124_status=$?
set -e
if [[ "$go124_status" -ne 0 ]]; then
  echo "go1.24.0 compatibility check failed with status $go124_status"
fi

echo
echo "== Cross-build matrix =="
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  out="$TMP_DIR/fuse-${goos}-${goarch}"
  echo "-- building ${goos}/${goarch}"
  GOOS="$goos" GOARCH="$goarch" go build -o "$out" ./cmd/fuse
done

echo
echo "== Release-check perf and compatibility harness (includes Codex shell smoke) =="
FUSE_RELEASE_CHECK=1 go test ./internal/releasecheck -count=1 -v
