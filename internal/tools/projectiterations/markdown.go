package projectiterations

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats a list of project iterations.
func FormatListMarkdown(out ListOutput) string {
	return iterationdata.FormatListMarkdown("Project Iterations", "No project iterations found.", out.Iterations, out.Pagination)
}

// FormatOutputMarkdown formats a single project iteration.
func FormatOutputMarkdown(out Output) string {
	return iterationdata.FormatOutputMarkdown(out)
}

func iterationState(s int64) string {
	return iterationdata.StateName(s)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
