// markdown_test.go asserts the whole Markdown document every cluster agent
// formatter writes: the two list tables, the agent card with its configuration
// project nested under it, and the token card whose secret is shown once.
//
// Every expectation is the complete document rather than a fragment of one. A
// substring assertion cannot see the defect this migration closes, where a card
// row written into an open table ends the table and renders as literal pipes.
package clusteragents

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// agentHints is the guidance section every agent card closes with.
const agentHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.cluster_agent_token_list' to see the tokens issued for this agent\n" +
	"- Use action 'admin.cluster_agent_list' to see the other agents in the project\n"

// tokenHints is the guidance section every token card closes with.
const tokenHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.cluster_agent_token_list' to see the other tokens issued for this agent\n" +
	"- Use action 'admin.cluster_agent_token_revoke' to revoke this token\n"

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatAgentsListMarkdown verifies the agent table and the sentence an
// empty page renders instead of a heading counting zero.
func TestFormatAgentsListMarkdown(t *testing.T) {
	t.Run("with agents", func(t *testing.T) {
		assertRendered(t, FormatAgentsListMarkdown(ListAgentsOutput{
			Agents: []AgentItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
		}),
			"## Cluster Agents (2)\n\n"+
				"| ID | Name |\n"+
				"| --- | --- |\n"+
				"| 1 | a |\n"+
				"| 2 | b |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'admin.cluster_agent_get' to read one agent with its configuration project\n"+
				"- Use action 'admin.cluster_agent_token_list' to see the tokens issued for an agent\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatAgentsListMarkdown(ListAgentsOutput{}), "No cluster agents found.\n")
	})
}

// TestFormatAgentMarkdown verifies the agent card: its identity, the timestamp
// rendered in the display form rather than the wire form it arrives in, and the
// configuration project as the nested object it is.
func TestFormatAgentMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatAgentMarkdown(AgentItem{
			ID:              5,
			Name:            "test-agent",
			CreatedAt:       "2024-01-02T03:04:05Z",
			CreatedByUserID: 10,
			ConfigProject:   ConfigProjectOutput{ID: 99, Name: "cfg", PathWithNamespace: "grp/cfg"},
		}),
			"## Cluster Agent #5\n\n"+
				"- **ID**: 5\n"+
				"- **Name**: test-agent\n"+
				"- **Created At**: 2 Jan 2024 03:04 UTC\n"+
				"- **Created By User ID**: 10\n"+
				"- **Config Project**:\n"+
				"  - **ID**: 99\n"+
				"  - **Name**: grp/cfg\n"+
				agentHints)
	})
	t.Run("config project name fallback", func(t *testing.T) {
		assertRendered(t, FormatAgentMarkdown(AgentItem{
			ID: 5, Name: "a", ConfigProject: ConfigProjectOutput{ID: 7, Name: "fallback"},
		}),
			"## Cluster Agent #5\n\n"+
				"- **ID**: 5\n"+
				"- **Name**: a\n"+
				"- **Config Project**:\n"+
				"  - **ID**: 7\n"+
				"  - **Name**: fallback\n"+
				agentHints)
	})
}

// TestFormatAgentMarkdown_Receptive verifies that an agent GitLab reports as
// receptive says so and explains what that means, and that an ordinary agent
// carries neither line. The flag is read off the captured response because the
// SDK does not model it, and it decides which way the connection is made.
func TestFormatAgentMarkdown_Receptive(t *testing.T) {
	t.Run("receptive", func(t *testing.T) {
		assertRendered(t, FormatAgentMarkdown(AgentItem{ID: 1, Name: "prod", IsReceptive: true}),
			"## Cluster Agent #1\n\n"+
				"- **ID**: 1\n"+
				"- **Name**: prod\n"+
				"- "+toolutil.EmojiLink+" **Receptive**\n\n"+
				"A receptive agent is one GitLab connects out to, rather than one that connects in to GitLab.\n"+
				agentHints)
	})
	t.Run("ordinary", func(t *testing.T) {
		assertRendered(t, FormatAgentMarkdown(AgentItem{ID: 1, Name: "prod"}),
			"## Cluster Agent #1\n\n"+
				"- **ID**: 1\n"+
				"- **Name**: prod\n"+
				agentHints)
	})
}

