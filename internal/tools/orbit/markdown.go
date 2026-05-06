package orbit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown[StatusOutput](FormatStatusMarkdown)
	toolutil.RegisterMarkdown[SchemaOutput](FormatSchemaMarkdown)
	toolutil.RegisterMarkdown[ToolsOutput](FormatToolsMarkdown)
	toolutil.RegisterMarkdown[QueryOutput](FormatQueryMarkdown)
	toolutil.RegisterMarkdown[GraphStatusOutput](FormatGraphStatusMarkdown)
}

// FormatStatusMarkdown formats Orbit status output for LLM consumption.
func FormatStatusMarkdown(out StatusOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Status\n\n")
	if out.FormattedText != "" {
		fmt.Fprintf(&b, "```text\n%s\n```\n", out.FormattedText)
		toolutil.WriteHints(&b, "Use gitlab_orbit graph_status to inspect indexing status for a namespace or project")
		return b.String()
	}
	if out.Status == "" && len(out.Components) == 0 {
		b.WriteString("No Orbit status data returned.\n")
		return b.String()
	}
	writeKV(&b, "Status", out.Status)
	writeKV(&b, "Version", out.Version)
	writeKV(&b, "Timestamp", out.Timestamp)
	if len(out.Components) > 0 {
		b.WriteString("\n| Component | Status | Replicas |\n")
		b.WriteString("|---|---|---|\n")
		for _, component := range out.Components {
			replicas := ""
			if component.Replicas != nil {
				replicas = fmt.Sprintf("%d/%d", component.Replicas.Ready, component.Replicas.Desired)
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", component.Name, component.Status, replicas)
		}
	}
	toolutil.WriteHints(&b, "Use gitlab_orbit graph_status to inspect indexing status for a namespace or project")
	return b.String()
}

// FormatSchemaMarkdown formats Orbit schema output for LLM consumption.
func FormatSchemaMarkdown(out SchemaOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Schema\n\n")
	writeKV(&b, "Schema version", out.SchemaVersion)
	fmt.Fprintf(&b, "- Domains: %d\n", len(out.Domains))
	fmt.Fprintf(&b, "- Nodes: %d\n", len(out.Nodes))
	fmt.Fprintf(&b, "- Edges: %d\n", len(out.Edges))
	if len(out.Domains) > 0 {
		b.WriteString("\n| Domain | Description | Nodes |\n")
		b.WriteString("|---|---|---|\n")
		for _, domain := range out.Domains {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", domain.Name, domain.Description, strings.Join(domain.NodeNames, ", "))
		}
	}
	toolutil.WriteHints(&b,
		"Use gitlab_orbit tools to inspect the live query/tool manifest",
		"Use gitlab_orbit query after choosing a supported query shape from the manifest")
	return b.String()
}

// FormatToolsMarkdown formats Orbit tool manifest output for LLM consumption.
func FormatToolsMarkdown(out ToolsOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Tools\n\n")
	if len(out.Tools) == 0 {
		b.WriteString("No Orbit tools returned.\n")
		return b.String()
	}
	b.WriteString("| Tool | Description |\n")
	b.WriteString("|---|---|\n")
	for _, tool := range out.Tools {
		fmt.Fprintf(&b, "| `%s` | %s |\n", tool.Name, tool.Description)
	}
	toolutil.WriteHints(&b,
		"Use the returned parameters JSON to build gitlab_orbit query input",
		"Use gitlab_orbit schema to understand node and edge names")
	return b.String()
}

// FormatQueryMarkdown formats Orbit query output for LLM consumption.
func FormatQueryMarkdown(out QueryOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Query Result\n\n")
	writeKV(&b, "Query type", out.QueryType)
	if out.RowCount > 0 {
		fmt.Fprintf(&b, "- Row count: %d\n", out.RowCount)
	}
	if len(out.RawQueryStrings) > 0 {
		b.WriteString("\n### Raw Query Strings\n\n")
		for _, raw := range out.RawQueryStrings {
			fmt.Fprintf(&b, "```text\n%s\n```\n", raw)
		}
	}
	if out.Result != nil {
		b.WriteString("\n### Result\n\n")
		fmt.Fprintf(&b, "```json\n%s\n```\n", prettyAny(out.Result))
	}
	toolutil.WriteHints(&b, "Use gitlab_orbit graph_status if query results look stale or incomplete")
	return b.String()
}

// FormatGraphStatusMarkdown formats Orbit graph indexing output for LLM consumption.
func FormatGraphStatusMarkdown(out GraphStatusOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Graph Status\n\n")
	if out.FormattedText != "" {
		fmt.Fprintf(&b, "```text\n%s\n```\n", out.FormattedText)
		toolutil.WriteHints(&b, "Use gitlab_orbit query after indexing reaches a healthy state")
		return b.String()
	}
	if out.Projects != nil {
		fmt.Fprintf(&b, "- Indexed projects: %d\n", out.Projects.Indexed)
		fmt.Fprintf(&b, "- Total known projects: %d\n", out.Projects.TotalKnown)
	}
	if out.Indexing != nil {
		writeKV(&b, "Indexing state", out.Indexing.State)
		writeKV(&b, "Last started at", out.Indexing.LastStartedAt)
		writeKV(&b, "Last completed at", out.Indexing.LastCompletedAt)
		if out.Indexing.LastDurationMs > 0 {
			fmt.Fprintf(&b, "- Last duration: %d ms\n", out.Indexing.LastDurationMs)
		}
		writeKV(&b, "Last error", out.Indexing.LastError)
	}
	if len(out.Domains) > 0 {
		b.WriteString("\n| Domain | Counts |\n")
		b.WriteString("|---|---|\n")
		for _, domain := range out.Domains {
			var counts []string
			for _, item := range domain.Items {
				counts = append(counts, fmt.Sprintf("%s: %d", item.Name, item.Count))
			}
			fmt.Fprintf(&b, "| %s | %s |\n", domain.Name, strings.Join(counts, ", "))
		}
	}
	toolutil.WriteHints(&b, "Use gitlab_orbit query after indexing reaches a healthy state")
	return b.String()
}

func writeKV(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", key, value)
}

func prettyAny(value any) string {
	buf, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(buf)
}
