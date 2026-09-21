// dockerfile_templates_test.go contains unit tests for the Dockerfile template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dockerfiletemplates

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/templates/dockerfiles (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/dockerfiles")
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
// The mock GitLab API at /api/v4/templates/dockerfiles/Ruby (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/dockerfiles/Ruby")
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
	md := FormatListMarkdown(ListOutput{Templates: []TemplateListItem{{Key: "R", Name: "R"}}})
	if !strings.Contains(md, "R") {
		t.Error("missing")
	}
}

// TestFormatGetMarkdown verifies the GetMarkdown Markdown formatter for a representative get input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
	out, err := List(context.Background(), client, ListInput{
		Page: 2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Templates))
	}
}

// TestList_WithKeysetAndOrder verifies that List forwards keyset pagination
// (pagination, page_token) and ordering (order_by, sort) parameters onto the
// underlying GitLab templates request.
func TestList_WithKeysetAndOrder(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" || q.Get("page_token") != "tok" {
			t.Errorf("expected pagination=keyset&page_token=tok, got %s", r.URL.RawQuery)
		}
		if q.Get("order_by") != "name" || q.Get("sort") != "asc" {
			t.Errorf("expected order_by=name&sort=asc, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Go"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		OrderBy:    "name",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok",
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
		if spec.OwnerPackage != "dockerfiletemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_dockerfile_template"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_dockerfile_template should define key parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newDockerfileRouteSpecs(t)

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
	handler.HandleFunc("GET /api/v4/templates/dockerfiles", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/dockerfiles/Bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	specByTool := dockerfileTemplateSpecsByTool(ActionSpecs(client))

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

// newDockerfileRouteSpecs constructs dockerfile route specs test fixtures.
func newDockerfileRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/dockerfiles", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Ruby","name":"Ruby"},{"key":"Go","name":"Go"}]`)
	})
	handler.HandleFunc("GET /api/v4/templates/dockerfiles/Ruby", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Ruby","content":"FROM ruby:latest"}`)
	})

	client := testutil.NewTestClient(t, handler)
	return dockerfileTemplateSpecsByTool(ActionSpecs(client))
}

// dockerfileTemplateSpecsByTool supports dockerfile template specs by tool assertions in dockerfiletemplates tests.
func dockerfileTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// ---------------------------------------------------------------------------
// What a caller is actually handed
// ---------------------------------------------------------------------------.

// TestList_DistinctKeyAndName_KeepTheirOwnFields holds the mapping from
// GitLab's template list onto [TemplateListItem]. The key is the identifier
// the get action takes and the name is the label shown beside it, so the two
// are not interchangeable: a model that reads the name into `key` asks for a
// template that does not exist. Every other fixture in this file gives a
// template the same string for both, which leaves the two assignments free to
// swap with nothing failing; here they differ, so the test says which value
// belongs in which field.
func TestList_DistinctKeyAndName_KeepTheirOwnFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/dockerfiles")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"Go","name":"Dockerfile for Go"}]`)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Templates) != 1 {
		t.Fatalf("len(Templates) = %d, want 1", len(out.Templates))
	}
	if got := out.Templates[0].Key; got != "Go" {
		t.Errorf("Templates[0].Key = %q, want %q", got, "Go")
	}
	if got := out.Templates[0].Name; got != "Dockerfile for Go" {
		t.Errorf("Templates[0].Name = %q, want %q", got, "Dockerfile for Go")
	}
}

