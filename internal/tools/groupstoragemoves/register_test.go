// register_test.go contains integration tests for the group storage move tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.
package groupstoragemoves

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerStorageMoveJSON = `[{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}]`
const registerSingleMoveJSON = `{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group storage
// move tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 6 group storage move tools can be
// called through MCP in-memory transport, covering every handler closure in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(path, "/repository_storage_moves") || strings.HasSuffix(path, "/repository_storage_moves/") {
				testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerSingleMoveJSON)
			}
		case http.MethodPost:
			if strings.Contains(path, "all") {
				testutil.RespondJSON(w, http.StatusAccepted, `{"message":"202 Accepted"}`)
			} else {
				testutil.RespondJSON(w, http.StatusCreated, registerSingleMoveJSON)
			}
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
		{"gitlab_retrieve_all_group_storage_moves", map[string]any{}},
		{"gitlab_retrieve_group_storage_moves", map[string]any{"group_id": 42}},
		{"gitlab_get_group_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_group_storage_move_for_group", map[string]any{"group_id": 42, "id": 1}},
		{"gitlab_schedule_group_storage_move", map[string]any{"group_id": 42, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_group_storage_moves", map[string]any{"source_storage_name": "default", "destination_storage_name": "storage2"}},
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
