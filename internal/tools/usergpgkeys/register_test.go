// register_test.go contains integration tests for the user GPG key tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.

package usergpgkeys

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerGPGKeyJSON = `{"id":1,"primary_key_id":1,"key_id":"ABC123","public_key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-01-01T00:00:00Z","user":{"id":1,"username":"admin"}}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all GPG key tools
// without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered GPG key tools
// can be called through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[`+registerGPGKeyJSON+`]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerGPGKeyJSON)
		case http.MethodDelete:
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
		{"gitlab_list_gpg_keys", map[string]any{}},
		{"gitlab_list_gpg_keys_for_user", map[string]any{"user_id": 1}},
		{"gitlab_get_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_get_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
		{"gitlab_add_gpg_key", map[string]any{"key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_add_gpg_key_for_user", map[string]any{"user_id": 1, "key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_delete_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_delete_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
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
