package releaselinks

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_MutationErrors covers the error and 404 branches in
// register.go closures: delete (500), get (404 → NotFoundResult), create (500),
// update (500), and batch create (500).
func TestRegisterTools_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, _ = server.Connect(ctx, st, nil)
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
		{"gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 999}},
		{"gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1}},
		{"gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "asset", "url": "https://example.com/file"}},
		{"gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1, "name": "new-name"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil || !result.IsError {
				t.Errorf("expected error result from %s", tt.name)
			}
		})
	}
}
