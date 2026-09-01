// ciyamltemplates_test.go contains unit tests for the CI YAML template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package ciyamltemplates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/templates/gitlab_ci_ymls (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/gitlab_ci_ymls")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"},{"key":"Python","name":"Python"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 2 {
		t.Fatalf("len = %d, want 2", len(out.Templates))
	}
	if out.Templates[0].Key != "Go" {
		t.Errorf("Key = %q, want Go", out.Templates[0].Key)
	}
}

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet verifies the Get handler.
// The mock GitLab API at /api/v4/templates/gitlab_ci_ymls/Go (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/gitlab_ci_ymls/Go")
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"stages:\n  - test"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{Key: "Go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "Go" {
		t.Errorf("Name = %q, want Go", out.Name)
	}
}

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{Key: "missing"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_EmptyKey verifies the Get_EmptyKey handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "key is required") {
		t.Errorf("error = %q, want key is required", err.Error())
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{Templates: []TemplateListItem{{Key: "Go", Name: "Go"}}}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "Go") {
		t.Error("missing key")
	}
}

// TestFormatGetMarkdown verifies the GetMarkdown Markdown formatter for a representative get input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "Go", Content: "stages:\n  - test"})
	if !strings.Contains(md, "Go") || !strings.Contains(md, "stages") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — canceled context, pagination, empty result
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/templates/gitlab_ci_ymls (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/templates/gitlab_ci_ymls" {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK,
				`[{"key":"Go","name":"Go"},{"key":"Python","name":"Python"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "2"},
			)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Templates) != 2 {
		t.Fatalf("len(Templates) = %d, want 2", len(out.Templates))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestList_OrderSortKeyset verifies that List forwards order_by, sort, and
// keyset pagination parameters (pagination, page_token) onto the GitLab API
// query string.
func TestList_OrderSortKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("order_by"); got != "name" {
			t.Errorf("order_by = %q, want name", got)
		}
		if got := q.Get("sort"); got != "desc" {
			t.Errorf("sort = %q, want desc", got)
		}
		if got := q.Get("pagination"); got != "keyset" {
			t.Errorf("pagination = %q, want keyset", got)
		}
		if got := q.Get("page_token"); got != "Go" {
			t.Errorf("page_token = %q, want Go", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Python","name":"Python"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "Go",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len(Templates) = %d, want 1", len(out.Templates))
	}
}

// TestApplyOrderSort_NilGuard verifies applyOrderSort tolerates a nil opts
// argument without panicking.
func TestApplyOrderSort_NilGuard(t *testing.T) {
	applyOrderSort(nil, "name", "asc")
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Templates) != 0 {
		t.Fatalf("len(Templates) = %d, want 0", len(out.Templates))
	}
}

// ---------------------------------------------------------------------------
// Get — canceled context, empty key
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{Key: "Go"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestGet_EmptyKey_Cov verifies the Get_EmptyKey_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_EmptyKey_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestGetOutput_Fields verifies the GetOutput_Fields handler.
// The mock GitLab API at /api/v4/templates/gitlab_ci_ymls/Python (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGetOutput_Fields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/templates/gitlab_ci_ymls/Python" {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"Python","content":"image: python:3.11\ntest:\n  script: pytest"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{Key: "Python"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "Python" {
		t.Errorf("Name = %q, want %q", out.Name, "Python")
	}
	if !strings.Contains(out.Content, "pytest") {
		t.Errorf("Content missing 'pytest': %q", out.Content)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty, with data, with pagination
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No templates found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Key |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_WithPagination verifies the ListMarkdown_WithPagination Markdown formatter for a representative list_withpagination input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	out := ListOutput{
		Templates: []TemplateListItem{
			{Key: "Go", Name: "Go"},
			{Key: "Python", Name: "Python"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 5, Page: 1, PerPage: 2, TotalPages: 3, NextPage: 2},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## CI YAML Templates",
		"| Key | Name |",
		"| Go | Go |",
		"| Python | Python |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown_SpecialCharacters verifies the ListMarkdown_SpecialCharacters Markdown formatter for a representative list_specialcharacters input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_SpecialCharacters(t *testing.T) {
	out := ListOutput{
		Templates: []TemplateListItem{
			{Key: "Pipe|Test", Name: "Name|With|Pipes"},
		},
	}
	md := FormatListMarkdown(out)
	if strings.Count(md, "|") < 4 {
		t.Errorf("expected pipe characters to be escaped in table:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — with data, empty content
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies the GetMarkdown_AllFields Markdown formatter for a representative get_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Name:    "Ruby",
		Content: "image: ruby:3.2\ntest:\n  script: rspec",
	})

	for _, want := range []string{
		"## CI YAML Template: Ruby",
		"```yaml",
		"image: ruby:3.2",
		"rspec",
		"```",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatGetMarkdown_EmptyContent verifies the GetMarkdown_EmptyContent Markdown formatter for a representative get_emptycontent input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetMarkdown_EmptyContent(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "Empty", Content: ""})
	if !strings.Contains(md, "## CI YAML Template: Empty") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "```yaml") {
		t.Errorf("missing yaml block:\n%s", md)
	}
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "ciyamltemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newCIYAMLTemplatesRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_ci_yml_templates", map[string]any{}},
		{"list_with_pagination", "gitlab_list_ci_yml_templates", map[string]any{"page": 1, "per_page": 10}},
		{"get", "gitlab_get_ci_yml_template", map[string]any{"key": "Go"}},
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

// TestActionSpecs_CallRouteErrors validates the CallRouteErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	specByTool := ciyamlTemplateSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_ci_yml_templates", map[string]any{}},
		{"gitlab_get_ci_yml_template", map[string]any{"key": "Go"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.name)
			}
		})
	}
}

// newCIYAMLTemplatesRouteSpecs constructs ciyaml templates route specs test fixtures.
func newCIYAMLTemplatesRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	// List CI YAML templates
	handler.HandleFunc("GET /api/v4/templates/gitlab_ci_ymls", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"},{"key":"Python","name":"Python"}]`)
	})

	// Get CI YAML template by key
	handler.HandleFunc("GET /api/v4/templates/gitlab_ci_ymls/Go", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"stages:\n  - test"}`)
	})

	client := testutil.NewTestClient(t, handler)
	return ciyamlTemplateSpecsByTool(ActionSpecs(client))
}

// ciyamlTemplateSpecsByTool supports ciyaml template specs by tool assertions in ciyamltemplates tests.
func ciyamlTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
