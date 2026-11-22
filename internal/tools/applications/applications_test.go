// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// applications_test.go contains unit tests for the OAuth application MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package applications

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

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

// TestCreate_GranularScopesAndWarnings_AreReadOffTheCapture verifies the two
// keys 19.6 adds to a create answer, which client-go's Application does not
// model, are read off the captured response: each granular scope with the
// project or group it names, and the warnings GitLab raised.
func TestCreate_GranularScopesAndWarnings_AreReadOffTheCapture(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"application_name":"New App","scopes":["api"],`+
			`"granular_scopes":[{"access":"selected_memberships","permissions":["read_job"],"project_id":3,"group_id":null},`+
			`{"access":"selected_memberships","permissions":["read_project"],"project_id":null,"group_id":5}],`+
			`"warnings":["Tokens cannot use the MCP server without a user-level scope granting execute_mcp_tool."]}`)
	}))
	out, err := Create(t.Context(), client, CreateInput{Name: "New App", RedirectURI: "http://example.com", Scopes: "api"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	wantScopes := []toolutil.TokenGranularScopeOutput{
		{Access: "selected_memberships", Permissions: []string{"read_job"}, ProjectID: 3},
		{Access: "selected_memberships", Permissions: []string{"read_project"}, GroupID: 5},
	}
	if !reflect.DeepEqual(out.GranularScopes, wantScopes) {
		t.Errorf("granular_scopes = %+v, want %+v", out.GranularScopes, wantScopes)
	}
	wantWarnings := []string{"Tokens cannot use the MCP server without a user-level scope granting execute_mcp_tool."}
	if !reflect.DeepEqual(out.Warnings, wantWarnings) {
		t.Errorf("warnings = %q, want %q", out.Warnings, wantWarnings)
	}
}

// TestCreate_AWarningTheTypeCannotHold_IsReported verifies the create reports
// captured warnings in a shape the extra cannot hold.
func TestCreate_AWarningTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"warnings":"one"}`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "create", Call: func() error {
			_, err := Create(t.Context(), client, CreateInput{Name: "x", RedirectURI: "y", Scopes: "api"})
			return err
		}},
	})
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

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab
// API answers 404, and that the status hint sends the caller back for the
// numeric id this action takes. The same list row also carries an
// application_id, which is the OAuth client string, so a hint naming that field
// points a model at the wrong column of the row it just read and it retries
// with a string where an id belongs.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "verify application id") {
		t.Errorf("wrapped error missing 404 status hint %q: %v", "verify application id", err)
	}
	if strings.Contains(err.Error(), "application_id") {
		t.Errorf("404 hint names the application_id column instead of the id parameter: %v", err)
	}
}

// appListHints is the guidance section every application list ends with. The
// table carries no link, so the preserve-links reminder is not written.
const appListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use `admin.application_create` to register a new application\n"

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
		"| 1 | App1 | `aid-1` | http\\://localhost | " + toolutil.BoolEmoji(true) + " | api, read_user |\n" +
		"| 2 | App2 | `aid-2` | http\\://localhost/two | " + toolutil.BoolEmoji(false) + " |  |\n" +
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
		"- **Callback URL**: http\\://cb\n" +
		"- **Confidential**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Secret**: `sec`\n" +
		"- **Scopes**: api\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Store the secret securely. It cannot be retrieved later\n"

	if got := FormatCreateMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatCreateMarkdown_Warnings_AreShownInTheText pins the warnings
// GitLab 19.6 raises about a new application's granular scopes in the card,
// one row each and escaped, so a client that reads only the text learns of
// them as well as one that reads structuredContent.
func TestFormatCreateMarkdown_Warnings_AreShownInTheText(t *testing.T) {
	out := CreateOutput{
		ID: 4, ApplicationName: "Warned", ApplicationID: "aid-4", Secret: "sec", CallbackURL: "http://cb",
		Warnings: []string{
			"Tokens cannot use the MCP server without a user-level scope granting execute_mcp_tool.",
			"a | b",
		},
	}

	want := "## Application Created\n\n" +
		"- **ID**: 4\n" +
		"- **Name**: Warned\n" +
		"- **App ID**: `aid-4`\n" +
		"- **Callback URL**: http\\://cb\n" +
		"- **Confidential**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Secret**: `sec`\n" +
		"\n### Warnings\n\n" +
		"| Message |\n| --- |\n" +
		"| Tokens cannot use the MCP server without a user-level scope granting execute_mcp_tool. |\n" +
		"| a &#124; b |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Store the secret securely. It cannot be retrieved later\n"

	if got := FormatCreateMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%q\nwant:\n%q", got, want)
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
		"- **Callback URL**: http\\://cb\n" +
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
		"- **Callback URL**: http\\://cb\n" +
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

// TestList_WithSort verifies that List forwards order_by/sort to the GitLab
// API. The mock GitLab API at /api/v4/applications (GET) asserts each query
// value and responds with HTTP OK.
func TestList_WithSort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodGet {
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
		OrderBy: "id",
		Sort:    "desc",
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
