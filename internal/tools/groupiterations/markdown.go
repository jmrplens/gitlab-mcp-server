package groupiterations

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionListGroup   = "issue.iteration_list_group"
	actionListProject = "issue.iteration_list_project"
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
// card.
//
// Both iteration packages alias one output type, so the Markdown registry keys
// them together and whichever init runs first answers for both scopes. The
// card therefore names both list actions, in one order, so the two packages
// render the same answer and which registration won stops mattering.
func FormatOutputMarkdown(out Output) string {
	return iterationdata.FormatOutputMarkdown(
		out,
		toolutil.HintAction(actionListGroup, "see every iteration in the group"),
		toolutil.HintAction(actionListProject, "see the iterations a project takes part in"),
	)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
