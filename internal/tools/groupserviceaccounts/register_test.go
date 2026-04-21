// Package groupserviceaccounts register_test exercises all RegisterTools closures
// via MCP in-memory transport, covering every handler wired in register.go.
package groupserviceaccounts

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerAccountJSON = `{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}`
const registerAccountsJSON = `[{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}]`
const registerPATJSON = `{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}`
const registerPATsJSON = `[{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}]`

// TestRegisterTools_NoPanic verifies RegisterTools registers all tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 7 group service account tools can
// be called through MCP in-memory transport, covering every handler closure.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusOK, registerAccountsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerPATsJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerPATJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusCreated, registerAccountJSON)
		case r.Method == http.MethodPatch:
			testutil.RespondJSON(w, http.StatusOK, registerAccountJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_list", map[string]any{"group_id": "mygroup"}},
		{"gitlab_group_service_account_create", map[string]any{"group_id": "mygroup", "name": "svc", "username": "svc-user"}},
		{"gitlab_group_service_account_update", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "svc2"}},
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_list", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_create", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
}

// TestRegisterTools_DeleteErrors verifies that both delete and pat_revoke handlers
// return error results when the GitLab API fails, covering if-err-not-nil branches.
func TestRegisterTools_DeleteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.name, err)
			}
			if result == nil || !result.IsError {
				t.Errorf("expected error result from %s with failing backend", tt.name)
			}
		})
	}
}
