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

// target is one operating system and architecture pair a load can be made
// for, which is what a build constraint selects on.
type target struct {
	goos   string
	goarch string
}

// String spells the pair the way the toolchain's own listings do.
func (t target) String() string { return t.goos + "/" + t.goarch }

// buildTargets are the pairs this project is built for, the release's own
// list. A package holding a file for one of them is read again under it,
// since a constant only the Windows half of a package reads is read, and so
// is one only its arm64 half reads: setting the operating system alone would
// keep the host's architecture and leave an `_arm64.go` file out on amd64.
var buildTargets = []target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// auditConfig is one configured run: where to look, what to look at, which
// targets to look again under, and where the progress goes.
type auditConfig struct {
	dir      string
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository. Production
	// passes nil.
	overlay map[string][]byte
	// targets are re-read when the first load left a package's Go files out.
	// A test names none, because a fixture has no build constraints and a
	// cross-platform load of the whole repository is not what it is
	// measuring.
	targets []target
	verbose bool
	out     io.Writer
}

func main() {
	dir := flag.String("dir", ".", "repository root the patterns are resolved against")
	check := flag.Bool("check", false, "exit non-zero when a constant is never read")
	verbose := flag.Bool("v", false, "name the platform loads the run made")
	flag.Parse()

	os.Exit(run(auditConfig{
		dir:      *dir,
		patterns: patternsOrDefault(flag.Args()),
		targets:  buildTargets,
		verbose:  *verbose,
		out:      os.Stdout,
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

// audit loads the tree, once for the target it runs on and once more per
// target whose files that load left out, and holds what was declared against
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
// out, under each named target but the exact pair it is already running on.
//
// Without it the gate would be wrong in the worse of the two directions: a
// constant a Windows-only file reads would be reported dead by a Linux run,
// failing a build over code that is doing its job. The extra loads are cheap
// because the set is measured rather than assumed, and only a package that
// really has a file this target excluded is asked for again. Both halves of
// the pair are set, since a file constrained to an architecture is left out
// by the operating system's own load just as an `_windows.go` file is.
func readOtherPlatforms(cfg auditConfig, found *scanner) error {
	packagePaths := sortedKeys(found.platformPackages)
	if len(packagePaths) == 0 {
		return nil
	}
	host := target{goos: runtime.GOOS, goarch: runtime.GOARCH}
	for _, tgt := range cfg.targets {
		if tgt == host {
			continue
		}
		if cfg.verbose && cfg.out != nil {
			fmt.Fprintf(cfg.out, "%s: re-reading %d package(s) as %s\n", toolName, len(packagePaths), tgt)
		}
		loaded, err := goprogram.LoadWith(cfg.dir, packagePaths, goprogram.Options{
			Tests:   true,
			Overlay: cfg.overlay,
			Env:     append(os.Environ(), "GOOS="+tgt.goos, "GOARCH="+tgt.goarch),
		})
		if err != nil {
			return fmt.Errorf("load as %s: %w", tgt, err)
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
