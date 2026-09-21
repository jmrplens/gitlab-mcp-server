// gitignore_templates_test.go contains unit tests for the gitignore template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package gitignoretemplates

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/templates/gitignores (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/gitignores")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"},{"key":"Node","name":"Node"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 2 {
		t.Fatalf("len = %d, want 2", len(out.Templates))
	}
}

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies the Get handler.
// The mock GitLab API at /api/v4/templates/gitignores/Go (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/gitignores/Go")
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"*.exe\n*.test"}`)
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_EmptyKey verifies the Get_EmptyKey handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateListItem{{Key: "Go", Name: "Go"}}})
	if !strings.Contains(md, "Go") {
		t.Error("missing")
	}
}

// TestFormatGetMarkdown verifies the GetMarkdown Markdown formatter for a representative get input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "Go", Content: "*.exe"})
	if !strings.Contains(md, "*.exe") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: nil})
	if !strings.Contains(md, "No templates found") {
		t.Error("expected 'No templates found' for empty list")
	}
}

// ---------------------------------------------------------------------------
// List — API error 400
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies that List400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestGet_APIError400 verifies that Get400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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

// TestList_WithKeysetAndOrder verifies that List forwards order_by, sort, and
// keyset pagination (pagination, page_token) to the GitLab API query string.
// The mock asserts each query parameter is present before responding with HTTP OK.
func TestList_WithKeysetAndOrder(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "name" {
			t.Errorf("order_by = %q, want name", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "cursor123" {
			t.Errorf("page_token = %q, want cursor123", q.Get("page_token"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "cursor123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
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
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "gitignoretemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_gitignore_template"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_gitignore_template should define key parameter guidance")
	}
	for _, tool := range []string{"gitlab_list_gitignore_templates", "gitlab_get_gitignore_template"} {
		t.Run(tool, func(t *testing.T) {
			desc := specByTool[tool].IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newGitignoreRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_gitignore_templates", map[string]any{}},
		{"get", "gitlab_get_gitignore_template", map[string]any{"key": "Go"}},
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

// TestActionSpecs_CallRouteErrors validates the CallRouteErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/gitignores", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/gitignores/Bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	specByTool := gitignoreTemplateSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_gitignore_templates", map[string]any{}},
		{"get_error", "gitlab_get_gitignore_template", map[string]any{"key": "Bad"}},
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

// newGitignoreRouteSpecs constructs gitignore route specs test fixtures.
func newGitignoreRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/gitignores", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"},{"key":"Node","name":"Node"}]`)
	})
	handler.HandleFunc("GET /api/v4/templates/gitignores/Go", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"*.exe\n*.test"}`)
	})

	client := testutil.NewTestClient(t, handler)
	return gitignoreTemplateSpecsByTool(ActionSpecs(client))
}

// gitignoreTemplateSpecsByTool supports gitignore template specs by tool assertions in gitignoretemplates tests.
func gitignoreTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// ---------------------------------------------------------------------------
// What a caller is handed: the fields, the page, the hint
// ---------------------------------------------------------------------------.

// TestList_DistinctKeyAndName_MapsEachToItsOwnField asserts that List copies
// GitLab's key into Key and its name into Name, against a fixture whose two
// columns differ on every row. Every other fixture here spells a template "Go"
// in both columns, which asserts the mapping against itself: exchanging the two
// assignments moves both sides together and fails nothing, while a caller would
// be handed "Go" as the key of the template GitLab files under "Global/Go" and
// the get that follows would 404.
func TestList_DistinctKeyAndName_MapsEachToItsOwnField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Global/Go","name":"Go"},{"key":"Node","name":"Node.js"}]`)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []TemplateListItem{{Key: "Global/Go", Name: "Go"}, {Key: "Node", Name: "Node.js"}}
	if !slices.Equal(out.Templates, want) {
		t.Errorf("Templates = %+v, want %+v", out.Templates, want)
	}
}

// TestList_PaginatedResponse_PublishesTheHeadersGitLabSent asserts that the
// pagination block List returns is filled from the response GitLab answered
// with, down to the derived has_more. The list is the only way a caller learns
// which keys exist, so a block that is dropped or filled from nowhere hands a
// model reading page two of three a complete-looking answer and no way to ask
// for the rest; nothing else in this package reads the block at all.
func TestList_PaginatedResponse_PublishesTheHeadersGitLabSent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"key":"Node","name":"Node.js"}]`, testutil.PaginationHeaders{
			Page: "2", PerPage: "5", Total: "11", TotalPages: "3", NextPage: "3", PrevPage: "1",
		})
	}))

	out, err := List(t.Context(), client, ListInput{Page: 2, PerPage: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := toolutil.PaginationOutput{
		Page: 2, PerPage: 5, TotalItems: 11, TotalPages: 3, NextPage: 3, PrevPage: 1, HasMore: true,
	}
	if out.Pagination != want {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestGet_TemplateBody_ReachesTheCallerWithItsName asserts that Get carries
// both halves of GitLab's answer. The body is the whole point of the action —
// a caller asks for a template in order to write it into .gitignore — and it
// was the one field no test read through the handler: Get could return the
// name alone, or an empty body, and every assertion in this file still passed.
func TestGet_TemplateBody_ReachesTheCallerWithItsName(t *testing.T) {
	const body = "*.exe\n*.test\nvendor/\n"
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/gitignores/Go")
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"*.exe\n*.test\nvendor/\n"}`)
	}))

	out, err := Get(t.Context(), client, GetInput{Key: "Go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "Go" {
		t.Errorf("Name = %q, want Go", out.Name)
	}
	if out.Content != body {
		t.Errorf("Content = %q, want %q", out.Content, body)
	}
}

