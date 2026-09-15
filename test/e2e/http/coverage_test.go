//go:build httpe2e

// coverage_test.go points this module's server children at the coverage
// directory the run was given, so an instrumented binary writes its counters
// where `make e2e-go-coverage` looks for them.
//
// Nothing here instruments anything. The build carries -cover (the Makefile's
// COVER switch) and this only decides where the children write.
package httpe2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coverDirEnv names the directory every server child of this run writes its
// coverage data into. Empty, which is the default, measures nothing.
//
// It is the same variable the rebuilt suite's harness reads
// (test/e2e/internal/harness/coverage.go) and it means the same thing here:
// the leaf `make` hands one target under dist/e2e-cover. How it is resolved
// differs exactly the way E2E_SERVER_BINARY's does, and for the same reason —
// the harness reads it through the run's settings, which overlay .env and
// test/e2e/.env.docker, while this module reads the process environment and
// nothing else, so a value written into .env reaches the harness and not this.
//
// Read here rather than shared with the harness deliberately. That package is
// behind //go:build e2e, so nothing compiled under httpe2e can import it, and
// a package every one of the four build configurations could import would have
// to carry no constraint at all — which is the one shape the gate over
// test/e2e/internal refuses. The duplication is the same one E2E_SERVER_BINARY
// already carries in all four packages, and it is not silent: a copy that
// drifts leaves dist/e2e-cover/http empty, which `make e2e-go-coverage`
// refuses rather than reporting as zero percent.
const coverDirEnv = "E2E_COVER_DIR"

// goCoverDirVar is the toolchain's own variable, and the one thing an
// instrumented child reads. It is set on the child and never on this process:
// this module's test binary is not what is being measured.
const goCoverDirVar = "GOCOVERDIR"

// coverEnviron returns the environment entries one server child carries so its
// counters land where the run asked, and nothing at all when the run measures
// nothing.
//
// Absent rather than empty, which is the whole reason this is a function and
// not a string: the runtime of a child given an empty GOCOVERDIR prints
// "warning: GOCOVERDIR not set, no coverage data emitted" the moment it
// starts, from the meta-data emit the main package's init runs, and every
// child here has its stderr folded into the buffer each failure message
// prints. The stdio module's copy of this is stricter still, since a line that
// is not JSON fails a test there outright.
//
// The directory is created here because the toolchain refuses one that is not
// there rather than making it, and the refusal lands in that same buffer,
// where a run that measured nothing looks exactly like a run that measured
// zero.
func coverEnviron(t *testing.T) []string {
	t.Helper()

	dir := strings.TrimSpace(os.Getenv(coverDirEnv))
	if dir == "" {
		return nil
	}
	// Absolute, because a child inherits this process's working directory,
	// which is this package's own: a relative path would put the counters
	// under test/e2e/http and leave the directory the run named empty.
	if !filepath.IsAbs(dir) {
		t.Fatalf("%s must be an absolute path, and is %q: a relative one resolves against this package's "+
			"directory rather than the one the run named", coverDirEnv, dir)
	}
	//#nosec G703 -- the path is E2E_COVER_DIR, chosen by whoever runs these tests, and the directory is theirs to name: a run measures where it says it measures. It is already refused unless absolute, and the smaller half of what this run does with it, since the next thing is to hand it to a server child.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating the coverage directory %s: %v", dir, err)
	}
	return []string{goCoverDirVar + "=" + dir}
}
