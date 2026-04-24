// markdown.go provides Markdown formatting functions for group protected
// environment MCP tool output.

package groupprotectedenvs

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group protected environment as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Environment: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, "- **Required Approval Count**: %d\n", out.RequiredApprovalCount)
	if len(out.DeployAccessLevels) > 0 {
		b.WriteString("\n### Deploy Access Levels\n\n")
		b.WriteString("| ID | Level | Description |\n| --- | --- | --- |\n")
		for _, l := range out.DeployAccessLevels {
			fmt.Fprintf(&b, "| %d | %d | %s |\n", l.ID, l.AccessLevel, l.AccessLevelDescription)
		}
	}
	if len(out.ApprovalRules) > 0 {
		b.WriteString("\n### Approval Rules\n\n")
		b.WriteString("| ID | Level | Description | Required |\n| --- | --- | --- | --- |\n")
		for _, r := range out.ApprovalRules {
			fmt.Fprintf(&b, "| %d | %d | %s | %d |\n", r.ID, r.AccessLevel, r.AccessLevelDescription, r.RequiredApprovalCount)
		}
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_protected_environment_update to modify settings",
		"Use gitlab_group_protected_environment_unprotect to remove protection",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of group protected environments as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return "No group protected environments found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Environments), out.Pagination)
	b.WriteString("| Name | Approval Count | Deploy Levels | Rules |\n| --- | --- | --- | --- |\n")
	for _, e := range out.Environments {
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
			toolutil.EscapeMdTableCell(e.Name),
			e.RequiredApprovalCount,
			len(e.DeployAccessLevels),
			len(e.ApprovalRules),
		)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_protected_environment_get with an environment name for details",
		"Use gitlab_group_protected_environment_protect to add new protection",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
