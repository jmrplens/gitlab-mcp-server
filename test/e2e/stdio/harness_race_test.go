//go:build stdioe2e && race

// harness_race_test.go is the race-detector half of the harness build seam. The
// go tool sets the `race` build tag when -race is used, so this file is what is
// compiled by `go test -race -tags stdioe2e ./test/e2e/stdio/` and
// harness_norace_test.go is what is compiled otherwise.
package stdioe2e

import "time"

// serverBuildArgs returns the `go build` arguments for the server under test,
// with the detector on.
//
// This seam exists because `go test -race` instruments the test binary and
// nothing else. The server is a separate process built by the harness, so
// without passing the flag on, a race run would watch the harness's own
// goroutines and say nothing about the server's, which are the ones this module
// exists to reach: what is under test is the process, its pipes and its
// streams.
func serverBuildArgs(out string) []string {
	return []string{"build", "-race", "-o", out, "./cmd/server"}
}

// serverBuildTimeout bounds that build. A race build shares no object cache
// with an ordinary one, so it is a cold build of the whole dependency tree even
// on a machine that has just compiled these tests.
const serverBuildTimeout = 15 * time.Minute

// raceEnviron is the extra environment an instrumented server is started with.
//
// Without halt_on_error the race runtime prints its report to stderr and lets
// the process continue, setting the exit status only at a clean exit. A server
// that is terminated at the end of a test, which is most of them here, would
// then take the report to the log with it and the run would stay green.
// Halting fails whichever test was talking to it, with the report in the
// captured stderr.
func raceEnviron() []string {
	return []string{"GORACE=halt_on_error=1"}
}

// prebuiltBinaryRefusal says why a server staged by something else cannot be
// driven by this run, or "" when it can.
//
// Under the detector it cannot. The seam above passes -race on to the build
// this package does, and nothing here can tell whether a path handed in
// through E2E_SERVER_BINARY was built the same way: it almost certainly was
// not, since the Makefile target that stages one compiles it plainly. Driving
// an uninstrumented server would leave the run watching the test binary alone
// and reporting no race because nothing was watching, which is worse than a
// failure — .github/workflows/race.yml runs this module precisely to watch the
// server. So the variable is refused here rather than honored, and a race run
// that wants to skip the build has to say so by unsetting it.
func prebuiltBinaryRefusal() string {
	return "this run is under the race detector, which needs the instrumented server this package builds"
}
