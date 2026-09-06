package mrchanges

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
			// The old path is a repository path a committer chose, and git
			// allows every byte but NUL and the separator inside a component.
			status = "renamed from " + toolutil.EscapeMdTableCell(c.OldPath)
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
			//gitlab:allow-unescaped v.State: a merge request diff state GitLab records, one of its own fixed set.
			//gitlab:allow-unescaped short: the head commit SHA, hexadecimal digits git computed, truncated here.
			//gitlab:allow-unescaped baseSHA: the base commit SHA, hexadecimal digits git computed, truncated here.
			//gitlab:allow-unescaped v.CreatedAt: a timestamp this package formatted with time.Time.Format as RFC 3339.
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
	//gitlab:allow-unescaped out.State: a merge request diff state GitLab records, one of its own fixed set.
	fmt.Fprintf(&b, toolutil.FmtMdState, out.State)
	//gitlab:allow-unescaped out.HeadCommitSHA: a commit SHA, hexadecimal digits git computed.
	fmt.Fprintf(&b, "- **Head SHA**: %s\n", out.HeadCommitSHA)
	//gitlab:allow-unescaped out.BaseCommitSHA: a commit SHA, hexadecimal digits git computed.
	fmt.Fprintf(&b, "- **Base SHA**: %s\n", out.BaseCommitSHA)
	//gitlab:allow-unescaped out.StartCommitSHA: a commit SHA, hexadecimal digits git computed.
	fmt.Fprintf(&b, "- **Start SHA**: %s\n", out.StartCommitSHA)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	if out.RealSize != "" {
		//gitlab:allow-unescaped out.RealSize: GitLab's own count of the files in the diff, decimal digits with a trailing plus when the diff overflowed.
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
			//gitlab:allow-unescaped short: a commit SHA, hexadecimal digits git computed, truncated here.
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
				status = "renamed from " + toolutil.EscapeMdTableCell(d.OldPath)
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
	b.WriteString(toolutil.MarkdownFencedBlock("diff", out.RawDiff))
	toolutil.WriteHints(&b, "Use action 'changes_get' for file-level change summary")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionsListMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionGetMarkdown)
	toolutil.RegisterMarkdown(FormatRawDiffsMarkdown)
}
