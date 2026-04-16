// Package epics register_test exercises all RegisterTools closures
// via MCP in-memory transport, covering every handler wired in register.go.
package epics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerEpicJSON = `{"id":1,"iid":1,"title":"Epic","description":"desc","state":"opened","web_url":"https://gl.example.com/groups/g/-/epics/1","author":{"id":1,"username":"user"}}`
const registerEpicsJSON = `[{"id":1,"iid":1,"title":"Epic","description":"desc","state":"opened","web_url":"https://gl.example.com/groups/g/-/epics/1","author":{"id":1,"username":"user"}}]`

// TestRegisterTools_NoPanic verifies RegisterTools registers all tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 6 epic tools can
// be called through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/epic_links"):
			testutil.RespondJSON(w, http.StatusOK, registerEpicsJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/epics"):
			testutil.RespondJSON(w, http.StatusOK, registerEpicsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/epics/"):
			testutil.RespondJSON(w, http.StatusOK, registerEpicJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerEpicJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerEpicJSON)
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
		{"gitlab_epic_list", map[string]any{"group_id": "42"}},
		{"gitlab_epic_get", map[string]any{"group_id": "42", "epic_iid": 1}},
		{"gitlab_epic_get_links", map[string]any{"group_id": "42", "epic_iid": 1}},
		{"gitlab_epic_create", map[string]any{"group_id": "42", "title": "New Epic"}},
		{"gitlab_epic_update", map[string]any{"group_id": "42", "epic_iid": 1, "title": "Updated"}},
		{"gitlab_epic_delete", map[string]any{"group_id": "42", "epic_iid": 1}},
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
