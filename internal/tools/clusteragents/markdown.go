package clusteragents

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatAgentsListMarkdown renders cluster agents as a compact Markdown table.
func FormatAgentsListMarkdown(out ListAgentsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Cluster Agents\n\n")
	toolutil.WriteListSummary(&sb, len(out.Agents), out.Pagination)
	if len(out.Agents) == 0 {
		sb.WriteString("No cluster agents found.\n")
		return sb.String()
	}
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name"))
	for _, a := range out.Agents {
		fmt.Fprintf(&sb, "| %d | %s |\n", a.ID, toolutil.EscapeMdTableCell(a.Name))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use action 'get' with agent_id for full agent details")
	return sb.String()
}

// FormatAgentMarkdown renders a single cluster agent summary.
func FormatAgentMarkdown(a AgentItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Cluster Agent\n\n- **ID**: %d\n- **Name**: %s\n", a.ID, a.Name)
	toolutil.WriteHints(&b, "Use action 'list_tokens' to see tokens for this agent")
	return b.String()
}

// FormatTokensListMarkdown renders cluster agent tokens as a compact Markdown table.
func FormatTokensListMarkdown(out ListAgentTokensOutput) string {
	var sb strings.Builder
	sb.WriteString("## Agent Tokens\n\n")
	toolutil.WriteListSummary(&sb, len(out.Tokens), out.Pagination)
	if len(out.Tokens) == 0 {
		sb.WriteString("No agent tokens found.\n")
		return sb.String()
	}
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Status"))
	for _, t := range out.Tokens {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", t.ID, toolutil.EscapeMdTableCell(t.Name), t.Status)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_cluster_agent_token` to view token details")
	return sb.String()
}

// FormatTokenMarkdown renders a single cluster agent token summary.
func FormatTokenMarkdown(t AgentTokenItem) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Agent Token\n\n- **ID**: %d\n- **Name**: %s\n- **Status**: %s\n", t.ID, t.Name, t.Status)
	if t.Token != "" {
		fmt.Fprintf(&sb, "- **Token**: %s\n", t.Token)
	}
	toolutil.WriteHints(&sb, "Store the token value securely — it cannot be retrieved later")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatAgentsListMarkdown)
	toolutil.RegisterMarkdown(FormatAgentMarkdown)
	toolutil.RegisterMarkdown(FormatTokensListMarkdown)
	toolutil.RegisterMarkdown(FormatTokenMarkdown)
}
