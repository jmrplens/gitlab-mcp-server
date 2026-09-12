package securityscanprofiles

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMutationMarkdown renders an attach or detach confirmation as the card
// of one result: what GitLab did, the profile it did it with, and the targets
// the request named.
func FormatMutationMarkdown(out MutationOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Security Scan Profile")
	// The status and the message are this server's own words, and they are
	// written through the card's row escaper like any other value: a row is
	// where they land, and the row decides the containment, not the author.
	c.Field("Result", out.Message)
	c.Field("Status", out.Status)
	// Echoed from the caller's own argument.
	c.Code("Profile", out.SecurityScanProfileID)
	c.Field("Projects", joinInts(out.ProjectIDs))
	c.Field("Groups", joinInts(out.GroupIDs))
	c.End(
		toolutil.HintAction(actionListProjectStatuses, "see which scan profiles a project carries now"),
		toolutil.HintAction(actionVulnList, "read the vulnerabilities a scan found"),
	)
	return b.String()
}

// FormatListProjectStatusesMarkdown renders per-project scan profile statuses
// as a Markdown table: a collection of objects that share columns. The profile
// ID is a column because it is what the detach action takes, and the list used
// to be the only place a reader could have found it.
func FormatListProjectStatusesMarkdown(out ListProjectStatusesOutput) string {
	if len(out.Statuses) == 0 {
		return toolutil.EmptyMessage("scan profile statuses")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Scan Profile Statuses: "+out.ProjectFullPath, len(out.Statuses), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Scan Type", "Profile", "Status"))
	for _, s := range out.Statuses {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(s.ScanProfile.ID),
			toolutil.EscapeMdTableCell(s.ScanProfile.ScanType),
			toolutil.EscapeMdTableCell(s.ScanProfile.Name),
			toolutil.EscapeMdTableCell(s.Status),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionAttach, "attach a scan profile to more projects or groups"),
		toolutil.HintAction(actionDetach, "detach one, naming the profile ID above"),
	)
	return b.String()
}

// joinInts renders a slice of int64 IDs as a comma-separated string.
func joinInts(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ", ")
}

func init() {
	toolutil.RegisterMarkdown(FormatMutationMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectStatusesMarkdown)
}
