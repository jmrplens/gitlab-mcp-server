package issuelinks

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	fmt.Fprintf(&b, "- **Source Issue**: %s\n", issueRefLine(v.SourceIssue))
	fmt.Fprintf(&b, "- **Target Issue**: %s\n", issueRefLine(v.TargetIssue))
	toolutil.WriteHints(&b, "Use `gitlab_issue_link_list` to see all links for this issue")
	return b.String()
}

// issueRefLine renders an issue reference ("IID N (project M) — Title") for a
// source/target issue object, or a placeholder when the object is absent.
func issueRefLine(ref *IssueRefOutput) string {
	if ref == nil {
		return "(not available)"
	}
	return fmt.Sprintf("IID %d (project %d)%s", ref.IID, ref.ProjectID, issueRefSuffix(ref))
}

// FormatListMarkdown renders a list of issue relations as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Relations) == 0 {
		return "No linked issues found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Relations (%d)\n\n", len(out.Relations))
	b.WriteString("| ID | IID | Title | State | Link Type | Link ID | Author |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range out.Relations {
		author := ""
		if r.Author != nil {
			author = r.Author.Username
		}
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %d | %s |\n",
			r.ID, r.IID, toolutil.MdTitleLink(r.Title, r.WebURL), r.State, r.LinkType, r.IssueLinkID, author)
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks, "Use `gitlab_issue_link_create` to add a new link between issues")
	return b.String()
}

// issueRefSuffix renders a " — Title" suffix for a source/target issue object,
// or "" when the object is absent.
func issueRefSuffix(ref *IssueRefOutput) string {
	if ref == nil || ref.Title == "" {
		return ""
	}
	return " — " + ref.Title
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
