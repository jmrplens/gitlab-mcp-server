// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"io"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// toolName is how the report names itself.
const toolName = "audit_sdk_context"

// usage is the help text of the gate's flags.
var usage = sourcegate.Usage{
	Dir:     "repository root the patterns are resolved against",
	Check:   "exit non-zero when a call reaches client-go without the caller's context",
	Verbose: "also list the calls a declaration excuses",
}

// defaultPatterns is this repository's own library source. The end-to-end
// packages under test/ are left out: their scenario packages are test files
// this load does not read, and their library code, the harness and the
// fixture library, is outside the rule by decision, since it runs in a test
// process rather than inside a handler. Not every request that code makes
// passes a context, and the command's documentation says so.
var defaultPatterns = []string{"./internal/...", "./cmd/..."}

// auditConfig is one configured run: where to look, what to look at, and the
// declarations it is held to.
type auditConfig struct {
	dir      string
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository. Production
	// passes nil.
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
			declared: withoutContextOnPurpose,
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

// audit loads the tree without its test files and judges every call in it.
//
// Test files are left out deliberately. A test may build a request with no
// context on purpose, to show what the fixture that catches the defect does
// with one, and the scenario packages of the end-to-end suite are test files
// in their entirety; holding them to this rule would be a second rule with a
// declaration table of its own. They are the gate's blind spot, and the
// command's documentation says so.
//
// Files are named below the root the load resolved, made absolute before
// anything was loaded.
func audit(cfg auditConfig) (Report, error) {
	root, loaded, err := goprogram.LoadRooted(cfg.dir, cfg.patterns, goprogram.Options{Overlay: cfg.overlay})
	if err != nil {
		return Report{}, err
	}
	found := newScanner(root, cfg.overlay)
	found.observe(loaded)
	return buildReport(found, cfg.declared), nil
}
