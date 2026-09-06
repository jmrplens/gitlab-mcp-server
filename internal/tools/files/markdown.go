package files

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const fmtSizeBytes = "- **Size**: %d bytes\n"

type fileNotFoundOutput struct {
	Identifier string `json:"identifier"`
}

func formatFileNotFound(out fileNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult("File", out.Identifier,
		"Use gitlab_file_metadata to verify the path and ref",
		"Use gitlab_repository_tree to list repository paths")
}

// FormatOutputMarkdown renders file metadata as a Markdown summary.
// For image and binary files, it includes content type information instead of content.
func FormatOutputMarkdown(f Output) string {
	if f.FilePath == "" {
		return ""
	}
	var b strings.Builder
	// A repository path is whatever whoever added the file named it, and git
	// forbids only NUL and the separator inside a path component.
	fmt.Fprintf(&b, "## File: %s\n\n", toolutil.EscapeMdHeading(f.FilePath))
	fmt.Fprintf(&b, fmtSizeBytes, f.Size)
	fmt.Fprintf(&b, "- **Ref**: %s\n", toolutil.EscapeMdTableCell(f.Ref))
	//gitlab:allow-unescaped f.Encoding: a content encoding GitLab emits as the literal "base64", never text anybody typed.
	fmt.Fprintf(&b, "- **Encoding**: %s\n", f.Encoding)
	//gitlab:allow-unescaped f.BlobID: a git blob object id, hexadecimal digits only.
	fmt.Fprintf(&b, "- **Blob ID**: %s\n", f.BlobID)
	switch f.ContentCategory {
	case "image":
		//gitlab:allow-unescaped f.ImageMIMEType: a MIME type this package derived from the file extension through toolutil.ImageMIMEType, which returns a compiled-in constant.
		fmt.Fprintf(&b, "- **Content type**: image (%s)\n", f.ImageMIMEType)
		b.WriteString("\n> \U0001F5BC\uFE0F Image content is attached below as ImageContent for multimodal viewing.\n")
	case "binary":
		b.WriteString("- **Content type**: binary (content omitted, not viewable as text)\n")
	}
	toolutil.WriteHints(
		&b,
		"Use action 'file_update' to modify this file",
		"Use action 'file_blame' to see who changed each line",
		"Use action 'file_delete' to remove this file",
	)
	return b.String()
}

func fileGetResult(out Output) *mcp.CallToolResult {
	md := FormatOutputMarkdown(out)
	switch out.ContentCategory {
	case "image":
		return toolutil.ToolResultWithImage(md, toolutil.ContentDetail, out.ImageData, out.ImageMIMEType)
	case "binary":
		return toolutil.ToolResultAnnotated(md, toolutil.ContentDetail)
	default:
		return toolutil.ToolResultAnnotated(md, toolutil.ContentDetail)
	}
}

// FormatFileInfoMarkdown renders file info (create/update result).
func FormatFileInfoMarkdown(out FileInfoOutput) string {
	var b strings.Builder
	b.WriteString("## File Operation Result\n\n")
	fmt.Fprintf(&b, "- **File**: %s\n", toolutil.EscapeMdTableCell(out.FilePath))
	fmt.Fprintf(&b, "- **Branch**: %s\n", toolutil.EscapeMdTableCell(out.Branch))
	if out.CommitID != "" {
		//gitlab:allow-unescaped out.CommitID: a git commit SHA, hexadecimal digits only.
		fmt.Fprintf(&b, "- **Commit ID**: %s\n", out.CommitID)
	}
	if out.LastCommitID != "" {
		//gitlab:allow-unescaped out.LastCommitID: a git commit SHA, hexadecimal digits only.
		fmt.Fprintf(&b, "- **Last commit ID**: %s\n", out.LastCommitID)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_file_get` to verify the file content",
		"Use `gitlab_commit_list` to see the commit history",
	)
	return b.String()
}

// FormatBlameMarkdown renders blame information as Markdown.
func FormatBlameMarkdown(out BlameOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## File Blame: %s\n\n", toolutil.EscapeMdHeading(out.FilePath))
	if len(out.Ranges) == 0 {
		b.WriteString("No blame data found.\n")
		return b.String()
	}
	for i, r := range out.Ranges {
		// Named rather than sliced inline so the exemption below can refer to
		// it: a directive's expression is cut at its first colon.
		shortID := r.Commit.ID[:minLen(len(r.Commit.ID), 8)]
		//gitlab:allow-unescaped shortID: the first eight characters of a git commit SHA, hexadecimal digits only.
		fmt.Fprintf(&b, "### Range %d: %s (%s)\n\n", i+1,
			toolutil.EscapeMdTableCell(r.Commit.AuthorName), shortID)
		fmt.Fprintf(&b, "**%s**\n\n", toolutil.EscapeMdTableCell(r.Commit.Message))
		b.WriteString(toolutil.MarkdownFencedBlock(langFromPath(out.FilePath), strings.Join(r.Lines, "\n")))
		b.WriteString("\n")
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_commit_get` to view commit details for a blame range",
		"Use `gitlab_file_get` to view the current file content",
	)
	return b.String()
}

