package projectiterations

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The hints below name the canonical action IDs declared in action_specs.go,
// the one form every surface resolves.

// FormatListMarkdown renders a page of project iterations as the shared
// iteration table, with the project's own title and empty-list sentence.
func FormatListMarkdown(out ListOutput) string {
	return iterationdata.FormatListMarkdown(
		"Project Iterations",
		toolutil.EmptyMessage("project iterations"),
		out.Iterations,
		out.Pagination,
	)
}

// FormatOutputMarkdown renders one project iteration as the shared iteration
// card.
//
// Both iteration packages alias one output type, so the Markdown registry keys
// them together and whichever init runs first answers for both scopes. The
// card therefore names both list actions, in one order, so the two packages
// render the same answer and which registration won stops mattering; this
// card used to carry no hint at all, which made the two scopes differ on
// nothing a reader could have chosen.
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
