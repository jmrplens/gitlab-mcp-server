package dependencies

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerDepListJSON = `[{"name":"rails","version":"7.0.0","package_manager":"bundler","dependency_file_path":"Gemfile.lock"}]`
const registerExportJSON = `{"id":1,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":""}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all dependency
// tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 4 dependency tools can be called
// through MCP in-memory transport, covering every handler closure in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/dependencies") && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerDepListJSON)
		case strings.Contains(path, "/dependency_list_exports") && strings.HasSuffix(path, "/download"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"bomFormat":"CycloneDX"}`))
		case strings.Contains(path, "/dependency_list_exports") && r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerExportJSON)
		case strings.Contains(path, "/dependency_list_exports") && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"has_finished":true,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":"https://gitlab.example.com/api/v4/dependency_list_exports/1/download"}`)
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
		{"gitlab_list_project_dependencies", map[string]any{"project_id": "42"}},
		{"gitlab_create_dependency_list_export", map[string]any{"pipeline_id": 100}},
		{"gitlab_get_dependency_list_export", map[string]any{"export_id": 1}},
		{"gitlab_download_dependency_list_export", map[string]any{"export_id": 1}},
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
