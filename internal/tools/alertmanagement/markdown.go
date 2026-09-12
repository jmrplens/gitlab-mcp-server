package alertmanagement

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionList   = "admin.alert_metric_image_list"
	actionUpload = "admin.alert_metric_image_upload"
	actionUpdate = "admin.alert_metric_image_update"
)

// metricLinkCell renders the image's address as the link a reader follows,
// labeled with the caption whoever uploaded it typed and with the address
// itself when they typed none. The label used to be the file name, which said
// nothing the Filename column had not already said and lost the caption
// entirely.
func metricLinkCell(img MetricImageItem) string {
	if img.URL == "" {
		return ""
	}
	label := img.URLText
	if label == "" {
		label = img.URL
	}
	return toolutil.MdTitleLink(label, img.URL)
}

// FormatListMarkdown renders a page of alert metric images as a Markdown
// table: a collection of objects that share columns.
func FormatListMarkdown(out ListMetricImagesOutput) string {
	if len(out.Images) == 0 {
		return toolutil.EmptyMessage("metric images")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Alert Metric Images", len(out.Images), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Filename", "File Path", "Metric Link"))
	linked := false
	for _, img := range out.Images {
		linked = linked || img.URL != ""
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(img.ID, 10),
			// Both the file name and the path are chosen by whoever uploaded
			// the image.
			toolutil.EscapeMdTableCell(img.Filename),
			toolutil.EscapeMdTableCell(img.FilePath),
			metricLinkCell(img),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, linked,
		toolutil.HintAction(actionUpload, "add another metric image to the alert"),
	)
	return sb.String()
}

// FormatImageMarkdown renders a single metric image as the card of one object.
func FormatImageMarkdown(img MetricImageItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Metric Image #%d", img.ID))
	c.Int("ID", img.ID)
	// Both are chosen by whoever uploaded the image.
	c.Field("Filename", img.Filename)
	c.Field("File Path", img.FilePath)
	c.Markdown("Metric Link", metricLinkCell(img))
	c.Field("URL Text", img.URLText)
	c.Time("Created", img.CreatedAt)
	c.End(
		toolutil.HintAction(actionList, "see every metric image on this alert"),
		toolutil.HintAction(actionUpdate, "change this image's caption or link"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatImageMarkdown)
}
