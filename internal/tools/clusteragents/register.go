// register.go wires clusteragents MCP tools to the MCP server.

package clusteragents

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all cluster agent tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_cluster_agents",
		Title:       toolutil.TitleFromName("gitlab_list_cluster_agents"),
		Description: "List cluster agents for a GitLab project\n\nSee also: gitlab_register_cluster_agent, gitlab_list_cluster_agent_tokens\n\nReturns: JSON array of cluster agents with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAgentsInput) (*mcp.CallToolResult, ListAgentsOutput, error) {
		start := time.Now()
		out, err := ListAgents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_cluster_agents", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAgentsListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_cluster_agent",
		Title:       toolutil.TitleFromName("gitlab_get_cluster_agent"),
		Description: "Get details of a cluster agent\n\nSee also: gitlab_list_cluster_agents, gitlab_list_cluster_agent_tokens\n\nReturns: JSON with cluster agent details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetAgentInput) (*mcp.CallToolResult, AgentItem, error) {
		start := time.Now()
		out, err := GetAgent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_cluster_agent", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAgentMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_register_cluster_agent",
		Title:       toolutil.TitleFromName("gitlab_register_cluster_agent"),
		Description: "Register a new cluster agent for a GitLab project\n\nSee also: gitlab_list_cluster_agents, gitlab_create_cluster_agent_token\n\nReturns: JSON with the registered agent details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RegisterAgentInput) (*mcp.CallToolResult, AgentItem, error) {
		start := time.Now()
		out, err := RegisterAgent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_register_cluster_agent", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAgentMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_cluster_agent",
		Title:       toolutil.TitleFromName("gitlab_delete_cluster_agent"),
		Description: "Delete a cluster agent\n\nSee also: gitlab_list_cluster_agents, gitlab_revoke_cluster_agent_token\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteAgentInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete cluster agent %d from project %s?", input.AgentID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteAgent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_cluster_agent", start, err)
		r, o, _ := toolutil.DeleteResult("cluster agent")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_cluster_agent_tokens",
		Title:       toolutil.TitleFromName("gitlab_list_cluster_agent_tokens"),
		Description: "List tokens for a cluster agent\n\nSee also: gitlab_create_cluster_agent_token, gitlab_get_cluster_agent\n\nReturns: JSON array of agent tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAgentTokensInput) (*mcp.CallToolResult, ListAgentTokensOutput, error) {
		start := time.Now()
		out, err := ListAgentTokens(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_cluster_agent_tokens", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTokensListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_cluster_agent_token",
		Title:       toolutil.TitleFromName("gitlab_get_cluster_agent_token"),
		Description: "Get details of a cluster agent token\n\nSee also: gitlab_list_cluster_agent_tokens, gitlab_revoke_cluster_agent_token\n\nReturns: JSON with agent token details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetAgentTokenInput) (*mcp.CallToolResult, AgentTokenItem, error) {
		start := time.Now()
		out, err := GetAgentToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_cluster_agent_token", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTokenMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_cluster_agent_token",
		Title:       toolutil.TitleFromName("gitlab_create_cluster_agent_token"),
		Description: "Create a token for a cluster agent\n\nSee also: gitlab_list_cluster_agent_tokens, gitlab_get_cluster_agent\n\nReturns: JSON with the created token details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateAgentTokenInput) (*mcp.CallToolResult, AgentTokenItem, error) {
		start := time.Now()
		out, err := CreateAgentToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_cluster_agent_token", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTokenMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_revoke_cluster_agent_token",
		Title:       toolutil.TitleFromName("gitlab_revoke_cluster_agent_token"),
		Description: "Revoke a cluster agent token\n\nSee also: gitlab_list_cluster_agent_tokens, gitlab_create_cluster_agent_token\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RevokeAgentTokenInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke cluster agent token %d for agent %d in project %s?", input.TokenID, input.AgentID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := RevokeAgentToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_revoke_cluster_agent_token", start, err)
		r, o, _ := toolutil.DeleteResult("cluster agent token")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

// RegisterMeta registers the gitlab_cluster_agent meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_agents":        toolutil.RouteAction(client, ListAgents),
		"get_agent":          toolutil.RouteAction(client, GetAgent),
		"register_agent":     toolutil.RouteAction(client, RegisterAgent),
		"delete_agent":       toolutil.DestructiveVoidAction(client, DeleteAgent),
		"list_agent_tokens":  toolutil.RouteAction(client, ListAgentTokens),
		"get_agent_token":    toolutil.RouteAction(client, GetAgentToken),
		"create_agent_token": toolutil.RouteAction(client, CreateAgentToken),
		"revoke_agent_token": toolutil.DestructiveVoidAction(client, RevokeAgentToken),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_cluster_agent",
		Title: toolutil.TitleFromName("gitlab_cluster_agent"),
		Description: `Manage cluster agents and their tokens in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_agents: List cluster agents for a project. Params: project_id (required), page, per_page
- get_agent: Get a single cluster agent. Params: project_id (required), agent_id (required, int)
- register_agent: Register a new cluster agent. Params: project_id (required), name (required)
- delete_agent: Delete a cluster agent. Params: project_id (required), agent_id (required, int)
- list_agent_tokens: List tokens for a cluster agent. Params: project_id (required), agent_id (required, int), page, per_page
- get_agent_token: Get a single agent token. Params: project_id (required), agent_id (required, int), token_id (required, int)
- create_agent_token: Create a token for a cluster agent. Params: project_id (required), agent_id (required, int), name (required), description
- revoke_agent_token: Revoke a cluster agent token. Params: project_id (required), agent_id (required, int), token_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconRunner,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_cluster_agent", routes, nil))
}
