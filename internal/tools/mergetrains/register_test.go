// Package mergetrains register_test exercises all RegisterTools closures
// via MCP in-memory transport, covering every handler wired in register.go.
package mergetrains

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerTrainJSON = `{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}`
const registerTrainsJSON = `[{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}]`

// TestRegisterTools_NoPanic verifies RegisterTools registers all tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 4 merge train tools can
// be called through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/merge_trains/"):
			testutil.RespondJSON(w, http.StatusOK, registerTrainJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/merge_trains"):
			testutil.RespondJSON(w, http.StatusOK, registerTrainsJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerTrainsJSON)
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
		{"gitlab_list_project_merge_trains", map[string]any{"project_id": "42"}},
		{"gitlab_list_merge_request_in_merge_train", map[string]any{"project_id": "42", "target_branch": "main"}},
		{"gitlab_get_merge_request_on_merge_train", map[string]any{"project_id": "42", "merge_request_id": 10}},
		{"gitlab_add_merge_request_to_merge_train", map[string]any{"project_id": "42", "merge_request_id": 10}},
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
