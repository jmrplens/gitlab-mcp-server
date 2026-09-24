package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// toolName is how the report names itself.
const toolName = "audit_sdk_context"

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
	declared map[string]declaration
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs the audit it describes, returning
// the process exit code: 2 for arguments it cannot parse, and otherwise what
// [run] decides.
func runMain(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "repository root the patterns are resolved against")
	check := flags.Bool("check", false, "exit non-zero when a call reaches client-go without the caller's context")
	verbose := flags.Bool("v", false, "also list the calls a declaration excuses")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		patterns = defaultPatterns
	}
	return run(auditConfig{
		dir:      *dir,
		patterns: patterns,
		declared: withoutContextOnPurpose,
	}, *check, *verbose, stdout, stderr)
}

// run scans, reports, and returns the process exit code.
//
// Exit 1 covers two different things on purpose, told apart by which stream
// spoke: a run that could not be made at all says so on stderr, and a run that
// found something says it on stdout and only fails under -check. A report is
// worth having without a gate, and a gate is worth having without ceremony.
func run(cfg auditConfig, check, verbose bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	report.write(stdout, verbose)
	if check && !report.ok() {
		return 1
	}
	return 0
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
// The root is made absolute before anything is loaded, so a run whose working
// directory is gone stops on that error instead of naming files against
// wherever the process happens to be.
func audit(cfg auditConfig) (Report, error) {
	root, err := filepath.Abs(cfg.dir)
	if err != nil {
		return Report{}, err
	}
	loaded, err := goprogram.LoadWith(cfg.dir, cfg.patterns, goprogram.Options{Overlay: cfg.overlay})
	if err != nil {
		return Report{}, err
	}
	found := newScanner(root, cfg.overlay)
	found.observe(loaded)
	return buildReport(found, cfg.declared), nil
}
