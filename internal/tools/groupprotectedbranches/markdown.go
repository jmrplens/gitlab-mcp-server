// markdown.go provides Markdown formatting functions for group protected branch
// MCP tool output.

package groupprotectedbranches

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group protected branch as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Branch: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **Allow Force Push**: %t\n", out.AllowForcePush)
	fmt.Fprintf(&b, "- **Code Owner Approval Required**: %t\n", out.CodeOwnerApprovalRequired)
	writeAccessLevels(&b, "Push Access Levels", out.PushAccessLevels)
	writeAccessLevels(&b, "Merge Access Levels", out.MergeAccessLevels)
	writeAccessLevels(&b, "Unprotect Access Levels", out.UnprotectAccessLevels)
	toolutil.WriteHints(&b,
		"Use gitlab_group_protected_branch_update to modify settings",
		"Use gitlab_group_protected_branch_unprotect to remove protection",
	)
	return b.String()
}

func writeAccessLevels(b *strings.Builder, heading string, levels []AccessLevelOutput) {
	if len(levels) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n", heading)
	b.WriteString("| ID | Level | Description |\n| --- | --- | --- |\n")
	for _, l := range levels {
		fmt.Fprintf(b, "| %d | %d | %s |\n", l.ID, l.AccessLevel, l.AccessLevelDescription)
	}
}

// FormatListMarkdown renders a paginated list of group protected branches as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Branches) == 0 {
		return "No group protected branches found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Branches), out.Pagination)
	b.WriteString("| ID | Name | Force Push | Code Owner |\n| --- | --- | --- | --- |\n")
	for _, br := range out.Branches {
		fmt.Fprintf(&b, "| %d | %s | %t | %t |\n",
			br.ID,
			toolutil.EscapeMdTableCell(br.Name),
			br.AllowForcePush,
			br.CodeOwnerApprovalRequired,
		)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_protected_branch_get with a branch name for details",
		"Use gitlab_group_protected_branch_protect to add new rules",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
