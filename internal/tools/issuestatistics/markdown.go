package issuestatistics

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// init registers the [StatisticsOutput] Markdown formatter with the
// package's type-keyed formatter registry.
func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}

// FormatMarkdown renders a [StatisticsOutput] value as the card of one
// statistics object: the all, opened and closed counts GitLab answered with.
//
// The heading names no scope. One type answers the instance, group and project
// routes alike, so the registry has a single key for all three and the label
// the formatter used to take was always the same word whichever route asked:
// every rendering said "All Issue Statistics", which reads as a scope the
// server cannot know. The counts below it are the scope's own.
//
// The zero of each count is an answer GitLab gave ("no issues are open"), so
// the rows are written with [toolutil.Card.Int] rather than the count form that
// hides a zero.
func FormatMarkdown(out StatisticsOutput) string {
	var b strings.Builder
	counts := out.Statistics.Counts
	c := toolutil.NewCard(&b, "Issue Statistics")
	c.Int("All", counts.All)
	c.Int("Opened", counts.Opened)
	c.Int("Closed", counts.Closed)
	c.End(toolutil.HintAction("issue.list", "see the individual issues behind these counts"))
	return b.String()
}
