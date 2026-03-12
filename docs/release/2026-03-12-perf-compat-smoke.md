# Release Perf/Compat Baseline

- Generated: 2026-03-12 06:27:55Z
- Host: darwin/arm64
- Current Go: go version go1.26.1 darwin/arm64
- Compatibility minimum under test: go1.21.13

## COMPAT-002 Go Version Matrix

```text
$ go test -count=1 ./...
ok  	github.com/runger/fuse	1.936s
?   	github.com/runger/fuse/cmd/fuse	[no test files]
ok  	github.com/runger/fuse/internal/adapters	0.701s
ok  	github.com/runger/fuse/internal/approve	0.433s
ok  	github.com/runger/fuse/internal/cli	1.829s
?   	github.com/runger/fuse/internal/config	[no test files]
ok  	github.com/runger/fuse/internal/core	1.683s
ok  	github.com/runger/fuse/internal/db	1.360s
?   	github.com/runger/fuse/internal/events	[no test files]
ok  	github.com/runger/fuse/internal/inspect	0.955s
ok  	github.com/runger/fuse/internal/policy	1.117s
ok  	github.com/runger/fuse/internal/releasecheck	1.494s

$ GOTOOLCHAIN=go1.21.13 go test -count=1 ./...
command failed with exit code 1
```

## Release-Check Suite

```text
$ FUSE_RELEASE_CHECK=1 go test ./internal/releasecheck -count=1 -v
=== RUN   TestReleaseCheckShellWarmPathPerf
    releasecheck_test.go:128: PERF-001 n=1000 p50=47.708µs p95=74.542µs p99=132.375µs max=291.25µs
--- PASS: TestReleaseCheckShellWarmPathPerf (0.06s)
=== RUN   TestReleaseCheckShellColdPathPerf
=== RUN   TestReleaseCheckShellColdPathPerf/PERF-002_safe
    releasecheck_test.go:188: PERF-002 safe n=25 p50=9.246375ms p95=11.763584ms p99=12.285125ms max=799.809875ms
=== RUN   TestReleaseCheckShellColdPathPerf/PERF-002_approval
    releasecheck_test.go:188: PERF-002 approval n=25 p50=12.070709ms p95=16.535417ms p99=16.535459ms max=16.801708ms
--- PASS: TestReleaseCheckShellColdPathPerf (8.69s)
    --- PASS: TestReleaseCheckShellColdPathPerf/PERF-002_safe (1.03s)
    --- PASS: TestReleaseCheckShellColdPathPerf/PERF-002_approval (0.32s)
=== RUN   TestReleaseCheckMCPWarmPathPerf
    releasecheck_test.go:204: PERF-002A n=2000 p50=167ns p95=250ns p99=292ns max=6.166µs
--- PASS: TestReleaseCheckMCPWarmPathPerf (0.00s)
=== RUN   TestReleaseCheckRegexPathologicalPerf
=== RUN   TestReleaseCheckRegexPathologicalPerf/rm-repeat
    releasecheck_test.go:264: PERF-003 rm-repeat n=25 p50=22.279792ms p95=22.920667ms p99=22.936458ms max=24.115917ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/uppercase-32k
    releasecheck_test.go:264: PERF-003 uppercase-32k n=25 p50=102.440292ms p95=103.401208ms p99=103.866625ms max=105.242042ms
    releasecheck_test.go:266: uppercase-32k p95 = 103.401208ms, want < 100ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/uppercase-64k
    releasecheck_test.go:264: PERF-003 uppercase-64k n=25 p50=204.622334ms p95=205.316792ms p99=205.462042ms max=205.503791ms
    releasecheck_test.go:266: uppercase-64k p95 = 205.316792ms, want < 100ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/terraform-repeat
    releasecheck_test.go:264: PERF-003 terraform-repeat n=25 p50=3.229792ms p95=3.665958ms p99=3.696417ms max=3.7825ms
=== NAME  TestReleaseCheckRegexPathologicalPerf
    releasecheck_test.go:272: PERF-003 uppercase ratio p95=1.99x
--- FAIL: TestReleaseCheckRegexPathologicalPerf (8.32s)
    --- PASS: TestReleaseCheckRegexPathologicalPerf/rm-repeat (0.56s)
    --- FAIL: TestReleaseCheckRegexPathologicalPerf/uppercase-32k (2.57s)
    --- FAIL: TestReleaseCheckRegexPathologicalPerf/uppercase-64k (5.11s)
    --- PASS: TestReleaseCheckRegexPathologicalPerf/terraform-repeat (0.08s)
=== RUN   TestReleaseCheckShellWrapperCompatibility
=== RUN   TestReleaseCheckShellWrapperCompatibility/bash
=== RUN   TestReleaseCheckShellWrapperCompatibility/zsh
=== RUN   TestReleaseCheckShellWrapperCompatibility/fish
--- PASS: TestReleaseCheckShellWrapperCompatibility (7.82s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/bash (0.34s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/zsh (0.03s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/fish (0.14s)
=== RUN   TestReleaseCheckLocaleInvariantClassification
--- PASS: TestReleaseCheckLocaleInvariantClassification (0.00s)
FAIL
FAIL	github.com/runger/fuse/internal/releasecheck	25.138s
FAIL
release-check suite failed with exit code 1
```
