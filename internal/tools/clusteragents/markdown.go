package clusteragents

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. They used to name one surface's tool
// ("Use action 'get'", "Use `gitlab_get_cluster_agent_token`"), which is a
// route the default dynamic surface does not answer and the meta surface
// spells differently; the catalog ID is the one form every surface resolves.
const (
	actionAgentGet    = "admin.cluster_agent_get"
	actionAgentList   = "admin.cluster_agent_list"
	actionTokenGet    = "admin.cluster_agent_token_get"
	actionTokenList   = "admin.cluster_agent_token_list"
	actionTokenCreate = "admin.cluster_agent_token_create"
	actionTokenRevoke = "admin.cluster_agent_token_revoke"
)

// FormatAgentsListMarkdown renders cluster agents as a compact Markdown table.
func FormatAgentsListMarkdown(out ListAgentsOutput) string {
	if len(out.Agents) == 0 {
		return toolutil.EmptyMessage("cluster agents")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Cluster Agents", len(out.Agents), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name"))
	for _, a := range out.Agents {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(a.ID, 10),
			toolutil.EscapeMdTableCell(a.Name),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		toolutil.HintAction(actionAgentGet, "read one agent with its configuration project"),
		toolutil.HintAction(actionTokenList, "see the tokens issued for an agent"),
	)
	return sb.String()
}

// FormatAgentMarkdown renders a single cluster agent as the card of one object.
func FormatAgentMarkdown(a AgentItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Cluster Agent #%d", a.ID))
	c.Int("ID", a.ID)
	c.Field("Name", a.Name)
	// The agent's timestamps arrive on the output struct in the wire form, so
	// the card is what renders them for a reader.
	c.Time("Created At", a.CreatedAt)
	c.Count("Created By User ID", a.CreatedByUserID)
	// Receptive is stated only when it holds: GitLab connecting out to the
	// agent is the exception, and a cross beside every ordinary agent says
	// nothing a reader needs.
	c.Flag(toolutil.EmojiLink, "Receptive", a.IsReceptive)
	if a.ConfigProject.ID != 0 {
		project := c.Sub("Config Project")
		project.Int("ID", a.ConfigProject.ID)
		project.Field("Name", configProjectName(a.ConfigProject))
	}
	if a.IsReceptive {
		c.Note("A receptive agent is one GitLab connects out to, rather than one that connects in to GitLab.")
	}
	c.End(
		toolutil.HintAction(actionTokenList, "see the tokens issued for this agent"),
		toolutil.HintAction(actionAgentList, "see the other agents in the project"),
	)
	return b.String()
}

// configProjectName is the fullest name GitLab sent for the agent's
// configuration project: its path with namespace, or its bare name.
func configProjectName(cp ConfigProjectOutput) string {
	if cp.PathWithNamespace != "" {
		return cp.PathWithNamespace
	}
	return cp.Name
}

// FormatTokensListMarkdown renders cluster agent tokens as a compact Markdown
// table.
func FormatTokensListMarkdown(out ListAgentTokensOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("agent tokens")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Agent Tokens", len(out.Tokens), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Status"))
	for _, t := range out.Tokens {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(t.Status),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		toolutil.HintAction(actionTokenGet, "read one token's details"),
		toolutil.HintAction(actionTokenCreate, "issue another token for the agent"),
	)
	return sb.String()
}

// FormatTokenMarkdown renders a single cluster agent token as the card of one
// object.
//
// The secret is a code span written by the card rather than a fence of the
// formatter's own: the value has to be copied back verbatim, and a hand-written
// fence is closed by the first backtick run the value happens to contain.
// [toolutil.Card.Secret] also supplies the "store it securely" hint, so the
// sentence is written once, where it also reaches next_steps.
func FormatTokenMarkdown(t AgentTokenItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Agent Token #%d", t.ID))
	c.Int("ID", t.ID)
	c.Field("Name", t.Name)
	c.Field("Status", t.Status)
	// The description is whatever the person who issued the token typed.
	c.Text("Description", t.Description)
	c.Time("Created At", t.CreatedAt)
	c.Time("Last Used At", t.LastUsedAt)
	c.Secret("Token", t.Token)
	c.End(
		toolutil.HintAction(actionTokenList, "see the other tokens issued for this agent"),
		toolutil.HintAction(actionTokenRevoke, "revoke this token"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatAgentsListMarkdown)
	toolutil.RegisterMarkdown(FormatAgentMarkdown)
	toolutil.RegisterMarkdown(FormatTokensListMarkdown)
	toolutil.RegisterMarkdown(FormatTokenMarkdown)
}
