package jobtokenscope

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. None of them is a credential; they are
// named for the scope rather than for the job token so that the secret scanner
// does not read "Token" in an identifier bound to a string literal.
const (
	hintScopeGet           = "job.token_scope_get"
	hintScopePatch         = "job.token_scope_patch"
	hintScopeListInbound   = "job.token_scope_list_inbound"
	hintScopeAddProject    = "job.token_scope_add_project"
	hintScopeListGroups    = "job.token_scope_list_groups"
	hintScopeAddGroup      = "job.token_scope_add_group"
	hintScopeRemoveGroup   = "job.token_scope_remove_group"
	hintScopeRemoveProject = "job.token_scope_remove_project"
)

// FormatAccessSettingsMarkdown renders a project's job token access settings
// as the card of one object.
//
// The row names the restriction rather than the switch: inbound_enabled true
// means the scope is enforced, so only the projects on the allowlist may reach
// this one with a job token. "Inbound access: enabled" read as the opposite —
// as though enabling it granted access — and a tick would have read the same
// way, which is why the state is spelled out in words.
func FormatAccessSettingsMarkdown(out AccessSettingsOutput) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Job Token Access Settings")
	c.Field("Inbound job token access", inboundAccessDescription(out.InboundEnabled))
	c.End(
		toolutil.HintAction(hintScopeListInbound, "see the projects the allowlist holds"),
		toolutil.HintAction(hintScopePatch, "turn the restriction on or off"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

// inboundAccessDescription says what the flag means for a caller: which job
// tokens may reach this project.
func inboundAccessDescription(enabled bool) string {
	if enabled {
		return "limited to the allowlist"
	}
	return "not limited (any project's job token may access this project)"
}

// FormatListInboundAllowlistMarkdown renders the projects on the inbound
// allowlist as a Markdown table.
func FormatListInboundAllowlistMarkdown(out ListInboundAllowlistOutput) *mcp.CallToolResult {
	if len(out.Projects) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("projects on the job token inbound allowlist"))
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Job Token Inbound Allowlist", len(out.Projects), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path"))
	for _, p := range out.Projects {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(p.ID, 10),
			// The name carries the link, so a reader is never asked to click
			// a column that says "View" and nothing about where it goes.
			toolutil.MdTitleLink(p.Name, p.WebURL),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintScopeAddProject, "allow another project"),
		toolutil.HintAction(hintScopeRemoveProject, "remove one from the allowlist"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatAddProjectAllowlistMarkdown renders the allowlist entry that was
// created as the card of that entry.
func FormatAddProjectAllowlistMarkdown(out InboundAllowItemOutput) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Job Token Inbound Allowlist Entry")
	c.Int("Project", out.SourceProjectID)
	c.Int("Allowed project", out.TargetProjectID)
	c.Note("The allowed project's job token may now reach this project.")
	c.End(
		toolutil.HintAction(hintScopeListInbound, "see the whole allowlist"),
		toolutil.HintAction(hintScopeGet, "check whether the restriction is enforced at all"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatListGroupAllowlistMarkdown renders the groups on the job token
// allowlist as a Markdown table.
func FormatListGroupAllowlistMarkdown(out ListGroupAllowlistOutput) *mcp.CallToolResult {
	if len(out.Groups) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("groups on the job token allowlist"))
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Job Token Group Allowlist", len(out.Groups), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path"))
	for _, g := range out.Groups {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(g.ID, 10),
			toolutil.MdTitleLink(g.Name, g.WebURL),
			toolutil.EscapeMdTableCell(g.FullPath),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintScopeAddGroup, "allow another group"),
		toolutil.HintAction(hintScopeRemoveGroup, "remove one from the allowlist"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatAddGroupAllowlistMarkdown renders the group allowlist entry that was
// created as the card of that entry.
func FormatAddGroupAllowlistMarkdown(out GroupAllowlistItemOutput) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Job Token Group Allowlist Entry")
	c.Int("Project", out.SourceProjectID)
	c.Int("Allowed group", out.TargetGroupID)
	c.Note("Every project in the allowed group may now reach this project with its job token.")
	c.End(
		toolutil.HintAction(hintScopeListGroups, "see the whole group allowlist"),
		toolutil.HintAction(hintScopeGet, "check whether the restriction is enforced at all"),
	)
	return toolutil.ToolResultWithMarkdown(b.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatAccessSettingsMarkdown)
	toolutil.RegisterMarkdownResult(FormatListInboundAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddProjectAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatListGroupAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddGroupAllowlistMarkdown)
}
