// Command audit_dynamic_aliases checks the
// dynamic toolset compatibility alias catalog for governance issues.
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

	findings := dynamic.AuditDefaultActionAliases(catalog)
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
	default:
		for _, finding := range findings {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", finding.Severity, finding.Problem, finding.Alias, finding.Canonical, finding.Source, finding.Message)
		}
		if errorCount > 0 {
			fmt.Fprintf(stderr, "dynamic alias audit failed: %d error(s)\n", errorCount)
			return 1
		}
		fmt.Fprintf(stdout, "dynamic alias audit passed: %d finding(s)\n", len(findings))
	}
	if errorCount > 0 {
		return 1
	}
	return 0
}
