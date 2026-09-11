package repository

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionTree              = "repository.tree"
	actionCompare           = "repository.compare"
	actionRawBlob           = "repository.raw_blob"
	actionChangelogAdd      = "repository.changelog_add"
	actionChangelogGenerate = "repository.changelog_generate"
	actionFileGet           = "repository.file_get"
	actionCommitGet         = "repository.commit_get"
	actionCommitList        = "repository.commit_list"
	actionReleaseCreate     = "release.create"
)

// imageNote is the sentence a card writes where the bytes themselves are
// attached to the result rather than printed.
const imageNote = "\U0001F5BC️ Image content is attached below as ImageContent for multimodal viewing."

// treeNodeIcon marks what a tree entry is. A submodule is a commit object
// pinned inside the tree, not a file, and marking it as one told a reader it
// could be read with file_get.
func treeNodeIcon(nodeType string) string {
	switch nodeType {
	case "tree":
		return toolutil.EmojiFolder
	case "commit":
		return toolutil.EmojiLink
	default:
		return toolutil.EmojiFile
	}
}

// FormatTreeMarkdown renders a page of a repository tree as a Markdown table.
func FormatTreeMarkdown(out TreeOutput) string {
	if len(out.Tree) == 0 {
		return toolutil.EmptyMessage("files or directories")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Repository Tree", len(out.Tree), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Type", "Name", "Path"))
	for _, n := range out.Tree {
		b.WriteString(toolutil.MarkdownTableRow(
			treeNodeIcon(n.Type),
			toolutil.EscapeMdTableCell(n.Name),
			toolutil.MdCodeSpanCell(n.Path),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionFileGet, "read one file's content"),
		toolutil.HintAction(actionCompare, "see differences between branches or commits"),
	)
	return b.String()
}

// FormatCompareMarkdown renders a comparison of two refs as a card whose
// commits and changed files are the nested collections they are.
func FormatCompareMarkdown(out CompareOutput) string {
	var b strings.Builder
	if out.CompareSameRef {
		c := toolutil.NewCard(&b, "Repository Compare: same ref")
		c.Note("Both references point to the same commit.")
		c.End(toolutil.HintAction(actionCompare, "compare two different refs"))
		return b.String()
	}
	if out.CompareTimeout {
		c := toolutil.NewCard(&b, "Repository Compare: timeout")
		c.Note("The comparison timed out. Try again with a smaller range.")
		c.End(toolutil.HintAction(actionCompare, "compare a smaller range"))
		return b.String()
	}
	c := toolutil.NewCard(&b, "Repository Compare")
	c.Int("Commits", int64(len(out.Commits)))
	c.Int("Changed Files", int64(len(out.Diffs)))
	c.URL(out.WebURL)
	if len(out.Commits) > 0 {
		t := c.Table("Commits", "Short ID", "Title", "Author")
		for _, commit := range out.Commits {
			t.Row(
				toolutil.MdCodeSpanCell(commit.ShortID),
				toolutil.EscapeMdTableCell(commit.Title),
				toolutil.EscapeMdTableCell(commit.AuthorName),
			)
		}
	}
	if len(out.Diffs) > 0 {
		t := c.Table("Changed Files", "Status", "Path")
		for _, d := range out.Diffs {
			t.Row(diffStatus(d), toolutil.MdCodeSpanCell(d.NewPath))
		}
	}
	c.End(
		toolutil.HintAction(actionCommitGet, "view one of these commits"),
		toolutil.HintAction(actionFileGet, "read a changed file"),
	)
	return b.String()
}

// diffStatus names what happened to a file in a diff.
func diffStatus(d DiffOutput) string {
	switch {
	case d.NewFile:
		return "added"
	case d.DeletedFile:
		return "deleted"
	case d.RenamedFile:
		return "renamed"
	default:
		return "modified"
	}
}

// FormatContributorsMarkdown renders a page of repository contributors as a
// Markdown table.
func FormatContributorsMarkdown(out ContributorsOutput) string {
	if len(out.Contributors) == 0 {
		return toolutil.EmptyMessage("contributors")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Repository Contributors", len(out.Contributors), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Email", "Commits", "Additions", "Deletions"))
	for _, c := range out.Contributors {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(c.Name),
			toolutil.EscapeMdTableCell(c.Email),
			strconv.FormatInt(c.Commits, 10),
			strconv.FormatInt(c.Additions, 10),
			strconv.FormatInt(c.Deletions, 10),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionCommitList, "view commits by one contributor"),
	)
	return b.String()
}

