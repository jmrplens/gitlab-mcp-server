package groupiterations

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a page of group iterations as the shared
// iteration table, with the group's own title and empty-list sentence.
func FormatListMarkdown(out ListOutput) string {
	return iterationdata.FormatListMarkdown(
		"Group Iterations",
		toolutil.EmptyMessage("group iterations"),
		out.Iterations,
		out.Pagination,
	)
}

// FormatOutputMarkdown renders one group iteration as the shared iteration
// card, [iterationdata.FormatCardMarkdown]. [Output] is that package's type,
// so the card is registered there once rather than here and in the project
// iteration package, where the registry kept whichever init ran first.
func FormatOutputMarkdown(out Output) string {
	return iterationdata.FormatCardMarkdown(out)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
