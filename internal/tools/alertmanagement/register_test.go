// register_test.go contains integration tests for the alert management tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.

package alertmanagement

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_MutationErrors verifies that delete/update handler closures in
// register.go return error results when the GitLab API returns internal server error,
// covering the if-err-not-nil branches.
func TestRegisterTools_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete, http.MethodPut:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{}`)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, _ = server.Connect(ctx, st, nil)
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
		{"gitlab_update_alert_metric_image", map[string]any{"project_id": "42", "alert_iid": 1, "metric_image_id": 1}},
		{"gitlab_upload_alert_metric_image", map[string]any{"project_id": "42", "alert_iid": 1}},
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
