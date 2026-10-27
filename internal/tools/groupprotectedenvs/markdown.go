// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupprotectedenvs

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/protectedenvdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one group-level protected environment as the
// card of a single object: the approvals it needs, then the deploy access
// levels and the approval rules as nested collections, as
// [protectedenvdata.WriteRules] writes them for a project protected
// environment too.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Environment: "+out.Name)
	protectedenvdata.WriteRules(c, out.RequiredApprovalCount, out.DeployAccessLevels, out.ApprovalRules)
	c.End(
		toolutil.HintAction(actionUpdate, "change the deploy access levels or approval rules"),
		toolutil.HintAction(actionUnprotect, "remove this protection from the group"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of a group's protected environments as a
// Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return toolutil.EmptyMessage("group protected environments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Protected Environments", len(out.Environments), out.Pagination)
	b.WriteString(protectedenvdata.ListHeader())
	for _, e := range out.Environments {
		b.WriteString(protectedenvdata.ListRow(e.Name, e.RequiredApprovalCount, e.DeployAccessLevels, e.ApprovalRules))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGet, "see one environment's rules in full"),
		toolutil.HintAction(actionProtect, "protect another environment tier"),
		toolutil.HintAction(actionList, "page through the rest of the group's protected environments"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
