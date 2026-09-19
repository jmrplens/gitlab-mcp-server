package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// toolName is how the report names itself.
const toolName = "audit_dead_consts"

// defaultPatterns is this repository's own Go source, minus the end-to-end
// packages, which sit behind build tags of their own and are loaded by the
// gate that owns them.
var defaultPatterns = []string{"./internal/...", "./cmd/..."}

// buildPlatforms are the operating systems this project is built for. A
// package holding a file for one of them is read again under it, since a
// constant only the Windows half of a package reads is read.
var buildPlatforms = []string{"linux", "darwin", "windows"}

// auditConfig is one configured run: where to look, what to look at, which
// platforms to look again under, and where the progress goes.
type auditConfig struct {
	dir      string
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository. Production
	// passes nil.
	overlay map[string][]byte
	// platforms are re-read when the first load left a package's Go files
	// out. A test names none, because a fixture has no build constraints and
	// a cross-platform load of the whole repository is not what it is
	// measuring.
	platforms []string
	verbose   bool
	out       io.Writer
}

func main() {
	dir := flag.String("dir", ".", "repository root the patterns are resolved against")
	check := flag.Bool("check", false, "exit non-zero when a constant is never read")
	verbose := flag.Bool("v", false, "name the platform loads the run made")
	flag.Parse()

	os.Exit(run(auditConfig{
		dir:       *dir,
		patterns:  patternsOrDefault(flag.Args()),
		platforms: buildPlatforms,
		verbose:   *verbose,
		out:       os.Stdout,
	}, *check, os.Stdout, os.Stderr))
}

// run scans, reports, and returns the process exit code.
//
// Exit 1 covers two different things on purpose, told apart by which stream
// spoke: a run that could not be made at all says so on stderr, and a run that
// found something says it on stdout and only fails under -check. A report is
// worth having without a gate, and a gate is worth having without ceremony.
func run(cfg auditConfig, check bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	report.write(stdout, cfg.verbose)
	if check && !report.ok() {
		return 1
	}
	return 0
}

// audit loads the tree, once for the platform it runs on and once more per
// platform whose files that load left out, and holds what was declared against
// what was read.
func audit(cfg auditConfig) (Report, error) {
	root, err := filepath.Abs(cfg.dir)
	if err != nil {
		return Report{}, err
	}
	found := newScanner(root)
	loaded, err := goprogram.LoadWith(cfg.dir, cfg.patterns, goprogram.Options{Tests: true, Overlay: cfg.overlay})
	if err != nil {
		return Report{}, err
	}
	found.observe(loaded)
	if platformErr := readOtherPlatforms(cfg, found); platformErr != nil {
		return Report{}, platformErr
	}
	return buildReport(found.dead(), len(found.declared), found.packages), nil
}

// readOtherPlatforms re-reads the packages whose Go files the first load left
// out, under each named platform it is not already running on.
//
// Without it the gate would be wrong in the worse of the two directions: a
// constant a Windows-only file reads would be reported dead by a Linux run,
// failing a build over code that is doing its job. The extra loads are cheap
// because the set is measured rather than assumed, and only a package that
// really has a file this platform excluded is asked for again.
func readOtherPlatforms(cfg auditConfig, found *scanner) error {
	packagePaths := sortedKeys(found.platformPackages)
	if len(packagePaths) == 0 {
		return nil
	}
	for _, platform := range cfg.platforms {
		if platform == runtime.GOOS {
			continue
		}
		if cfg.verbose && cfg.out != nil {
			fmt.Fprintf(cfg.out, "%s: re-reading %d package(s) as %s\n", toolName, len(packagePaths), platform)
		}
		loaded, err := goprogram.LoadWith(cfg.dir, packagePaths, goprogram.Options{
			Tests:   true,
			Overlay: cfg.overlay,
			Env:     append(os.Environ(), "GOOS="+platform),
		})
		if err != nil {
			return fmt.Errorf("load as %s: %w", platform, err)
		}
		found.observe(loaded)
	}
	return nil
}

// patternsOrDefault is what a run with no arguments audits.
func patternsOrDefault(patterns []string) []string {
	if len(patterns) == 0 {
		return defaultPatterns
	}
	return patterns
}

// sortedKeys renders a set in a stable order, so two runs load the same
// packages in the same order and a verbose report reads the same twice.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
