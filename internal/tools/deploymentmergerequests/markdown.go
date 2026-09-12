package deploymentmergerequests

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionMergeRequestGet     = "merge_request.get"
	actionMergeRequestChanges = "mr_review.changes_get"
)

// FormatListMarkdown renders the merge requests a deployment shipped.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders the deployment's merge requests as a
// Markdown table, in the row shape every merge request table in the tree uses:
// the IID carries the link, the title is a cell of its own, and the state
// carries its glyph. Linking the title instead put a GitLab-authored value in
// a link label, and left a reader with no reference to call the MR by.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("merge requests for this deployment")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Deployment Merge Requests", len(out.MergeRequests), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Source -> Target"))
	for _, mr := range out.MergeRequests {
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			mrStateCell(mr.State),
			toolutil.MdUserHandle(author),
			// A branch name is not an identifier: git check-ref-format permits
			// '|', '<' and '>'.
			toolutil.EscapeMdTableCell(mr.SourceBranch)+" -> "+toolutil.EscapeMdTableCell(mr.TargetBranch),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, true,
		toolutil.HintAction(actionMergeRequestGet, "read one of these merge requests in full"),
		toolutil.HintAction(actionMergeRequestChanges, "see what one of them changed"),
	)
	return sb.String()
}

// mrStateCell renders a merge request state with its emoji, the way every merge
// request row in the tree shows it, and nothing when GitLab sent no state.
func mrStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.MRStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
}
