package clusteragents

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for cluster agent tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		clusterAgentReadSpec("cluster_agent_list", toolutil.RouteAction(client, ListAgents), "gitlab_list_cluster_agents"),
		clusterAgentReadSpec("cluster_agent_get", toolutil.RouteAction(client, GetAgent), "gitlab_get_cluster_agent"),
		clusterAgentCreateSpec("cluster_agent_register", toolutil.RouteAction(client, RegisterAgent), "gitlab_register_cluster_agent"),
		clusterAgentDeleteSpec("cluster_agent_delete", toolutil.DestructiveAction(client, DeleteAgentOutput), "gitlab_delete_cluster_agent"),
		clusterAgentReadSpec("cluster_agent_token_list", toolutil.RouteAction(client, ListAgentTokens), "gitlab_list_cluster_agent_tokens"),
		clusterAgentReadSpec("cluster_agent_token_get", toolutil.RouteAction(client, GetAgentToken), "gitlab_get_cluster_agent_token"),
		clusterAgentCreateSpec("cluster_agent_token_create", toolutil.RouteAction(client, CreateAgentToken), "gitlab_create_cluster_agent_token"),
		clusterAgentDeleteSpec("cluster_agent_token_revoke", toolutil.DestructiveAction(client, RevokeAgentTokenOutput), "gitlab_revoke_cluster_agent_token"),
	}
}

func clusterAgentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, clusterAgentOptions(individualTool))
}

func clusterAgentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, clusterAgentOptions(individualTool))
}

func clusterAgentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, clusterAgentOptions(individualTool))
}

func clusterAgentOptions(individualTool string) toolutil.ActionSpecOptions {
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
		OpenWorld:      true,
		OwnerPackage:   "clusteragents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
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
