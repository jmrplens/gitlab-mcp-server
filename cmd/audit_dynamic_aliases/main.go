package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// toolName is the command's own name, used as the flag set's name so a usage
// message names the command rather than the test binary that drove it.
const toolName = "audit_dynamic_aliases"

// osExit is os.Exit behind a variable, so the one line main carries is
// reachable from a test rather than only from a process.
var osExit = os.Exit

// Catalog construction hooks, replaceable in tests.
//
// Neither can be made to fail from anything this command accepts: [run] passes
// no client and a fixed set of options, and the action specs both read are
// compiled in and validated by their own tests. The branches that report the
// failure exist for the day one of those facts changes, and would otherwise
// never run.
var (
	buildActionCatalog   = tools.BuildActionCatalog
	addStandaloneCatalog = dynamic.AddStandaloneCatalog
)

// main audits default dynamic action aliases against canonical catalog routes.
// It builds the action catalog, adds standalone dynamic routes, then runs
// dynamic.AuditDefaultActionAliases to emit one TSV line per finding as:
// Severity, Problem, Alias, Canonical, Message. Findings with Severity="error"
// fail the command; warnings and informational findings are printed for review.
func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses args, the command line with the program name already removed,
// and returns the process exit code.
//
// The flag set is ContinueOnError rather than the package-level ExitOnError
// one, so a bad flag is an exit code this function returns instead of an
// os.Exit the seam above never sees. flag has already printed the error and
// the usage by the time Parse returns; -h is the one failure that exits clean,
// as ExitOnError would.
func runMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("output", "tsv", "output format: tsv or json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	return run(stdout, stderr, *format)
}

// run is the testable entry point that builds the catalog, adds standalone
// dynamic routes, runs [dynamic.AuditDefaultActionAliases], and writes
// findings to stdout in the requested format. Returns a process-style exit
// code: 0 when no error-severity findings remain, 1 otherwise.
func run(stdout, stderr io.Writer, format string) int {
	catalog, err := buildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(stderr, "build action catalog: %v\n", err)
		return 1
	}
	catalog, err = addStandaloneCatalog(catalog, nil, dynamic.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "add standalone dynamic catalog: %v\n", err)
		return 1
	}

	return writeFindings(stdout, stderr, dynamic.AuditDefaultActionAliases(catalog), format)
}

// writeFindings writes the audit findings to stdout in the requested format
// and returns the process exit code: 0 when no error-severity finding is
// present, 1 when one is (or the JSON encoding fails), and 2 for an unknown
// format.
func writeFindings(stdout, stderr io.Writer, findings []dynamic.AliasAuditFinding, format string) int {
	errorCount := 0
	for _, finding := range findings {
		if finding.Severity == "error" {
			errorCount++
		}
	}

	switch format {
	case "json":
		if encErr := json.NewEncoder(stdout).Encode(findings); encErr != nil {
			fmt.Fprintf(stderr, "encode json: %v\n", encErr)
			return 1
		}
	case "tsv":
		for _, finding := range findings {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", finding.Severity, finding.Problem, finding.Alias, finding.Canonical, finding.Source, finding.Message)
		}
		if errorCount > 0 {
			fmt.Fprintf(stderr, "dynamic alias audit failed: %d error(s)\n", errorCount)
			return 1
		}
		fmt.Fprintf(stdout, "dynamic alias audit passed: %d finding(s)\n", len(findings))
	default:
		fmt.Fprintf(stderr, "invalid -output %q (want tsv or json)\n", format)
		return 2
	}
	if errorCount > 0 {
		return 1
	}
	return 0
}
