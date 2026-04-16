package deployments

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_MutationErrors verifies that delete, create, update, and
// approve/reject handler closures in register.go return error results when
// the GitLab API responds with errors. Covers the if-err branches and the
// NotFoundResult 404 path for get.
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
		{"gitlab_deployment_get", map[string]any{"project_id": "42", "deployment_id": 999}},
		{"gitlab_deployment_create", map[string]any{"project_id": "42", "environment": "prod", "ref": "main", "sha": "abc123", "tag": false, "status": "created"}},
		{"gitlab_deployment_update", map[string]any{"project_id": "42", "deployment_id": 1, "status": "failed"}},
		{"gitlab_deployment_delete", map[string]any{"project_id": "42", "deployment_id": 1}},
		{"gitlab_deployment_approve_or_reject", map[string]any{"project_id": "42", "deployment_id": 1, "status": "approved"}},
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
