package groupcredentials

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerPATJSON = `[{"id":99,"name":"test-pat","scopes":["api"],"state":"active","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01"}]`
const registerSSHKeyJSON = `[{"id":5,"title":"test-key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z"}]`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group
// credential tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 4 group credential tools can be
// called through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerPATJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/ssh_keys"):
			testutil.RespondJSON(w, http.StatusOK, registerSSHKeyJSON)
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "mygroup"}},
		{"gitlab_list_group_ssh_keys", map[string]any{"group_id": "mygroup"}},
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}},
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
