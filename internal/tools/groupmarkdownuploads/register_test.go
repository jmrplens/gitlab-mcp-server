package groupmarkdownuploads

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const registerUploadListJSON = `[{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-01T00:00:00Z"}]`

// TestRegisterTools_CallThroughMCP verifies all 3 group markdown upload tools can be
// called through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/uploads"):
			testutil.RespondJSON(w, http.StatusOK, registerUploadListJSON)
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
		{"gitlab_list_group_markdown_uploads", map[string]any{"group_id": "5"}},
		{"gitlab_delete_group_markdown_upload_by_id", map[string]any{"group_id": "5", "upload_id": 1}},
		{"gitlab_delete_group_markdown_upload_by_secret", map[string]any{"group_id": "5", "secret": "abc", "filename": "image.png"}},
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

// TestFormatListMarkdownString verifies the markdown formatter covers
// FormatListMarkdownString function registered via init().
func TestFormatListMarkdownString(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := FormatListMarkdownString(ListOutput{})
		if out == "" {
			t.Fatal("expected non-empty markdown for empty list")
		}
	})
	t.Run("with uploads", func(t *testing.T) {
		out := FormatListMarkdownString(ListOutput{
			Uploads: []UploadItem{{ID: 1, Filename: "test.png", Size: 1024, CreatedAt: "2026-01-01"}},
		})
		if out == "" {
			t.Fatal("expected non-empty markdown")
		}
	})
}

// TestMarkdownInit_Registry verifies the init() markdown formatter is registered.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListOutput")
	}
}
