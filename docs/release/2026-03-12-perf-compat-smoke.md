# 2026-03-12 Perf And Compatibility Smoke

## Scope

This is the summarized release-check baseline for:

- `scripts/run-release-checks.sh`
- `internal/releasecheck/releasecheck_test.go`

Raw command output is preserved separately in:

- `docs/release/2026-03-12-perf-compat-baseline.md`

Machine used:

- `go version go1.26.1 darwin/arm64`
- minimum supported Go under test: `go1.24.0`

## Current Results

### Toolchain and test surface

- `go test -count=1 ./...` passes
- `GOTOOLCHAIN=go1.24.0 go test -count=1 ./...` passes

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
  - `p50 47.542µs`
  - `p95 72.958µs`
  - `p99 153.833µs`
  - `max 264.458µs`
- `PERF-002` cold shell hook path:
  - safe command `git status`: `p95 11.520125ms`
  - approval command `python nonexistent_script.py`: `p95 11.124709ms`
- `PERF-002A` MCP hot-path classification:
  - `p50 167ns`
  - `p95 250ns`
  - `p99 292ns`
  - `max 11.25µs`
- `PERF-003` pathological inputs:
  - `rm-repeat` p95 `22.016625ms`
  - `terraform-repeat` p95 `3.6645ms`
  - `uppercase-32k` p95 `101.950584ms` and currently fails the `<100ms` target
  - `uppercase-64k` p95 `204.103375ms` and currently fails the `<100ms` target
  - `32k -> 64k` p95 ratio is `2.00x`, which passes the `<=2.5x` scaling check

## Confirmed Gaps

- `PERF-003` currently fails on large unmatched uppercase inputs
- `PERF-002B`, `PERF-004`, `PERF-005`, `COMPAT-005`, `COMPAT-006`, and `COMPAT-007` still need dedicated release-check coverage

## Honest Conclusion

This branch now has repeatable performance and compatibility evidence, but not full closure:

- several hot-path claims are now supported by measurement on the baseline machine
- the declared Go `1.24` support floor is now validated
- pathological long-input performance still misses one written target
