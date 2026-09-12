package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// toolName prefixes every line this command writes about itself, so a failure
// in a suite of gates names the gate that failed.
const toolName = "audit_md_cards"

// exit is os.Exit behind a variable, so the one line main carries is reachable
// from a test rather than only from a process.
var exit = os.Exit

// auditRun is one configured run: what to write, and what to fail on.
type auditRun struct {
	jsonPath         string
	rules            string
	check            bool
	census           bool
	verbose          bool
	failUnrenderable bool
}

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses the command line and performs the sweep.
//
// It returns 0 when the run is clean, 1 when -check found something, and 2
// when the audit itself could not do its job, which is the split its sibling
// audits use: a gate that cannot run must not read as a gate that passed.
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(errOut)
	cfg := auditRun{}
	flags.StringVar(&cfg.jsonPath, "json", "", "write the JSON work list to this path")
	flags.StringVar(&cfg.rules, "rules", allRules,
		"shape rules to judge: "+allRules+", or a comma-separated list of "+ruleNames())
	flags.BoolVar(&cfg.check, "check", false, "exit non-zero when a response mixes card and table shapes")
	flags.BoolVar(&cfg.census, "census", false, "print the card/table census by package")
	flags.BoolVar(&cfg.verbose, "v", false, "list the types the audit could not render as well as the findings")
	flags.BoolVar(&cfg.failUnrenderable, "fail-unrenderable", false, "count a type the audit cannot render as a failure")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	return execute(cfg, out, errOut)
}

// execute performs one configured run, so a test can drive the whole audit
// without going through a command line.
func execute(cfg auditRun, out, errOut io.Writer) int {
	sel, err := parseRules(cfg.rules)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", toolName, err)
		return 2
	}
	report := sel.apply(audit())
	writeReport(out, report, cfg)

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
	return gate(out, report, cfg.failUnrenderable)
}

// gate turns the report into the check-mode verdict.
func gate(out io.Writer, report Report, failUnrenderable bool) int {
	failures := len(report.Findings)
	if failUnrenderable {
		failures += len(report.Unrenderable)
	}
	if failures > 0 {
		fmt.Fprintf(out, "check: FAIL. %d block(s) break %s, %d type(s) unrenderable\n",
			len(report.Findings), report.Rules, len(report.Unrenderable))
		return 1
	}
	fmt.Fprintf(out, "check: PASS. %d output type(s) hold %s (%d unrenderable)\n",
		report.Types, report.Rules, len(report.Unrenderable))
	return 0
}

// writeReport prints the work list.
func writeReport(out io.Writer, report Report, cfg auditRun) {
	fmt.Fprintf(out, "%s: %d output types rendered, %d finding(s)\n\n", toolName, report.Types, len(report.Findings))
	for _, finding := range report.Findings {
		fmt.Fprintf(out, "%s (%s, %s sample) line %d\n  %s\n  %s\n  %s\n\n",
			finding.Type, finding.Package, finding.Sample, finding.Line, finding.Kind, finding.Text, finding.Why)
	}
	// The census is printed only when asked for, and the list of packages
	// writing both shapes goes with it rather than above it.
	//
	// A package that answers one question with a card and another with a table
	// is doing the right thing — a get and a list — and the list read as a work
	// list when it sat beside the findings. What the issue counted was a
	// package rendering one *card* in two shapes, which the rule now makes
	// unrepresentable, so no counter here can carry that meaning again.
	if cfg.census {
		fmt.Fprintln(out, "census (package, cards, tables):")
		for _, entry := range report.Census {
			fmt.Fprintf(out, "  %-40s %4d %4d\n", entry.Package, entry.Cards, entry.Tables)
		}
		fmt.Fprintln(out)
		if len(report.MixedPackages) > 0 {
			fmt.Fprintf(out, "packages that answer with both a card and a table (%d), which a get and a list do:\n",
				len(report.MixedPackages))
			for _, pkg := range report.MixedPackages {
				fmt.Fprintf(out, "  %s\n", pkg)
			}
			fmt.Fprintln(out)
		}
	}
	if cfg.verbose && len(report.Unrenderable) > 0 {
		fmt.Fprintf(out, "unrenderable (%d):\n", len(report.Unrenderable))
		for _, entry := range report.Unrenderable {
			fmt.Fprintf(out, "  %s (%s): %s\n", entry.Type, entry.Sample, entry.Reason)
		}
		fmt.Fprintln(out)
	}
}

// writeJSON writes the work list, so a fixing pass reads the same list the
// gate judged.
func writeJSON(path string, report Report) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}