// TestList_ResponseHeaders_FillThePaginationBlock holds the pagination block
// to the headers GitLab answered with. The block is the only way a caller
// learns that the page it holds is not the whole catalog, so a handler that
// left it at its zero value, or filled it from another response, would tell
// every caller it had everything. No test asserted a single one of its
// fields, and a list whose block is empty looks exactly like a complete one.
func TestList_ResponseHeaders_FillThePaginationBlock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"key":"Go","name":"Dockerfile for Go"}]`, testutil.PaginationHeaders{
			Page:       "2",
			PerPage:    "1",
			Total:      "3",
			TotalPages: "3",
			NextPage:   "3",
			PrevPage:   "1",
		})
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := toolutil.PaginationOutput{
		Page:       2,
		PerPage:    1,
		TotalItems: 3,
		TotalPages: 3,
		NextPage:   3,
		PrevPage:   1,
		HasMore:    true,
	}
	if out.Pagination != want {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestGet_NameAndContent_KeepTheirOwnFields holds the two fields of a single
// template apart. The content is what a caller commits as its Dockerfile and
// the name is the label above it, so a handler that published the name as the
// content would hand back a one-word file. The existing get tests assert the
// name alone, and the formatter test looks only for "FROM", so nothing held
// the content to what GitLab sent.
func TestGet_NameAndContent_KeepTheirOwnFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/dockerfiles/Go")
		testutil.RespondJSON(w, http.StatusOK, `{"name":"Go","content":"FROM golang:1\nWORKDIR /src\n"}`)
	}))

	out, err := Get(t.Context(), client, GetInput{Key: "Go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "Go" {
		t.Errorf("Name = %q, want %q", out.Name, "Go")
	}
	if want := "FROM golang:1\nWORKDIR /src\n"; out.Content != want {
		t.Errorf("Content = %q, want %q", out.Content, want)
	}
}

// ---------------------------------------------------------------------------
// The hint each status carries
// ---------------------------------------------------------------------------.

// TestList_ErrorHint_OnlyOnForbidden holds the scope hint to the one status
// it answers. A 403 on the template catalog means the token cannot read the
// API, which is a thing the caller can fix, so the error says so; any other
// status has a different cause and must not carry that suggestion, or a model
// rotates a perfectly good token in response to an outage. The existing error
// tests assert only that some error came back, which is true of every wrong
// answer this could give.
func TestList_ErrorHint_OnlyOnForbidden(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "forbidden", status: http.StatusForbidden, wantHint: true},
		{name: "server error", status: http.StatusInternalServerError, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))

			_, err := List(t.Context(), client, ListInput{})
			if err == nil {
				t.Fatalf("expected an error for status %d", tc.status)
			}
			if !strings.HasPrefix(err.Error(), "list_dockerfile_templates: ") {
				t.Errorf("error = %q, want it to name the operation", err.Error())
			}
			const hint = "Suggestion: verify your token has read_api scope"
			if got := strings.Contains(err.Error(), hint); got != tc.wantHint {
				t.Errorf("error %q carries %q = %v, want %v", err.Error(), hint, got, tc.wantHint)
			}
		})
	}
}

// TestGet_ErrorHint_OnlyOnNotFound is the get half of
// [TestList_ErrorHint_OnlyOnForbidden]: a 404 means the key is not one GitLab
// serves, and the way out is the list action, so the error names it. A 500 is
// the instance failing and the same suggestion would send the caller looking
// for a typo that is not there.
func TestGet_ErrorHint_OnlyOnNotFound(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "not found", status: http.StatusNotFound, wantHint: true},
		{name: "server error", status: http.StatusInternalServerError, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))

			_, err := Get(t.Context(), client, GetInput{Key: "Nope"})
			if err == nil {
				t.Fatalf("expected an error for status %d", tc.status)
			}
			if !strings.HasPrefix(err.Error(), "get_dockerfile_template: ") {
				t.Errorf("error = %q, want it to name the operation", err.Error())
			}
			const hint = "Suggestion: verify name with template.dockerfile_list"
			if got := strings.Contains(err.Error(), hint); got != tc.wantHint {
				t.Errorf("error %q carries %q = %v, want %v", err.Error(), hint, got, tc.wantHint)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// What the rendered page shows
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_KeyAndNameRenderInTheirOwnColumns holds the rendered
// row to the two-column table the renderer declares. The key column is what a
// reader copies into the get action, so a formatter that rendered the name
// twice, or swapped the columns, would publish a page that reads correctly
// and cannot be acted on. The existing formatter test passes a template whose
// key and name are the same letter, which both faults survive.
func TestFormatListMarkdown_KeyAndNameRenderInTheirOwnColumns(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Templates: []TemplateListItem{{Key: "Go", Name: "Dockerfile for Go"}}})

	if !strings.Contains(md, "| Key | Name |\n") {
		t.Errorf("markdown = %q, want the two-column header", md)
	}
	if !strings.Contains(md, "| Go | Dockerfile for Go |\n") {
		t.Errorf("markdown = %q, want the key and the name in their own columns", md)
	}
}

// TestFormatGetMarkdown_NamesTheTemplateAndFencesItAsDockerfile holds the
// detail page to its two jobs: saying which template this is, and fencing the
// body as a Dockerfile so a client renders it as a file rather than as
// Markdown of the response. The existing test looks for "FROM" alone, which a
// page missing both the heading and the fence still contains.
func TestFormatGetMarkdown_NamesTheTemplateAndFencesItAsDockerfile(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "Go", Content: "FROM golang:1\n"})

	if !strings.Contains(md, "## Dockerfile Template: Go\n") {
		t.Errorf("markdown = %q, want the heading to name the template", md)
	}
	if !strings.Contains(md, "```dockerfile\nFROM golang:1\n```\n") {
		t.Errorf("markdown = %q, want the content fenced as dockerfile", md)
	}
}

// TestMarkdownRegistry_BothOutputs_ReachThisPackagesFormatters holds the
// init() registration. The surfaces render a result by looking its type up in
// the shared registry, so a formatter that is written and never registered
// publishes no Markdown at all and the caller is handed the raw JSON; nothing
// in this package noticed, because both formatter tests call the functions
// directly. An unregistered type resolves to nil, which is what this would
// catch.
func TestMarkdownRegistry_BothOutputs_ReachThisPackagesFormatters(t *testing.T) {
	list := ListOutput{Templates: []TemplateListItem{{Key: "Go", Name: "Dockerfile for Go"}}}
	get := GetOutput{Name: "Go", Content: "FROM golang:1\n"}

	cases := []struct {
		name   string
		output any
		want   string
	}{
		{name: "list output", output: list, want: FormatListMarkdown(list)},
		{name: "get output", output: get, want: FormatGetMarkdown(get)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dockerfileTemplateResultText(t, toolutil.MarkdownForResult(tc.output))
			if want := toolutil.NormalizeResultMarkdown(tc.want); got != want {
				t.Errorf("registry rendered %q, want %q", got, want)
			}
		})
	}
}

// dockerfileTemplateResultText unwraps the single text block a registered
// formatter produces, failing when the registry resolved the type to nothing.
func dockerfileTemplateResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if result == nil {
		t.Fatal("registry returned no result: the output type has no registered formatter")
	}
	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}
