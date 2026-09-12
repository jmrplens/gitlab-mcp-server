package uploads

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// uploadHint is the one next step an upload result offers.
const uploadHint = "Use the Markdown reference in issue or MR descriptions to embed this file"

// FormatUploadMarkdown renders an uploaded file result as the card of one
// object.
func FormatUploadMarkdown(u UploadOutput) string {
	var b strings.Builder
	writeUploadCard(&b, u, "")
	return b.String()
}

// writeUploadCard writes the upload card into b, with an optional embed
// between the rows and the guidance section.
//
// The embed goes here rather than after the formatter returns because
// [toolutil.WriteHints] closes the response: anything appended after the
// guidance section leaves it neither leading nor trailing, which is the one
// position [toolutil.ExtractHints] refuses to read, so the hint stopped
// reaching next_steps the moment the embed was appended behind it.
func writeUploadCard(b *strings.Builder, u UploadOutput, embed string) {
	c := toolutil.NewCard(b, "File Uploaded")
	c.Count("ID", u.ID)
	c.Field("Alt", u.Alt)
	// The relative form is the /uploads/<secret>/<name> reference the Markdown
	// snippet is built around, so it is labeled Path; the URL row is the
	// address a reader can open.
	c.Field("Path", u.URL)
	c.URL(u.FullURL)
	// GitLab builds this snippet around the file name whoever uploaded it
	// chose, and a reader copies it verbatim, so it is a code span sized to
	// its content rather than a fixed pair of backticks around an escaped
	// value: an entity inside a span renders as its own characters.
	c.Code("Markdown", u.Markdown)
	if embed != "" {
		c.Note(embed)
	}
	c.End(uploadHint)
}

// FormatListMarkdown renders a list of project markdown uploads as Markdown.
func FormatListMarkdown(o ListOutput) string {
	if len(o.Uploads) == 0 {
		return toolutil.EmptyMessage("uploads")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project Markdown Uploads", len(o.Uploads), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Filename", "Size (bytes)", "Created", "Uploaded By"))
	for _, u := range o.Uploads {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.EscapeMdTableCell(u.Filename),
			strconv.FormatInt(u.Size, 10),
			toolutil.FormatTime(u.CreatedAt),
			toolutil.EscapeMdTableCell(uploadedByLabel(u.UploadedBy)),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, o.Pagination, false,
		"Use `gitlab_project_upload_delete` with an ID from the table to remove one")
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
