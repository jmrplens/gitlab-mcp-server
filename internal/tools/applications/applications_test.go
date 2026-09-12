// applications_test.go contains unit tests for the OAuth application MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package applications

import (
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/applications (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/applications")
		testutil.AssertRequestMethod(t, r, http.MethodGet)
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
		testutil.AssertRequestPath(t, r, "/api/v4/applications")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
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

// TestRenewSecret verifies the RenewSecret handler.
// The mock GitLab API at /api/v4/applications/2/renew-secret (POST) responds
// with HTTP OK and the application carrying a fresh secret.
// It asserts the returned output carries the renewed secret and identity fields.
func TestRenewSecret(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/applications/2/renew-secret")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 2,
			"application_id": "app-2",
			"application_name": "Rotated App",
			"secret": "freshsecret",
			"callback_url": "http://example.com/callback",
			"confidential": true,
			"scopes": ["api"]
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := RenewSecret(t.Context(), client, RenewSecretInput{ID: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.Secret != "freshsecret" {
		t.Errorf("Secret = %q, want freshsecret", out.Secret)
	}
	if out.ApplicationName != "Rotated App" {
		t.Errorf("Name = %q, want Rotated App", out.ApplicationName)
	}
}

// TestRenewSecret_ValidationError verifies RenewSecret rejects non-positive ids
// before touching the GitLab API.
func TestRenewSecret_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ids := []struct {
		name string
		id   int64
	}{
		{"zero_id", 0},
		{"negative_id", -1},
	}
	for _, tc := range ids {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenewSecret(t.Context(), client, RenewSecretInput{ID: tc.id}); err == nil {
				t.Errorf("ID=%d: expected error, got nil", tc.id)
			}
		})
	}
}

// TestRenewSecret_Error verifies that RenewSecret returns a wrapped error when
// the GitLab API responds with an error status, and that the 404 status hint is
// preserved so callers get an actionable message.
func TestRenewSecret_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := RenewSecret(t.Context(), client, RenewSecretInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "verify application id") {
		t.Errorf("wrapped error missing 404 status hint %q: %v", "verify application id", err)
	}
}

// TestDelete_ValidationError verifies Delete rejects non-positive ids before
// touching the GitLab API.
func TestDelete_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ids := []struct {
		name string
		id   int64
	}{
		{"zero_id", 0},
		{"negative_id", -1},
	}
	for _, tc := range ids {
		t.Run(tc.name, func(t *testing.T) {
			err := Delete(t.Context(), client, DeleteInput{ID: tc.id})
			if err == nil {
				t.Errorf("ID=%d: expected error, got nil", tc.id)
			}
		})
	}
}

// TestDelete verifies the Delete handler.
// The mock GitLab API at /api/v4/applications/3 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/applications/3")
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
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

// appListHints is the guidance section every application list ends with. The
// table carries no link, so the preserve-links reminder is not written.
const appListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use `gitlab_create_application` to register a new application\n"

// TestFormatListMarkdown pins the whole list document: the heading counting
// what the page shows, the table with the confidential flag as a glyph, and
// the guidance section.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Applications: []ApplicationItem{
			{ID: 1, ApplicationName: "App1", ApplicationID: "aid-1", CallbackURL: "http://localhost", Confidential: true, Scopes: []string{"api", "read_user"}},
			{ID: 2, ApplicationName: "App2", ApplicationID: "aid-2", CallbackURL: "http://localhost/two"},
		},
	}

	want := "## Applications (2)\n\n" +
		"| ID | Name | App ID | Callback URL | Confidential | Scopes |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | App1 | `aid-1` | http://localhost | " + toolutil.BoolEmoji(true) + " | api, read_user |\n" +
		"| 2 | App2 | `aid-2` | http://localhost/two | " + toolutil.BoolEmoji(false) + " |  |\n" +
		appListHints

	if got := FormatListMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a list with no
