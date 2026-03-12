# 2026-03-12 Perf And Compatibility Smoke

## Scope

This is the first machine-readable release-check baseline produced by:

- `scripts/run-release-checks.sh`
- `internal/releasecheck/releasecheck_test.go`

Machine used:

- `go version go1.26.1 darwin/arm64`

## Current Results

### Current toolchain and test surface

- `go test -count=1 ./...` passes

### Platform and shell compatibility

- cross-build passes for:
  - `darwin/arm64`
  - `darwin/amd64`
  - `linux/amd64`
  - `linux/arm64`
- shell wrapper compatibility passes locally for:
  - `bash`
  - `zsh`
  - `fish`
- locale invariance passes locally for:
  - `LC_ALL=C`
  - `LC_ALL=en_US.UTF-8`
  - `LANG=ja_JP.UTF-8`

### Performance smoke

- `PERF-001` shell warm path:
  - `p50 41.5µs`
  - `p95 55.541µs`
  - `p99 161.209µs`
  - `max 358.334µs`
- `PERF-002` cold shell hook path:
  - safe command `git status`: `p95 10.282416ms`
  - approval command `python nonexistent_script.py`: `p95 11.956875ms`
- `PERF-002A` MCP hot-path classification:
  - `p50 167ns`
  - `p95 250ns`
  - `p99 292ns`
  - `max 11.083µs`
- `PERF-003` pathological inputs:
  - `rm-repeat` p95 `22.568084ms`
  - `terraform-repeat` p95 `4.0835ms`
  - `uppercase-32k` p95 `104.592458ms` and currently fails the `<100ms` target
  - `uppercase-64k` p95 `208.465875ms` and currently fails the `<100ms` target
  - `32k -> 64k` p95 ratio is `1.99x`, which passes the `<=2.5x` scaling check

## Confirmed Gaps

- `COMPAT-002` currently fails:
  - `GOTOOLCHAIN=go1.21.13 go test -count=1 ./...`
  - failure: `go.mod requires go >= 1.25.0`
- `PERF-003` currently fails on large unmatched uppercase inputs
- `PERF-002B`, `PERF-004`, `PERF-005`, `COMPAT-005`, `COMPAT-006`, and `COMPAT-007` still need dedicated release-check coverage

## Honest Conclusion

This branch now has repeatable performance and compatibility evidence, but not full closure:

- several hot-path claims are now supported by measurement on the baseline machine
- the written Go-version support floor is currently false
- pathological long-input performance still misses one written target
