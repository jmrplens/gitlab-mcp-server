package groupwikis

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerWikiJSON = `{
	"format": "markdown",
	"slug": "home",
	"title": "Home",
	"content": "# Welcome",
	"encoding": "UTF-8"
}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group wiki
// tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered group wiki tools
// can be called through MCP in-memory transport, covering handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[`+registerWikiJSON+`]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerWikiJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerWikiJSON)
		case http.MethodDelete:
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
		{"gitlab_group_wiki_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_wiki_get", map[string]any{"group_id": "42", "slug": "home"}},
		{"gitlab_group_wiki_create", map[string]any{"group_id": "42", "title": "Home", "content": "# Welcome"}},
		{"gitlab_group_wiki_edit", map[string]any{"group_id": "42", "slug": "home", "content": "# Updated"}},
		{"gitlab_group_wiki_delete", map[string]any{"group_id": "42", "slug": "home"}},
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
