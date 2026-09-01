// Output-quality audit functions (formerly audit_output).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// finding describes one output-format audit finding for a registered MCP tool.
//
// tool is the MCP tool name that triggered the finding. category is the
// rule family used to group findings in the report (for example
// "output-schema" or "description-returns"). detail is the human-readable
// explanation rendered verbatim into the Markdown table.
type finding struct {
	tool     string
	category string
	detail   string
}

// toolQualityStats counts how many tools in a population satisfy each
// quality dimension checked by [collectToolQualityStats].
//
// Schema counts tools with a non-nil OutputSchema. Returns counts tools
// whose description mentions "returns". Title counts tools with a non-empty
// MCP Title field. SeeAlso counts tools whose description cross-references
// related tools via "see also:".
type toolQualityStats struct {
	Schema  int
	Returns int
	Title   int
	SeeAlso int
}

// runOutputAudit runs the output-quality checks (OutputSchema presence,
// "Returns:" info, Title field, "See also:" cross-refs, route OutputSchema)
// and prints the report to stdout.
func runOutputAudit(client *gitlabclient.Client) {
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

	printOutputReport(individual, meta, findings)
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

// printOutputReport prints the audit results as a formatted table.
func printOutputReport(individual, meta []*mcp.Tool, fs []finding) {
	if outputJSON {
		entries := make([]jsonEntry, len(fs))
		for i, f := range fs {
			entries[i] = jsonEntry{f.tool, f.category, f.detail}
		}
		report := struct {
			View            string      `json:"view"`
			IndividualTools int         `json:"individual_tools"`
			MetaTools       int         `json:"meta_tools"`
			Findings        int         `json:"findings"`
			Entries         []jsonEntry `json:"entries"`
		}{"output", len(individual), len(meta), len(fs), entries}
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		}
		return
	}
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
	fmt.Printf("| Description has 'See also' | %d/%d (%d%%) |. |\n\n",
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
		fmt.Println("**No findings. All quality checks pass.**")
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
