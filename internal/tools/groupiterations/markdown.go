package groupiterations

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats a list of group iterations.
func FormatListMarkdown(out ListOutput) string {
	return iterationdata.FormatListMarkdown("Group Iterations", "No group iterations found.", out.Iterations, out.Pagination)
}

// FormatOutputMarkdown formats a single group iteration.
func FormatOutputMarkdown(out Output) string {
	return iterationdata.FormatOutputMarkdown(
		out,
		"Use `gitlab_list_group_iterations` to view all iterations",
	)
}

func iterationState(s int64) string {
	return iterationdata.StateName(s)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
