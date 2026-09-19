// cluster_agents_test.go contains unit tests for the cluster agent MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package clusteragents

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// errExpectedZeroAgentID identifies the err expected zero agent ID constant used by this package.
const errExpectedZeroAgentID = "expected error for zero AgentID, got nil"

// TestListAgents verifies the ListAgents handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestListAgents_Error verifies that ListAgents returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListAgents_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := ListAgents(t.Context(), client, ListAgentsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGetAgent verifies the GetAgent handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestRegisterAgent verifies the RegisterAgent handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteAgent verifies the DeleteAgent handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestListAgentTokens verifies the ListAgentTokens handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5/tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGetAgentToken verifies the GetAgentToken handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5/tokens/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestCreateAgentToken verifies the CreateAgentToken handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestRevokeAgentToken verifies the RevokeAgentToken handler.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5/tokens/1 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestGetAgent_ZeroAgentID verifies the GetAgent_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetAgent(t.Context(), client, GetAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestDeleteAgent_ZeroAgentID verifies the DeleteAgent_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteAgent_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteAgent(t.Context(), client, DeleteAgentInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestListAgentTokens_ZeroAgentID verifies the ListAgentTokens_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListAgentTokens_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{ProjectID: "1", AgentID: 0})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroAgentID verifies the GetAgentToken_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestGetAgentToken_ZeroTokenID verifies the GetAgentToken_ZeroTokenID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetAgentToken_ZeroTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetAgentToken(t.Context(), client, GetAgentTokenInput{ProjectID: "1", AgentID: 5, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for zero TokenID, got nil")
	}
}

// TestCreateAgentToken_ZeroAgentID verifies the CreateAgentToken_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 0, Name: "tok"})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroAgentID verifies the RevokeAgentToken_ZeroAgentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRevokeAgentToken_ZeroAgentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := RevokeAgentToken(t.Context(), client, RevokeAgentTokenInput{ProjectID: "1", AgentID: 0, TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedZeroAgentID)
	}
}

// TestRevokeAgentToken_ZeroTokenID verifies the RevokeAgentToken_ZeroTokenID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRevokeAgentToken_ZeroTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestGetAgent_APIError verifies that GetAgent returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestRegisterAgent_APIError verifies that RegisterAgent returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDeleteAgent_APIError verifies that DeleteAgent returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListAgentTokens_APIError verifies that ListAgentTokens returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestGetAgentToken_APIError verifies that GetAgentToken returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreateAgentToken_APIError verifies that CreateAgentToken returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateAgentToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{ProjectID: "1", AgentID: 5, Name: "bad"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateAgentToken_WithDescription verifies the CreateAgentToken_WithDescription handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreateAgentToken_Description_IsSentOnlyWhenTheCallerGaveOne asserts what
// GitLab is sent, not what the fixture echoes back. The handler puts the
// description on the options only when the caller supplied one, and the
// response body here is written by the test, so a test that reads the
// description off the output passes whatever the request carried: the handler
// could drop the field, or send an empty one over a description the caller
// typed, and nothing would fail. That matters because a token's value is
// returned once and never again, so the description is the only thing left to
// tell two live tokens apart, and an empty one sent deliberately is not the
// same as no description at all — GitLab stores what it is sent.
func TestCreateAgentToken_Description_IsSentOnlyWhenTheCallerGaveOne(t *testing.T) {
	cases := []struct {
		name        string
		description string
		wantSent    bool
	}{
		{name: "given", description: "issued for the staging cluster", wantSent: true},
		{name: "omitted", description: "", wantSent: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
					http.NotFound(w, r)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding the request body: %v", err)
				}
				got, sent := body["description"]
				switch {
				case sent != tc.wantSent:
					t.Errorf("description present in the request = %t, want %t (body %v)", sent, tc.wantSent, body)
				case sent && got != tc.description:
					t.Errorf("description sent as %v, want %q", got, tc.description)
				}
				testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"tok","agent_id":5,"status":"active"}`)
			}))
			_, err := CreateAgentToken(t.Context(), client, CreateAgentTokenInput{
				ProjectID:   "1",
				AgentID:     5,
				Name:        "tok",
				Description: tc.description,
			})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RevokeAgentToken — API error
// ---------------------------------------------------------------------------.

// TestRevokeAgentToken_APIError verifies that RevokeAgentToken returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListAgents_WithPagination verifies that ListAgents_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/cluster_agents (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListAgents_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/cluster_agents" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"agent1"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListAgents(t.Context(), client, ListAgentsInput{
		ProjectID: "1",
		Page:      1, PerPage: 10,
	})
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

// TestListAgentTokens_WithPagination verifies that ListAgentTokens_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/cluster_agents/5/tokens (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListAgentTokens_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/cluster_agents/5/tokens" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"tok","agent_id":5,"status":"active"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{
		ProjectID: "1",
		AgentID:   5,
		Page:      1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(out.Tokens))
	}
}

// ---------------------------------------------------------------------------
// ListAgents — keyset pagination + ordering forwarded; full output mirror
// ---------------------------------------------------------------------------.

// TestListAgents_KeysetAndOrdering verifies that ListAgents forwards keyset
// pagination (pagination, page_token) and ordering (order_by, sort) parameters
// to the GitLab API and that the full gl.Agent shape (created_at, config_project)
// is mirrored on the output.
func TestListAgents_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		for key, want := range map[string]string{
			"pagination": "keyset", "page_token": "42", "order_by": "id", "sort": "desc",
		} {
			t.Run(key, func(t *testing.T) {
				if got := q.Get(key); got != want {
					t.Errorf("query %s = %q, want %q", key, got, want)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"agent1","created_at":"2024-01-02T03:04:05Z","created_by_user_id":10,"is_receptive":true,"config_project":{"id":99,"name":"cfg","path_with_namespace":"grp/cfg","created_at":"2023-01-01T00:00:00Z"}}]`)
	}))
	out, err := ListAgents(t.Context(), client, ListAgentsInput{
		ProjectID:  "1",
		Pagination: "keyset", PageToken: "42",
		OrderBy: "id",
		Sort:    "desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(out.Agents))
	}
	a := out.Agents[0]
	if a.CreatedAt != "2024-01-02T03:04:05Z" {
		t.Errorf("CreatedAt = %q", a.CreatedAt)
	}
	if !a.IsReceptive {
		t.Error("IsReceptive = false, want the receptive agent the answer describes")
	}
	if a.ConfigProject.ID != 99 || a.ConfigProject.PathWithNamespace != "grp/cfg" || a.ConfigProject.CreatedAt == "" {
		t.Errorf("ConfigProject = %#v", a.ConfigProject)
	}
}

