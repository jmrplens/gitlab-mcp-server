package projectstatistics

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders a project's fetch statistics as a card: the total is
// the object's one field, and the per-day counts are the nested collection
// under it.
func FormatMarkdown(out GetOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Statistics (Last 30 Days)")
	c.Int("Total Fetches", out.TotalFetches)
	if len(out.Days) > 0 {
		table := c.Table("Daily Fetches", "Date", "Count")
		for _, d := range out.Days {
			table.Row(toolutil.FormatTime(d.Date), strconv.FormatInt(d.Count, 10))
		}
	}
	c.End("Use fetcher counts to track project activity trends")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}
