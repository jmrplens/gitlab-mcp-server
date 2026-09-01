package clusteragents

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Per-action individual-tool descriptions following the 1:1-audit R-META
// convention: one sentence on intent, a "Returns:" clause naming the surfaced
// fields, and a "See also:" clause cross-linking sibling tools.
const (
	descListAgents = "List the Kubernetes agents registered for a project. Returns: agents with id, name, created_at, created_by_user_id, a config_project reference (id, name, paths, description, created_at), and pagination metadata. See also: gitlab_get_cluster_agent, gitlab_register_cluster_agent, gitlab_list_cluster_agent_tokens."

	descGetAgent = "Get details about a single Kubernetes agent. Returns: the agent's id, name, created_at, created_by_user_id, and config_project reference (id, name, paths, description, created_at). See also: gitlab_list_cluster_agents, gitlab_list_cluster_agent_tokens, gitlab_delete_cluster_agent."

	descRegisterAgent = "Register a new Kubernetes agent with a project. Returns: the created agent's id, name, created_at, created_by_user_id, and config_project reference. See also: gitlab_list_cluster_agents, gitlab_create_cluster_agent_token, gitlab_delete_cluster_agent."

	descDeleteAgent = "Delete a registered Kubernetes agent and all of its tokens. Returns: a success status and confirmation message. See also: gitlab_list_cluster_agents, gitlab_register_cluster_agent, gitlab_revoke_cluster_agent_token."

	descListAgentTokens = "List the tokens issued for a Kubernetes agent. Returns: tokens with id, name, description, agent_id, status, created_at, created_by_user_id, last_used_at, and pagination metadata (the secret token value is only present at creation). See also: gitlab_get_cluster_agent_token, gitlab_create_cluster_agent_token, gitlab_revoke_cluster_agent_token."

	descTokGet = "Get a single Kubernetes agent token's metadata. Returns: the token's id, name, description, agent_id, status, created_at, created_by_user_id, and last_used_at (the confidential value is not returned for stored tokens). See also: gitlab_list_cluster_agent_tokens, gitlab_create_cluster_agent_token, gitlab_revoke_cluster_agent_token."

	descTokCreate = "Create a new token for a Kubernetes agent. An agent may hold at most two active tokens; revoke an existing token first if creation fails. Returns: the created token's id, name, description, agent_id, status, created_at, created_by_user_id, last_used_at, and the confidential token value (shown only once, store it securely). See also: gitlab_list_cluster_agent_tokens, gitlab_revoke_cluster_agent_token, gitlab_get_cluster_agent."

	descTokRevoke = "Revoke a Kubernetes agent token. Returns: a success status and confirmation message. Revocation is irreversible. The token cannot be reactivated. See also: gitlab_list_cluster_agent_tokens, gitlab_create_cluster_agent_token, gitlab_delete_cluster_agent."
)

// ActionSpecs returns canonical specs for cluster agent tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		clusterAgentReadSpec("cluster_agent_list", toolutil.RouteAction(client, ListAgents), "gitlab_list_cluster_agents", descListAgents),
		clusterAgentReadSpec("cluster_agent_get", toolutil.RouteAction(client, GetAgent), "gitlab_get_cluster_agent", descGetAgent),
		clusterAgentCreateSpec("cluster_agent_register", toolutil.RouteAction(client, RegisterAgent), "gitlab_register_cluster_agent", descRegisterAgent),
		clusterAgentDeleteSpec("cluster_agent_delete", toolutil.DestructiveAction(client, DeleteAgentOutput), "gitlab_delete_cluster_agent", descDeleteAgent),
		clusterAgentReadSpec("cluster_agent_token_list", toolutil.RouteAction(client, ListAgentTokens), "gitlab_list_cluster_agent_tokens", descListAgentTokens),
		clusterAgentReadSpec("cluster_agent_token_get", toolutil.RouteAction(client, GetAgentToken), "gitlab_get_cluster_agent_token", descTokGet),
		clusterAgentCreateSpec("cluster_agent_token_create", toolutil.RouteAction(client, CreateAgentToken), "gitlab_create_cluster_agent_token", descTokCreate),
		clusterAgentDeleteSpec("cluster_agent_token_revoke", toolutil.DestructiveAction(client, RevokeAgentTokenOutput), "gitlab_revoke_cluster_agent_token", descTokRevoke),
	}
}

func clusterAgentReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, clusterAgentOptions(individualTool, description))
}

func clusterAgentCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, clusterAgentOptions(individualTool, description))
}

func clusterAgentDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, clusterAgentOptions(individualTool, description))
}

func clusterAgentOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"cluster-agent"},
		Usage:          "Manage GitLab Kubernetes agents and agent tokens (list/get/register/delete/list tokens/get token/create token/revoke token).",
		RelatedActions: []string{"environment.list", "deployment.list"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path that owns the agent.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"agent_id": {
				SemanticRole:   "cluster_agent_id",
				ValueSource:    "Agent numeric ID from list/get results.",
				ExampleBinding: "params.agent_id:5",
			},
		},
		OpenWorld:    true,
		OwnerPackage: "clusteragents",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}

// DeleteAgentOutput deletes a cluster agent and returns the canonical success message shape.
func DeleteAgentOutput(ctx context.Context, client *gitlabclient.Client, input DeleteAgentInput) (toolutil.DeleteOutput, error) {
	if err := DeleteAgent(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted cluster agent."}, nil
}

// RevokeAgentTokenOutput revokes a cluster agent token and returns the canonical success message shape.
func RevokeAgentTokenOutput(ctx context.Context, client *gitlabclient.Client, input RevokeAgentTokenInput) (toolutil.DeleteOutput, error) {
	if err := RevokeAgentToken(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted cluster agent token."}, nil
}
