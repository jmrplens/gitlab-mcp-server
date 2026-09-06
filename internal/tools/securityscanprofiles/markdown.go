package securityscanprofiles

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatMutationMarkdown renders an attach or detach confirmation as Markdown.
func FormatMutationMarkdown(out MutationOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Security Scan Profile\n\n%s\n\n", out.Message)
	// Echoed from the caller's own argument.
	fmt.Fprintf(&sb, "- Profile: `%s`\n", toolutil.EscapeMdTableCell(out.SecurityScanProfileID))
	if len(out.ProjectIDs) > 0 {
		fmt.Fprintf(&sb, "- Projects: %s\n", joinInts(out.ProjectIDs))
	}
	if len(out.GroupIDs) > 0 {
		fmt.Fprintf(&sb, "- Groups: %s\n", joinInts(out.GroupIDs))
	}
	return sb.String()
}

// FormatListProjectStatusesMarkdown renders per-project scan profile statuses
// as a Markdown table.
func FormatListProjectStatusesMarkdown(out ListProjectStatusesOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	fmt.Fprintf(&sb, "## Scan Profile Statuses: %s\n\n", toolutil.EscapeMdHeading(out.ProjectFullPath))

	if len(out.Statuses) == 0 {
		sb.WriteString("No scan profile statuses found.\n")
		return sb.String()
	}

	sb.WriteString("| Scan Type | Profile | Status |\n")
	sb.WriteString("|-----------|---------|--------|\n")
	for _, s := range out.Statuses {
		fmt.Fprintf(
			&sb, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(s.ScanProfile.ScanType),
			toolutil.EscapeMdTableCell(s.ScanProfile.Name),
			toolutil.EscapeMdTableCell(s.Status),
		)
	}
	sb.WriteString("\n")
	return sb.String()
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
