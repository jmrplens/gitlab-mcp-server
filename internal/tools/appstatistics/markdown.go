package appstatistics

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// approximationNote states what GitLab's own documentation says about these
// counts: above ten thousand they are approximations rather than totals. The
// card used to present every figure as exact, which is the one thing a reader
// would act on wrongly.
const approximationNote = "Counts of 10000 and above are approximate rather than exact."

// FormatGetMarkdown renders the instance statistics as the card of one object:
// one row per metric, in the order a reader reads them, then the note about
// what GitLab counts.
func FormatGetMarkdown(out GetOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Application Statistics")
	// Zero is an answer here, not an absence: an instance with no snippets
	// reports none, and hiding the row would read as GitLab not saying.
	c.Int("Active Users", out.ActiveUsers)
	c.Int("Users", out.Users)
	c.Int("Projects", out.Projects)
	c.Int("Groups", out.Groups)
	c.Int("Issues", out.Issues)
	c.Int("Merge Requests", out.MergeRequests)
	c.Int("Notes", out.Notes)
	c.Int("Forks", out.Forks)
	c.Int("Snippets", out.Snippets)
	c.Int("SSH Keys", out.SSHKeys)
	c.Int("Milestones", out.Milestones)
	c.Note(approximationNote)
	c.End("Use individual resource tools to explore specific statistics")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}
