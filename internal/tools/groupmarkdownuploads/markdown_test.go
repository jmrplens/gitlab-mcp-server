// markdown_test.go contains unit tests for the group markdown upload list
// formatter.
//
// Every expectation here is the whole rendered response: the formatter used to
// write its guidance section before the table, which glued the table header to
// a list item and left no table at all, and a substring assertion saw none of
// that.
package groupmarkdownuploads

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestFormatListMarkdownString verifies the whole upload list: the heading
// counts what GitLab reported, the timestamp is in the display form every
// other table uses, and the uploader is named.
func TestFormatListMarkdownString(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{
		Uploads: []UploadItem{
			{ID: 1, Size: 1024, Filename: testFilename, CreatedAt: "2026-01-01T09:30:00Z", UploadedBy: &UploadedByOutput{Username: "bob", Name: "Bob"}},
			{ID: 2, Size: 512, Filename: "no-date.bin"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2, PerPage: 20},
	})

	want := "## Group Markdown Uploads (2)\n\n" +
		"| ID | Filename | Size (bytes) | Created | Uploaded By |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | image.png | 1024 | 1 Jan 2026 09:30 UTC | Bob (@bob) |\n" +
		"| 2 | no-date.bin | 512 |  |  |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.group_upload_delete_by_id' to remove one of these uploads\n"
	if md != want {
		t.Errorf("upload list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdownString_Empty verifies an empty list is the one
// sentence and nothing else.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	if md := FormatListMarkdownString(ListOutput{}); md != "No group markdown uploads found.\n" {
		t.Errorf("empty upload list = %q, want the one-sentence empty message", md)
	}
}

// TestFormatListMarkdownString_EscapesTheFilename verifies a filename carrying
// a pipe cannot end its cell.
func TestFormatListMarkdownString_EscapesTheFilename(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{
		Uploads: []UploadItem{{ID: 5, Size: 256, Filename: "file|with|pipes.txt"}},
	})
	if !strings.Contains(md, "| 5 | file&#124;with&#124;pipes.txt | 256 |") {
		t.Errorf("the pipe was not neutralized:\n%s", md)
	}
}
