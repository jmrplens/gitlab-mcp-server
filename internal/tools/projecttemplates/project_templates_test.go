// project_templates_test.go contains unit tests for the project template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package projecttemplates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestList verifies List.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/1/templates/licenses")
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

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "1", TemplateType: "licenses"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/1/templates/licenses/mit")
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: ""})
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
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies the whole list render, with the popular
// column as the flag glyph rather than the word "Yes".
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateItem{{Key: "mit", Name: "MIT", Popular: true}}})
	want := "## Project Templates (1)\n\n" +
		"| Key | Name | Popular |\n| --- | --- | --- |\n" +
		"| mit | MIT | " + toolutil.EmojiSuccess + " |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use `gitlab_get_project_template` to view a specific template\n"
	if md != want {
		t.Errorf("project template list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown verifies the whole card of one project template.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "MIT", Key: "mit", Content: "text", Permissions: []string{"use"}})
	want := "## Project Template: MIT\n\n" +
		"- **Key**: mit\n" +
		"- **Permissions**: use\n" +
		"\n### Content\n\n```\ntext\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use this template when creating new project files\n"
	if md != want {
		t.Errorf("project template card:\n got %q\nwant %q", md, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies that an empty page is the one sentence
// and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: nil})
	if want := "No templates found.\n"; md != want {
		t.Errorf("empty template list:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — non-popular item
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_NonPopular verifies that a template GitLab does not
// list among the popular ones renders the cross glyph, where the column used
// to be blank and said nothing at all.
func TestFormatListMarkdown_NonPopular(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateItem{
		{Key: "test", Name: "Test", Popular: false},
	}})
	want := "## Project Templates (1)\n\n" +
		"| Key | Name | Popular |\n| --- | --- | --- |\n" +
		"| test | Test | " + toolutil.EmojiCross + " |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use `gitlab_get_project_template` to view a specific template\n"
	if md != want {
		t.Errorf("project template list:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies the whole card when GitLab answered
// with every field a project template can carry.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Key:         "mit",
		Name:        "MIT License",
		Nickname:    "MIT",
		Popular:     true,
		Description: "A permissive license",
		Permissions: []string{"commercial-use"},
		Conditions:  []string{"include-copyright"},
		Limitations: []string{"no-liability"},
		Content:     "MIT License text",
	})
	want := "## Project Template: MIT License\n\n" +
		"- **Key**: mit\n" +
		"- **Nickname**: MIT\n" +
		"- **Popular**: " + toolutil.EmojiSuccess + "\n" +
		"- **Description**: A permissive license\n" +
		"- **Permissions**: commercial-use\n" +
		"- **Conditions**: include-copyright\n" +
		"- **Limitations**: no-liability\n" +
		"\n### Content\n\n```\nMIT License text\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use this template when creating new project files\n"
	if md != want {
		t.Errorf("project template card:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — minimal fields
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_MinimalFields verifies that a template GitLab answered
// with a key and a name renders those two rows and no absent value: no
// nickname, no popular flag and no content section.
func TestFormatGetMarkdown_MinimalFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Key:  "basic",
		Name: "Basic",
	})
	want := "## Project Template: Basic\n\n" +
		"- **Key**: basic\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use this template when creating new project files\n"
	if md != want {
		t.Errorf("minimal project template card:\n got %q\nwant %q", md, want)
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
	_, err := List(context.Background(), client, ListInput{ProjectID: "1", TemplateType: "licenses"})
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
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", TemplateType: "licenses", Key: "bad"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — with pagination params
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "3" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected page=3&per_page=10, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"go","name":"Go"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1", TemplateType: "gitignores",
		Page: 3, PerPage: 10,
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

// TestList_ZeroPagination verifies List when zero pagination.
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

// TestList_WithFiltersAndKeyset verifies that id, type, order_by, sort, and
// keyset pagination inputs are wired onto the request query string.
func TestList_WithFiltersAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{
			"id":         "42",
			"type":       "licenses",
			"order_by":   "name",
			"sort":       "asc",
			"pagination": "keyset",
			"page_token": "tok123",
		} {
			t.Run(key, func(t *testing.T) {
				if q.Get(key) != want {
					t.Errorf("query %s = %q, want %q (raw=%s)", key, q.Get(key), want, r.URL.RawQuery)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:    "1",
		TemplateType: "licenses",
		ID:           42,
		Type:         "licenses",
		OrderBy:      "name",
		Sort:         "asc",
		Pagination:   "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
}

// TestActionSpecs_Metadata verifies project template action spec metadata.
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
		if spec.OwnerPackage != "projecttemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_project_template"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_project_template should define key parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates project template canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newProjectTemplateRouteSpecs(t)

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

// TestActionSpecs_CallRouteErrors validates project template route errors.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/templates/licenses/bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	specByTool := projectTemplateSpecsByTool(ActionSpecs(client))

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
// Helper: route specs factory
// ---------------------------------------------------------------------------.

// newProjectTemplateRouteSpecs constructs project template route specs test fixtures.
func newProjectTemplateRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
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
	return projectTemplateSpecsByTool(ActionSpecs(client))
}

// projectTemplateSpecsByTool supports project template specs by tool assertions in projecttemplates tests.
func projectTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
