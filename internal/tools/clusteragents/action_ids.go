package clusteragents

// Canonical action IDs this package publishes. They used to name one surface's
// tool ("Use action 'get'", "Use `gitlab_get_cluster_agent_token`"), which is a
// route the default dynamic surface does not answer and the meta surface
// spells differently; the catalog ID is the one form every surface resolves.
//
// The hints a Markdown formatter writes and the RelatedActions an ActionSpec
// carries read this one block rather than each keeping its own. Kept apart,
// the two drifted: the formatters named the catalog IDs and the specs named a
// domain the catalog does not have, so the cross-link a model was invited to
// follow answered "unknown action".
const (
	actionAgentGet    = "admin.cluster_agent_get"
	actionAgentList   = "admin.cluster_agent_list"
	actionTokenGet    = "admin.cluster_agent_token_get"
	actionTokenList   = "admin.cluster_agent_token_list"
	actionTokenCreate = "admin.cluster_agent_token_create"
	actionTokenRevoke = "admin.cluster_agent_token_revoke"

	// A cluster agent is what deploys to an environment, so the two sibling
	// reads worth naming from here are the environments and the deployments
	// that ran against them. The deployment actions are aggregated into the
	// environment group, which is why the ID is not "deployment.list".
	actionEnvironmentList = "environment.list"
	actionDeploymentList  = "environment.deployment_list"
)

// publishedActionIDs returns every canonical action ID this package hands a
// model as a cross-link, which is what the catalog test holds against the
// catalog.
func publishedActionIDs() []string {
	return []string{
		actionAgentGet,
		actionAgentList,
		actionTokenGet,
		actionTokenList,
		actionTokenCreate,
		actionTokenRevoke,
		actionEnvironmentList,
		actionDeploymentList,
	}
}
