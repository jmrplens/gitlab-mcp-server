// register_test.go contains integration tests for the group protected
// environment tool closures in register.go. Tests exercise mutation error
// paths via an in-memory MCP session with a mock GitLab API.
package groupprotectedenvs

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerProtEnvJSON = `{"name":"production","deploy_access_levels":[{"access_level":40}]}`
const registerProtEnvListJSON = `[{"name":"production","deploy_access_levels":[{"access_level":40}]}]`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group
// protected environment tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 5 group protected environment tools
// can be called through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_environments"):
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvListJSON)
		case r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerProtEnvJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvJSON)
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
		{"gitlab_group_protected_environment_list", map[string]any{"group_id": "mygroup"}},
		{"gitlab_group_protected_environment_get", map[string]any{"group_id": "mygroup", "environment_name": "production"}},
		{"gitlab_group_protected_environment_protect", map[string]any{"group_id": "mygroup", "name": "staging", "deploy_access_levels": []any{map[string]any{"access_level": 40}}}},
		{"gitlab_group_protected_environment_update", map[string]any{"group_id": "mygroup", "environment_name": "production", "deploy_access_levels": []any{map[string]any{"access_level": 30}}}},
		{"gitlab_group_protected_environment_unprotect", map[string]any{"group_id": "mygroup", "environment_name": "production"}},
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
