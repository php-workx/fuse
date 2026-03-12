#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

if [ "$#" -gt 0 ]; then
  go_files=()
  for path in "$@"; do
    if [ -f "$path" ] && [[ "$path" == *.go ]]; then
      go_files+=("$path")
    fi
  done
else
  while IFS= read -r path; do
    [ -n "$path" ] || continue
    go_files+=("$path")
  done <<EOF
$(git ls-files '*.go')
EOF
fi

if [ "${#go_files[@]}" -eq 0 ]; then
  exit 0
fi

if ! command -v goimports >/dev/null 2>&1; then
  echo "goimports is required for format checks. Run: make install-dev" >&2
  exit 1
fi

gofmt_output="$(gofmt -l "${go_files[@]}")"
if [ -n "$gofmt_output" ]; then
  echo "gofmt check failed. Run: gofmt -w" >&2
  echo "$gofmt_output" >&2
  exit 1
fi

goimports_output="$(goimports -l "${go_files[@]}")"
if [ -n "$goimports_output" ]; then
  echo "goimports check failed. Run: goimports -w" >&2
  echo "$goimports_output" >&2
  exit 1
fi
