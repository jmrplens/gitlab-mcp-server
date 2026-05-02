package snippets

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const hintUpdateSnippet = "Use action 'update' to modify this snippet"

// FormatMarkdown formats a single snippet as markdown.
func FormatMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snippet #%d: %s\n\n", out.ID, out.Title)
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Title | %s |\n", out.Title)
	if out.FileName != "" {
		fmt.Fprintf(&b, "| File Name | %s |\n", out.FileName)
	}
	if out.Description != "" {
		fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	}
	fmt.Fprintf(&b, "| Visibility | %s |\n", out.Visibility)
	fmt.Fprintf(&b, "| Author | %s (@%s) |\n", out.Author.Name, out.Author.Username)
	if out.ProjectID != 0 {
		if pp := extractProjectPath(out.WebURL); pp != "" {
			fmt.Fprintf(&b, "| Project | %s |\n", pp)
		} else {
			fmt.Fprintf(&b, "| Project ID | %d |\n", out.ProjectID)
		}
	}
	fmt.Fprintf(&b, "| Web URL | %s |\n", toolutil.MdTitleLink(out.Title, out.WebURL))
	if len(out.Files) > 0 {
		b.WriteString("\n### Files\n\n")
		b.WriteString("| Path | Raw URL |\n|---|---|\n")
		for _, f := range out.Files {
			fmt.Fprintf(&b, "| %s | %s |\n", toolutil.EscapeMdTableCell(f.Path), toolutil.MdTitleLink(f.Path, f.RawURL))
		}
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'content' to read snippet content",
		hintUpdateSnippet,
		"Use action 'delete' to remove this snippet",
	)
	return b.String()
}

// FormatListMarkdown formats a list of snippets as markdown.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snippets (%d)\n\n", len(out.Snippets))
	toolutil.WriteListSummary(&b, len(out.Snippets), out.Pagination)
	if len(out.Snippets) == 0 {
		b.WriteString("No snippets found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}

	if snippetsHaveProject(out.Snippets) {
		writeProjectSnippetTable(&b, out.Snippets)
	} else {
		writeSimpleSnippetTable(&b, out.Snippets)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with snippet_id for full details",
		"Use action 'create' to add a new snippet",
	)
	return b.String()
}

// FormatContentMarkdown formats snippet content as markdown.
func FormatContentMarkdown(out ContentOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snippet #%d Content\n\n", out.SnippetID)
	b.WriteString("```\n")
	b.WriteString(out.Content)
	b.WriteString("\n```\n")
	toolutil.WriteHints(&b,
		"Use action 'file_content' to get content of a specific file",
		hintUpdateSnippet,
	)
	return b.String()
}

// FormatFileContentMarkdown formats snippet file content as markdown.
func FormatFileContentMarkdown(out FileContentOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snippet #%d File: %s (ref: %s)\n\n", out.SnippetID, out.FileName, out.Ref)
	b.WriteString("```\n")
	b.WriteString(out.Content)
	b.WriteString("\n```\n")
	toolutil.WriteHints(&b,
		"Use action 'content' to get the full snippet content",
		hintUpdateSnippet,
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatContentMarkdown)
	toolutil.RegisterMarkdown(FormatFileContentMarkdown)
}
