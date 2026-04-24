// terraformstates_test.go contains unit tests for the Terraform state MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package terraformstates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const fmtUnexpErr = "unexpected error: %v"

// TestList verifies the behavior of list.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.States) != 1 || out.States[0].Name != "state1" {
		t.Errorf("unexpected states: %+v", out.States)
	}
}

// TestList_Error verifies that List handles the error scenario correctly.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectPath: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3}}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LatestSerial != 3 {
		t.Errorf("expected serial 3, got %d", out.LatestSerial)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectPath: "x", Name: "y"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDelete verifies the behavior of delete.
func TestDelete(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies that Delete handles the error scenario correctly.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteVersion verifies the behavior of delete version.
func TestDeleteVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestLock verifies the behavior of lock.
func TestLock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestLock_Error verifies that Lock handles the error scenario correctly.
func TestLock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already locked"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUnlock verifies the behavior of unlock.
func TestUnlock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestUnlock_Error verifies that Unlock handles the error scenario correctly.
func TestUnlock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"not locked"}`)
	}))
	_, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{States: []StateItem{{Name: "state1", LatestSerial: 3}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// DeleteVersion — error
// ---------------------------------------------------------------------------.

// TestDeleteVersion_Error verifies the behavior of delete version error.
func TestDeleteVersion_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 99})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown
// ---------------------------------------------------------------------------.

// TestFormatStateMarkdown_Coverage verifies the behavior of format state markdown coverage.
func TestFormatStateMarkdown_Coverage(t *testing.T) {
	md := FormatStateMarkdown(StateItem{Name: "prod-state", LatestSerial: 42, DownloadPath: "/dl/path"})
	for _, want := range []string{"prod-state", "42", "/dl/path"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatLockMarkdown
// ---------------------------------------------------------------------------.

// TestFormatLockMarkdown_Coverage verifies the behavior of format lock markdown coverage.
func TestFormatLockMarkdown_Coverage(t *testing.T) {
	md := FormatLockMarkdown(LockOutput{Success: true, Message: "State 'x' locked"})
	if !strings.Contains(md, "true") {
		t.Error("missing success in markdown")
	}
	if !strings.Contains(md, "locked") {
		t.Error("missing lock message in markdown")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{States: nil})
	if !strings.Contains(md, "No Terraform states found") {
		t.Error("missing empty message")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all 6 individual tools
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip validates m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	session := newTerraformMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_terraform_states", map[string]any{"project_path": "group/project"}},
		{"get", "gitlab_get_terraform_state", map[string]any{"project_path": "group/project", "name": "state1"}},
		{"delete", "gitlab_delete_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
		{"delete_version", "gitlab_delete_terraform_state_version", map[string]any{"project_id": "1", "name": "state1", "serial": float64(5)}},
		{"lock", "gitlab_lock_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
		{"unlock", "gitlab_unlock_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
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
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — meta-tool
// ---------------------------------------------------------------------------.

// TestMCPRound_TripMeta validates m c p round trip meta across multiple scenarios using table-driven subtests.
func TestMCPRound_TripMeta(t *testing.T) {
	session := newTerraformMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list", "list", map[string]any{"project_path": "group/project"}},
		{"get", "get", map[string]any{"project_path": "group/project", "name": "state1"}},
		{"delete", "delete", map[string]any{"project_id": "1", "name": "state1"}},
		{"lock", "lock", map[string]any{"project_id": "1", "name": "state1"}},
		{"unlock", "unlock", map[string]any{"project_id": "1", "name": "state1"}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_terraform_state",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(meta/%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(meta/%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(meta/%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory (individual tools)
// ---------------------------------------------------------------------------.

// newTerraformMCPSession is an internal helper for the terraformstates package.
func newTerraformMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := terraformHandler()
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

// ---------------------------------------------------------------------------
// Helper: MCP session factory (meta-tool)
// ---------------------------------------------------------------------------.

// newTerraformMetaMCPSession is an internal helper for the terraformstates package.
func newTerraformMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := terraformHandler()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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

// ---------------------------------------------------------------------------
// Shared mock handler
// ---------------------------------------------------------------------------.

// terraformHandler is an internal helper for the terraformstates package.
func terraformHandler() http.Handler {
	mux := http.NewServeMux()

	graphQLListResp := `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`
	graphQLGetResp := `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3,"downloadPath":"/dl/state1"}}}}}`

	// GraphQL endpoint for List and Get
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		bodyStr := string(body[:n])
		if strings.Contains(bodyStr, "terraformStates") {
			testutil.RespondJSON(w, http.StatusOK, graphQLListResp)
		} else {
			testutil.RespondJSON(w, http.StatusOK, graphQLGetResp)
		}
	})

	// Delete state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete version
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/versions/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Lock state
	mux.HandleFunc("POST /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Unlock state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}
