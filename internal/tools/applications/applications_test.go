// applications_test.go contains unit tests for the OAuth application MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package applications

import (
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// fmtUnexpMethod identifies the fmt unexp method constant used by this package.
const fmtUnexpMethod = "unexpected method: %s"

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/applications (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 1, "application_id": "app-1", "application_name": "My App", "secret": "sec", "callback_url": "http://localhost", "confidential": true, "scopes": ["api", "read_user"]}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Applications) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Applications))
	}
	if out.Applications[0].ApplicationName != "My App" {
		t.Errorf("Name = %q, want My App", out.Applications[0].ApplicationName)
	}
	if out.Applications[0].ID != 1 {
		t.Errorf("ID = %d, want 1", out.Applications[0].ID)
	}
	if out.Applications[0].Scopes == nil || len(out.Applications[0].Scopes) != 2 {
		t.Errorf("Scopes = %v, want [\"api\", \"read_user\"]", out.Applications[0].Scopes)
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
		t.Fatal(errExpectedNil)
	}
}

// TestCreate verifies the Create handler.
// The mock GitLab API at /api/v4/applications (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 2,
			"application_id": "app-2",
			"application_name": "New App",
			"secret": "newsecret",
			"callback_url": "http://example.com/callback",
			"confidential": false,
			"scopes": ["api", "read_user"]
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		Name:        "New App",
		RedirectURI: "http://example.com/callback",
		Scopes:      "api read_user",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.ApplicationName != "New App" {
		t.Errorf("Name = %q, want New App", out.ApplicationName)
	}
	if out.Secret != "newsecret" {
		t.Errorf("Secret = %q, want newsecret", out.Secret)
	}
	if out.Scopes == nil || len(out.Scopes) != 2 {
		t.Errorf("Scopes = %v, want [\"api\", \"read_user\"]", out.Scopes)
	}
}

// TestCreate_Error verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Create(t.Context(), client, CreateInput{Name: "x", RedirectURI: "y", Scopes: "z"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_ValidationError verifies that Delete_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	}))
	for _, id := range []int64{0, -1} {
		err := Delete(t.Context(), client, DeleteInput{ID: id})
		if err == nil {
			t.Errorf("ID=%d: expected error, got nil", id)
		}
	}
}

// TestDelete verifies the Delete handler.
// The mock GitLab API at /api/v4/applications/3 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications/3" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Applications: []ApplicationItem{
			{ID: 1, ApplicationName: "App1", ApplicationID: "aid-1", CallbackURL: "http://localhost", Confidential: true},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "App1") {
		t.Error("missing app name")
	}
	if !strings.Contains(md, "aid-1") {
		t.Error("missing app id")
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Applications: nil}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No applications found") {
		t.Error("missing empty message")
	}
}

// TestFormatCreateMarkdown verifies the CreateMarkdown Markdown formatter for a representative create input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatCreateMarkdown(t *testing.T) {
	out := CreateOutput{ApplicationItem: ApplicationItem{
		ID: 2, ApplicationName: "New", ApplicationID: "aid-2", Secret: "sec", CallbackURL: "http://cb", Confidential: false,
	}}
	md := FormatCreateMarkdown(out)
	if !strings.Contains(md, "New") {
		t.Error("missing app name")
	}
	if !strings.Contains(md, "sec") {
		t.Error("missing secret")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — with pagination
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/applications (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodGet {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 5, "application_id": "app-5", "application_name": "Paged", "secret": "s", "callback_url": "http://cb", "confidential": false, "scopes": ["api"]}
			]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Applications) != 1 {
		t.Fatalf("expected 1 app, got %d", len(out.Applications))
	}
}

