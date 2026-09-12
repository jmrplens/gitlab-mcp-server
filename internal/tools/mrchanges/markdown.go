package mrchanges

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// diffBudgetBytes bounds how much patch text one changes result carries.
//
// The diffs used to be dropped entirely: the result named the files and never
// showed a line of what changed, so a reviewer had to make a second call for
// every file. They are shown now, in the order GitLab sent them, and a patch
// that no longer fits is named in the note rather than silently omitted, so the
// reader knows the rest is reachable through the raw diff rather than absent.
const diffBudgetBytes = 60_000

// changeStatus is the word a file's row shows: what happened to it, and for a
// rename the path it came from.
func changeStatus(d FileDiffOutput) string {
	switch {
	case d.NewFile:
		return "added"
	case d.DeletedFile:
		return "deleted"
	case d.RenamedFile:
		// The old path is a repository path a committer chose, and git allows
		// every byte but NUL and the separator inside a component.
		return "renamed from " + toolutil.EscapeMdTableCell(d.OldPath)
	default:
		return "modified"
	}
}

// writeChangeTable writes the file table of a set of diffs under the card's
// own heading, or none when there are no files.
func writeChangeTable(c *toolutil.Card, title string, diffs []FileDiffOutput) {
	if len(diffs) == 0 {
		return
	}
	t := c.Table(title, "File", "Status")
	for _, d := range diffs {
		t.Row(toolutil.EscapeMdTableCell(d.NewPath), changeStatus(d))
	}
}

// writeDiffBodies writes each file's patch as a fenced block until the budget
// is spent, and returns the files whose patch was left out. A patch is fenced
// by [toolutil.Card.Fence], which sizes the fence past the longest backtick run
// the patch holds, so a diff of a Markdown file cannot close it early.
func writeDiffBodies(c *toolutil.Card, diffs []FileDiffOutput) []string {
	budget := diffBudgetBytes
	var elided []string
	for _, d := range diffs {
		if d.Diff == "" {
			continue
		}
		if len(d.Diff) > budget {
			elided = append(elided, d.NewPath)
			continue
		}
		budget -= len(d.Diff)
		c.Fence(d.NewPath, "diff", d.Diff)
	}
	return elided
}

// FormatOutputMarkdown renders the file changes of a merge request: the files
// as a collection sharing columns, then each patch as a fenced block.
func FormatOutputMarkdown(out Output) string {
	if len(out.Changes) == 0 {
		return toolutil.EmptyMessage("file changes")
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("MR !%d Changes", out.MRIID))
	c.Int("Files", int64(len(out.Changes)))
	c.Count("Truncated by GitLab", int64(len(out.TruncatedFiles)))
	writeChangeTable(c, "Files", out.Changes)
	elided := writeDiffBodies(c, out.Changes)
	if len(elided) > 0 {
		c.Note(fmt.Sprintf("%d of %d patches are not shown here: the response would be too large. Read them with action '%s'.", len(elided), len(out.Changes), actionRawDiffs))
	}
	hints := []string{toolutil.HintAction(actionDiffVersionsList, "list every diff version of this merge request")}
	if len(out.TruncatedFiles) > 0 {
		// The paths themselves are in truncated_files: naming them here would
		// put a committer's paths into the guidance section, and the structured
		// field is where a caller reads them anyway.
		hints = append(hints, fmt.Sprintf("GitLab truncated %d file diff(s); truncated_files names them. Use action '%s' with a version_id for the full patch", len(out.TruncatedFiles), actionDiffVersionGet))
	}
	c.End(hints...)
	return b.String()
}

// FormatDiffVersionsListMarkdown renders the diff versions of a merge request
// as a Markdown table.
func FormatDiffVersionsListMarkdown(out DiffVersionsListOutput) string {
	if len(out.DiffVersions) == 0 {
		return toolutil.EmptyMessage("diff versions")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Diff Versions", len(out.DiffVersions), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "State", "Head SHA", "Base SHA", "Created"))
	for _, v := range out.DiffVersions {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(v.ID, 10),
			toolutil.EscapeMdTableCell(v.State),
			toolutil.MdCodeSpanCell(shortSHA(v.HeadCommitSHA)),
			toolutil.MdCodeSpanCell(shortSHA(v.BaseCommitSHA)),
			toolutil.FormatTime(v.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionDiffVersionGet, "read one version's commits and file diffs"),
	)
	return b.String()
}

// shortSHA abbreviates a commit SHA to the eight characters GitLab shows, and
// leaves a shorter one alone.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// FormatDiffVersionGetMarkdown renders one diff version as the card of one
// object, with its commits and its file changes as nested collections.
func FormatDiffVersionGetMarkdown(out DiffVersionOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Diff Version %d", out.ID))
	c.Int("ID", out.ID)
	c.Field("State", out.State)
	c.Code("Head SHA", out.HeadCommitSHA)
	c.Code("Base SHA", out.BaseCommitSHA)
	c.Code("Start SHA", out.StartCommitSHA)
	c.Code("Patch ID SHA", out.PatchIDSHA)
	c.Time("Created", out.CreatedAt)
	c.Field("Real Size", out.RealSize)
	if len(out.Commits) > 0 {
		t := c.Table(fmt.Sprintf("Commits (%d)", len(out.Commits)), "SHA", "Author", "Title")
		for _, commit := range out.Commits {
			short := commit.ShortID
			if short == "" {
				short = shortSHA(commit.ID)
			}
			t.Row(
				toolutil.MdCodeSpanCell(short),
				toolutil.EscapeMdTableCell(commit.AuthorName),
				toolutil.EscapeMdTableCell(commit.Title),
			)
		}
	}
	writeChangeTable(c, fmt.Sprintf("File Changes (%d)", len(out.Diffs)), out.Diffs)
	c.End(toolutil.HintAction(actionDiffVersionsList, "list every diff version of this merge request"))
	return b.String()
}

// FormatRawDiffsMarkdown renders the raw patch of a merge request as one fenced
// block.
func FormatRawDiffsMarkdown(out RawDiffsOutput) string {
	if out.RawDiff == "" {
		return toolutil.EmptyMessage("diffs")
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("MR !%d Raw Diffs", out.MRIID))
	c.Fence("", "diff", out.RawDiff)
	c.End(toolutil.HintAction(actionChangesGet, "see the file-by-file change summary"))
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionsListMarkdown)
	toolutil.RegisterMarkdown(FormatDiffVersionGetMarkdown)
	toolutil.RegisterMarkdown(FormatRawDiffsMarkdown)
}
