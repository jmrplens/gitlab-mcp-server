// markdown.go provides Markdown formatting functions for alert management MCP tool output.
package alertmanagement

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats metric images as markdown.
func FormatListMarkdown(out ListMetricImagesOutput) string {
	var sb strings.Builder
	sb.WriteString("## Alert Metric Images\n\n")
	toolutil.WriteListSummary(&sb, len(out.Images), out.Pagination)
	if len(out.Images) == 0 {
		sb.WriteString("No metric images found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Filename | URL |\n|----|----------|-----|\n")
	for _, img := range out.Images {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", img.ID, toolutil.EscapeMdTableCell(img.Filename), toolutil.MdTitleLink(img.Filename, img.URL))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Use `gitlab_upload_alert_metric_image` to add a new metric image",
	)
	return sb.String()
}

// FormatImageMarkdown formats a single metric image as markdown.
func FormatImageMarkdown(img MetricImageItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Metric Image\n\n- **ID**: %d\n- **Filename**: %s\n", img.ID, img.Filename)
	fmt.Fprintf(&b, toolutil.FmtMdURL, img.URL)
	fmt.Fprintf(&b, "- **URL Text**: %s\n", img.URLText)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_list_alert_metric_images` to see all metric images for an alert",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatImageMarkdown)
}
