package mergetrains

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// userName returns the display username for a merge-train user sub-object, or
// "" when the user is absent.
func userName(u *toolutil.BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// FormatListMarkdown formats a list of merge train entries.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Trains) == 0 {
		return "No merge trains found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Merge Trains\n\n")
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("| ID | MR | Title | Branch | Status | User | Duration |\n")
	sb.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, t := range out.Trains {
		mr := toolutil.MdTitleLink(fmt.Sprintf("!%d", t.MergeRequest.IID), t.MergeRequest.WebURL)
		// A branch name is not an identifier: git check-ref-format permits
		// '|', '<' and '>'.
		//gitlab:allow-unescaped t.Status: a merge-train car state GitLab's own state machine writes (created, idle, stale, fresh, merging, merged).
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s | %ds |\n",
			t.ID, mr, toolutil.EscapeMdTableCell(t.MergeRequest.Title),
			toolutil.EscapeMdTableCell(t.TargetBranch), t.Status,
			toolutil.EscapeMdTableCell(userName(t.User)), t.Duration)
	}
	toolutil.WriteListSummary(&sb, len(out.Trains), out.Pagination)
	toolutil.WritePagination(&sb, out.Pagination)
	return sb.String()
}

// FormatOutputMarkdown formats a single merge train entry.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Merge Train #%d\n\n", out.ID)
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	//gitlab:allow-unescaped out.Status: a merge-train car state GitLab's own state machine writes (created, idle, stale, fresh, merging, merged).
	toolutil.WriteMdFieldRendered(&sb, "Status", out.Status)
	toolutil.WriteMdField(&sb, "Target Branch", out.TargetBranch)
	mr := fmt.Sprintf("%s - %s",
		toolutil.MdTitleLink(fmt.Sprintf("!%d", out.MergeRequest.IID), out.MergeRequest.WebURL),
		toolutil.EscapeMdTableCell(out.MergeRequest.Title))
	toolutil.WriteMdFieldRendered(&sb, "Merge Request", mr)
	if name := userName(out.User); name != "" {
		toolutil.WriteMdField(&sb, "User", name)
	}
	if out.Pipeline != nil && out.Pipeline.ID > 0 {
		toolutil.WriteMdFieldRendered(&sb, "Pipeline", fmt.Sprintf("#%d", out.Pipeline.ID))
	}
	toolutil.WriteMdFieldRendered(&sb, "Duration", fmt.Sprintf("%ds", out.Duration))
	toolutil.WriteMdFieldTime(&sb, "Created", out.CreatedAt)
	toolutil.WriteMdFieldTime(&sb, "Merged At", out.MergedAt)
	toolutil.WriteHints(
		&sb,
		"Use `gitlab_list_project_merge_trains` to view all merge trains",
		"Use `gitlab_add_merge_request_to_merge_train` to add another MR to the train",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
