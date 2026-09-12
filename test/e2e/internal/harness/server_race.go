//go:build e2e && race

// server_race.go is the race-detector half of the build seam. The go tool
// sets the race build tag when -race is used, so this file is what a
// `go test -race -tags e2e` run compiles and server_norace.go is what every
// other run compiles.

package harness

import "time"

// serverBuildArgs returns the go build arguments for the server under test,
// with the detector on.
//
// The seam exists because `go test -race` instruments the test binary and
// nothing else. The server is a separate process built by this harness, so
// without passing the flag on, a race run would watch the harness's own
// goroutines and say nothing about the server's, which are the ones the suite
// exists to reach.
func serverBuildArgs(out string) []string {
	return []string{"build", "-race", "-o", out, "./cmd/server"}
}

// serverBuildTimeout bounds that build. A race build shares no object cache
// with an ordinary one, so it is a cold build of the whole dependency tree
// even on a machine that has just compiled these tests.
const serverBuildTimeout = 15 * time.Minute

// raceChildEnv is the extra environment an instrumented child is started with.
//
// Without halt_on_error the race runtime prints its report to stderr and lets
// the process continue, setting the exit status only at a clean exit. A server
// the harness stops at the end of a test would take the report to its log with
// it and the run would stay green. Halting fails whichever test was talking to
// it, with the report in that child's captured stderr.
func raceChildEnv() map[string]string {
	return map[string]string{"GORACE": "halt_on_error=1"}
}