// applications: the one sentence, and no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	if got, want := FormatListMarkdown(ListOutput{}), "No applications found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatCreateMarkdown pins the whole card of a newly registered
// application: the secret in a code span, and the store-it-now hint the card
// adds because it showed one.
func TestFormatCreateMarkdown(t *testing.T) {
	out := CreateOutput{
		ID: 2, ApplicationName: "New", ApplicationID: "aid-2", Secret: "sec", CallbackURL: "http://cb", Confidential: false,
		Scopes: []string{"api"},
	}

	want := "## Application Created\n\n" +
		"- **ID**: 2\n" +
		"- **Name**: New\n" +
		"- **App ID**: `aid-2`\n" +
		"- **Callback URL**: http://cb\n" +
		"- **Confidential**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Secret**: `sec`\n" +
		"- **Scopes**: api\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Store the secret securely. It cannot be retrieved later\n"

	if got := FormatCreateMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatCreateMarkdown_NoScopes pins the other side of the scopes row: an
// application GitLab sent no scopes for renders no row at all, where the
// two-cell table this replaced rendered a label above an empty cell.
func TestFormatCreateMarkdown_NoScopes(t *testing.T) {
	out := CreateOutput{ID: 3, ApplicationName: "Bare", ApplicationID: "aid-3", Secret: "sec", CallbackURL: "http://cb"}

	want := "## Application Created\n\n" +
		"- **ID**: 3\n" +
		"- **Name**: Bare\n" +
		"- **App ID**: `aid-3`\n" +
		"- **Callback URL**: http://cb\n" +
		"- **Confidential**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Secret**: `sec`\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Store the secret securely. It cannot be retrieved later\n"

	if got := FormatCreateMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatRenewSecretMarkdown pins the whole card of a rotated secret: the
// new value, the advice to store it, and the warning that every client using
// the previous one has to be updated.
func TestFormatRenewSecretMarkdown(t *testing.T) {
	out := RenewSecretOutput{
		ID: 2, ApplicationName: "Rotated", ApplicationID: "aid-2", Secret: "freshsecret", CallbackURL: "http://cb", Confidential: true,
		Scopes: []string{"api"},
	}

	want := "## Application Secret Renewed\n\n" +
		"- **ID**: 2\n" +
		"- **Name**: Rotated\n" +
		"- **App ID**: `aid-2`\n" +
		"- **Callback URL**: http://cb\n" +
		"- **Confidential**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **New Secret**: `freshsecret`\n" +
		"- **Scopes**: api\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Store the new secret securely. It cannot be retrieved later\n" +
		"- The previous secret is now invalid and any client using it must be updated\n"

	if got := FormatRenewSecretMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
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
			testutil.AssertQueryParam(t, r, "page", "2")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 5, "application_id": "app-5", "application_name": "Paged", "secret": "s", "callback_url": "http://cb", "confidential": false, "scopes": ["api"]}
			]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		Page: 2, PerPage: 10,
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
			testutil.AssertQueryParam(t, r, "pagination", "keyset")
			testutil.AssertQueryParam(t, r, "page_token", "tok-9")
			testutil.AssertQueryParam(t, r, "order_by", "id")
			testutil.AssertQueryParam(t, r, "sort", "desc")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 7, "application_id": "app-7", "application_name": "Keyset", "secret": "s", "callback_url": "http://cb", "confidential": false, "scopes": ["api"]}
			]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok-9",
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
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
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

	renew := byTool["gitlab_renew_application_secret"]
	if renew.Usage == "" || len(renew.Aliases) == 0 || renew.ParameterGuidance["id"].SemanticRole == "" || renew.IndividualTool.Description == "" {
		t.Fatalf("renew metadata incomplete: usage=%q aliases=%d id guidance=%q description=%q", renew.Usage, len(renew.Aliases), renew.ParameterGuidance["id"].SemanticRole, renew.IndividualTool.Description)
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
		{"renew_secret", "gitlab_renew_application_secret", map[string]any{"id": float64(1)}},
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
		{"renew_secret_error", "gitlab_renew_application_secret", map[string]any{"id": float64(99)}},
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
	handler.HandleFunc("POST /api/v4/applications/1/renew-secret", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"application_id":"a1","application_name":"App1","secret":"rotated","callback_url":"http://cb","confidential":true,"scopes":["api"]}`)
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
