//go:build e2e

// clusteragents_test.go tests the cluster agent MCP tools against a live GitLab instance.
// Exercises the full lifecycle: register agent → list → get → create token →
// list tokens → get token → revoke token → delete agent, using both individual
// tools and the gitlab_project meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/clusteragents"
)

// TestIndividual_ClusterAgents exercises the cluster agent lifecycle using
// individual MCP tools: register → list → get → create token → list tokens →
// get token → revoke token → delete agent.
func TestIndividual_ClusterAgents(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var agentID int64
	var tokenID int64

	t.Run("Register", func(t *testing.T) {
		out, err := callToolOn[clusteragents.AgentItem](ctx, sess.individual, "gitlab_register_cluster_agent", clusteragents.RegisterAgentInput{
			ProjectID: proj.pidOf(),
			Name:      "e2e-agent",
		})
		requireNoError(t, err, "register cluster agent")
		requireTrue(t, out.ID > 0, "expected agent ID > 0, got %d", out.ID)
		agentID = out.ID
		t.Logf("Registered cluster agent %d (%s)", out.ID, out.Name)
	})

	t.Run("List", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.ListAgentsOutput](ctx, sess.individual, "gitlab_list_cluster_agents", clusteragents.ListAgentsInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list cluster agents")
		requireTrue(t, len(out.Agents) >= 1, "expected at least 1 agent, got %d", len(out.Agents))
		t.Logf("Listed %d cluster agent(s)", len(out.Agents))
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.AgentItem](ctx, sess.individual, "gitlab_get_cluster_agent", clusteragents.GetAgentInput{
			ProjectID: proj.pidOf(),
			AgentID:   agentID,
		})
		requireNoError(t, err, "get cluster agent")
		requireTrue(t, out.ID == agentID, "expected agent %d, got %d", agentID, out.ID)
	})

	t.Run("CreateToken", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.AgentTokenItem](ctx, sess.individual, "gitlab_create_cluster_agent_token", clusteragents.CreateAgentTokenInput{
			ProjectID:   proj.pidOf(),
			AgentID:     agentID,
			Name:        "e2e-token",
			Description: "E2E test token",
		})
		requireNoError(t, err, "create agent token")
		requireTrue(t, out.ID > 0, "expected token ID > 0, got %d", out.ID)
		tokenID = out.ID
		t.Logf("Created agent token %d (%s)", out.ID, out.Name)
	})

	t.Run("ListTokens", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.ListAgentTokensOutput](ctx, sess.individual, "gitlab_list_cluster_agent_tokens", clusteragents.ListAgentTokensInput{
			ProjectID: proj.pidOf(),
			AgentID:   agentID,
		})
		requireNoError(t, err, "list agent tokens")
		requireTrue(t, len(out.Tokens) >= 1, "expected at least 1 token, got %d", len(out.Tokens))
		t.Logf("Listed %d agent token(s)", len(out.Tokens))
	})

	t.Run("GetToken", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[clusteragents.AgentTokenItem](ctx, sess.individual, "gitlab_get_cluster_agent_token", clusteragents.GetAgentTokenInput{
			ProjectID: proj.pidOf(),
			AgentID:   agentID,
			TokenID:   tokenID,
		})
		requireNoError(t, err, "get agent token")
		requireTrue(t, out.ID == tokenID, "expected token %d, got %d", tokenID, out.ID)
	})

	t.Run("RevokeToken", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_revoke_cluster_agent_token", clusteragents.RevokeAgentTokenInput{
			ProjectID: proj.pidOf(),
			AgentID:   agentID,
			TokenID:   tokenID,
		})
		requireNoError(t, err, "revoke agent token")
		t.Logf("Revoked agent token %d", tokenID)
	})

	t.Run("DeleteAgent", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_cluster_agent", clusteragents.DeleteAgentInput{
			ProjectID: proj.pidOf(),
			AgentID:   agentID,
		})
		requireNoError(t, err, "delete cluster agent")
		t.Logf("Deleted cluster agent %d", agentID)
	})
}

// TestMeta_ClusterAgents exercises the cluster agent lifecycle via the
// gitlab_admin meta-tool: register → list → get → create token →
// list tokens → get token → revoke token → delete agent.
func TestMeta_ClusterAgents(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var agentID int64
	var tokenID int64

	t.Run("Meta/ClusterAgent/Register", func(t *testing.T) {
		out, err := callToolOn[clusteragents.AgentItem](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_register",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-agent-meta",
			},
		})
		requireNoError(t, err, "register cluster agent meta")
		requireTrue(t, out.ID > 0, "expected agent ID > 0")
		agentID = out.ID
		t.Logf("Registered cluster agent (meta) %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/ClusterAgent/List", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.ListAgentsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "list cluster agents meta")
		requireTrue(t, len(out.Agents) >= 1, "expected at least 1 agent")
	})

	t.Run("Meta/ClusterAgent/Get", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.AgentItem](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"agent_id":   agentID,
			},
		})
		requireNoError(t, err, "get cluster agent meta")
		requireTrue(t, out.ID == agentID, "expected agent %d, got %d", agentID, out.ID)
	})

	t.Run("Meta/ClusterAgent/CreateToken", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.AgentTokenItem](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_token_create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"agent_id":    agentID,
				"name":        "e2e-token-meta",
				"description": "E2E meta test token",
			},
		})
		requireNoError(t, err, "create agent token meta")
		requireTrue(t, out.ID > 0, "expected token ID > 0")
		tokenID = out.ID
		t.Logf("Created agent token (meta) %d", out.ID)
	})

	t.Run("Meta/ClusterAgent/ListTokens", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		out, err := callToolOn[clusteragents.ListAgentTokensOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_token_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"agent_id":   agentID,
			},
		})
		requireNoError(t, err, "list agent tokens meta")
		requireTrue(t, len(out.Tokens) >= 1, "expected at least 1 token")
	})

	t.Run("Meta/ClusterAgent/GetToken", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[clusteragents.AgentTokenItem](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_token_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"agent_id":   agentID,
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "get agent token meta")
		requireTrue(t, out.ID == tokenID, "expected token %d, got %d", tokenID, out.ID)
	})

	t.Run("Meta/ClusterAgent/RevokeToken", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_token_revoke",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"agent_id":   agentID,
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "revoke agent token meta")
		t.Logf("Revoked agent token (meta) %d", tokenID)
	})

	t.Run("Meta/ClusterAgent/Delete", func(t *testing.T) {
		requireTrue(t, agentID > 0, "agentID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "cluster_agent_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"agent_id":   agentID,
			},
		})
		requireNoError(t, err, "delete cluster agent meta")
		t.Logf("Deleted cluster agent (meta) %d", agentID)
	})
}
