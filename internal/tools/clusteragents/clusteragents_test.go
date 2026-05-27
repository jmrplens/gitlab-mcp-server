// clusteragents_test.go contains unit tests for the cluster agent MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package clusteragents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// errAPIShouldNotCallZeroAgentID identifies the err API should not call zero agent ID constant used by this package.
const errAPIShouldNotCallZeroAgentID = "API should not be called when AgentID is 0"

// errExpectedZeroAgentID identifies the err expected zero agent ID constant used by this package.
const errExpectedZeroAgentID = "expected error for zero AgentID, got nil"

// TestListAgents verifies ListAgents.
func TestListAgents(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"agent1","created_by_user_id":10}]`)
	}))
	out, err := ListAgents(t.Context(), client, ListAgentsInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Agents) != 1 || out.Agents[0].Name != "agent1" {
		t.Errorf("unexpected agents: %+v", out.Agents)
	}
}

// TestListAgents_Error verifies ListAgents when error.
func TestListAgents_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := ListAgents(t.Context(), client, ListAgentsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGetAgent verifies GetAgent.
func TestGetAgent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":5,"name":"agent5"}`)
	}))
	out, err := GetAgent(t.Context(), client, GetAgentInput{ProjectID: "1", AgentID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("expected ID 5, got %d", out.ID)
	}
}

// TestRegisterAgent verifies RegisterAgent.
func TestRegisterAgent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"new-agent"}`)
	}))
	out, err := RegisterAgent(t.Context(), client, RegisterAgentInput{ProjectID: "1", Name: "new-agent"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "new-agent" {
		t.Errorf("expected new-agent, got %s", out.Name)
	}
}

// TestDeleteAgent verifies DeleteAgent.
func TestDeleteAgent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteAgent(t.Context(), client, DeleteAgentInput{ProjectID: "1", AgentID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListAgentTokens verifies ListAgentTokens.
func TestListAgentTokens(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5/tokens" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"token1","agent_id":5,"status":"active"}]`)
	}))
	out, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
}

// TestGetAgentToken verifies GetAgentToken.
func TestGetAgentToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5/tokens/1" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"token1","agent_id":5,"status":"active","token":"secret"}`)
	}))
	out, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "secret" {
		t.Errorf("expected secret, got %s", out.Token)
	}
}

