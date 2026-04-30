// markdown.go provides Markdown formatting functions for merge request changes MCP tool output.
package mrchanges

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders the list of file changes in a merge request.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR !%d Changes (%d files)\n\n", out.MRIID, len(out.Changes))
	if len(out.Changes) == 0 {
		b.WriteString("No file changes found.\n")
		return b.String()
	}
	var truncated []string
	b.WriteString("| File | Status |\n")
	b.WriteString("| --- | --- |\n")
	for _, c := range out.Changes {
		status := "modified"
		switch {
		case c.NewFile:
			status = "added"
		case c.DeletedFile:
			status = "deleted"
		case c.RenamedFile:
			status = fmt.Sprintf("renamed from %s", c.OldPath)
		}
		if c.Diff == "" && !c.DeletedFile {
			truncated = append(truncated, c.NewPath)
		}
		fmt.Fprintf(&b, "| %s | %s |\n",
			toolutil.EscapeMdTableCell(c.NewPath), status)
	}
	hints := []string{
		"Use 'diff_versions_list' to list all diff versions of this MR",
	}
	if len(truncated) > 0 {
		hints = append(hints,
			fmt.Sprintf("Some file diffs are empty due to GitLab truncation (%s). Use 'diff_versions_list' to get version IDs, then 'diff_version_get' with a version_id to retrieve full diffs",
				strings.Join(truncated, ", ")))
	}
	toolutil.WriteHints(&b, hints...)
	return b.String()
}

// FormatDiffVersionsListMarkdown renders the list of diff versions as markdown.
func FormatDiffVersionsListMarkdown(out DiffVersionsListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Diff Versions (%d)\n\n", len(out.DiffVersions))
	toolutil.WriteListSummary(&b, len(out.DiffVersions), out.Pagination)
	if len(out.DiffVersions) == 0 {
		b.WriteString("No diff versions found.\n")
		return b.String()
	}
	b.WriteString("| ID | State | Head SHA | Base SHA | Created |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, v := range out.DiffVersions {
		short := v.HeadCommitSHA
		if len(short) > 8 {
			short = short[:8]
		}
		baseSHA := v.BaseCommitSHA
		if len(baseSHA) > 8 {
			baseSHA = baseSHA[:8]
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			v.ID, v.State, short, baseSHA, v.CreatedAt)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use action 'diff_version_get' with version ID for detailed diffs")
	return b.String()
}

// FormatDiffVersionGetMarkdown renders a single diff version detail as markdown.
func FormatDiffVersionGetMarkdown(out DiffVersionOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Diff Version %d\n\n", out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdState, out.State)
	fmt.Fprintf(&b, "- **Head SHA**: %s\n", out.HeadCommitSHA)
	fmt.Fprintf(&b, "- **Base SHA**: %s\n", out.BaseCommitSHA)
	fmt.Fprintf(&b, "- **Start SHA**: %s\n", out.StartCommitSHA)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	if out.RealSize != "" {
		fmt.Fprintf(&b, "- **Real Size**: %s\n", out.RealSize)
	}

	if len(out.Commits) > 0 {
		fmt.Fprintf(&b, "\n### Commits (%d)\n\n", len(out.Commits))
		b.WriteString("| SHA | Author | Title |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, c := range out.Commits {
			short := c.ShortID
			if short == "" && len(c.ID) > 8 {
				short = c.ID[:8]
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				short, toolutil.EscapeMdTableCell(c.AuthorName), toolutil.EscapeMdTableCell(c.Title))
		}
	}

	if len(out.Diffs) > 0 {
		fmt.Fprintf(&b, "\n### File Changes (%d)\n\n", len(out.Diffs))
		b.WriteString("| File | Status |\n")
		b.WriteString("| --- | --- |\n")
		for _, d := range out.Diffs {
			status := "modified"
			switch {
			case d.NewFile:
				status = "added"
			case d.DeletedFile:
				status = "deleted"
			case d.RenamedFile:
				status = fmt.Sprintf("renamed from %s", d.OldPath)
			}
			fmt.Fprintf(&b, "| %s | %s |\n",
				toolutil.EscapeMdTableCell(d.NewPath), status)
		}
	}
	toolutil.WriteHints(&b, "Use 'diff_versions_list' to list all diff versions of this MR")
	return b.String()
}

// FormatRawDiffsMarkdown renders the raw diff output as a fenced code block.
func FormatRawDiffsMarkdown(out RawDiffsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR !%d Raw Diffs\n\n", out.MRIID)
	if out.RawDiff == "" {
		b.WriteString("No diffs found.\n")
		return b.String()
	}
	b.WriteString("```diff\n")
	b.WriteString(out.RawDiff)
	if !strings.HasSuffix(out.RawDiff, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("```\n")
	toolutil.WriteHints(&b, "Use action 'changes_get' for file-level change summary")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionsListMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionGetMarkdown)
	toolutil.RegisterMarkdown(FormatRawDiffsMarkdown)
}
