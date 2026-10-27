// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"io"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// toolName is how the report names itself.
const toolName = "audit_test_closed_servers"

// usage is the help text of the gate's flags.
var usage = sourcegate.Usage{
	Dir:     "module root the patterns are resolved against",
	Check:   "exit non-zero when an httptest server is closed while its address is still used",
	Verbose: "also list the closes a declaration excuses",
}

// defaultPatterns is the whole module: a closed server is a hazard in any
// test, the end-to-end packages included, and the loader is asked for every
// package's tests.
var defaultPatterns = []string{"./..."}

// analysisTags are the build tags every load of this gate sets, so the test
// files behind each end-to-end suite's tag are read. It must stay every one:
// it is GO_ANALYSIS_TAGS in the Makefile, which
// TestAnalysisTags_AreTheMakefilesAnalysisTags holds it to, and a suite added
// behind a new tag is invisible here until the tag is added to both.
var analysisTags = []string{"collectore2e", "e2e", "httpe2e", "orbitlive", "stdioe2e"}

// auditConfig is one configured run: where to look, what to look at, under
// which build tags, and the declarations it is held to.
type auditConfig struct {
	dir      string
	patterns []string
	tags     []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a file of its own. Production passes nil.
	overlay map[string][]byte
	// declared is the declaration table, which a test replaces so an excused
	// site and a stale entry can be exercised without editing the real one.
	declared map[string]sourcegate.Declaration
}

func main() {
	sourcegate.Main(runMain)
}

// runMain parses the command line and runs the audit it describes, returning
// the process exit code: 0 for a request for help, 2 for arguments it cannot
// parse, and otherwise what [run] decides.
func runMain(args []string, stdout, stderr io.Writer) int {
	return sourcegate.Run(toolName, usage, args, stderr, func(cmd *sourcegate.Command) int {
		return run(auditConfig{
			dir:      cmd.Dir,
			patterns: cmd.Patterns(defaultPatterns),
			tags:     analysisTags,
			declared: usedAfterTheCloseOnPurpose,
		}, cmd.Check, cmd.Verbose, stdout, stderr)
	})
}

// run scans, reports, and returns the process exit code [sourcegate.Finish]
// decides: a run that could not be made fails on stderr, and one that found
// something says so on stdout and fails only under -check.
func run(cfg auditConfig, check, verbose bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	return sourcegate.Finish(toolName, report, err, check, verbose, stdout, stderr)
}

// audit loads the tree with its tests under the configured tags and judges
// every function in it, naming files below the root the load resolved.
func audit(cfg auditConfig) (Report, error) {
	root, loaded, err := goprogram.LoadRooted(cfg.dir, cfg.patterns, goprogram.Options{
		Tests:     true,
		BuildTags: cfg.tags,
		Overlay:   cfg.overlay,
	})
	if err != nil {
		return Report{}, err
	}
	found := newScanner(root, cfg.overlay)
	found.observe(loaded)
	return buildReport(found, cfg.declared), nil
}