// TestCreateAgentToken verifies CreateAgentToken.
func TestCreateAgentToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"new-token","agent_id":5,"status":"active","token":"newsecret"}`)
	}))
	out, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 5, Name: "new-token"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "newsecret" {
		t.Errorf("expected newsecret, got %s", out.Token)
	}
}

// TestRevokeAgentToken verifies RevokeAgentToken.
func TestRevokeAgentToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5/tokens/1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestFormatAgentsListMarkdown verifies FormatAgentsListMarkdown.
func TestFormatAgentsListMarkdown(t *testing.T) {
	md := FormatAgentsListMarkdown(ListAgentsOutput{Agents: []AgentItem{{ID: 1, Name: "a"}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatTokensListMarkdown verifies FormatTokensListMarkdown.
func TestFormatTokensListMarkdown(t *testing.T) {
	md := FormatTokensListMarkdown(ListAgentTokensOutput{Tokens: []AgentTokenItem{{ID: 1, Name: "t", Status: "active"}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestGetAgent_ZeroAgentID verifies GetAgent when zero agent ID.
func TestGetAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := GetAgent(t.Context(), client, GetAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestDeleteAgent_ZeroAgentID verifies DeleteAgent when zero agent ID.
func TestDeleteAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	err := DeleteAgent(t.Context(), client, DeleteAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestListAgentTokens_ZeroAgentID verifies ListAgentTokens when zero agent ID.
func TestListAgentTokens_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroAgentID verifies GetAgentToken when zero agent ID.
func TestGetAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroTokenID verifies GetAgentToken when zero token ID.
func TestGetAgentToken_ZeroTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TokenID is 0")
	}))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for zero TokenID, got nil")
	}
}

// TestCreateAgentToken_ZeroAgentID verifies CreateAgentToken when zero agent ID.
func TestCreateAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 0, Name: "tok"})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroAgentID verifies RevokeAgentToken when zero agent ID.
func TestRevokeAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroTokenID verifies RevokeAgentToken when zero token ID.
func TestRevokeAgentToken_ZeroTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TokenID is 0")
	}))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for zero TokenID, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// GetAgent — API error
// ---------------------------------------------------------------------------.

// TestGetAgent_APIError verifies GetAgent when API error.
func TestGetAgent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetAgent(t.Context(), client, GetAgentInput{ProjectID: "1", AgentID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// RegisterAgent — API error
// ---------------------------------------------------------------------------.

// TestRegisterAgent_APIError verifies RegisterAgent when API error.
func TestRegisterAgent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := RegisterAgent(t.Context(), client, RegisterAgentInput{ProjectID: "1", Name: "bad"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteAgent — API error
// ---------------------------------------------------------------------------.

// TestDeleteAgent_APIError verifies DeleteAgent when API error.
func TestDeleteAgent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteAgent(t.Context(), client, DeleteAgentInput{ProjectID: "1", AgentID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListAgentTokens — API error
// ---------------------------------------------------------------------------.

// TestListAgentTokens_APIError verifies ListAgentTokens when API error.
func TestListAgentTokens_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetAgentToken — API error
// ---------------------------------------------------------------------------.

// TestGetAgentToken_APIError verifies GetAgentToken when API error.
func TestGetAgentToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// CreateAgentToken — API error, with description
// ---------------------------------------------------------------------------.

// TestCreateAgentToken_APIError verifies CreateAgentToken when API error.
func TestCreateAgentToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 5, Name: "bad"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateAgentToken_WithDescription verifies CreateAgentToken when with description.
func TestCreateAgentToken_WithDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"desc-token","description":"A token with desc","agent_id":5,"status":"active","token":"secret123"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{
		ProjectID:   "1",
		AgentID:     5,
		Name:        "desc-token",
		Description: "A token with desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != "A token with desc" {
		t.Errorf("expected description, got %q", out.Description)
	}
}

// ---------------------------------------------------------------------------
// RevokeAgentToken — API error
// ---------------------------------------------------------------------------.

// TestRevokeAgentToken_APIError verifies RevokeAgentToken when API error.
func TestRevokeAgentToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListAgents — with pagination params
// ---------------------------------------------------------------------------.

// TestListAgents_WithPagination verifies ListAgents when with pagination.
func TestListAgents_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/cluster_agents" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"agent1"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListAgents(t.Context(), client, ListAgentsInput{ProjectID: "1", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(out.Agents))
	}
}

// ---------------------------------------------------------------------------
// ListAgentTokens — with pagination params
// ---------------------------------------------------------------------------.

// TestListAgentTokens_WithPagination verifies ListAgentTokens when with pagination.
func TestListAgentTokens_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/cluster_agents/5/tokens" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"tok","agent_id":5,"status":"active"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 5, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(out.Tokens))
	}
}

// ---------------------------------------------------------------------------
// Formatters — empty lists
// ---------------------------------------------------------------------------.

// TestFormatAgentsListMarkdown_Empty verifies FormatAgentsListMarkdown when empty.
func TestFormatAgentsListMarkdown_Empty(t *testing.T) {
	md := FormatAgentsListMarkdown(ListAgentsOutput{})
	if !strings.Contains(md, "No cluster agents found.") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatTokensListMarkdown_Empty verifies FormatTokensListMarkdown when empty.
func TestFormatTokensListMarkdown_Empty(t *testing.T) {
	md := FormatTokensListMarkdown(ListAgentTokensOutput{})
	if !strings.Contains(md, "No agent tokens found.") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatAgentMarkdown_Content verifies FormatAgentMarkdown when content.
func TestFormatAgentMarkdown_Content(t *testing.T) {
	md := FormatAgentMarkdown(AgentItem{ID: 5, Name: "test-agent"})
	if !strings.Contains(md, "test-agent") {
		t.Errorf("expected agent name, got: %s", md)
	}
}

// TestFormatTokenMarkdown_WithToken verifies FormatTokenMarkdown when with token.
func TestFormatTokenMarkdown_WithToken(t *testing.T) {
	md := FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active", Token: "s3cr3t"})
	if !strings.Contains(md, "s3cr3t") {
		t.Errorf("expected token value, got: %s", md)
	}
}

// TestFormatTokenMarkdown_WithoutToken verifies FormatTokenMarkdown when without token.
func TestFormatTokenMarkdown_WithoutToken(t *testing.T) {
	md := FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active"})
	if strings.Contains(md, "Token") && strings.Contains(md, "s3cr3t") {
		t.Error("should not contain token secret when empty")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs — metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for cluster agent actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := clusterAgentSpecsByTool(t, specs)

	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, toolName := range []string{"gitlab_delete_cluster_agent", "gitlab_revoke_cluster_agent_token"} {
		if !byTool[toolName].Route.Destructive {
			t.Fatalf("%s should be destructive", toolName)
		}
	}
	if byTool["gitlab_list_cluster_agents"].Usage == "" {
		t.Fatal("gitlab_list_cluster_agents should define usage")
	}
	if len(byTool["gitlab_get_cluster_agent"].Aliases) == 0 {
		t.Fatal("gitlab_get_cluster_agent should define aliases")
	}
	if byTool["gitlab_create_cluster_agent_token"].ParameterGuidance["agent_id"].SemanticRole == "" {
		t.Fatal("gitlab_create_cluster_agent_token should define agent_id parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates cluster agent routes through canonical specs.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newClusterAgentRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_agents", "gitlab_list_cluster_agents", map[string]any{"project_id": "1", "page": float64(1), "per_page": float64(20)}},
		{"get_agent", "gitlab_get_cluster_agent", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"register_agent", "gitlab_register_cluster_agent", map[string]any{"project_id": "1", "name": "new-agent"}},
		{"delete_agent", "gitlab_delete_cluster_agent", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"list_tokens", "gitlab_list_cluster_agent_tokens", map[string]any{"project_id": "1", "agent_id": float64(5), "page": float64(1), "per_page": float64(20)}},
		{"get_token", "gitlab_get_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "token_id": float64(1)}},
		{"create_token", "gitlab_create_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "name": "tok", "description": "test token"}},
		{"revoke_token", "gitlab_revoke_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "token_id": float64(1)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_ErrorPaths verifies API errors through canonical routes.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	byTool := clusterAgentSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_agents", "gitlab_list_cluster_agents", map[string]any{"project_id": "1"}},
		{"get_agent", "gitlab_get_cluster_agent", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"register_agent", "gitlab_register_cluster_agent", map[string]any{"project_id": "1", "name": "a"}},
		{"delete_agent", "gitlab_delete_cluster_agent", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"list_tokens", "gitlab_list_cluster_agent_tokens", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"get_token", "gitlab_get_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "token_id": float64(1)}},
		{"create_token", "gitlab_create_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "name": "t"}},
		{"revoke_token", "gitlab_revoke_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "token_id": float64(1)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error for 403", tt.tool)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := clusterAgentSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_cluster_agent", map[string]any{"project_id": "1", "agent_id": float64(5)}},
		{"gitlab_revoke_cluster_agent_token", map[string]any{"project_id": "1", "agent_id": float64(5), "token_id": float64(1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test cluster agent destructive confirmation.",
				Icons:       toolutil.IconRunner,
			})

			st, ct := mcp.NewInMemoryTransports()
			ctx := context.Background()
			serverSession, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatalf("server connect: %v", err)
			}
			mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
				ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					return &mcp.ElicitResult{Action: "decline"}, nil
				},
			})
			session, connectErr := mcpClient.Connect(ctx, ct, nil)
			if connectErr != nil {
				t.Fatalf("client connect: %v", connectErr)
			}
			t.Cleanup(func() {
				session.Close()
				_ = serverSession.Wait()
			})

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for %s declined confirmation", tt.name)
			}
			found := false
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected non-empty text content in %s cancellation result", tt.name)
			}
		})
	}
}

// newClusterAgentRouteSpecs constructs cluster agent route specs test fixtures.
func newClusterAgentRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	agentJSON := `{"id":5,"name":"test-agent","created_by_user_id":10}`
	tokenJSON := `{"id":1,"name":"tok","description":"","agent_id":5,"status":"active","token":"secret"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/cluster_agents", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+agentJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/cluster_agents/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, agentJSON)
	})

	handler.HandleFunc("POST /api/v4/projects/1/cluster_agents", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, agentJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/cluster_agents/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler.HandleFunc("GET /api/v4/projects/1/cluster_agents/5/tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+tokenJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/cluster_agents/5/tokens/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, tokenJSON)
	})

	handler.HandleFunc("POST /api/v4/projects/1/cluster_agents/5/tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, tokenJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/cluster_agents/5/tokens/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return clusterAgentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))
}

// clusterAgentSpecsByTool supports cluster agent specs by tool assertions in clusteragents tests.
func clusterAgentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