// TestFormatTokensListMarkdown verifies the token table and its empty sentence.
func TestFormatTokensListMarkdown(t *testing.T) {
	t.Run("with tokens", func(t *testing.T) {
		assertRendered(t, FormatTokensListMarkdown(ListAgentTokensOutput{
			Tokens: []AgentTokenItem{{ID: 1, Name: "t", Status: "active"}},
		}),
			"## Agent Tokens (1)\n\n"+
				"| ID | Name | Status |\n"+
				"| --- | --- | --- |\n"+
				"| 1 | t | active |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'admin.cluster_agent_token_get' to read one token's details\n"+
				"- Use action 'admin.cluster_agent_token_create' to issue another token for the agent\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatTokensListMarkdown(ListAgentTokensOutput{}), "No agent tokens found.\n")
	})
}

// TestFormatTokenMarkdown verifies the token card: the optional rows appear only
// when GitLab sent them, the secret is a code span the reader copies rather than
// a hand-rolled fence, and showing it adds the hint to store it.
func TestFormatTokenMarkdown(t *testing.T) {
	t.Run("stored token", func(t *testing.T) {
		assertRendered(t, FormatTokenMarkdown(AgentTokenItem{
			ID: 1, Name: "tok", Status: "active",
			Description: "desc", CreatedAt: "2024-02-03T04:05:06Z", LastUsedAt: "2024-03-04T05:06:07Z",
		}),
			"## Agent Token #1\n\n"+
				"- **ID**: 1\n"+
				"- **Name**: tok\n"+
				"- **Status**: active\n"+
				"- **Description**: desc\n"+
				"- **Created At**: 3 Feb 2024 04:05 UTC\n"+
				"- **Last Used At**: 4 Mar 2024 05:06 UTC\n"+
				tokenHints)
	})
	t.Run("freshly created token", func(t *testing.T) {
		assertRendered(t, FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active", Token: "s3cr3t"}),
			"## Agent Token #1\n\n"+
				"- **ID**: 1\n"+
				"- **Name**: tok\n"+
				"- **Status**: active\n"+
				"- **Token**: `s3cr3t`\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Store the token securely. It cannot be retrieved later\n"+
				"- Use action 'admin.cluster_agent_token_list' to see the other tokens issued for this agent\n"+
				"- Use action 'admin.cluster_agent_token_revoke' to revoke this token\n")
	})
	t.Run("no token means no store hint", func(t *testing.T) {
		rendered := FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active"})
		if strings.Contains(rendered, "Store the token") {
			t.Errorf("a stored token must not carry the store-it-securely hint\n---\n%s", rendered)
		}
	})
}

// TestClusterAgentFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestClusterAgentFormattersAreRegistered(t *testing.T) {
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{
			name:     "ListAgentsOutput",
			output:   ListAgentsOutput{Agents: []AgentItem{{ID: 1, Name: "a"}}},
			rendered: FormatAgentsListMarkdown(ListAgentsOutput{Agents: []AgentItem{{ID: 1, Name: "a"}}}),
		},
		{
			name:     "AgentItem",
			output:   AgentItem{ID: 1, Name: "a"},
			rendered: FormatAgentMarkdown(AgentItem{ID: 1, Name: "a"}),
		},
		{
			name:     "ListAgentTokensOutput",
			output:   ListAgentTokensOutput{Tokens: []AgentTokenItem{{ID: 1, Name: "t", Status: "active"}}},
			rendered: FormatTokensListMarkdown(ListAgentTokensOutput{Tokens: []AgentTokenItem{{ID: 1, Name: "t", Status: "active"}}}),
		},
		{
			name:     "AgentTokenItem",
			output:   AgentTokenItem{ID: 1, Name: "t", Status: "active"},
			rendered: FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "t", Status: "active"}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRendered(t, markdownText(t, toolutil.MarkdownForResult(tc.output)), tc.rendered)
		})
	}
}

// markdownText unwraps the single text block a registered string formatter
// produces, failing when the registry returned nothing for the type.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("MarkdownForResult returned nil, want a registered formatter for the type")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}