// FormatMetaDataMarkdown renders file metadata as Markdown.
func FormatMetaDataMarkdown(out MetaDataOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## File Metadata: %s\n\n", toolutil.EscapeMdHeading(out.FilePath))
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(out.FileName))
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	fmt.Fprintf(&b, "- **Ref**: %s\n", toolutil.EscapeMdTableCell(out.Ref))
	//gitlab:allow-unescaped out.Encoding: a content encoding GitLab emits as the literal "base64", never text anybody typed.
	fmt.Fprintf(&b, "- **Encoding**: %s\n", out.Encoding)
	//gitlab:allow-unescaped out.BlobID: a git blob object id, hexadecimal digits only.
	fmt.Fprintf(&b, "- **Blob ID**: %s\n", out.BlobID)
	fmt.Fprintf(&b, "- **Commit ID**: %s\n", out.CommitID)
	fmt.Fprintf(&b, "- **Last Commit ID**: %s\n", out.LastCommitID)
	//gitlab:allow-unescaped out.SHA256: a SHA-256 digest of the file content, hexadecimal digits only.
	fmt.Fprintf(&b, "- **SHA-256**: %s\n", out.SHA256)
	if out.ExecuteFilemode {
		b.WriteString("- **Executable**: yes\n")
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_file_get` to read the file content",
		"Use `gitlab_file_blame` to see blame information",
	)
	return b.String()
}

// FormatRawMarkdown renders raw file content as Markdown.
func FormatRawMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Raw File: %s\n\n", toolutil.EscapeMdHeading(out.FilePath))
	fmt.Fprintf(&b, fmtSizeBytes+"\n", out.Size)
	b.WriteString(toolutil.MarkdownFencedBlock(langFromPath(out.FilePath), out.Content))
	toolutil.WriteHints(
		&b,
		"Use `gitlab_file_update` to modify this file",
		"Use `gitlab_file_blame` to see who last changed each line",
	)
	return b.String()
}

// FormatRawImageMarkdown renders metadata for a raw image file.
func FormatRawImageMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Image File: %s\n\n", toolutil.EscapeMdHeading(out.FilePath))
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	//gitlab:allow-unescaped out.ImageMIMEType: a MIME type this package derived from the file extension through toolutil.ImageMIMEType, which returns a compiled-in constant.
	fmt.Fprintf(&b, "- **Content type**: %s\n", out.ImageMIMEType)
	b.WriteString("\n> \U0001F5BC\uFE0F Image content is attached below as ImageContent for multimodal viewing.\n")
	return b.String()
}

// FormatRawBinaryMarkdown renders metadata for a raw binary file.
func FormatRawBinaryMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Binary File: %s\n\n", toolutil.EscapeMdHeading(out.FilePath))
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	b.WriteString("- **Content type**: binary (content omitted, not viewable as text)\n")
	toolutil.WriteHints(
		&b,
		"Use `gitlab_file_metadata` to get additional file properties",
	)
	return b.String()
}

func fileRawResult(out RawOutput) *mcp.CallToolResult {
	switch out.ContentCategory {
	case "image":
		md := FormatRawImageMarkdown(out)
		return toolutil.ToolResultWithImage(md, toolutil.ContentAssistant, out.ImageData, out.ImageMIMEType)
	case "binary":
		md := FormatRawBinaryMarkdown(out)
		return toolutil.ToolResultAnnotated(md, toolutil.ContentAssistant)
	default:
		md := FormatRawMarkdown(out)
		return toolutil.ToolResultAnnotated(md, toolutil.ContentAssistant)
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
