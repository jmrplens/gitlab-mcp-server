package repositorysubmodules

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
// These actions are routes on the gitlab_repository catalog group, so each ID
// carries that domain.
const (
	hintListSubmodules    = "repository.list_submodules"
	hintReadSubmoduleFile = "repository.read_submodule_file"
	hintUpdateSubmodule   = "repository.update_submodule"
)

// shortSHA is a git object id abbreviated to the eight characters a reader
// compares by, or the whole id when it is shorter.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// FormatListMarkdown renders the submodules of a repository as a Markdown
// table.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if out.Count == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("submodules"))
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Repository Submodules", out.Count, toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Path", "Commit SHA", "Resolved Project"))
	for _, s := range out.Submodules {
		// The name, the path and the resolved project all come out of the
		// repository's own .gitmodules, so all three are text somebody typed;
		// the path and the SHA are code spans a reader copies, and a code span
		// sized by the helper cannot be closed from inside the value.
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.Name),
			toolutil.MdCodeSpanCell(s.Path),
			toolutil.MdCodeSpanCell(shortSHA(s.CommitSHA)),
			toolutil.EscapeMdTableCell(s.ResolvedProject),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(hintReadSubmoduleFile, "read a file out of one submodule"),
		toolutil.HintAction(hintUpdateSubmodule, "move a submodule to another commit"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatReadMarkdown renders a file read out of a submodule as the card of the
// file, with the body fenced under a section of its own.
func FormatReadMarkdown(out ReadOutput) *mcp.CallToolResult {
	ext := ""
	if idx := strings.LastIndex(out.FileName, "."); idx >= 0 {
		ext = out.FileName[idx+1:]
	}

	var b strings.Builder
	c := toolutil.NewCard(&b, "File from Submodule")
	c.Code("Submodule", out.SubmodulePath)
	c.Field("Resolved Project", out.ResolvedProject)
	c.Code("Commit", shortSHA(out.CommitSHA))
	c.Code("File", out.FilePath)
	c.Int("Size (bytes)", out.Size)
	c.Field("Encoding", out.Encoding)
	// The body is a file of the submodule's own repository, so whoever can push
	// there chooses it: a three-backtick fence would be closed by the first run
	// of three the file contains, and everything after it would render as
	// Markdown of this response. The extension is read off the file name and is
	// as much the pusher's choice, which is why the info string goes through the
	// same helper rather than into the fence line by hand.
	c.Fence("Content", ext, out.Content)
	c.End(toolutil.HintAction(hintUpdateSubmodule, "change the commit SHA this submodule is pinned to"))
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatUpdateMarkdown renders the commit a submodule update created as the
// card of that commit.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Submodule Updated")
	c.Code("Commit", out.ShortID)
	c.Code("Full SHA", out.ID)
	// The title, the ident and the message are what a person wrote in the
	// commit, and this package's own update action supplies the message. The
	// email is a plain row rather than an angle-bracketed ident, which GFM
	// turns into a mailto autolink.
	c.Field("Title", out.Title)
	c.Field("Author", out.AuthorName)
	c.Field("Author Email", out.AuthorEmail)
	c.Time("Committed", out.CommittedDate)
	c.Text("Message", out.Message)
	c.Field("Status", out.Status)
	c.End(
		toolutil.HintAction(hintListSubmodules, "see every submodule of this repository"),
		toolutil.HintAction(hintReadSubmoduleFile, "read a file at the new commit"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatReadMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
