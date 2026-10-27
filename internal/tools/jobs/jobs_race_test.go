// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build race

// jobs_race_test.go is the race-detector half of this package's test seam. The
// go tool sets the race build tag when -race is used, so this file is what
// `go test -race ./internal/tools/jobs/` compiles and jobs_norace_test.go is
// what every other run compiles.
package jobs

// raceEnabled reports that this test binary was built with -race, where heap
// allocation is not what the server pays: a race build's sync.Pool drops a
// quarter of what is put back, so the regexp machines and fmt printers an
// ordinary build reuses are allocated again. Measured on the 64 MiB log of
// TestJobTrace_LongLog_ReadsOnlyItsEnd, regexp.(*Regexp).get and fmt's printer
// pool allocated about 45 MB and 44 MB under -race and almost nothing without
// it, which put Trace about 62 MB over client-go's read against a bound of
// twenty 1 MB windows.
const raceEnabled = true
