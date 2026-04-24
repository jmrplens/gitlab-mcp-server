// projecttemplates_test.go contains unit tests for the project template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package projecttemplates

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
		if r.URL.Path != "/api/v4/projects/1/templates/licenses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License","popular":true}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", TemplateType: "licenses"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
	if out.Templates[0].Key != "mit" {
		t.Errorf("Key = %q", out.Templates[0].Key)
	}
}

// TestList_Error verifies that List handles the error scenario correctly.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "1", TemplateType: "licenses"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/templates/licenses/mit" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","content":"MIT text","permissions":["commercial-use"]}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: "mit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "MIT License" {
		t.Errorf("Name = %q", out.Name)
	}
}

// TestGet_EmptyKey verifies that Get returns a validation error when the key is empty.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called with empty key")
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "key is required") {
		t.Errorf("error = %q, want mention of key", err.Error())
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateItem{{Key: "mit", Name: "MIT", Popular: true}}})
	if !strings.Contains(md, "MIT") || !strings.Contains(md, "Yes") {
		t.Error("missing content")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{TemplateItem: TemplateItem{Name: "MIT", Key: "mit", Content: "text", Permissions: []string{"use"}}})
	if !strings.Contains(md, "MIT") || !strings.Contains(md, "use") {
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
// FormatListMarkdown — non-popular item (no "Yes" in Popular column)
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_NonPopular verifies the behavior of format list markdown non popular.
func TestFormatListMarkdown_NonPopular(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateItem{
		{Key: "test", Name: "Test", Popular: false},
	}})
	if !strings.Contains(md, "test") {
		t.Error("expected template key in output")
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies the behavior of format get markdown all fields.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{TemplateItem: TemplateItem{
		Key:         "mit",
		Name:        "MIT License",
		Nickname:    "MIT",
		Popular:     true,
		Description: "A permissive license",
		Permissions: []string{"commercial-use"},
		Conditions:  []string{"include-copyright"},
		Limitations: []string{"no-liability"},
		Content:     "MIT License text",
	}})
	for _, want := range []string{"MIT License", "MIT", "Popular", "A permissive license", "commercial-use", "include-copyright", "no-liability", "MIT License text"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown output", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — minimal fields
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_MinimalFields verifies the behavior of format get markdown minimal fields.
func TestFormatGetMarkdown_MinimalFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{TemplateItem: TemplateItem{
		Key:  "basic",
		Name: "Basic",
	}})
	if !strings.Contains(md, "Basic") {
		t.Error("expected template name")
	}
	if strings.Contains(md, "Nickname") {
		t.Error("should not contain Nickname")
	}
	if strings.Contains(md, "Popular") {
		t.Error("should not contain Popular")
	}
	if strings.Contains(md, "Content") {
		t.Error("should not contain Content section")
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
	_, err := List(context.Background(), client, ListInput{ProjectID: "1", TemplateType: "licenses"})
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
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: "bad"})
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
		if r.URL.Query().Get("page") != "3" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected page=3&per_page=10, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"go","name":"Go"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1", TemplateType: "gitignores", Page: 3, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
}

// ---------------------------------------------------------------------------
// List — zero pagination (no Page/PerPage set)
// ---------------------------------------------------------------------------.

// TestList_ZeroPagination verifies the behavior of list zero pagination.
func TestList_ZeroPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"ruby","name":"Ruby"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1", TemplateType: "dockerfiles",
	})
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
	session := newProjectTemplateMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_project_templates", map[string]any{"project_id": "1", "template_type": "licenses", "page": float64(1), "per_page": float64(20)}},
		{"get", "gitlab_get_project_template", map[string]any{"project_id": "1", "template_type": "licenses", "key": "mit"}},
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
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses/bad", func(w http.ResponseWriter, _ *http.Request) {
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
		{"list_error", "gitlab_list_project_templates", map[string]any{"project_id": "1", "template_type": "licenses", "page": float64(1), "per_page": float64(20)}},
		{"get_error", "gitlab_get_project_template", map[string]any{"project_id": "1", "template_type": "licenses", "key": "bad"}},
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

// newProjectTemplateMCPSession is an internal helper for the projecttemplates package.
func newProjectTemplateMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	templateJSON := `{"key":"mit","name":"MIT License","popular":true,"content":"MIT text","permissions":["commercial-use"]}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+templateJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses/mit", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, templateJSON)
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
