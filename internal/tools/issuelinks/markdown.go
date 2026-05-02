package issuelinks

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single issue link as Markdown.
func FormatOutputMarkdown(v Output) string {
	if v.ID == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Issue Link\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, v.ID)
	fmt.Fprintf(&b, "- **Link Type**: %s\n", v.LinkType)
	fmt.Fprintf(&b, "- **Source Issue IID**: %d (project %d)\n", v.SourceIssueIID, v.SourceProjectID)
	fmt.Fprintf(&b, "- **Target Issue IID**: %d (project %d)\n", v.TargetIssueIID, v.TargetProjectID)
	toolutil.WriteHints(&b, "Use `gitlab_issue_link_list` to see all links for this issue")
	return b.String()
}

// FormatListMarkdown renders a list of issue relations as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Relations) == 0 {
		return "No linked issues found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Relations (%d)\n\n", len(out.Relations))
	b.WriteString("| ID | IID | Title | State | Link Type | Link ID |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range out.Relations {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %d |\n",
			r.ID, r.IID, toolutil.MdTitleLink(r.Title, r.WebURL), r.State, r.LinkType, r.IssueLinkID)
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks, "Use `gitlab_issue_link_create` to add a new link between issues")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