// FormatBlobMarkdown renders a blob's metadata as a card. The bytes are not
// printed here: raw_blob is the action that decodes them.
func FormatBlobMarkdown(out BlobOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Repository Blob")
	// The blob SHA is not GitLab's echo of anything it validated: it is the
	// caller's own argument, filtered only by a remote endpoint answering 200.
	c.Code("SHA", out.SHA)
	c.Int("Size (bytes)", int64(out.Size))
	switch out.ContentCategory {
	case "image":
		c.Field("Content Type", "image ("+out.ImageMIMEType+")")
		c.Note(imageNote)
	case "binary":
		c.Field("Content Type", "binary (content omitted, not viewable as text)")
	default:
		c.Field("Content Type", "text")
		c.Int("Characters", int64(len(out.Content)))
	}
	c.End(toolutil.HintAction(actionRawBlob, "read the decoded text content"))
	return b.String()
}

// FormatRawBlobContentMarkdown renders a blob's decoded content as a card
// whose body is fenced.
func FormatRawBlobContentMarkdown(out RawBlobContentOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Repository Raw Blob Content")
	// The blob SHA is not GitLab's echo of anything it validated: it is the
	// caller's own argument, filtered only by a remote endpoint answering 200.
	c.Code("SHA", out.SHA)
	c.Int("Size (bytes)", int64(out.Size))
	switch out.ContentCategory {
	case "image":
		c.Field("Content Type", "image ("+out.ImageMIMEType+")")
		c.Note(imageNote)
	case "binary":
		c.Field("Content Type", "binary (content omitted, not viewable as text)")
	default:
		// The blob is a file of the repository, so a fixed fence is closed by
		// the first run of three backticks whoever pushed it wrote.
		c.Fence("", "", out.Content)
	}
	c.End(toolutil.HintAction(actionFileGet, "view the file with its metadata"))
	return b.String()
}

func blobResult(out BlobOutput) *mcp.CallToolResult {
	if out.ContentCategory == "image" {
		return toolutil.ToolResultWithImage(FormatBlobMarkdown(out), toolutil.ContentAssistant, out.ImageData, out.ImageMIMEType)
	}
	return toolutil.ToolResultWithMarkdown(FormatBlobMarkdown(out))
}

func rawBlobResult(out RawBlobContentOutput) *mcp.CallToolResult {
	if out.ContentCategory == "image" {
		return toolutil.ToolResultWithImage(FormatRawBlobContentMarkdown(out), toolutil.ContentAssistant, out.ImageData, out.ImageMIMEType)
	}
	return toolutil.ToolResultWithMarkdown(FormatRawBlobContentMarkdown(out))
}

// FormatArchiveMarkdown renders the archive download address as a card.
func FormatArchiveMarkdown(out ArchiveOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Repository Archive")
	// Archive makes no API call at all, so all three are the caller's own
	// arguments echoed back, format included: nothing checks it against the
	// eight values its schema lists.
	c.Field("Project", out.ProjectID)
	c.Field("Format", out.Format)
	c.Field("SHA/Ref", out.SHA)
	c.URL(out.URL)
	c.End(toolutil.HintAction(actionTree, "browse the repository instead of downloading it"))
	return b.String()
}

// FormatAddChangelogMarkdown renders the outcome of committing changelog data.
func FormatAddChangelogMarkdown(out AddChangelogOutput) string {
	var b strings.Builder
	if !out.Success {
		c := toolutil.NewCard(&b, "Changelog Update Failed")
		c.Field("Version", out.Version)
		c.End(toolutil.HintAction(actionChangelogGenerate, "generate the notes without committing them"))
		return b.String()
	}
	c := toolutil.NewCard(&b, "Changelog Updated")
	// The version is the caller's own argument echoed back, so it is a row the
	// card escapes rather than bold text interpolated into a sentence.
	c.Field("Version", out.Version)
	c.Note("The changelog data was committed successfully.")
	c.End(toolutil.HintAction(actionChangelogGenerate, "generate the notes for another range"))
	return b.String()
}

// FormatChangelogDataMarkdown renders generated changelog notes.
//
// The notes are fenced rather than written raw: they are release prose built
// out of commit messages, so a commit title carrying a heading, a list item or
// a guidance section used to become part of this response.
func FormatChangelogDataMarkdown(out ChangelogDataOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Generated Changelog Data")
	if out.Notes == "" {
		c.Note("GitLab generated no changelog entries for this range.")
		c.End(toolutil.HintAction(actionChangelogGenerate, "widen the range with from and to"))
		return b.String()
	}
	c.Fence("", "markdown", out.Notes)
	c.End(
		toolutil.HintAction(actionChangelogAdd, "commit this changelog to the repository"),
		toolutil.HintAction(actionReleaseCreate, "create a release with these notes"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTreeMarkdown)
	toolutil.RegisterMarkdown(FormatCompareMarkdown)
	toolutil.RegisterMarkdown(FormatContributorsMarkdown)
	toolutil.RegisterMarkdownResult(blobResult)
	toolutil.RegisterMarkdownResult(rawBlobResult)
	toolutil.RegisterMarkdown(FormatArchiveMarkdown)
	toolutil.RegisterMarkdown(FormatAddChangelogMarkdown)
	toolutil.RegisterMarkdown(FormatChangelogDataMarkdown)
}
