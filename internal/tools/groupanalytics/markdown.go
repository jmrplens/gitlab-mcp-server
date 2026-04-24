// markdown.go provides human-readable Markdown formatters for group activity analytics.

package groupanalytics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtGroupRow = "| Group | `%s` |\n"

// FormatIssuesCountMarkdown formats a recently created issues count as Markdown.
func FormatIssuesCountMarkdown(out IssuesCountOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Recently Created Issues Count\n\n")
	fmt.Fprint(&sb, toolutil.TblFieldValue)
	fmt.Fprintf(&sb, fmtGroupRow, out.GroupPath)
	fmt.Fprintf(&sb, "| Issues Count (last 90 days) | **%d** |\n", out.IssuesCount)
	toolutil.WriteHints(&sb,
		"Use `gitlab_get_recently_created_mr_count` to compare with merge request activity",
		"Use `gitlab_issue_list_group` to view the actual issues",
	)
	return sb.String()
}

// FormatMRCountMarkdown formats a recently created merge requests count as Markdown.
func FormatMRCountMarkdown(out MRCountOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Recently Created Merge Requests Count\n\n")
	fmt.Fprint(&sb, toolutil.TblFieldValue)
	fmt.Fprintf(&sb, fmtGroupRow, out.GroupPath)
	fmt.Fprintf(&sb, "| Merge Requests Count (last 90 days) | **%d** |\n", out.MergeRequestsCount)
	toolutil.WriteHints(&sb,
		"Use `gitlab_get_recently_created_issues_count` to compare with issue activity",
		"Use `gitlab_mr_list_group` to view the actual merge requests",
	)
	return sb.String()
}

// FormatMembersCountMarkdown formats a recently added members count as Markdown.
func FormatMembersCountMarkdown(out MembersCountOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Recently Added Members Count\n\n")
	fmt.Fprint(&sb, toolutil.TblFieldValue)
	fmt.Fprintf(&sb, fmtGroupRow, out.GroupPath)
	fmt.Fprintf(&sb, "| New Members Count (last 90 days) | **%d** |\n", out.NewMembersCount)
	toolutil.WriteHints(&sb,
		"Use `gitlab_group_members_list` to view the actual members",
		"Use `gitlab_get_recently_created_issues_count` to see group development activity",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatIssuesCountMarkdown)  // IssuesCountOutput
	toolutil.RegisterMarkdown(FormatMRCountMarkdown)      // MRCountOutput
	toolutil.RegisterMarkdown(FormatMembersCountMarkdown) // MembersCountOutput
}
