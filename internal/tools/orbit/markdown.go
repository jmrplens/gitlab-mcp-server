package orbit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// orbitNotFoundOutput is returned by MCP Orbit tools when the requested resource or feature is not found (HTTP 404).
// Used to provide actionable hints for missing Orbit endpoints or disabled features.
type orbitNotFoundOutput struct {
	Resource   string
	Identifier string
}

// init registers all Markdown formatters for Orbit MCP tool outputs.
//
// Each formatter converts the tool output struct to a Markdown summary for LLM and user-facing documentation.
func init() {
	toolutil.RegisterMarkdownResult(formatOrbitNotFound)
	toolutil.RegisterMarkdown[StatusOutput](FormatStatusMarkdown)
	toolutil.RegisterMarkdown[SchemaOutput](FormatSchemaMarkdown)
	toolutil.RegisterMarkdown[ToolsOutput](FormatToolsMarkdown)
	toolutil.RegisterMarkdown[DSLOutput](FormatDSLMarkdown)
	toolutil.RegisterMarkdown[QueryOutput](FormatQueryMarkdown)
	toolutil.RegisterMarkdown[GraphStatusOutput](FormatGraphStatusMarkdown)
}

// formatOrbitNotFound returns a [*mcp.CallToolResult] with actionable hints when an Orbit resource is not found.
// Used by all Orbit MCP tool handlers to provide LLM-friendly error output for HTTP 404.
func formatOrbitNotFound(out orbitNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		out.Resource, out.Identifier,
		"Verify GitLab Orbit is enabled on GitLab.com for the requested token",
		"Check that the token can access a Knowledge Graph-enabled namespace or project",
	)
}

// FormatStatusMarkdown returns a Markdown-formatted summary of Orbit cluster health for LLM consumption.
//
// Includes status, version, timestamp, and a table of subsystem components with replica counts.
// If FormattedText is present, it is rendered as a fenced code block.
func FormatStatusMarkdown(out StatusOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Status\n\n")
	if out.FormattedText != "" {
		b.WriteString(fencedBlock("text", out.FormattedText))
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

// FormatSchemaMarkdown returns a Markdown-formatted summary of the Orbit Knowledge Graph schema.
//
// Includes schema version, domain table, and counts of nodes and edges. Used for LLM and user-facing docs.
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
			fmt.Fprintf(
				&b, "| %s | %s | %s |\n",
				toolutil.EscapeMdTableCell(domain.Name),
				toolutil.EscapeMdTableCell(domain.Description),
				toolutil.EscapeMdTableCell(strings.Join(domain.NodeNames, ", ")),
			)
		}
	}
	toolutil.WriteHints(&b,
		"Use gitlab_orbit tools to inspect the live query/tool manifest",
		"Use gitlab_orbit query after choosing a supported query shape from the manifest")
	return b.String()
}

// FormatToolsMarkdown returns a Markdown-formatted table of Orbit MCP tool definitions.
//
// Each row shows the tool name and description. Used for LLM tool discovery and user docs.
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
		safeName := strings.ReplaceAll(tool.Name, "`", "")
		fmt.Fprintf(
			&b, "| `%s` | %s |\n",
			toolutil.EscapeMdTableCell(safeName),
			toolutil.EscapeMdTableCell(tool.Description),
		)
	}
	toolutil.WriteHints(&b,
		"Use the returned parameters JSON to build gitlab_orbit query input",
		"Use gitlab_orbit schema to understand node and edge names")
	return b.String()
}

// FormatDSLMarkdown returns a Markdown-formatted fenced code block with the Orbit query DSL.
//
// Used to display the DSL grammar or schema for LLMs and users.
func FormatDSLMarkdown(out DSLOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit DSL\n\n")
	if out.Content == "" {
		b.WriteString("No Orbit DSL data returned.\n")
		return b.String()
	}
	language := "json"
	if strings.EqualFold(out.ResponseFormat, string(gl.OrbitResponseFormatLLM)) {
		language = "text"
	}
	b.WriteString(fencedBlock(language, out.Content))
	toolutil.WriteHints(&b,
		"Use gitlab_orbit query after choosing a supported query shape from the DSL",
		"Use gitlab_orbit schema to understand node and edge names")
	return b.String()
}

// FormatQueryMarkdown returns a Markdown-formatted summary of an Orbit query result.
//
// If FormattedText is present, it is rendered as a fenced code block. Otherwise, the result is shown as JSON.
func FormatQueryMarkdown(out QueryOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Query Result\n\n")
	if out.FormattedText != "" {
		b.WriteString(fencedBlock("text", out.FormattedText))
		toolutil.WriteHints(&b, "Use gitlab_orbit graph_status if query results look stale or incomplete")
		return b.String()
	}
	writeKV(&b, "Query type", out.QueryType)
	if out.RowCount > 0 {
		fmt.Fprintf(&b, "- Row count: %d\n", out.RowCount)
	}
	if len(out.RawQueryStrings) > 0 {
		b.WriteString("\n### Raw Query Strings\n\n")
		for _, raw := range out.RawQueryStrings {
			b.WriteString(fencedBlock("text", raw))
		}
	}
	if out.Result != nil {
		b.WriteString("\n### Result\n\n")
		b.WriteString(fencedBlock("json", prettyAny(out.Result)))
	}
	toolutil.WriteHints(&b, "Use gitlab_orbit graph_status if query results look stale or incomplete")
	return b.String()
}

// FormatGraphStatusMarkdown returns a Markdown-formatted summary of Orbit graph indexing status.
//
// Includes indexed project counts, domain node counts, and indexing pipeline state. Used for LLM and user docs.
func FormatGraphStatusMarkdown(out GraphStatusOutput) string {
	var b strings.Builder
	b.WriteString("## Orbit Graph Status\n\n")
	if out.FormattedText != "" {
		b.WriteString(fencedBlock("text", out.FormattedText))
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
			fmt.Fprintf(
				&b, "| %s | %s |\n",
				toolutil.EscapeMdTableCell(domain.Name),
				toolutil.EscapeMdTableCell(strings.Join(counts, ", ")),
			)
		}
	}
	toolutil.WriteHints(&b, "Use gitlab_orbit query after indexing reaches a healthy state")
	return b.String()
}

// writeKV writes a Markdown bullet list item for a key-value pair, skipping empty values.
// Used by all Orbit Markdown formatters for summary fields.
func writeKV(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", key, value)
}

// prettyAny returns a pretty-printed JSON string for any value, or falls back to fmt.Sprint on error.
// Used to render Orbit query results in Markdown.
func prettyAny(value any) string {
	buf, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(buf)
}

// fencedBlock returns a Markdown fenced code block for the given language and content.
// The fence length is auto-detected to avoid conflicts with backticks in the content.
func fencedBlock(language, content string) string {
	fence := markdownFence(content)
	if language != "" {
		return fmt.Sprintf("%s%s\n%s\n%s\n", fence, language, content, fence)
	}
	return fmt.Sprintf("%s\n%s\n%s\n", fence, content, fence)
}

// markdownFence returns the appropriate Markdown code fence for a content block.
// If the content contains 3 or more consecutive backticks, the fence is lengthened to avoid collision.
func markdownFence(content string) string {
	longest := 0
	current := 0
	for _, char := range content {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}
