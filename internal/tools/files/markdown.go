package files

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionFileGet      = "repository.file_get"
	actionFileUpdate   = "repository.file_update"
	actionFileDelete   = "repository.file_delete"
	actionFileBlame    = "repository.file_blame"
	actionFileMetadata = "repository.file_metadata"
	actionCommitGet    = "repository.commit_get"
	actionCommitList   = "repository.commit_list"
)

// imageNote is the sentence a card writes where the bytes themselves are
// attached to the result rather than printed.
const imageNote = "\U0001F5BC️ Image content is attached below as ImageContent for multimodal viewing."

type fileNotFoundOutput struct {
	Identifier string `json:"identifier"`
}

func formatFileNotFound(out fileNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult("File", out.Identifier,
		"Use gitlab_file_metadata to verify the path and ref",
		"Use gitlab_repository_tree to list repository paths")
}

// FormatOutputMarkdown renders one repository file as a card: its metadata,
// then its body.
//
// A text file's body is fenced here rather than dropped: the card used to fall
// through its content-category switch with nothing for a file that is neither
// an image nor binary, which is every ordinary source file this action is
// asked for.
func FormatOutputMarkdown(f Output) string {
	if f.FilePath == "" {
		return ""
	}
	var b strings.Builder
	// A repository path is whatever whoever added the file named it, and git
	// forbids only NUL and the separator inside a path component.
	c := toolutil.NewCard(&b, "File: "+f.FilePath)
	c.Field("Name", f.FileName)
	c.Int("Size (bytes)", f.Size)
	c.Field("Ref", f.Ref)
	c.Field("Encoding", f.Encoding)
	c.Code("Blob ID", f.BlobID)
	c.Code("Commit ID", f.CommitID)
	c.Code("Last Commit ID", f.LastCommitID)
	c.Code("SHA-256", f.SHA256)
	c.Bool("Executable", f.ExecuteFilemode)
	switch f.ContentCategory {
	case "image":
		c.Field("Content Type", "image ("+f.ImageMIMEType+")")
		c.Note(imageNote)
	case "binary":
		c.Field("Content Type", "binary (content omitted, not viewable as text)")
	default:
		c.Fence("Content", langFromPath(f.FilePath), f.Content)
	}
	c.End(
		toolutil.HintAction(actionFileUpdate, "modify this file"),
		toolutil.HintAction(actionFileBlame, "see who changed each line"),
		toolutil.HintAction(actionFileDelete, "remove this file"),
	)
	return b.String()
}

func fileGetResult(out Output) *mcp.CallToolResult {
	md := FormatOutputMarkdown(out)
	if out.ContentCategory == "image" {
		return toolutil.ToolResultWithImage(md, toolutil.ContentDetail, out.ImageData, out.ImageMIMEType)
	}
	return toolutil.ToolResultAnnotated(md, toolutil.ContentDetail)
}

// FormatFileInfoMarkdown renders the commit a file create, update or delete
// produced as the card of that result.
func FormatFileInfoMarkdown(out FileInfoOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "File Operation Result")
	c.Field("File", out.FilePath)
	c.Field("Branch", out.Branch)
	c.Code("Commit ID", out.CommitID)
	c.Code("Last Commit ID", out.LastCommitID)
	c.End(
		toolutil.HintAction(actionFileGet, "verify the file content"),
		toolutil.HintAction(actionCommitList, "see the commit history"),
	)
	return b.String()
}

// FormatBlameMarkdown renders the blame of a file: one section per range, each
// naming the commit that last touched it and fencing the lines themselves.
func FormatBlameMarkdown(out BlameOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "File Blame: "+out.FilePath)
	if len(out.Ranges) == 0 {
		c.Note("GitLab returned no blame ranges for this file.")
		c.End(toolutil.HintAction(actionFileGet, "read the current file content"))
		return b.String()
	}
	for i, r := range out.Ranges {
		section := c.Section(blameRangeHeading(i, r))
		section.Code("Commit", shortSHA(r.Commit.ID))
		section.Field("Author", r.Commit.AuthorName)
		section.Time("Committed", r.Commit.CommittedDate)
		section.Text("Message", r.Commit.Message)
		section.Fence("", langFromPath(out.FilePath), strings.Join(r.Lines, "\n"))
	}
	c.End(
		toolutil.HintAction(actionCommitGet, "view one blame range's commit in full"),
		toolutil.HintAction(actionFileGet, "read the current file content"),
	)
	return b.String()
}

