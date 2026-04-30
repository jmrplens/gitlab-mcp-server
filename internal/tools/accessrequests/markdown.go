// markdown.go provides Markdown formatting functions for access request MCP tool output.
package accessrequests

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single access request as markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Access Request #%d\n\n", out.ID)
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Username | %s |\n", out.Username)
	fmt.Fprintf(&b, "| Name | %s |\n", out.Name)
	fmt.Fprintf(&b, "| State | %s |\n", out.State)
	fmt.Fprintf(&b, "| Access Level | %d |\n", out.AccessLevel)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.RequestedAt != "" {
		fmt.Fprintf(&b, "| Requested At | %s |\n", toolutil.FormatTime(out.RequestedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'approve' to approve this access request",
		"Use action 'deny_project' to deny a project access request",
		"Use action 'deny_group' to deny a group access request",
	)
	return b.String()
}

// FormatListMarkdown formats a list of access requests as markdown.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Access Requests (%d)\n\n", len(out.AccessRequests))
	toolutil.WriteListSummary(&b, len(out.AccessRequests), out.Pagination)
	if len(out.AccessRequests) == 0 {
		b.WriteString("No access requests found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString("| ID | Username | Name | State | Access Level |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, ar := range out.AccessRequests {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d |\n",
			ar.ID, ar.Username, ar.Name, ar.State, ar.AccessLevel)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'approve', 'deny_project', or 'deny_group' with request ID to manage requests",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