// TestList_WithKeysetAndSort verifies that List forwards keyset pagination
// parameters (pagination, page_token) and order_by/sort to the GitLab API.
// The mock GitLab API at /api/v4/applications (GET) asserts each query value
// and responds with HTTP OK.
func TestList_WithKeysetAndSort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodGet {
			q := r.URL.Query()
			if q.Get("pagination") != "keyset" {
				t.Errorf("expected pagination=keyset, got %s", q.Get("pagination"))
			}
			if q.Get("page_token") != "tok-9" {
				t.Errorf("expected page_token=tok-9, got %s", q.Get("page_token"))
			}
			if q.Get("order_by") != "id" {
				t.Errorf("expected order_by=id, got %s", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %s", q.Get("sort"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 7, "application_id": "app-7", "application_name": "Keyset", "secret": "s", "callback_url": "http://cb", "confidential": false, "scopes": ["api"]}
			]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		OrderBy:               "id",
		Sort:                  "desc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok-9"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Applications) != 1 {
		t.Fatalf("expected 1 app, got %d", len(out.Applications))
	}
}

// ---------------------------------------------------------------------------
// Create — with confidential flag
// ---------------------------------------------------------------------------.

// TestCreate_WithConfidential verifies the Create_WithConfidential handler.
// The mock GitLab API at /api/v4/applications (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreate_WithConfidential(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 10, "application_id": "app-10", "application_name": "Conf App",
				"secret": "csec", "callback_url": "http://cb", "confidential": true, "scopes": ["read_user"]
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	conf := true
	out, err := Create(t.Context(), client, CreateInput{
		Name:         "Conf App",
		RedirectURI:  "http://cb",
		Scopes:       "api",
		Confidential: &conf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Confidential {
		t.Error("expected confidential=true")
	}
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	byTool := applicationSpecsByTool(client)
	for _, spec := range specs {
		if spec.OwnerPackage != "applications" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}

	list := byTool["gitlab_list_applications"]
	if list.Usage == "" || len(list.Aliases) == 0 || list.IndividualTool.Description == "" {
		t.Fatalf("list metadata incomplete: usage=%q aliases=%d description=%q", list.Usage, len(list.Aliases), list.IndividualTool.Description)
	}

	create := byTool["gitlab_create_application"]
	if create.Usage == "" || len(create.Aliases) == 0 || create.ParameterGuidance["redirect_uri"].SemanticRole == "" {
		t.Fatalf("create metadata incomplete: usage=%q aliases=%d redirect_uri guidance=%q", create.Usage, len(create.Aliases), create.ParameterGuidance["redirect_uri"].SemanticRole)
	}

	deleteSpec := byTool["gitlab_delete_application"]
	if deleteSpec.Usage == "" || len(deleteSpec.Aliases) == 0 || deleteSpec.ParameterGuidance["id"].SemanticRole == "" {
		t.Fatalf("delete metadata incomplete: usage=%q aliases=%d id guidance=%q", deleteSpec.Usage, len(deleteSpec.Aliases), deleteSpec.ParameterGuidance["id"].SemanticRole)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newApplicationsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_applications", map[string]any{}},
		{"create", "gitlab_create_application", map[string]any{
			"name": "Test App", "redirect_uri": "http://cb", "scopes": "api",
		}},
		{"delete", "gitlab_delete_application", map[string]any{"id": float64(1)}},
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
	specByTool := newErrorRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_applications", map[string]any{}},
		{"create_error", "gitlab_create_application", map[string]any{
			"name": "X", "redirect_uri": "http://cb", "scopes": "api",
		}},
		{"delete_error", "gitlab_delete_application", map[string]any{"id": float64(99)}},
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

// newErrorRouteSpecs constructs error route specs test fixtures.
func newErrorRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	return applicationSpecsByTool(client)
}

// newApplicationsRouteSpecs constructs applications route specs test fixtures.
func newApplicationsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/applications", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"application_id":"a1","application_name":"App1","secret":"s","callback_url":"http://cb","confidential":true,"scopes":["api","read_user"]}]`)
	})
	handler.HandleFunc("POST /api/v4/applications", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"application_id":"a2","application_name":"Test App","secret":"s2","callback_url":"http://cb","confidential":false,"scopes":["api"]}`)
	})
	handler.HandleFunc("DELETE /api/v4/applications/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	return applicationSpecsByTool(client)
}

// applicationSpecsByTool supports application specs by tool assertions in applications tests.
func applicationSpecsByTool(client *gitlabclient.Client) map[string]toolutil.ActionSpec {
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
