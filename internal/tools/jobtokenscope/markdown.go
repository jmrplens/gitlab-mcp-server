package jobtokenscope

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatAccessSettingsMarkdown formats access settings as markdown.
func FormatAccessSettingsMarkdown(out AccessSettingsOutput) *mcp.CallToolResult {
	status := "disabled"
	if out.InboundEnabled {
		status = "enabled"
	}
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("## Job Token Access Settings\n\nInbound access: **%s**", status))
}

// FormatPatchResultMarkdown formats the patch result as markdown.
func FormatPatchResultMarkdown(out toolutil.DeleteOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Job token access settings %s successfully.", out.Status))
}

// FormatListInboundAllowlistMarkdown formats the inbound allowlist as markdown.
func FormatListInboundAllowlistMarkdown(out ListInboundAllowlistOutput) *mcp.CallToolResult {
	if len(out.Projects) == 0 {
		return toolutil.ToolResultWithMarkdown("No projects in the job token inbound allowlist.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Job Token Inbound Allowlist (%d projects)\n\n", len(out.Projects))
	sb.WriteString("| ID | Name | Path | URL |\n")
	sb.WriteString("|----|------|------|-----|\n")
	for _, p := range out.Projects {
		fmt.Fprintf(&sb, "| %d | %s | %s | [View](%s) |\n", p.ID, toolutil.EscapeMdTableCell(p.Name), p.PathWithNamespace, p.WebURL)
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, "Use `gitlab_add_project_job_token_allowlist` to add a project")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatAddProjectAllowlistMarkdown formats the add project result as markdown.
func FormatAddProjectAllowlistMarkdown(out InboundAllowItemOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Project %d added to inbound allowlist of project %d.", out.TargetProjectID, out.SourceProjectID))
}

// FormatListGroupAllowlistMarkdown formats the group allowlist as markdown.
func FormatListGroupAllowlistMarkdown(out ListGroupAllowlistOutput) *mcp.CallToolResult {
	if len(out.Groups) == 0 {
		return toolutil.ToolResultWithMarkdown("No groups in the job token allowlist.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Job Token Group Allowlist (%d groups)\n\n", len(out.Groups))
	sb.WriteString("| ID | Name | Path | URL |\n")
	sb.WriteString("|----|------|------|-----|\n")
	for _, g := range out.Groups {
		fmt.Fprintf(&sb, "| %d | %s | %s | [View](%s) |\n", g.ID, toolutil.EscapeMdTableCell(g.Name), g.FullPath, g.WebURL)
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, "Use `gitlab_add_group_job_token_allowlist` to add a group")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatAddGroupAllowlistMarkdown formats the add group result as markdown.
func FormatAddGroupAllowlistMarkdown(out GroupAllowlistItemOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Group %d added to allowlist of project %d.", out.TargetGroupID, out.SourceProjectID))
}

func init() {
	toolutil.RegisterMarkdownResult(FormatAccessSettingsMarkdown)
	toolutil.RegisterMarkdownResult(FormatListInboundAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddProjectAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatListGroupAllowlistMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddGroupAllowlistMarkdown)
}