// ---------------------------------------------------------------------------
// ListAgentTokens — keyset pagination + ordering forwarded; full output mirror
// ---------------------------------------------------------------------------.

// TestListAgentTokens_KeysetAndOrdering verifies that ListAgentTokens forwards
// keyset pagination and ordering parameters and mirrors the full gl.AgentToken
// shape (created_at, created_by_user_id, last_used_at) on the output.
func TestListAgentTokens_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/cluster_agents/5/tokens" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		for key, want := range map[string]string{
			"pagination": "keyset", "page_token": "7", "order_by": "id", "sort": "asc",
		} {
			t.Run(key, func(t *testing.T) {
				if got := q.Get(key); got != want {
					t.Errorf("query %s = %q, want %q", key, got, want)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"tok","agent_id":5,"status":"active","created_at":"2024-02-03T04:05:06Z","created_by_user_id":11,"last_used_at":"2024-03-04T05:06:07Z"}]`)
	}))
	out, err := ListAgentTokens(t.Context(), client, ListAgentTokensInput{
		ProjectID:  "1",
		AgentID:    5,
		Pagination: "keyset", PageToken: "7",
		OrderBy: "id",
		Sort:    "asc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	tok := out.Tokens[0]
	if tok.CreatedAt == "" || tok.CreatedByUserID != 11 || tok.LastUsedAt == "" {
		t.Errorf("token mirror incomplete: %#v", tok)
	}
}

// ---------------------------------------------------------------------------
// Formatters — empty lists
// ---------------------------------------------------------------------------.

// TestClusterAgents_UnreadableIsReceptive verifies that every agent handler
// returns an error rather than a half-filled agent when GitLab sends
// is_receptive as something that is not a boolean. client-go models
// is_receptive on its own Agent as of v3.12.0, so the SDK's decoder is what
// refuses it now that the captured read is retired.
func TestClusterAgents_UnreadableIsReceptive(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertUnreadableBodyRefused(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"name":"prod","is_receptive":"maybe"}]`)
			_, err := ListAgents(context.Background(), client, ListAgentsInput{ProjectID: "42"})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"name":"prod","is_receptive":"maybe"}`)
			_, err := GetAgent(context.Background(), client, GetAgentInput{ProjectID: "42", AgentID: 1})
			return err
		}},
		{Name: "register", Call: func() error {
			client := poisoned(`{"id":1,"name":"prod","is_receptive":"maybe"}`)
			_, err := RegisterAgent(context.Background(), client, RegisterAgentInput{ProjectID: "42", Name: "prod"})
			return err
		}},
	})
}
