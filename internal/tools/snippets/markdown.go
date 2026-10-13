package snippets

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type snippetNotFoundOutput struct {
	Identifier string
}

func formatSnippetNotFound(out snippetNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Snippet", out.Identifier,
		"Use snippet.list to list your snippets",
		"Verify the snippet_id is correct",
		"The snippet may be private or have been deleted",
	)
}

// hintUpdateSnippet names the update of a personal snippet and, for a project
// snippet, the project action that updates one instead.
var hintUpdateSnippet = toolutil.HintAction("snippet.update", "modify a personal snippet; a project snippet is modified with snippet.project_update")

// authorCell renders a snippet author as the name and the handle, or nothing
// when GitLab sent no author.
func authorCell(a *SnippetAuthorOutput) string {
	if a == nil {
		return ""
	}
	handle := toolutil.MdUserHandle(a.Username)
	name := toolutil.EscapeMdTableCell(a.Name)
	// An if chain rather than a tagless switch: a case expression carries no
	// statement counter, so the mutation gate reports every mutant of one as
	// not covered whatever the tests assert.
	if name == "" {
		return handle
	}
	if handle == "" {
		return name
	}
	return name + " (" + handle + ")"
}

// FormatMarkdown renders one snippet as the card of one object.
//
// It used to open a "| Field | Value |" table and fill it row by row, which
// left "| Imported From |  |" on the page for every snippet GitLab marked
// imported without naming the platform, and put the two clone URLs — values a
// reader copies verbatim — in cells rather than in code spans.
func FormatMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Snippet #%d: %s", out.ID, out.Title))
	c.Int("ID", out.ID)
	c.Field("Title", out.Title)
	c.Field("File Name", out.FileName)
	c.Text("Description", out.Description)
	c.Field("Visibility", out.Visibility)
	c.Markdown("Author", authorCell(out.Author))
	if out.ProjectID != 0 {
		if pp := extractProjectPath(out.WebURL); pp != "" {
			c.Field("Project", pp)
		} else {
			c.Int("Project ID", out.ProjectID)
		}
	}
	c.URL(out.WebURL)
	c.Code("SSH URL to Repo", out.SSHURLToRepo)
	c.Code("HTTP URL to Repo", out.HTTPURLToRepo)
	c.Flag("", "Imported", out.Imported)
	c.Field("Imported From", out.ImportedFrom)
	if len(out.Files) > 0 {
		t := c.Table("Files", "Path", "Raw URL")
		for _, f := range out.Files {
			// The path names the column already; a link labeled with it a
			// second time says nothing, so the destination is its own label.
			t.Row(toolutil.EscapeMdTableCell(f.Path), toolutil.MdTitleLink(f.RawURL, f.RawURL))
		}
	}
	c.End(snippetHints(out)...)
	return b.String()
}

// snippetHints names the actions that apply to the snippet just rendered, and
// leads with the instruction to keep the links only when the card has one: a
// snippet with no web URL and no files carries no link to preserve.
func snippetHints(out Output) []string {
	var hints []string
	if out.ProjectID != 0 {
		hints = []string{
			toolutil.HintAction("snippet.project_get", "read it, given project_id and snippet_id; the personal snippet.get does not reach a project snippet"),
			toolutil.HintAction("snippet.project_update", "modify its content with files[]: set files[].action to 'update' and use the Path value as files[].file_path"),
			toolutil.HintAction("snippet.project_delete", "remove this project snippet"),
		}
	} else {
		hints = []string{
			toolutil.HintAction("snippet.content", "read the snippet's content"),
			hintUpdateSnippet,
			toolutil.HintAction("snippet.delete", "remove this snippet"),
		}
	}
	if snippetHasLink(out) {
		return toolutil.ListHints(hints...)
	}
	return hints
}

// snippetHasLink reports whether the card renders any clickable link.
func snippetHasLink(out Output) bool {
	if out.WebURL != "" {
		return true
	}
	for _, f := range out.Files {
		if f.RawURL != "" {
			return true
		}
	}
	return false
}

// FormatListMarkdown renders a page of snippets as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Snippets) == 0 {
		return toolutil.EmptyMessage("snippets")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Snippets", len(out.Snippets), out.Pagination)
	withProject := snippetsHaveProject(out.Snippets)
	if withProject {
		writeProjectSnippetTable(&b, out.Snippets)
	} else {
		writeSimpleSnippetTable(&b, out.Snippets)
	}
	toolutil.WriteListFooter(&b, out.Pagination, true, toolutil.ListHints(listHints(withProject)...)...)
	return b.String()
}

// listHints names the actions a reader of the list can take next. A page of
// project snippets is served by the project actions: the personal get and
// create it used to name answer 404 for every row on it.
func listHints(withProject bool) []string {
	if withProject {
		return []string{
			toolutil.HintAction("snippet.project_get", "see a snippet's full details, given project_id and snippet_id"),
			toolutil.HintAction("snippet.project_create", "add a new project snippet"),
		}
	}
	return []string{
		toolutil.HintAction("snippet.get", "see a snippet's full details, given its snippet_id"),
		toolutil.HintAction("snippet.create", "add a new snippet"),
	}
}

// FormatContentMarkdown renders snippet content as a fenced block under the
// card's heading: the content is a file, so it is contained rather than
// escaped.
func FormatContentMarkdown(out ContentOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Snippet #%d Content", out.SnippetID))
	c.Fence("", "", out.Content)
	c.End(
		toolutil.HintAction("snippet.file_content", "get the content of one file"),
		hintUpdateSnippet,
	)
	return b.String()
}

// FormatFileContentMarkdown renders one snippet file's content the same way.
func FormatFileContentMarkdown(out FileContentOutput) string {
	var b strings.Builder
	// Both are echoed from the caller's own arguments, and a git ref may hold
	// '|', '<' and '>'; NewCard escapes the composed heading.
	c := toolutil.NewCard(&b, fmt.Sprintf("Snippet #%d File: %s (ref: %s)", out.SnippetID, out.FileName, out.Ref))
	c.Fence("", "", out.Content)
	c.End(
		toolutil.HintAction("snippet.content", "get the whole snippet's content"),
		hintUpdateSnippet,
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatSnippetNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatContentMarkdown)
	toolutil.RegisterMarkdown(FormatFileContentMarkdown)
}
