package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// toolName prefixes every line this command writes about itself, so a failure
// in a suite of gates names the gate that failed.
const toolName = "audit_md_escaping"

// auditPatterns are the packages the audit loads. Every Markdown formatter in
// this project lives under them, as do the escaping helpers themselves.
var auditPatterns = []string{"./internal/..."}

// exit is os.Exit behind a variable, so the one line main carries is reachable
// from a test rather than only from a process.
var exit = os.Exit

// absRoot resolves the repository root, behind a variable for the same reason:
// filepath.Abs fails only when the working directory has been removed under
// the process, and a gate's error path is worth a test even when reaching it
// for real takes a deleted directory.
var absRoot = filepath.Abs

// auditRun is one configured run: where to look, what to judge, and what to
// write.
type auditRun struct {
	dir            string
	jsonPath       string
	contexts       string
	check          bool
	verbose        bool
	failUnresolved bool
	patterns       []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
}

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses the command line and performs the sweep.
//
// It returns 0 when the run is clean, 1 when -check found something, and 2 when
// the audit itself could not do its job, which is the split its sibling audits
// use: a gate that cannot run must not read as a gate that passed.
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(errOut)
	cfg := auditRun{patterns: auditPatterns}
	flags.StringVar(&cfg.dir, "dir", ".", "repository root to audit")
	flags.StringVar(&cfg.jsonPath, "json", "", "write the JSON work list to this path")
	flags.StringVar(&cfg.contexts, "contexts", allContexts,
		"Markdown contexts to judge: "+allContexts+", or a comma-separated list of "+contextNames())
	flags.BoolVar(&cfg.check, "check", false, "exit non-zero when a value still reaches a Markdown construct unescaped")
	flags.BoolVar(&cfg.verbose, "v", false, "list the excused and unresolved values as well as the failing ones")
	flags.BoolVar(&cfg.failUnresolved, "fail-unresolved", false, "count a value the audit cannot follow as a failure")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	// Package patterns after the flags narrow the sweep to part of the tree,
	// which is what makes checking one domain while working on it cheap.
	if patterns := flags.Args(); len(patterns) > 0 {
		cfg.patterns = patterns
	}
	return execute(cfg, out, errOut)
}

// execute performs one configured run, so a test can drive the whole audit
// over a fixture without going through a command line.
func execute(cfg auditRun, out, errOut io.Writer) int {
	sel, err := parseContexts(cfg.contexts)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", toolName, err)
		return 2
	}
	root, err := absRoot(cfg.dir)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", toolName, err)
		return 2
	}
	prog, err := loadProgram(root, cfg.patterns, cfg.overlay)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", toolName, err)
		return 2
	}

	report := audit(prog, sel, root)
	writeReport(out, report, cfg.verbose)

	if cfg.jsonPath != "" {
		if writeErr := writeJSON(cfg.jsonPath, report); writeErr != nil {
			fmt.Fprintf(errOut, "%s: %v\n", toolName, writeErr)
			return 2
		}
		fmt.Fprintf(out, "work list written to %s\n", cfg.jsonPath)
	}
	if !cfg.check {
		return 0
	}
	return gate(out, report, cfg.failUnresolved)
}

// gate turns the report into the check-mode verdict.
//
// A stale directive fails alongside a finding on purpose. An exemption that
// excuses nothing has outlived whatever made its value safe, and leaving it in
// place is how a gate widens without anyone deciding to widen it.
func gate(out io.Writer, report Report, failUnresolved bool) int {
	failures := report.Summary.Findings + report.Summary.Stale
	if failUnresolved {
		failures += report.Summary.Unresolved
	}
	if failures > 0 {
		fmt.Fprintf(out, "check: FAIL. %d value(s) reach a Markdown construct unescaped, %d directive(s) excuse nothing, %d unresolved\n",
			report.Summary.Findings, report.Summary.Stale, report.Summary.Unresolved)
		return 1
	}
	fmt.Fprintf(out, "check: PASS. Every value reaching %s is escaped or declared safe (%d excused by directive, %d unresolved)\n",
		report.Summary.Contexts, report.Summary.Excused, report.Summary.Unresolved)
	return 0
}
