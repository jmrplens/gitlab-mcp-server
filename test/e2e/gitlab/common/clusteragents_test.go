//go:build e2e

// clusteragents_test.go covers a project's cluster agent and its token
// through their whole life on every surface: the agent registered, listed
// and read; a token created, listed, read and revoked, after which GitLab
// answers the read with a not-found rather than a revoked status; and the
// agent deleted. The actions live on the admin tool although a project
// Maintainer may call them, so the run's token is what the Docker stack
// mints and nothing is declared about it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The names the agent and its token are registered under.
//
// A cluster agent's name is a DNS-1123 label, so GitLab caps it at 63
// characters, which a run-scoped name of this suite exceeds on its own.
// Each surface registers its agent in a project of its own, and a name is
// unique within its project, so a fixed short one is enough.
const (
	clusterAgentName      = "e2e-agent"
	clusterAgentTokenName = "e2e-agent-token"
)

// clusterAgentIDs lists the ids of an agent listing.
func clusterAgentIDs(listed []clusteragents.AgentItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, agent := range listed {
		ids = append(ids, agent.ID)
	}
	return ids
}

// clusterAgentTokenIDs lists the ids of a token listing.
func clusterAgentTokenIDs(listed []clusteragents.AgentTokenItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, token := range listed {
		ids = append(ids, token.ID)
	}
	return ids
}

// TestClusterAgents_Lifecycle_AgentAndToken registers an agent in a
// project of each surface's own, lists and reads it, creates a token for
// it, lists and reads the token, revokes it and shows the read refused,
// then deletes the agent and checks the listing lets it go.
//
// Replaces: TestIndividual_ClusterAgents, TestMeta_ClusterAgents
func TestClusterAgents_Lifecycle_AgentAndToken(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("agents"))
		params := map[string]any{"project_id": project.IDParam()}
		name := clusterAgentName

		registered := harness.Do[clusteragents.AgentItem](s, actionClusterAgentRegister, withParams(params, map[string]any{"name": name}))
		if registered.ID == 0 || registered.Name != name {
			e.T.Fatalf("cluster_agent_register answered %+v, want the agent %q with an ID", registered, name)
		}
		agent := withParams(params, map[string]any{"agent_id": registered.ID})

		listed := harness.Do[clusteragents.ListAgentsOutput](s, actionClusterAgentList, params)
		if !containsID(clusterAgentIDs(listed.Agents), registered.ID) {
			e.T.Errorf("the project lists the agents %v, want %d among them", clusterAgentIDs(listed.Agents), registered.ID)
		}
		got := harness.Do[clusteragents.AgentItem](s, actionClusterAgentGet, agent)
		if got.ID != registered.ID || got.Name != name {
			e.T.Errorf("cluster_agent_get answered agent %d %q, want %d %q", got.ID, got.Name, registered.ID, name)
		}

		assertAgentTokenRoundTrips(e, s, agent)

		harness.DoVoid(s, actionClusterAgentDelete, agent)
		remaining := harness.Do[clusteragents.ListAgentsOutput](s, actionClusterAgentList, params)
		if containsID(clusterAgentIDs(remaining.Agents), registered.ID) {
			e.T.Errorf("the project still lists agent %d after its delete", registered.ID)
		}
	})
}

// assertAgentTokenRoundTrips creates a token for the agent, lists and
// reads it, revokes it and shows the read refused afterwards.
func assertAgentTokenRoundTrips(e *harness.Env, s *harness.Session, agent map[string]any) {
	e.T.Helper()
	name := clusterAgentTokenName

	created := harness.Do[clusteragents.AgentTokenItem](s, actionClusterAgentTokenCreate, withParams(agent, map[string]any{"name": name, "description": "minted by the e2e suite"}))
	// The secret itself is never printed: a failing assertion goes to the run
	// log, and the token authenticates an agent against the instance.
	if created.ID == 0 || created.Name != name || created.Token == "" {
		e.T.Fatalf("cluster_agent_token_create answered id %d name %q with a secret: %t, want the token %q with an ID and its secret",
			created.ID, created.Name, created.Token != "", name)
	}
	token := withParams(agent, map[string]any{"token_id": created.ID})

	listed := harness.Do[clusteragents.ListAgentTokensOutput](s, actionClusterAgentTokenList, agent)
	if !containsID(clusterAgentTokenIDs(listed.Tokens), created.ID) {
		e.T.Errorf("the agent lists the tokens %v, want %d among them", clusterAgentTokenIDs(listed.Tokens), created.ID)
	}
	got := harness.Do[clusteragents.AgentTokenItem](s, actionClusterAgentTokenGet, token)
	if got.ID != created.ID || got.Name != name {
		e.T.Errorf("cluster_agent_token_get answered token %d %q, want %d %q", got.ID, got.Name, created.ID, name)
	}

	harness.DoVoid(s, actionClusterAgentTokenRevoke, token)
	// A revoked token is gone rather than readable with a revoked status:
	// GitLab 19.3 answers the read with a 404, which the old suite found
	// by asserting the status first and failing.
	refused := harness.Refused(s, actionClusterAgentTokenGet, token, harness.FailureNotFound)
	e.T.Logf("the read of the revoked token is refused: %s", firstLine(refused))
}