// blameRangeHeading names one blame range by its position and the commit it
// belongs to. The author's name is a row of the section rather than part of
// the heading: a heading carries no link and no code span, so a name that
// reads as Markdown belongs on a row the card escapes.
func blameRangeHeading(index int, r BlameRangeOutput) string {
	return "Range " + strconv.Itoa(index+1) + ": " + shortSHA(r.Commit.ID)
}

// shortSHA is a git object id abbreviated to the eight characters a reader
// compares by, or the whole id when it is shorter.
func shortSHA(sha string) string {
	return sha[:minLen(len(sha), 8)]
}

// FormatMetaDataMarkdown renders a file's metadata as a card, with no body.
func FormatMetaDataMarkdown(out MetaDataOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "File Metadata: "+out.FilePath)
	c.Field("Name", out.FileName)
	c.Int("Size (bytes)", out.Size)
	c.Field("Ref", out.Ref)
	c.Field("Encoding", out.Encoding)
	c.Code("Blob ID", out.BlobID)
	c.Code("Commit ID", out.CommitID)
	c.Code("Last Commit ID", out.LastCommitID)
	c.Code("SHA-256", out.SHA256)
	c.Bool("Executable", out.ExecuteFilemode)
	c.End(
		toolutil.HintAction(actionFileGet, "read the file content"),
		toolutil.HintAction(actionFileBlame, "see blame information"),
	)
	return b.String()
}

// FormatRawMarkdown renders raw file content as a card whose body is fenced.
func FormatRawMarkdown(out RawOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Raw File: "+out.FilePath)
	c.Int("Size (bytes)", int64(out.Size))
	c.Fence("", langFromPath(out.FilePath), out.Content)
	c.End(
		toolutil.HintAction(actionFileUpdate, "modify this file"),
		toolutil.HintAction(actionFileBlame, "see who last changed each line"),
	)
	return b.String()
}

// FormatRawImageMarkdown renders the metadata of a raw image file; the bytes
// themselves are attached to the result.
func FormatRawImageMarkdown(out RawOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Image File: "+out.FilePath)
	c.Int("Size (bytes)", int64(out.Size))
	c.Field("Content Type", out.ImageMIMEType)
	c.Note(imageNote)
	c.End(toolutil.HintAction(actionFileMetadata, "get additional file properties"))
	return b.String()
}

// FormatRawBinaryMarkdown renders the metadata of a raw binary file, whose
// content is not viewable as text.
func FormatRawBinaryMarkdown(out RawOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Binary File: "+out.FilePath)
	c.Int("Size (bytes)", int64(out.Size))
	c.Field("Content Type", "binary (content omitted, not viewable as text)")
	c.End(toolutil.HintAction(actionFileMetadata, "get additional file properties"))
	return b.String()
}

func fileRawResult(out RawOutput) *mcp.CallToolResult {
	switch out.ContentCategory {
	case "image":
		return toolutil.ToolResultWithImage(FormatRawImageMarkdown(out), toolutil.ContentAssistant, out.ImageData, out.ImageMIMEType)
	case "binary":
		return toolutil.ToolResultAnnotated(FormatRawBinaryMarkdown(out), toolutil.ContentAssistant)
	default:
		return toolutil.ToolResultAnnotated(FormatRawMarkdown(out), toolutil.ContentAssistant)
	}
}

func init() {
	toolutil.RegisterMarkdownResult(formatFileNotFound)
	toolutil.RegisterMarkdownResult(fileGetResult)
	toolutil.RegisterMarkdown(FormatFileInfoMarkdown)
	toolutil.RegisterMarkdown(FormatBlameMarkdown)
	toolutil.RegisterMarkdown(FormatMetaDataMarkdown)
	toolutil.RegisterMarkdownResult(fileRawResult)
}
