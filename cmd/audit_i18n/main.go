// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// toolName is how the report names itself.
const toolName = "audit_i18n"

// usage is the help text of the gate's flags.
var usage = sourcegate.Usage{
	Dir:     "repository root the patterns are resolved against",
	Check:   "exit non-zero when a person-facing text bypasses the message catalog or a catalog entry is dead",
	Verbose: "also list the key conversions a declaration excuses",
}

// defaultPatterns is this repository's own library source, the server and
// every command, without the end-to-end packages under test/, whose library
// code runs in a test process rather than inside a handler and serves nobody.
var defaultPatterns = []string{"./internal/...", "./cmd/..."}

// catalogPath is the import path of the message catalog.
//
// It is written out whole rather than joined onto goprogram.ModulePath,
// because the mutation tool cannot see a package-level declaration and reads
// the plus of a joined one as never covered.
// TestCatalogPath_UnderTheModulePath_MovesWithIt holds it to the module path.
const catalogPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"

// auditConfig is one configured run: where to look, what to look at, which
// package is the catalog, and the declarations it is held to.
type auditConfig struct {
	dir      string
	patterns []string
	// catalog is the import path of the catalog package, which a test points
	// at the stand-in of a planted module.
	catalog string
	// declared is the declaration table, which a test replaces so an excused
	// conversion and a stale entry can be exercised without editing the real
	// one.
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
			catalog:  catalogPath,
			declared: conversionsOnPurpose,
		}, cmd.Check, cmd.Verbose, stdout, stderr)
	})
}

// run audits, reports, and returns the process exit code [sourcegate.Finish]
// decides: a run that could not be made fails on stderr, and one that found
// something says so on stdout and fails only under -check.
func run(cfg auditConfig, check, verbose bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	return sourcegate.Finish(toolName, report, err, check, verbose, stdout, stderr)
}

// audit loads the tree without its test files, with the catalog package
// beside whatever the patterns name, reads the catalog out of its source and
// judges every package against it.
//
// The catalog is loaded whatever the patterns are, since every rule is a
// question about it: a run narrowed to one package still has to know which
// keys exist and what their templates hold. A load that cannot produce it
// fails, as any pattern that names nothing does. Whether a key is dead is
// asked only of a run over the default patterns, since over part of the tree
// a key the rest of it renders would read as one nothing does.
func audit(cfg auditConfig) (Report, error) {
	patterns := append(slices.Clone(cfg.patterns), cfg.catalog)
	root, loaded, err := goprogram.LoadRooted(cfg.dir, patterns, goprogram.Options{})
	if err != nil {
		return Report{}, err
	}
	cat, err := readCatalog(root, loaded, cfg.catalog)
	if err != nil {
		return Report{}, err
	}
	found := newScanner(root, cat, slices.Equal(cfg.patterns, defaultPatterns))
	found.observe(loaded)
	return buildReport(found, cfg.declared), nil
}
