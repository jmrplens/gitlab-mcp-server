// Package impersonationtokens register_test exercises all RegisterTools closures
// via MCP in-memory transport, covering every handler wired in register.go.
package impersonationtokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerTokenJSON = `{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}`
const registerTokensJSON = `[{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}]`

// TestRegisterTools_NoPanic verifies RegisterTools registers all tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 5 impersonation token tools can
// be called through MCP in-memory transport, covering every handler closure.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerTokensJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/impersonation_tokens/"):
			testutil.RespondJSON(w, http.StatusOK, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
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
		{"gitlab_list_impersonation_tokens", map[string]any{"user_id": 1}},
		{"gitlab_get_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_impersonation_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_revoke_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_personal_access_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
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
