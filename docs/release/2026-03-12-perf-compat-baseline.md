== Current toolchain ==
go version go1.26.1 darwin/arm64
ok  	github.com/runger/fuse	5.145s
?   	github.com/runger/fuse/cmd/fuse	[no test files]
ok  	github.com/runger/fuse/internal/adapters	1.478s
ok  	github.com/runger/fuse/internal/approve	1.582s
ok  	github.com/runger/fuse/internal/cli	2.851s
?   	github.com/runger/fuse/internal/config	[no test files]
ok  	github.com/runger/fuse/internal/core	2.406s
ok  	github.com/runger/fuse/internal/db	3.715s
?   	github.com/runger/fuse/internal/events	[no test files]
ok  	github.com/runger/fuse/internal/inspect	2.751s
ok  	github.com/runger/fuse/internal/policy	3.368s
ok  	github.com/runger/fuse/internal/releasecheck	4.110s

== Go version compatibility ==
ok  	github.com/runger/fuse	1.961s
?   	github.com/runger/fuse/cmd/fuse	[no test files]
ok  	github.com/runger/fuse/internal/adapters	0.560s
ok  	github.com/runger/fuse/internal/approve	0.675s
ok  	github.com/runger/fuse/internal/cli	1.868s
?   	github.com/runger/fuse/internal/config	[no test files]
ok  	github.com/runger/fuse/internal/core	1.379s
ok  	github.com/runger/fuse/internal/db	1.092s
?   	github.com/runger/fuse/internal/events	[no test files]
ok  	github.com/runger/fuse/internal/inspect	1.182s
ok  	github.com/runger/fuse/internal/policy	1.491s
ok  	github.com/runger/fuse/internal/releasecheck	1.552s

== Cross-build matrix ==
-- building darwin/arm64
-- building darwin/amd64
-- building linux/amd64
-- building linux/arm64

== Release-check perf and compatibility harness ==
=== RUN   TestReleaseCheckShellWarmPathPerf
    releasecheck_test.go:128: PERF-001 n=1000 p50=47.542µs p95=72.958µs p99=153.833µs max=264.458µs
--- PASS: TestReleaseCheckShellWarmPathPerf (0.05s)
=== RUN   TestReleaseCheckShellColdPathPerf
=== RUN   TestReleaseCheckShellColdPathPerf/PERF-002_safe
    releasecheck_test.go:188: PERF-002 safe n=25 p50=9.235041ms p95=11.520125ms p99=11.568667ms max=284.060167ms
=== RUN   TestReleaseCheckShellColdPathPerf/PERF-002_approval
    releasecheck_test.go:188: PERF-002 approval n=25 p50=10.31425ms p95=11.124709ms p99=11.293708ms max=15.173667ms
--- PASS: TestReleaseCheckShellColdPathPerf (8.15s)
    --- PASS: TestReleaseCheckShellColdPathPerf/PERF-002_safe (0.51s)
    --- PASS: TestReleaseCheckShellColdPathPerf/PERF-002_approval (0.26s)
=== RUN   TestReleaseCheckMCPWarmPathPerf
    releasecheck_test.go:204: PERF-002A n=2000 p50=167ns p95=250ns p99=292ns max=11.25µs
--- PASS: TestReleaseCheckMCPWarmPathPerf (0.00s)
=== RUN   TestReleaseCheckRegexPathologicalPerf
=== RUN   TestReleaseCheckRegexPathologicalPerf/rm-repeat
    releasecheck_test.go:264: PERF-003 rm-repeat n=25 p50=21.642541ms p95=22.016625ms p99=22.230667ms max=22.625792ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/uppercase-32k
    releasecheck_test.go:264: PERF-003 uppercase-32k n=25 p50=101.2815ms p95=101.950584ms p99=101.96575ms max=102.655375ms
    releasecheck_test.go:266: uppercase-32k p95 = 101.950584ms, want < 100ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/uppercase-64k
    releasecheck_test.go:264: PERF-003 uppercase-64k n=25 p50=202.932166ms p95=204.103375ms p99=204.404334ms max=211.871625ms
    releasecheck_test.go:266: uppercase-64k p95 = 204.103375ms, want < 100ms
=== RUN   TestReleaseCheckRegexPathologicalPerf/terraform-repeat
    releasecheck_test.go:264: PERF-003 terraform-repeat n=25 p50=3.165042ms p95=3.6645ms p99=3.674416ms max=3.685458ms
=== NAME  TestReleaseCheckRegexPathologicalPerf
    releasecheck_test.go:272: PERF-003 uppercase ratio p95=2.00x
--- FAIL: TestReleaseCheckRegexPathologicalPerf (8.24s)
    --- PASS: TestReleaseCheckRegexPathologicalPerf/rm-repeat (0.54s)
    --- FAIL: TestReleaseCheckRegexPathologicalPerf/uppercase-32k (2.53s)
    --- FAIL: TestReleaseCheckRegexPathologicalPerf/uppercase-64k (5.08s)
    --- PASS: TestReleaseCheckRegexPathologicalPerf/terraform-repeat (0.08s)
=== RUN   TestReleaseCheckShellWrapperCompatibility
=== RUN   TestReleaseCheckShellWrapperCompatibility/bash
=== RUN   TestReleaseCheckShellWrapperCompatibility/zsh
=== RUN   TestReleaseCheckShellWrapperCompatibility/fish
--- PASS: TestReleaseCheckShellWrapperCompatibility (10.38s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/bash (0.33s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/zsh (0.03s)
    --- PASS: TestReleaseCheckShellWrapperCompatibility/fish (0.18s)
=== RUN   TestReleaseCheckLocaleInvariantClassification
--- PASS: TestReleaseCheckLocaleInvariantClassification (0.00s)
FAIL
FAIL	github.com/runger/fuse/internal/releasecheck	27.077s
FAIL
