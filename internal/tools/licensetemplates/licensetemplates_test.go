// licensetemplates_test.go contains unit tests for the license template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package licensetemplates

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
		if r.URL.Path != "/api/v4/templates/licenses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License","featured":true}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
	if !out.Licenses[0].Featured {
		t.Error("Featured = false, want true")
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
		if r.URL.Path != "/api/v4/templates/licenses/mit" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","content":"MIT License\n\nCopyright...","permissions":["commercial-use"],"conditions":["include-copyright"],"limitations":["no-liability"]}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{Key: "mit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "MIT License" {
		t.Errorf("Name = %q", out.Name)
	}
	if len(out.Permissions) != 1 {
		t.Errorf("Permissions len = %d", len(out.Permissions))
	}
}

// TestGet_EmptyKey verifies that Get returns a validation error when the key is empty.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called with empty key")
	}))
	_, err := Get(t.Context(), client, GetInput{Key: ""})
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
	_, err := Get(t.Context(), client, GetInput{Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: []LicenseItem{{Key: "mit", Name: "MIT", Featured: true}}})
	if !strings.Contains(md, "MIT") {
		t.Error("missing")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{LicenseItem: LicenseItem{Name: "MIT", Content: "text", Permissions: []string{"use"}}})
	if !strings.Contains(md, "MIT") || !strings.Contains(md, "use") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: nil})
	if !strings.Contains(md, "No license templates found") {
		t.Error("expected 'No license templates found' for empty list")
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies the behavior of format get markdown all fields.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{LicenseItem: LicenseItem{
		Name:        "Apache 2.0",
		Description: "A permissive license",
		Permissions: []string{"commercial-use", "modification"},
		Conditions:  []string{"include-copyright", "document-changes"},
		Limitations: []string{"no-liability", "no-warranty"},
		Content:     "Apache License text here",
	}})
	for _, want := range []string{"Apache 2.0", "A permissive license", "commercial-use", "include-copyright", "no-liability", "Apache License text here"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown output", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — minimal fields (no description, no conditions, no content)
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_MinimalFields verifies the behavior of format get markdown minimal fields.
func TestFormatGetMarkdown_MinimalFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{LicenseItem: LicenseItem{
		Name: "Minimal",
	}})
	if !strings.Contains(md, "Minimal") {
		t.Error("expected license name")
	}
	if strings.Contains(md, "Description") {
		t.Error("should not contain Description when empty")
	}
	if strings.Contains(md, "```") {
		t.Error("should not contain code block when content is empty")
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
// List — with Popular filter
// ---------------------------------------------------------------------------.

// TestList_WithPopularFilter verifies the behavior of list with popular filter.
func TestList_WithPopularFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("popular") != "true" {
			t.Errorf("expected popular=true, got %s", r.URL.Query().Get("popular"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License","featured":true}]`)
	}))
	pop := true
	out, err := List(context.Background(), client, ListInput{Popular: &pop})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
}

// ---------------------------------------------------------------------------
// List — with pagination
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies the behavior of list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected page=2&per_page=10, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"apache-2.0","name":"Apache License 2.0"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
}

// ---------------------------------------------------------------------------
// Get — with optional Project and Fullname fields
// ---------------------------------------------------------------------------.

// TestGet_WithOptionalFields verifies the behavior of get with optional fields.
func TestGet_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("project") != "my-project" {
			t.Errorf("expected project=my-project, got %s", r.URL.Query().Get("project"))
		}
		if r.URL.Query().Get("fullname") != "John Doe" {
			t.Errorf("expected fullname=John Doe, got %s", r.URL.Query().Get("fullname"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","content":"MIT License\nCopyright (c) John Doe"}`)
	}))
	proj := "my-project"
	fullname := "John Doe"
	out, err := Get(context.Background(), client, GetInput{Key: "mit", Project: &proj, Fullname: &fullname})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "MIT License" {
		t.Errorf("Name = %q, want MIT License", out.Name)
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
	session := newLicenseMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_license_templates", map[string]any{}},
		{"get", "gitlab_get_license_template", map[string]any{"key": "mit"}},
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
	handler.HandleFunc("GET /api/v4/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/licenses/bad", func(w http.ResponseWriter, _ *http.Request) {
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
		{"list_error", "gitlab_list_license_templates", map[string]any{}},
		{"get_error", "gitlab_get_license_template", map[string]any{"key": "bad"}},
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

// newLicenseMCPSession is an internal helper for the licensetemplates package.
func newLicenseMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	licenseJSON := `{"key":"mit","name":"MIT License","featured":true,"description":"A short license","permissions":["commercial-use"],"conditions":["include-copyright"],"limitations":["no-liability"],"content":"MIT License text"}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+licenseJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/templates/licenses/mit", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, licenseJSON)
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
