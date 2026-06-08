// licensetemplates_test.go contains unit tests for the license template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package licensetemplates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestList verifies List.
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

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies Get.
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

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: []LicenseItem{{Key: "mit", Name: "MIT", Featured: true}}})
	if !strings.Contains(md, "MIT") {
		t.Error("missing")
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{LicenseItem: LicenseItem{Name: "MIT", Content: "text", Permissions: []string{"use"}}})
	if !strings.Contains(md, "MIT") || !strings.Contains(md, "use") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: nil})
	if !strings.Contains(md, "No license templates found") {
		t.Error("expected 'No license templates found' for empty list")
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies FormatGetMarkdown when all fields.
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
	if strings.Contains(md, "- **Permissions**") {
		t.Error("license details should use unbulleted field labels")
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — minimal fields (no description, no conditions, no content)
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_MinimalFields verifies FormatGetMarkdown when minimal fields.
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

// TestList_APIError400 verifies List when API error 400.
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

// TestGet_APIError400 verifies Get when API error 400.
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

// TestList_WithPopularFilter verifies List when with popular filter.
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

// TestList_WithPagination verifies List when with pagination.
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

// TestGet_WithOptionalFields verifies Get when with optional fields.
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

// TestActionSpecs_Metadata verifies license template action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "licensetemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_license_template"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_license_template should define key parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates license template canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newLicenseRouteSpecs(t)

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates license template route errors.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/licenses/bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	specByTool := licenseTemplateSpecsByTool(ActionSpecs(client))

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// boolString — false branch
// ---------------------------------------------------------------------------

// TestBoolString_TrueAndFalse verifies the boolString helper returns the
// expected string literal for both boolean states used by the featured
// column in FormatListMarkdown.
func TestBoolString_TrueAndFalse(t *testing.T) {
	if got := boolString(true); got != "true" {
		t.Errorf("boolString(true) = %q, want %q", got, "true")
	}
	if got := boolString(false); got != "false" {
		t.Errorf("boolString(false) = %q, want %q", got, "false")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — non-featured license
// ---------------------------------------------------------------------------

// TestFormatListMarkdown_NonFeaturedLicense verifies that non-featured
// licenses render the literal "false" attribute string via boolString.
func TestFormatListMarkdown_NonFeaturedLicense(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: []LicenseItem{
		{Key: "gpl-3.0", Name: "GPL 3.0", Featured: false},
	}})
	if !strings.Contains(md, "gpl-3.0") {
		t.Errorf("missing license key in markdown: %s", md)
	}
	if !strings.Contains(md, "false") {
		t.Errorf("expected boolString(false) output in markdown: %s", md)
	}
}

// ---------------------------------------------------------------------------
// Helper: route specs factory
// ---------------------------------------------------------------------------.

// newLicenseRouteSpecs constructs license route specs test fixtures.
func newLicenseRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
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
	return licenseTemplateSpecsByTool(ActionSpecs(client))
}

// licenseTemplateSpecsByTool supports license template specs by tool assertions in licensetemplates tests.
func licenseTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
