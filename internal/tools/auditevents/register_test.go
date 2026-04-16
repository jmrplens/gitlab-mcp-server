package auditevents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerEventJSON = `{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{"change":"updated"},"created_at":"2026-01-01T00:00:00Z"}`
const registerEventListJSON = `[{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{},"created_at":"2026-01-01T00:00:00Z"}]`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all audit event
// tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 6 audit event tools can be called
// through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/audit_events") || strings.HasSuffix(path, "/audit_events/"):
			testutil.RespondJSON(w, http.StatusOK, registerEventListJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, registerEventJSON)
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
		{"gitlab_list_instance_audit_events", map[string]any{}},
		{"gitlab_get_instance_audit_event", map[string]any{"event_id": 1}},
		{"gitlab_list_group_audit_events", map[string]any{"group_id": "5"}},
		{"gitlab_get_group_audit_event", map[string]any{"group_id": "5", "event_id": 1}},
		{"gitlab_list_project_audit_events", map[string]any{"project_id": "42"}},
		{"gitlab_get_project_audit_event", map[string]any{"project_id": "42", "event_id": 1}},
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