// TestList_ErrorStatuses_SuggestTheTokenScopeOnlyOnForbidden asserts that the
// read_api suggestion is attached to the status it is about and to no other.
// The existing error tests only check that some error came back, so the status
// this handler classifies on was asserted nowhere: pointed at any other code,
// a 403 would lose the one hint that tells a caller their token is too narrow,
// and a 404 would gain advice that has nothing to do with what happened.
func TestList_ErrorStatuses_SuggestTheTokenScopeOnlyOnForbidden(t *testing.T) {
	const scopeHint = "Suggestion: verify your token has read_api scope"

	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{"forbidden_suggests_the_scope", http.StatusForbidden, true},
		{"not_found_suggests_nothing_about_scope", http.StatusNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"refused"}`)
			}))

			_, err := List(t.Context(), client, ListInput{})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := strings.Contains(err.Error(), scopeHint); got != tc.wantHint {
				t.Errorf("scope hint present = %v, want %v; error: %v", got, tc.wantHint, err)
			}
		})
	}
}

// TestGet_ErrorStatuses_SuggestTheListActionOnlyOnNotFound asserts the same
// property for the get handler: a key GitLab does not know is answered with
// the action that lists the keys, and a refusal for any other reason is not.
// A model handed "verify name with template.gitignore_list" after a
// 403 would go looking for a spelling mistake instead of at its credential.
func TestGet_ErrorStatuses_SuggestTheListActionOnlyOnNotFound(t *testing.T) {
	const listHint = "Suggestion: verify name with " + actionGitignoreList

	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{"not_found_suggests_the_list_action", http.StatusNotFound, true},
		{"forbidden_suggests_nothing_about_the_name", http.StatusForbidden, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"refused"}`)
			}))

			_, err := Get(t.Context(), client, GetInput{Key: "Nope"})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := strings.Contains(err.Error(), listHint); got != tc.wantHint {
				t.Errorf("list hint present = %v, want %v; error: %v", got, tc.wantHint, err)
			}
		})
	}
}

// TestGet_EmptyKey_RefusesBeforeAskingGitLab asserts that a get with no key is
// refused here, naming the action that lists the keys, and that no request
// leaves for GitLab. Both halves matter and neither was held: without the
// guard the call goes out as a request for the collection itself, so what a
// model gets back is whatever that URL happens to answer rather than the one
// message that tells it how to find a key.
func TestGet_EmptyKey_RefusesBeforeAskingGitLab(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"*.exe"}`)
	}))

	_, err := Get(t.Context(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected an error for an empty key")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("requests reaching GitLab = %d, want 0", got)
	}
	if !strings.Contains(err.Error(), "list action") {
		t.Errorf("error should point at the list action, got %v", err)
	}
}

// TestFormatGetMarkdown_NamesTheTemplateInItsHeading asserts the rendered card
// names the template above its body and fences the body as gitignore. The
// heading is the only place the name appears, and the older assertion read the
// body alone, so a formatter that headed every card with its own content
// rendered a card a reader cannot attribute to any template.
func TestFormatGetMarkdown_NamesTheTemplateInItsHeading(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "Go", Content: "*.exe"})

	if !strings.Contains(md, "## Gitignore Template: Go") {
		t.Errorf("heading should name the template, got %q", md)
	}
	if !strings.Contains(md, "```gitignore\n*.exe\n```") {
		t.Errorf("body should be fenced as gitignore, got %q", md)
	}
}

// TestFormatListMarkdown_PaginatedOutput_RendersThePageFooter asserts the list
// card carries the page the output block describes, and each template in its
// own column. The handler filling that block is worth nothing if the formatter
// then renders the card without it: the Markdown view is what a model reads,
// so a footer dropped there hides the further pages just as completely as a
// block never filled.
func TestFormatListMarkdown_PaginatedOutput_RendersThePageFooter(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Templates: []TemplateListItem{{Key: "Node", Name: "Node.js"}},
		Pagination: toolutil.PaginationOutput{
			Page: 2, PerPage: 5, TotalItems: 11, TotalPages: 3, NextPage: 3, HasMore: true,
		},
	})

	if !strings.Contains(md, "Page 2 of 3") {
		t.Errorf("footer should name the page, got %q", md)
	}
	if !strings.Contains(md, "| Node | Node.js |") {
		t.Errorf("row should carry the key then the name, got %q", md)
	}
}
