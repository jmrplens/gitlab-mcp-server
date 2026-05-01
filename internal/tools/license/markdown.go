package license

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatLicenseMarkdown formats a license as markdown.
func FormatLicenseMarkdown(item Item) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## License #%d\n\n", item.ID)
	sb.WriteString("| Property | Value |\n")
	sb.WriteString("|----------|-------|\n")
	fmt.Fprintf(&sb, "| Plan | %s |\n", item.Plan)
	fmt.Fprintf(&sb, "| Expired | %v |\n", item.Expired)
	fmt.Fprintf(&sb, "| Active Users | %d |\n", item.ActiveUsers)
	fmt.Fprintf(&sb, "| User Limit | %d |\n", item.UserLimit)
	fmt.Fprintf(&sb, "| Maximum User Count | %d |\n", item.MaximumUserCount)
	fmt.Fprintf(&sb, "| Historical Max | %d |\n", item.HistoricalMax)
	fmt.Fprintf(&sb, "| Overage | %d |\n", item.Overage)
	if item.StartsAt != "" {
		fmt.Fprintf(&sb, "| Starts At | %s |\n", toolutil.FormatTime(item.StartsAt))
	}
	if item.ExpiresAt != "" {
		fmt.Fprintf(&sb, "| Expires At | %s |\n", toolutil.FormatTime(item.ExpiresAt))
	}
	if item.CreatedAt != "" {
		fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(item.CreatedAt))
	}
	fmt.Fprintf(&sb, "| Licensee | %s (%s) — %s |\n",
		item.Licensee.Name, item.Licensee.Company, item.Licensee.Email)
	toolutil.WriteHints(&sb, "Check license expiry date and plan for renewal if needed")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetMarkdown formats a GetOutput.
func FormatGetMarkdown(output GetOutput) *mcp.CallToolResult {
	return FormatLicenseMarkdown(output.License)
}

// FormatAddMarkdown formats an AddOutput.
func FormatAddMarkdown(output AddOutput) *mcp.CallToolResult {
	return FormatLicenseMarkdown(output.License)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatLicenseMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddMarkdown)
}
