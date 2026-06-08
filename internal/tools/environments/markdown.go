package environments

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type environmentNotFoundOutput struct {
	Identifier string
}

func formatEnvironmentNotFound(out environmentNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Environment", out.Identifier,
		"Use gitlab_environment_list with project_id to list environments",
		"Verify the environment_id is correct for this project",
	)
}

// FormatOutputMarkdown renders a single environment as Markdown.
func FormatOutputMarkdown(e Output) string {
	if e.Name == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Environment: %s\n\n", toolutil.EscapeMdHeading(e.Name))
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| ID | %d |\n", e.ID)
	fmt.Fprintf(&b, "| Slug | %s |\n", toolutil.EscapeMdTableCell(e.Slug))
	fmt.Fprintf(&b, "| State | %s |\n", e.State)
	if e.Tier != "" {
		fmt.Fprintf(&b, "| Tier | %s |\n", toolutil.EscapeMdTableCell(e.Tier))
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(e.Description))
	}
	if e.ExternalURL != "" {
		fmt.Fprintf(&b, "| URL | %s |\n", toolutil.EscapeMdTableCell(e.ExternalURL))
	}
	if e.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(e.CreatedAt))
	}
	if e.UpdatedAt != "" {
		fmt.Fprintf(&b, "| Updated | %s |\n", toolutil.FormatTime(e.UpdatedAt))
	}
	if e.AutoStopAt != "" {
		fmt.Fprintf(&b, "| Auto-Stop At | %s |\n", toolutil.FormatTime(e.AutoStopAt))
	}
	toolutil.WriteHints(
		&b,
		"Use action 'stop' to stop this environment",
		"Use gitlab_deployment action 'list' with environment to see deployments",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of environments as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return "No environments found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Environments (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Environments), out.Pagination)
	b.WriteString("| ID | Name | State | Tier | External URL |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, e := range out.Environments {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			e.ID, toolutil.EscapeMdTableCell(e.Name), e.State, e.Tier, toolutil.EscapeMdTableCell(e.ExternalURL))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'get' with an environment_id to see details",
		"Use action 'create' to add a new environment",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatEnvironmentNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
