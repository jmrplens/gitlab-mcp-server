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
const toolName = "audit_test_closed_servers"

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
	dir := flags.String("dir", ".", "module root the patterns are resolved against")
	check := flags.Bool("check", false, "exit non-zero when an httptest server is closed while its address is still used")
	verbose := flags.Bool("v", false, "also list the closes a declaration excuses")
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
		tags:     analysisTags,
		declared: usedAfterTheCloseOnPurpose,
	}, *check, *verbose, stdout, stderr)
}

// run scans, reports, and returns the process exit code.
//
// Exit 1 covers two different things on purpose, told apart by which stream
// spoke: a run that could not be made at all says so on stderr, and a run that
// found something says it on stdout and only fails under -check.
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

// audit loads the tree with its tests under the configured tags and judges
// every function in it.
//
// The root is made absolute before anything is loaded, so a run whose working
// directory is gone stops on that error instead of naming files against
// wherever the process happens to be.
func audit(cfg auditConfig) (Report, error) {
	root, err := filepath.Abs(cfg.dir)
	if err != nil {
		return Report{}, err
	}
	loaded, err := goprogram.LoadWith(cfg.dir, cfg.patterns, goprogram.Options{
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
