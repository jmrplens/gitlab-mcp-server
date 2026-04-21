// markdown.go provides Markdown formatting functions for upload MCP tool output.

package uploads

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use the Markdown reference in issue or MR descriptions to embed this file",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(UploadToolResult)
}
