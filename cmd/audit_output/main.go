// Command audit_output generates a markdown report of MCP tool output quality.
// It creates in-memory MCP servers with all tools registered (individual + meta),
// inspects descriptions for "Returns:" info, OutputSchema presence, Title field,
// and content annotation readiness.
//
// Usage:
//
//	go run ./cmd/audit_output/
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// finding describes one output-format audit finding for a registered MCP tool.
type finding struct {
	tool     string
	category string
	detail   string
}

type toolQualityStats struct {
	Schema  int
	Returns int
	Title   int
	SeeAlso int
}

// main runs the MCP tool output audit and prints a report.
func main() {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	individual := listTools(client, false)
	meta := listTools(client, true)

	findings := make([]finding, 0, len(individual)+len(meta))
	findings = append(findings, auditOutputSchema(individual, "individual")...)
	findings = append(findings, auditOutputSchema(meta, "meta")...)
	findings = append(findings, auditDescriptionReturns(individual, "individual")...)
	findings = append(findings, auditDescriptionReturns(meta, "meta")...)
	findings = append(findings, auditTitle(individual, "individual")...)
	findings = append(findings, auditTitle(meta, "meta")...)
	findings = append(findings, auditSeeAlso(individual, "individual")...)
	findings = append(findings, auditRouteOutputSchema(client)...)

	printReport(individual, meta, findings)
}

// listTools returns all registered MCP tools by starting an in-memory server.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, nil)
	if meta {
		if err := tools.RegisterAllMeta(server, client, true); err != nil {
			fmt.Fprintf(os.Stderr, "register meta tools: %v\n", err)
			os.Exit(1)
		}
	} else {
		tools.RegisterAll(server, client, true)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTools: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}
	return result.Tools
}

// auditOutputSchema checks whether a tool declares an OutputSchema.
func auditOutputSchema(tls []*mcp.Tool, kind string) []finding {
	var fs []finding
	for _, t := range tls {
		if t.OutputSchema == nil {
			fs = append(fs, finding{t.Name, "output-schema", kind + " tool missing OutputSchema"})
		}
	}
	return fs
}

// auditDescriptionReturns checks whether a tool description contains a Returns section.
func auditDescriptionReturns(tls []*mcp.Tool, kind string) []finding {
	var fs []finding
	lower := strings.ToLower
	for _, t := range tls {
		desc := lower(t.Description)
		hasReturns := strings.Contains(desc, "returns") || strings.Contains(desc, "returns:")
		if !hasReturns {
			fs = append(fs, finding{
				t.Name, "description-returns",
				fmt.Sprintf("%s description lacks 'Returns:' info (%d chars)", kind, len(t.Description)),
			})
		}
	}
	return fs
}

// auditTitle checks whether a tool declares a non-empty Title field.
func auditTitle(tls []*mcp.Tool, kind string) []finding {
	var fs []finding
	for _, t := range tls {
		if t.Title == "" {
			fs = append(fs, finding{t.Name, "title", kind + " tool missing Title field"})
		}
	}
	return fs
}

// auditSeeAlso checks whether a tool description contains a See also section.
func auditSeeAlso(tls []*mcp.Tool, kind string) []finding {
	var fs []finding
	for _, t := range tls {
		if !strings.Contains(strings.ToLower(t.Description), "see also:") {
			fs = append(fs, finding{
				t.Name, "see-also",
				kind + " description lacks 'See also:' cross-references",
			})
		}
	}
	return fs
}

// auditRouteOutputSchema checks meta-tool routes for missing OutputSchema.
// Routes without OutputSchema are reported (these are typically void actions
// or plain Route() calls that lack typed output).
func auditRouteOutputSchema(client *gitlabclient.Client) []finding {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true})
	if err != nil {
		return []finding{{"gitlab_meta", "route-output-schema", fmt.Sprintf("failed to build action catalog: %v", err)}}
	}
	return collectRouteOutputSchemaFindings(catalog.ActionMaps())
}

