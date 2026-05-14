package clusteragents

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for cluster agent tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		clusterAgentReadSpec("cluster_agent_list", toolutil.RouteAction(client, ListAgents), "gitlab_list_cluster_agents"),
		clusterAgentReadSpec("cluster_agent_get", toolutil.RouteAction(client, GetAgent), "gitlab_get_cluster_agent"),
		clusterAgentCreateSpec("cluster_agent_register", toolutil.RouteAction(client, RegisterAgent), "gitlab_register_cluster_agent"),
		clusterAgentDeleteSpec("cluster_agent_delete", toolutil.DestructiveVoidAction(client, DeleteAgent), "gitlab_delete_cluster_agent"),
		clusterAgentReadSpec("cluster_agent_token_list", toolutil.RouteAction(client, ListAgentTokens), "gitlab_list_cluster_agent_tokens"),
		clusterAgentReadSpec("cluster_agent_token_get", toolutil.RouteAction(client, GetAgentToken), "gitlab_get_cluster_agent_token"),
		clusterAgentCreateSpec("cluster_agent_token_create", toolutil.RouteAction(client, CreateAgentToken), "gitlab_create_cluster_agent_token"),
		clusterAgentDeleteSpec("cluster_agent_token_revoke", toolutil.DestructiveVoidAction(client, RevokeAgentToken), "gitlab_revoke_cluster_agent_token"),
	}
}

func clusterAgentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := clusterAgentOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func clusterAgentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, clusterAgentOptions(individualTool))
}

func clusterAgentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := clusterAgentOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func clusterAgentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"cluster-agent"},
		OpenWorld:      true,
		OwnerPackage:   "clusteragents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
