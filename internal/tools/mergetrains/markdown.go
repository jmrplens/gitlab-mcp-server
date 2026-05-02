package mergetrains

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
		mr := fmt.Sprintf("!%d", t.MergeRequest.IID)
		if t.MergeRequest.WebURL != "" {
			mr = fmt.Sprintf("[!%d](%s)", t.MergeRequest.IID, t.MergeRequest.WebURL)
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s | %ds |\n",
			t.ID, mr, toolutil.EscapeMdTableCell(t.MergeRequest.Title),
			t.TargetBranch, t.Status, t.User, t.Duration)
	}
	toolutil.WriteListSummary(&sb, len(out.Trains), out.Pagination)
	toolutil.WritePagination(&sb, out.Pagination)
	return sb.String()
}

// FormatOutputMarkdown formats a single merge train entry.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Merge Train #%d\n\n", out.ID)
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&sb, "| Target Branch | %s |\n", out.TargetBranch)
	mr := fmt.Sprintf("!%d — %s", out.MergeRequest.IID, toolutil.EscapeMdTableCell(out.MergeRequest.Title))
	if out.MergeRequest.WebURL != "" {
		mr = fmt.Sprintf("[!%d](%s) — %s", out.MergeRequest.IID, out.MergeRequest.WebURL, toolutil.EscapeMdTableCell(out.MergeRequest.Title))
	}
	fmt.Fprintf(&sb, "| Merge Request | %s |\n", mr)
	if out.User != "" {
		fmt.Fprintf(&sb, "| User | %s |\n", out.User)
	}
	if out.PipelineID > 0 {
		fmt.Fprintf(&sb, "| Pipeline | #%d |\n", out.PipelineID)
	}
	fmt.Fprintf(&sb, "| Duration | %ds |\n", out.Duration)
	fmt.Fprintf(&sb, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	if out.MergedAt != "" {
		fmt.Fprintf(&sb, "| Merged At | %s |\n", toolutil.FormatTime(out.MergedAt))
	}
	toolutil.WriteHints(&sb,
		"Use `gitlab_list_project_merge_trains` to view all merge trains",
		"Use `gitlab_add_merge_request_to_merge_train` to add another MR to the train",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
