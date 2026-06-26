package uploads

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatUploadMarkdown renders an uploaded file result as Markdown.
func FormatUploadMarkdown(u UploadOutput) string {
	var b strings.Builder
	b.WriteString("## File Uploaded\n\n")
	fmt.Fprintf(&b, "- **Alt**: %s\n", u.Alt)
	fmt.Fprintf(&b, "- **URL**: %s\n", u.URL)
	if u.FullURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, u.FullURL)
	}
	fmt.Fprintf(&b, "- **Markdown**: `%s`\n", u.Markdown)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the Markdown reference in issue or MR descriptions to embed this file",
	)
	return b.String()
}

// FormatListMarkdown renders a list of project markdown uploads as Markdown.
func FormatListMarkdown(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Markdown Uploads (%d)\n\n", len(o.Uploads))
	toolutil.WriteListSummary(&b, len(o.Uploads), o.Pagination)
	if len(o.Uploads) == 0 {
		b.WriteString("No uploads found.\n")
		return b.String()
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	b.WriteString("| ID | Filename | Size | Created | Uploaded By |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, u := range o.Uploads {
		fmt.Fprintf(&b, "| %d | %s | %d | %s | %s |\n",
			u.ID,
			toolutil.EscapeMdTableCell(u.Filename),
			u.Size,
			toolutil.EscapeMdTableCell(u.CreatedAt),
			toolutil.EscapeMdTableCell(uploadedByLabel(u.UploadedBy)))
	}
	return b.String()
}

// uploadedByLabel renders a markdown upload's uploader as "name (@username)",
// falling back gracefully when fields are empty or the user is absent.
func uploadedByLabel(u *UploadedByOutput) string {
	if u == nil {
		return ""
	}
	switch {
	case u.Name != "" && u.Username != "":
		return fmt.Sprintf("%s (@%s)", u.Name, u.Username)
	case u.Username != "":
		return "@" + u.Username
	default:
		return u.Name
	}
}

func init() {
	toolutil.RegisterMarkdownResult(UploadToolResult)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
