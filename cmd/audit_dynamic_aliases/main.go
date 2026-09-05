package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
)

// main audits default dynamic action aliases against canonical catalog routes.
// It builds the action catalog, adds standalone dynamic routes, then runs
// dynamic.AuditDefaultActionAliases to emit one TSV line per finding as:
// Severity, Problem, Alias, Canonical, Message. Findings with Severity="error"
// fail the command; warnings and informational findings are printed for review.
func main() {
	format := flag.String("output", "tsv", "output format: tsv or json")
	flag.Parse()
	os.Exit(run(os.Stdout, os.Stderr, *format))
}

// run is the testable entry point that builds the catalog, adds standalone
// dynamic routes, runs [dynamic.AuditDefaultActionAliases], and writes
// findings to stdout in the requested format. Returns a process-style exit
// code: 0 when no error-severity findings remain, 1 otherwise.
func run(stdout, stderr io.Writer, format string) int {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(stderr, "build action catalog: %v\n", err)
		return 1
	}
	catalog, err = dynamic.AddStandaloneCatalog(catalog, nil, dynamic.StandaloneOptions{})
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
