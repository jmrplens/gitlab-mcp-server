// dockerfiletemplates_test.go contains unit tests for the Dockerfile template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dockerfiletemplates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestList verifies the behavior of list.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/templates/dockerfiles" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Ruby","name":"Ruby"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
}

// TestList_Error verifies that List handles the error scenario correctly.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/templates/dockerfiles/Ruby" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Ruby","content":"FROM ruby:latest"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{Key: "Ruby"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "Ruby" {
		t.Errorf("Name = %q, want Ruby", out.Name)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_EmptyKey verifies that Get handles the empty key scenario correctly.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call API with empty key")
	}))
	_, err := Get(t.Context(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "key is required") {
		t.Errorf("error = %q, want key is required", err.Error())
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateListItem{{Key: "R", Name: "R"}}})
	if !strings.Contains(md, "R") {
		t.Error("missing")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "R", Content: "FROM r"})
	if !strings.Contains(md, "FROM") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: nil})
	if !strings.Contains(md, "No templates found") {
		t.Error("expected 'No templates found' for empty list")
	}
}

// ---------------------------------------------------------------------------
// List — API error 400
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies the behavior of list a p i error400.
func TestList_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Get — API error 400
// ---------------------------------------------------------------------------.

// TestGet_APIError400 verifies the behavior of get a p i error400.
func TestGet_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "bad"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — with pagination params
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies the behavior of list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "5" {
			t.Errorf("expected page=2&per_page=5, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{Page: 2, PerPage: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newDockerfileMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_dockerfile_templates", map[string]any{}},
		{"get", "gitlab_get_dockerfile_template", map[string]any{"key": "Ruby"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — error paths
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_Errors validates m c p round trip errors across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_Errors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/dockerfiles", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/dockerfiles/Bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_dockerfile_templates", map[string]any{}},
		{"get_error", "gitlab_get_dockerfile_template", map[string]any{"key": "Bad"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("CallTool(%s) expected IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newDockerfileMCPSession is an internal helper for the dockerfiletemplates package.
func newDockerfileMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/dockerfiles", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Ruby","name":"Ruby"},{"key":"Go","name":"Go"}]`)
	})
	handler.HandleFunc("GET /api/v4/templates/dockerfiles/Ruby", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Ruby","content":"FROM ruby:latest"}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
