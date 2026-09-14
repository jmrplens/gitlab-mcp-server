//go:build e2e

// coverage.go points the server children at a coverage directory and reports
// what they wrote into it.
//
// It answers a question nothing else here asks. cmd/audit_e2e_coverage says
// which catalog actions dispatched, which is a statement about the surface and
// none at all about the program around it: the startup path, the catalog
// build, the middleware chain and the error branches no live-GitLab scenario
// reaches are invisible to it. The Go toolchain answers that for a binary
// built with -cover, which writes one meta-data file when the program starts
// and one counter file when it exits, both into the directory GOCOVERDIR
// names.
//
// Nothing here instruments anything: the build carries -cover (the Makefile's
// COVER switch, and CI's own build line), and this file only decides where the
// children write and says afterwards how many of them wrote. A run whose
// binary was built without it leaves the directory empty, which is what
// `make e2e-go-coverage` refuses rather than reporting as zero percent.

package harness

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	// coverDirEnv names the directory every server child of this run writes
	// its coverage data into. Empty, which is the default, measures nothing.
	//
	// It is read through the run's settings, like E2E_SERVER_BINARY and every
	// other key this harness configures itself with, rather than with
	// os.Getenv where it is used: configuration is resolved once, into a map,
	// and a child's environment is built from that map rather than from
	// whatever this process happens to carry.
	coverDirEnv = "E2E_COVER_DIR"

	// goCoverDirVar is the toolchain's own variable, and the one thing an
	// instrumented child reads. It is set on the child and never on this
	// process: the harness's own test binary is not what is being measured.
	goCoverDirVar = "GOCOVERDIR"

	// counterFilePrefix is how the runtime names what one exited process
	// contributed. The rest of the name is the meta-data hash, the process
	// id and the time, so two children can never collide and one directory
	// for the whole run is enough.
	counterFilePrefix = "covcounters."
)

// coverageDir resolves the directory the children write coverage into,
// creating it, and returns the empty string when the run asked for none.
//
// The path must be absolute for the same reason the calls directory must be:
// a child's working directory is its own session directory under the harness
// root, so a relative path would name a different directory for every child
// and every one of them would be deleted with that root at the end of the run.
// It is created here because the toolchain refuses a directory that is not
// there rather than making one, and the refusal lands on the child's stderr,
// where a run that measured nothing looks exactly like a run that measured
// zero.
func coverageDir(s settings) (string, error) {
	dir := strings.TrimSpace(s.get(coverDirEnv))
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%s must be an absolute path, and is %q: a server child runs in its own "+
			"session directory, so a relative one names a different directory for each of them, inside the "+
			"tree the run deletes when it ends", coverDirEnv, dir)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("creating the coverage directory %s: %w", dir, err)
	}
	return dir, nil
}

// coverageVariables returns the environment one child carries so its counters
// land where the run asked, or nothing when the run measures nothing.
//
// A map rather than a path, so the caller merges it the way it merges the
// telemetry variables and a run without coverage adds no key at all: a child
// given an empty GOCOVERDIR would report an error at exit instead of staying
// silent.
func coverageVariables(s settings) (map[string]string, error) {
	dir, err := coverageDir(s)
	if err != nil || dir == "" {
		return nil, err
	}
	return map[string]string{goCoverDirVar: dir}, nil
}

// reportCoverage says how many counter files the directory holds against how
// many children this package started, and must be called once those children
// have been collected.
//
// Both numbers, because either alone is unreadable. The count of files says
// nothing about what is missing, and the count of children says nothing about
// what arrived: a child that is killed rather than asked to stop runs no exit
// hook, so its counters are lost while its meta-data file is not, and the
// merged profile is quietly short by everything that child executed. That is
// the defect this whole seam exists around, and the one line here is what
// makes a return of it visible in the run's own log.
//
// The file count is cumulative and the child count is not: every package of a
// run writes into the one directory, and packages run one at a time, so the
// second package's line counts the first package's files too. Fewer files than
// this package alone started is therefore a floor on what was lost rather than
// the whole of it, which is all a count at this grain can honestly be.
func reportCoverage(s settings) {
	dir := strings.TrimSpace(s.get(coverDirEnv))
	if dir == "" {
		return
	}

	started := childrenStarted.Load()
	written, err := countCoverageCounters(dir)
	if err != nil {
		log.Printf("e2e: coverage: reading %s: %v", dir, err)
		return
	}

	log.Printf("e2e: coverage: %s holds %d counter files; this package started %d server children",
		dir, written, started)
	if int64(written) < started {
		log.Printf("e2e: coverage: at least %d children wrote none: a process that is killed runs no exit hook, "+
			"so whatever it executed is missing from the merged profile", started-int64(written))
	}
}

// countCoverageCounters counts the counter files one directory holds.
//
// Counter files only: the meta-data file is written once per binary when the
// program starts, so counting it would say a run flushed when every child of
// it had been killed.
func countCoverageCounters(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	counters := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), counterFilePrefix) {
			counters++
		}
	}
	return counters, nil
}
