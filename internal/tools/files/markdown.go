package files

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtSizeBytes = "- **Size**: %d bytes\n"

// FormatOutputMarkdown renders file metadata as a Markdown summary.
// For image and binary files, it includes content type information instead of content.
func FormatOutputMarkdown(f Output) string {
	if f.FilePath == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## File: %s\n\n", f.FilePath)
	fmt.Fprintf(&b, fmtSizeBytes, f.Size)
	fmt.Fprintf(&b, "- **Ref**: %s\n", f.Ref)
	fmt.Fprintf(&b, "- **Encoding**: %s\n", f.Encoding)
	fmt.Fprintf(&b, "- **Blob ID**: %s\n", f.BlobID)
	switch f.ContentCategory {
	case "image":
		fmt.Fprintf(&b, "- **Content type**: image (%s)\n", f.ImageMIMEType)
		b.WriteString("\n> 🖼️ Image content is attached below as ImageContent for multimodal viewing.\n")
	case "binary":
		b.WriteString("- **Content type**: binary (content omitted — not viewable as text)\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'file_update' to modify this file",
		"Use action 'file_blame' to see who changed each line",
		"Use action 'file_delete' to remove this file",
	)
	return b.String()
}

// FormatFileInfoMarkdown renders file info (create/update result).
func FormatFileInfoMarkdown(out FileInfoOutput) string {
	var b strings.Builder
	b.WriteString("## File Operation Result\n\n")
	fmt.Fprintf(&b, "- **File**: %s\n", out.FilePath)
	fmt.Fprintf(&b, "- **Branch**: %s\n", out.Branch)
	toolutil.WriteHints(&b,
		"Use `gitlab_file_get` to verify the file content",
		"Use `gitlab_commit_list` to see the commit history",
	)
	return b.String()
}

// FormatBlameMarkdown renders blame information as Markdown.
func FormatBlameMarkdown(out BlameOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## File Blame: %s\n\n", out.FilePath)
	if len(out.Ranges) == 0 {
		b.WriteString("No blame data found.\n")
		return b.String()
	}
	for i, r := range out.Ranges {
		fmt.Fprintf(&b, "### Range %d — %s (%s)\n\n", i+1,
			toolutil.EscapeMdTableCell(r.Commit.AuthorName), r.Commit.ID[:minLen(len(r.Commit.ID), 8)])
		fmt.Fprintf(&b, "**%s**\n\n", toolutil.EscapeMdTableCell(r.Commit.Message))
		fmt.Fprintf(&b, "```%s\n", langFromPath(out.FilePath))
		for _, line := range r.Lines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_get` to view commit details for a blame range",
		"Use `gitlab_file_get` to view the current file content",
	)
	return b.String()
}

// FormatMetaDataMarkdown renders file metadata as Markdown.
func FormatMetaDataMarkdown(out MetaDataOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## File Metadata: %s\n\n", out.FilePath)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.FileName)
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	fmt.Fprintf(&b, "- **Ref**: %s\n", out.Ref)
	fmt.Fprintf(&b, "- **Encoding**: %s\n", out.Encoding)
	fmt.Fprintf(&b, "- **Blob ID**: %s\n", out.BlobID)
	fmt.Fprintf(&b, "- **Commit ID**: %s\n", out.CommitID)
	fmt.Fprintf(&b, "- **Last Commit ID**: %s\n", out.LastCommitID)
	fmt.Fprintf(&b, "- **SHA-256**: %s\n", out.SHA256)
	if out.ExecuteFilemode {
		b.WriteString("- **Executable**: yes\n")
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_file_get` to read the file content",
		"Use `gitlab_file_blame` to see blame information",
	)
	return b.String()
}

// FormatRawMarkdown renders raw file content as Markdown.
func FormatRawMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Raw File: %s\n\n", out.FilePath)
	fmt.Fprintf(&b, fmtSizeBytes+"\n", out.Size)
	fmt.Fprintf(&b, "```%s\n", langFromPath(out.FilePath))
	b.WriteString(out.Content)
	if !strings.HasSuffix(out.Content, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("```\n")
	toolutil.WriteHints(&b,
		"Use `gitlab_file_update` to modify this file",
		"Use `gitlab_file_blame` to see who last changed each line",
	)
	return b.String()
}

// FormatRawImageMarkdown renders metadata for a raw image file.
func FormatRawImageMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Image File: %s\n\n", out.FilePath)
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	fmt.Fprintf(&b, "- **Content type**: %s\n", out.ImageMIMEType)
	b.WriteString("\n> 🖼️ Image content is attached below as ImageContent for multimodal viewing.\n")
	return b.String()
}

// FormatRawBinaryMarkdown renders metadata for a raw binary file.
func FormatRawBinaryMarkdown(out RawOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Binary File: %s\n\n", out.FilePath)
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	b.WriteString("- **Content type**: binary (content omitted — not viewable as text)\n")
	toolutil.WriteHints(&b,
		"Use `gitlab_file_metadata` to get additional file properties",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatFileInfoMarkdown)
	toolutil.RegisterMarkdown(FormatBlameMarkdown)
	toolutil.RegisterMarkdown(FormatMetaDataMarkdown)
	toolutil.RegisterMarkdown(FormatRawMarkdown)
}
