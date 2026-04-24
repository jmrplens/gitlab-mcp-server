// register_test.go contains integration tests for the group relations export
// tool closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.

package grouprelationsexport

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const registerExportStatusJSON = `[{"relation":"labels","status":0,"batched":false,"batches_count":0,"error":""}]`

// TestRegisterTools_CallThroughMCP verifies both group relations export tools can
// be called through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, "/export_relations"):
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.Contains(path, "/export_relations/status"):
			testutil.RespondJSON(w, http.StatusOK, registerExportStatusJSON)
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
		{"gitlab_schedule_group_relations_export", map[string]any{"group_id": "5"}},
		{"gitlab_list_group_relations_export_status", map[string]any{"group_id": "5"}},
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

// TestFormatListExportStatusMarkdownString verifies the markdown formatter covers
// the FormatListExportStatusMarkdownString function registered via init().
func TestFormatListExportStatusMarkdownString(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := FormatListExportStatusMarkdownString(ListExportStatusOutput{})
		if out == "" {
			t.Fatal("expected non-empty markdown for empty list")
		}
	})
	t.Run("with statuses", func(t *testing.T) {
		out := FormatListExportStatusMarkdownString(ListExportStatusOutput{
			Statuses: []ExportStatusItem{{Relation: "labels", Status: 0, Batched: false, BatchesCount: 0}},
		})
		if out == "" {
			t.Fatal("expected non-empty markdown")
		}
	})
}

// TestMarkdownInit_Registry verifies the init() markdown formatter is registered.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListExportStatusOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListExportStatusOutput")
	}
}