// collectRouteOutputSchemaFindings scans the given meta-tool route map and
// returns a finding for every action that does not declare an OutputSchema.
func collectRouteOutputSchemaFindings(allRoutes map[string]toolutil.ActionMap) []finding {
	var fs []finding
	for toolName, routes := range allRoutes {
		for action, route := range routes {
			if route.OutputSchema == nil {
				fs = append(fs, finding{
					tool:     toolName,
					category: "route-output-schema",
					detail:   fmt.Sprintf("action %q has no OutputSchema (void or untyped)", action),
				})
			}
		}
	}
	return fs
}

// printReport prints the audit results as a formatted table.
func printReport(individual, meta []*mcp.Tool, fs []finding) {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("# MCP Output Quality Audit Report\n\n")
	fmt.Printf("Generated: %s\n\n", now)

	individualStats := collectToolQualityStats(individual)
	metaStats := collectToolQualityStats(meta)

	fmt.Printf("## Summary\n\n")
	fmt.Printf("| Metric | Individual | Meta |\n")
	fmt.Printf("| --- | --- | --- |\n")
	fmt.Printf("| Total tools | %d | %d |\n", len(individual), len(meta))
	fmt.Printf("| OutputSchema present | %d/%d (%d%%) | %d/%d (%d%%) |\n",
		individualStats.Schema, len(individual), pct(individualStats.Schema, len(individual)),
		metaStats.Schema, len(meta), pct(metaStats.Schema, len(meta)))
	fmt.Printf("| Description has 'Returns' | %d/%d (%d%%) | %d/%d (%d%%) |\n",
		individualStats.Returns, len(individual), pct(individualStats.Returns, len(individual)),
		metaStats.Returns, len(meta), pct(metaStats.Returns, len(meta)))
	fmt.Printf("| Title field set | %d/%d (%d%%) | %d/%d (%d%%) |\n",
		individualStats.Title, len(individual), pct(individualStats.Title, len(individual)),
		metaStats.Title, len(meta), pct(metaStats.Title, len(meta)))
	fmt.Printf("| Description has 'See also' | %d/%d (%d%%) | — |\n\n",
		individualStats.SeeAlso, len(individual), pct(individualStats.SeeAlso, len(individual)))

	fmt.Printf("| Finding | Count |\n")
	fmt.Printf("| --- | --- |\n")
	cats := map[string]int{}
	for _, f := range fs {
		cats[f.category]++
	}
	for cat, n := range cats {
		fmt.Printf("| %s | %d |\n", cat, n)
	}
	fmt.Printf("| **Total findings** | **%d** |\n\n", len(fs))

	if len(fs) == 0 {
		fmt.Println("**No findings — all quality checks pass.**")
		return
	}

	// Group by category
	grouped := map[string][]finding{}
	for _, f := range fs {
		grouped[f.category] = append(grouped[f.category], f)
	}

	for cat, cfs := range grouped {
		fmt.Printf("## %s (%d)\n\n", cat, len(cfs))
		fmt.Printf("| Tool | Detail |\n")
		fmt.Printf("| --- | --- |\n")
		for _, f := range cfs {
			fmt.Printf("| `%s` | %s |\n", f.tool, f.detail)
		}
		fmt.Println()
	}
}

func collectToolQualityStats(toolList []*mcp.Tool) toolQualityStats {
	var stats toolQualityStats
	for _, tool := range toolList {
		if tool.OutputSchema != nil {
			stats.Schema++
		}
		description := strings.ToLower(tool.Description)
		if strings.Contains(description, "returns") {
			stats.Returns++
		}
		if strings.Contains(description, "see also:") {
			stats.SeeAlso++
		}
		if tool.Title != "" {
			stats.Title++
		}
	}
	return stats
}

// pct calculates a percentage from count and total.
func pct(n, total int) int {
	if total == 0 {
		return 0
	}
	return n * 100 / total
}
