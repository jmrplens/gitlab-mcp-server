// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
// card, [iterationdata.FormatCardMarkdown]. [Output] is that package's type,
// so the card is registered there once rather than here and in the group
// iteration package, where the registry kept whichever init ran first.
func FormatOutputMarkdown(out Output) string {
	return iterationdata.FormatCardMarkdown(out)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
