// markdown_test.go contains unit tests for the group import/export Markdown
// formatters: the two one-line confirmations and the download card.
//
// Every expectation here is the whole rendered response. The three
// confirmations used to be written with no newline of their own, so the
// guidance rule that followed turned the sentence into a setext heading and
// the hints never reached next_steps; only a whole-output assertion sees that.
package groupimportexport

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// importExportText renders a formatter's result as the text a client receives.
func importExportText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("formatter returned no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want text", result.Content[0])
	}
	return text.Text
}

// TestFormatScheduleExportMarkdown verifies the whole schedule confirmation,
// and that an output carrying no message renders nothing at all.
func TestFormatScheduleExportMarkdown(t *testing.T) {
	md := importExportText(t, FormatScheduleExportMarkdown(ScheduleExportOutput{Message: "Group export scheduled successfully"}))

	want := "## ✅ Group export scheduled successfully\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.group_export_download' to download the export once it is complete\n"
	if md != want {
		t.Errorf("schedule confirmation:\n got %q\nwant %q", md, want)
	}

	if result := FormatScheduleExportMarkdown(ScheduleExportOutput{}); result != nil {
		t.Error("an output with no message rendered something")
	}
}

// TestFormatExportDownloadMarkdown verifies the whole download card: the size
// is a row, the base64 note is the server's own prose after it, and an empty
// download renders nothing.
func TestFormatExportDownloadMarkdown(t *testing.T) {
	md := importExportText(t, FormatExportDownloadMarkdown(ExportDownloadOutput{ContentBase64: "dGVzdA==", SizeBytes: 512}))

	want := "## ✅ Group export archive downloaded\n\n" +
		"- **Size (bytes)**: 512\n\n" +
		"The archive is base64-encoded in the content_base64 field.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.group_import_file' to import the archive into another group\n"
	if md != want {
		t.Errorf("download card:\n got %q\nwant %q", md, want)
	}

	if result := FormatExportDownloadMarkdown(ExportDownloadOutput{}); result != nil {
		t.Error("an empty download rendered something")
	}
}

// TestFormatImportFileMarkdown verifies the whole import confirmation.
func TestFormatImportFileMarkdown(t *testing.T) {
	md := importExportText(t, FormatImportFileMarkdown(ImportFileOutput{Message: "Group import started successfully"}))

	want := "## ✅ Group import started successfully\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.list' to verify the imported group appears\n"
	if md != want {
		t.Errorf("import confirmation:\n got %q\nwant %q", md, want)
	}

	if result := FormatImportFileMarkdown(ImportFileOutput{}); result != nil {
		t.Error("an output with no message rendered something")
	}
}
