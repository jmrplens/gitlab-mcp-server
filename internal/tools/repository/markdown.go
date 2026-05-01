package repository

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatTreeMarkdown renders a paginated repository tree as a Markdown table.
func FormatTreeMarkdown(out TreeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Repository Tree (%d entries)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Tree), out.Pagination)
	if len(out.Tree) == 0 {
		b.WriteString("No files or directories found.\n")
		return b.String()
	}
	b.WriteString("| Type | Name | Path |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, n := range out.Tree {
		icon := toolutil.EmojiFile
		if n.Type == "tree" {
			icon = toolutil.EmojiFolder
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", icon, toolutil.EscapeMdTableCell(n.Name), toolutil.EscapeMdTableCell(n.Path))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use gitlab_repository action 'file_get' with a file path to read file content",
		"Use gitlab_repository action 'compare' to see differences between branches or commits",
	)
	return b.String()
}

// FormatCompareMarkdown renders a repository comparison result.
func FormatCompareMarkdown(out CompareOutput) string {
	var b strings.Builder
	if out.CompareSameRef {
		b.WriteString("## Repository Compare: same ref\n\nBoth references point to the same commit.\n")
		return b.String()
	}
	if out.CompareTimeout {
		b.WriteString("## Repository Compare: timeout\n\nThe comparison timed out. Try with a smaller range.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "## Repository Compare\n\n")
	fmt.Fprintf(&b, "**Commits**: %d | **Diffs**: %d\n\n", len(out.Commits), len(out.Diffs))
	if len(out.Commits) > 0 {
		b.WriteString("### Commits\n\n")
		b.WriteString("| Short ID | Title | Author |\n")
		b.WriteString(toolutil.TblSep3Col)
		for _, c := range out.Commits {
			fmt.Fprintf(&b, toolutil.FmtRow3Str, c.ShortID, toolutil.EscapeMdTableCell(c.Title), toolutil.EscapeMdTableCell(c.AuthorName))
		}
	}
	if len(out.Diffs) > 0 {
		b.WriteString("\n### Changed Files\n\n")
		b.WriteString("| Status | Path |\n")
		b.WriteString("| --- | --- |\n")
		for _, d := range out.Diffs {
			status := "modified"
			if d.NewFile {
				status = "added"
			} else if d.DeletedFile {
				status = "deleted"
			} else if d.RenamedFile {
				status = "renamed"
			}
			fmt.Fprintf(&b, "| %s | %s |\n", status, toolutil.EscapeMdTableCell(d.NewPath))
		}
	}
	if out.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURLNewline, out.WebURL)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_get` to view a specific commit",
		"Use `gitlab_file_get` to read a changed file",
	)
	return b.String()
}

// FormatContributorsMarkdown renders a list of contributors.
func FormatContributorsMarkdown(out ContributorsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Repository Contributors (%d)\n\n", len(out.Contributors))
	toolutil.WriteListSummary(&b, len(out.Contributors), out.Pagination)
	if len(out.Contributors) == 0 {
		b.WriteString("No contributors found.\n")
		return b.String()
	}
	b.WriteString("| Name | Email | Commits | Additions | Deletions |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, c := range out.Contributors {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n",
			toolutil.EscapeMdTableCell(c.Name), toolutil.EscapeMdTableCell(c.Email),
			c.Commits, c.Additions, c.Deletions)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_list` to view commits by a contributor",
	)
	return b.String()
}

// FormatBlobMarkdown renders blob metadata.
func FormatBlobMarkdown(out BlobOutput) string {
	var b strings.Builder
	b.WriteString("## Repository Blob\n\n")
	fmt.Fprintf(&b, "- **SHA**: %s\n", out.SHA)
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.Size)
	switch out.ContentCategory {
	case "image":
		fmt.Fprintf(&b, "- **Content type**: image (%s)\n", out.ImageMIMEType)
		b.WriteString("\n> 🖼️ Image content is attached below as ImageContent for multimodal viewing.\n")
	case "binary":
		b.WriteString("- **Content type**: binary (content omitted — not viewable as text)\n")
	default:
		fmt.Fprintf(&b, "- **Content**: text (%d chars)\n", len(out.Content))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_repository_raw_blob` for decoded text content",
	)
	return b.String()
}

// FormatRawBlobContentMarkdown renders raw blob content.
func FormatRawBlobContentMarkdown(out RawBlobContentOutput) string {
	var b strings.Builder
	b.WriteString("## Repository Raw Blob Content\n\n")
	fmt.Fprintf(&b, "- **SHA**: %s\n", out.SHA)
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.Size)
	switch out.ContentCategory {
	case "image":
		fmt.Fprintf(&b, "- **Content type**: image (%s)\n", out.ImageMIMEType)
		b.WriteString("\n> 🖼️ Image content is attached below as ImageContent for multimodal viewing.\n")
	case "binary":
		b.WriteString("- **Content type**: binary (content omitted — not viewable as text)\n")
	default:
		b.WriteString("\n```\n")
		b.WriteString(out.Content)
		b.WriteString("\n```\n")
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_file_get` to view file with metadata",
	)
	return b.String()
}

// FormatArchiveMarkdown renders archive download info.
func FormatArchiveMarkdown(out ArchiveOutput) string {
	var b strings.Builder
	b.WriteString("## Repository Archive\n\n")
	fmt.Fprintf(&b, "- **Project**: %s\n", out.ProjectID)
	fmt.Fprintf(&b, "- **Format**: %s\n", out.Format)
	if out.SHA != "" {
		fmt.Fprintf(&b, "- **SHA/Ref**: %s\n", out.SHA)
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, out.URL)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_repository_tree` to browse the repository instead",
	)
	return b.String()
}

// FormatAddChangelogMarkdown renders changelog addition confirmation.
func FormatAddChangelogMarkdown(out AddChangelogOutput) string {
	var b strings.Builder
	if out.Success {
		fmt.Fprintf(&b, "## Changelog Updated\n\nVersion **%s** changelog data committed successfully.\n", out.Version)
	} else {
		b.WriteString("## Changelog Update Failed\n")
	}
	toolutil.WriteHints(&b, "Use gitlab_repository action 'get_changelog' to view the full changelog")
	return b.String()
}

// FormatChangelogDataMarkdown renders generated changelog notes.
func FormatChangelogDataMarkdown(out ChangelogDataOutput) string {
	var b strings.Builder
	b.WriteString("## Generated Changelog Data\n\n")
	if out.Notes == "" {
		b.WriteString("No changelog entries found.\n")
		return b.String()
	}
	b.WriteString(out.Notes)
	b.WriteString("\n")
	toolutil.WriteHints(&b,
		"Use `gitlab_repository_changelog_add` to commit this changelog to the repository",
		"Use `gitlab_release_create` to create a release with these notes",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTreeMarkdown)
	toolutil.RegisterMarkdown(FormatCompareMarkdown)
	toolutil.RegisterMarkdown(FormatContributorsMarkdown)
	toolutil.RegisterMarkdown(FormatBlobMarkdown)
	toolutil.RegisterMarkdown(FormatRawBlobContentMarkdown)
	toolutil.RegisterMarkdown(FormatArchiveMarkdown)
	toolutil.RegisterMarkdown(FormatAddChangelogMarkdown)
	toolutil.RegisterMarkdown(FormatChangelogDataMarkdown)
}
