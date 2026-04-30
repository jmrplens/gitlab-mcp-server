// markdown.go provides Markdown formatting functions for deployment MCP tool output.
package deployments

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single deployment as Markdown.
func FormatOutputMarkdown(d Output) string {
	if d.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Deployment #%d\n\n", d.ID)
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| IID | %d |\n", d.IID)
	fmt.Fprintf(&b, "| Ref | %s |\n", toolutil.EscapeMdTableCell(d.Ref))
	fmt.Fprintf(&b, "| SHA | %s |\n", d.SHA)
	fmt.Fprintf(&b, "| Status | %s |\n", d.Status)
	if d.UserName != "" {
		fmt.Fprintf(&b, "| User | %s |\n", toolutil.EscapeMdTableCell(d.UserName))
	}
	if d.EnvironmentName != "" {
		fmt.Fprintf(&b, "| Environment | %s |\n", toolutil.EscapeMdTableCell(d.EnvironmentName))
	}
	if d.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(d.CreatedAt))
	}
	if d.UpdatedAt != "" {
		fmt.Fprintf(&b, "| Updated | %s |\n", toolutil.FormatTime(d.UpdatedAt))
	}
	toolutil.WriteHints(&b,
		"Use gitlab_environment action 'list' to see all environments",
		"Use gitlab_merge_request to see related merge request details",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of deployments as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Deployments) == 0 {
		return "No deployments found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Deployments (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Deployments), out.Pagination)
	b.WriteString("| ID | IID | Ref | Status | Environment | User |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, d := range out.Deployments {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %s |\n",
			d.ID, d.IID, toolutil.EscapeMdTableCell(d.Ref), d.Status, toolutil.EscapeMdTableCell(d.EnvironmentName), toolutil.EscapeMdTableCell(d.UserName))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with a deployment_id to see details",
		"Use gitlab_environment action 'list' to see all environments",
	)
	return b.String()
}

// FormatApproveOrRejectMarkdown renders the approve/reject result as Markdown.
func FormatApproveOrRejectMarkdown(o ApproveOrRejectOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", toolutil.EmojiSuccess, o.Message)
	toolutil.WriteHints(&b, "Use action 'list' to see all deployments for this environment")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatApproveOrRejectMarkdown)
}
