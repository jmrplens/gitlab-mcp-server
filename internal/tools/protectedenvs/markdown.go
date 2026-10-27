// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package protectedenvs

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/protectedenvdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one protected environment as the card of a
// single object: the approvals it needs, then the deploy access levels and the
// approval rules as nested collections, as [protectedenvdata.WriteRules]
// writes them for a group protected environment too.
func FormatOutputMarkdown(pe Output) string {
	if pe.Name == "" {
		return ""
	}
	var b strings.Builder
	// An environment name is written in .gitlab-ci.yml or typed in the UI.
	c := toolutil.NewCard(&b, "Protected Environment: "+pe.Name)
	protectedenvdata.WriteRules(c, pe.RequiredApprovalCount, pe.DeployAccessLevels, pe.ApprovalRules)
	c.End(
		toolutil.HintAction(actionEnvProtectedUpdate, "change the deploy access levels or approval rules"),
		toolutil.HintAction(actionEnvProtectedUnprotect, "remove this environment's protection"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of a project's protected environments as a
// Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return toolutil.EmptyMessage("protected environments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Protected Environments", len(out.Environments), out.Pagination)
	b.WriteString(protectedenvdata.ListHeader())
	for _, pe := range out.Environments {
		b.WriteString(protectedenvdata.ListRow(pe.Name, pe.RequiredApprovalCount, pe.DeployAccessLevels, pe.ApprovalRules))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionEnvProtectedGet, "see one environment's rules in full"),
		toolutil.HintAction(actionEnvProtectedProtect, "protect another environment or wildcard"),
		toolutil.HintAction(actionEnvProtectedList, "page through the rest of the project's protected environments"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
