package main

import (
	"fmt"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
)

func main() {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build action catalog: %v\n", err)
		os.Exit(1)
	}
	catalog, err = dynamic.AddStandaloneCatalog(catalog, nil, dynamic.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone dynamic catalog: %v\n", err)
		os.Exit(1)
	}

	findings := dynamic.AuditDefaultActionAliases(catalog)
	errorCount := 0
	for _, finding := range findings {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", finding.Severity, finding.Problem, finding.Alias, finding.Canonical, finding.Message)
		if finding.Severity == "error" {
			errorCount++
		}
	}
	if errorCount > 0 {
		fmt.Fprintf(os.Stderr, "dynamic alias audit failed: %d error(s)\n", errorCount)
		os.Exit(1)
	}
	fmt.Printf("dynamic alias audit passed: %d finding(s)\n", len(findings))
}
