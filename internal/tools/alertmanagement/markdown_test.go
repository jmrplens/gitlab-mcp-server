// markdown_test.go asserts the whole Markdown document each alert metric image
// formatter writes: the list table, which now carries the file path and labels
// the link with the caption whoever uploaded the image typed, and the card one
// image renders as.
package alertmanagement

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// captionedImage carries the caption and the file path GitLab sends beside the
// image itself.
var captionedImage = MetricImageItem{
	ID:        1,
	Filename:  "img.png",
	FilePath:  "/uploads/-/system/alert_metric_image/file/1/img.png",
	URL:       "https://example.com/img.png",
	URLText:   "CPU saturation",
	CreatedAt: "2026-06-01T10:00:00Z",
}

// plainImage is an upload with no caption, where the link falls back to the
// address itself rather than to the file name.
var plainImage = MetricImageItem{
	ID:       2,
	Filename: "second.png",
	FilePath: "/uploads/-/system/alert_metric_image/file/2/second.png",
	URL:      "https://example.com/second.png",
}

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatListMarkdown verifies the metric image table and the sentence an
// empty page renders instead of a heading counting zero.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with images", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListMetricImagesOutput{
			Images: []MetricImageItem{captionedImage, plainImage},
		}),
			"## Alert Metric Images (2)\n\n"+
				"| ID | Filename | File Path | Metric Link |\n"+
				"| --- | --- | --- | --- |\n"+
				"| 1 | img.png | /uploads/-/system/alert_metric_image/file/1/img.png | [CPU saturation](https://example.com/img.png) |\n"+
				"| 2 | second.png | /uploads/-/system/alert_metric_image/file/2/second.png | [https://example.com/second.png](https://example.com/second.png) |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- "+toolutil.HintPreserveLinks+"\n"+
				"- Use action 'admin.alert_metric_image_upload' to add another metric image to the alert\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListMetricImagesOutput{}), "No metric images found.\n")
	})
}

// TestFormatImageMarkdown verifies the metric image card: the file path GitLab
// sends, the link labeled with the caption, and the optional rows dropped when
// GitLab sent nothing for them.
func TestFormatImageMarkdown(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.alert_metric_image_list' to see every metric image on this alert\n" +
		"- Use action 'admin.alert_metric_image_update' to change this image's caption or link\n"
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatImageMarkdown(captionedImage),
			"## Metric Image #1\n\n"+
				"- **ID**: 1\n"+
				"- **Filename**: img.png\n"+
				"- **File Path**: /uploads/-/system/alert_metric_image/file/1/img.png\n"+
				"- **Metric Link**: [CPU saturation](https://example.com/img.png)\n"+
				"- **URL Text**: CPU saturation\n"+
				"- **Created**: 1 Jun 2026 10:00 UTC\n"+
				hints)
	})
	t.Run("filename only", func(t *testing.T) {
		assertRendered(t, FormatImageMarkdown(MetricImageItem{ID: 1, Filename: "img.png"}),
			"## Metric Image #1\n\n"+
				"- **ID**: 1\n"+
				"- **Filename**: img.png\n"+
				hints)
	})
}

// TestAlertMetricImageFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestAlertMetricImageFormattersAreRegistered(t *testing.T) {
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{
			name:     "ListMetricImagesOutput",
			output:   ListMetricImagesOutput{Images: []MetricImageItem{captionedImage}},
			rendered: FormatListMarkdown(ListMetricImagesOutput{Images: []MetricImageItem{captionedImage}}),
		},
		{
			name:     "MetricImageItem",
			output:   captionedImage,
			rendered: FormatImageMarkdown(captionedImage),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tc.output)
			if result == nil {
				t.Fatal("MarkdownForResult returned nil, want a registered formatter for the type")
			}
			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(result.Content))
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
			}
			assertRendered(t, text.Text, tc.rendered)
		})
	}
}
