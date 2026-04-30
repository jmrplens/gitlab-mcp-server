// markdown.go provides Markdown formatting functions for branch MCP tool output.
package branches

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single branch as a Markdown summary.
func FormatOutputMarkdown(br Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Branch: %s\n\n", toolutil.EscapeMdHeading(br.Name))
	fmt.Fprintf(&b, "- **Protected**: %v\n", br.Protected)
	fmt.Fprintf(&b, "- **Default**: %v\n", br.Default)
	fmt.Fprintf(&b, "- **Merged**: %v\n", br.Merged)
	fmt.Fprintf(&b, "- **Commit**: %s\n", br.CommitID)
	if br.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, br.WebURL)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_merge_request action 'create' to open an MR from this branch",
		"Use gitlab_repository action 'commit_list' to see recent commits on this branch",
		"Use action 'delete' to remove the branch after merging",
	)
	return b.String()
}

// FormatListMarkdown renders a list of branches as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Branches (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Branches), out.Pagination)
	if len(out.Branches) == 0 {
		b.WriteString("No branches found.\n")
		return b.String()
	}
	b.WriteString("| Name | Protected | Default | Merged |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, br := range out.Branches {
		name := toolutil.EscapeMdTableCell(br.Name)
		if br.WebURL != "" {
			name = fmt.Sprintf("[%s](%s)", name, br.WebURL)
		}
		fmt.Fprintf(&b, "| %s | %v | %v | %v |\n", name, br.Protected, br.Default, br.Merged)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a branch name to see full details",
		"Use action 'create' to create a new branch",
		"Use action 'protect' to protect a branch",
	)
	return b.String()
}

// FormatProtectedMarkdown renders a single protected branch as Markdown.
func FormatProtectedMarkdown(pb ProtectedOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Branch: %s\n\n", toolutil.EscapeMdHeading(pb.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, pb.ID)
	fmt.Fprintf(&b, "- **Push Access Level**: %d\n", pb.PushAccessLevel)
	fmt.Fprintf(&b, "- **Merge Access Level**: %d\n", pb.MergeAccessLevel)
	fmt.Fprintf(&b, "- **Allow Force Push**: %v\n", pb.AllowForcePush)
	toolutil.WriteHints(&b,
		"Use action 'list_protected' to see all protected branches",
		"Use action 'unprotect' to remove branch protection",
	)
	return b.String()
}

// FormatProtectedListMarkdown renders a list of protected branches as a Markdown table.
func FormatProtectedListMarkdown(out ProtectedListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Branches (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Branches), out.Pagination)
	if len(out.Branches) == 0 {
		b.WriteString("No protected branches found.\n")
		return b.String()
	}
	b.WriteString("| Name | Push Level | Merge Level | Force Push |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, pb := range out.Branches {
		fmt.Fprintf(&b, "| %s | %d | %d | %v |\n", toolutil.EscapeMdTableCell(pb.Name), pb.PushAccessLevel, pb.MergeAccessLevel, pb.AllowForcePush)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get_protected' with a branch name for full details",
		"Use action 'protect' to add branch protection",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatProtectedMarkdown)
	toolutil.RegisterMarkdown(FormatProtectedListMarkdown)
}
