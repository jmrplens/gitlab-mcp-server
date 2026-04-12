// clusteragents_test.go contains unit tests for the cluster agent MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package clusteragents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpErr = "unexpected error: %v"

const errAPIShouldNotCallZeroAgentID = "API should not be called when AgentID is 0"
const errExpectedZeroAgentID = "expected error for zero AgentID, got nil"

// TestListAgents verifies the behavior of list agents.
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

// TestListAgents_Error verifies the behavior of list agents error.
func TestListAgents_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := ListAgents(t.Context(), client, ListAgentsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGetAgent verifies the behavior of get agent.
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

// TestRegisterAgent verifies the behavior of register agent.
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

// TestDeleteAgent verifies the behavior of delete agent.
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

// TestListAgentTokens verifies the behavior of list agent tokens.
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

// TestGetAgentToken verifies the behavior of get agent token.
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

// TestCreateAgentToken verifies the behavior of create agent token.
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

// TestRevokeAgentToken verifies the behavior of revoke agent token.
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

// TestFormatAgentsListMarkdown verifies the behavior of format agents list markdown.
func TestFormatAgentsListMarkdown(t *testing.T) {
	md := FormatAgentsListMarkdown(ListAgentsOutput{Agents: []AgentItem{{ID: 1, Name: "a"}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatTokensListMarkdown verifies the behavior of format tokens list markdown.
func TestFormatTokensListMarkdown(t *testing.T) {
	md := FormatTokensListMarkdown(ListAgentTokensOutput{Tokens: []AgentTokenItem{{ID: 1, Name: "t", Status: "active"}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestGetAgent_ZeroAgentID verifies the behavior of get agent zero agent i d.
func TestGetAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := GetAgent(t.Context(), client, GetAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestDeleteAgent_ZeroAgentID verifies the behavior of delete agent zero agent i d.
func TestDeleteAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	err := DeleteAgent(t.Context(), client, DeleteAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestListAgentTokens_ZeroAgentID verifies the behavior of list agent tokens zero agent i d.
func TestListAgentTokens_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroAgentID verifies the behavior of get agent token zero agent i d.
func TestGetAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroTokenID verifies the behavior of get agent token zero token i d.
func TestGetAgentToken_ZeroTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TokenID is 0")
	}))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for zero TokenID, got nil")
	}
}

// TestCreateAgentToken_ZeroAgentID verifies the behavior of create agent token zero agent i d.
func TestCreateAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 0, Name: "tok"})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroAgentID verifies the behavior of revoke agent token zero agent i d.
func TestRevokeAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPIShouldNotCallZeroAgentID)
	}))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroTokenID verifies the behavior of revoke agent token zero token i d.
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

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// GetAgent — API error
// ---------------------------------------------------------------------------.

// TestGetAgent_APIError verifies the behavior of get agent a p i error.
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

// TestRegisterAgent_APIError verifies the behavior of register agent a p i error.
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

// TestDeleteAgent_APIError verifies the behavior of delete agent a p i error.
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

// TestListAgentTokens_APIError verifies the behavior of list agent tokens a p i error.
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

// TestGetAgentToken_APIError verifies the behavior of get agent token a p i error.
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

// TestCreateAgentToken_APIError verifies the behavior of create agent token a p i error.
func TestCreateAgentToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 5, Name: "bad"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateAgentToken_WithDescription verifies the behavior of create agent token with description.
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

// TestRevokeAgentToken_APIError verifies the behavior of revoke agent token a p i error.
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

// TestListAgents_WithPagination verifies the behavior of list agents with pagination.
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

// TestListAgentTokens_WithPagination verifies the behavior of list agent tokens with pagination.
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

// TestFormatAgentsListMarkdown_Empty verifies the behavior of format agents list markdown empty.
func TestFormatAgentsListMarkdown_Empty(t *testing.T) {
	md := FormatAgentsListMarkdown(ListAgentsOutput{})
	if !strings.Contains(md, "No cluster agents found.") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatTokensListMarkdown_Empty verifies the behavior of format tokens list markdown empty.
func TestFormatTokensListMarkdown_Empty(t *testing.T) {
	md := FormatTokensListMarkdown(ListAgentTokensOutput{})
	if !strings.Contains(md, "No agent tokens found.") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatAgentMarkdown_Content verifies the behavior of format agent markdown content.
func TestFormatAgentMarkdown_Content(t *testing.T) {
	md := FormatAgentMarkdown(AgentItem{ID: 5, Name: "test-agent"})
	if !strings.Contains(md, "test-agent") {
		t.Errorf("expected agent name, got: %s", md)
	}
}

// TestFormatTokenMarkdown_WithToken verifies the behavior of format token markdown with token.
func TestFormatTokenMarkdown_WithToken(t *testing.T) {
	md := FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active", Token: "s3cr3t"})
	if !strings.Contains(md, "s3cr3t") {
		t.Errorf("expected token value, got: %s", md)
	}
}

// TestFormatTokenMarkdown_WithoutToken verifies the behavior of format token markdown without token.
func TestFormatTokenMarkdown_WithoutToken(t *testing.T) {
	md := FormatTokenMarkdown(AgentTokenItem{ID: 1, Name: "tok", Status: "active"})
	if strings.Contains(md, "Token") && strings.Contains(md, "s3cr3t") {
		t.Error("should not contain token secret when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newClusterAgentsMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newClusterAgentsMCPSession is an internal helper for the clusteragents package.
func newClusterAgentsMCPSession(t *testing.T) *mcp.ClientSession {
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

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
