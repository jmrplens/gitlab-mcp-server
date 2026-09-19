// error_tracking_test.go contains unit tests for the error tracking MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package errortracking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestGetSettings verifies the GetSettings handler.
// The mock GitLab API at /api/v4/projects/1/error_tracking/settings (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGetSettings(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/settings" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"active":true,"project_name":"test","sentry_external_url":"https://sentry.io","api_url":"https://sentry.io/api","integrated":false}`)
	}))
	out, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Active {
		t.Error("expected active=true")
	}
	if out.ProjectName != "test" {
		t.Errorf("expected project_name=test, got %s", out.ProjectName)
	}
	if out.Integrated {
		t.Error("expected integrated=false")
	}
}

// TestGetSettings_Error verifies that GetSettings returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetSettings_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEnableDisable verifies the EnableDisable handler.
// The mock GitLab API at /api/v4/projects/1/error_tracking/settings (PATCH) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestEnableDisable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/settings" || r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"active":true,"project_name":"test","integrated":true}`)
	}))
	active := true
	integrated := true
	out, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1", Active: &active, Integrated: &integrated})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Active || !out.Integrated {
		t.Error("expected active=true, integrated=true")
	}
}

// TestEnableDisable_Error verifies that EnableDisable returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnableDisable_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListClientKeys verifies the ListClientKeys handler.
// The mock GitLab API at /api/v4/projects/1/error_tracking/client_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListClientKeys(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"active":true,"public_key":"pk1","sentry_dsn":"dsn1"},{"id":2,"active":false,"public_key":"pk2","sentry_dsn":"dsn2"}]`)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(out.Keys))
	}
	if out.Keys[0].PublicKey != "pk1" {
		t.Errorf("expected pk1, got %s", out.Keys[0].PublicKey)
	}
}

// TestListClientKeys_Error verifies that ListClientKeys returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListClientKeys_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateClientKey verifies the CreateClientKey handler.
// The mock GitLab API at /api/v4/projects/1/error_tracking/client_keys (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreateClientKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"active":true,"public_key":"newpk","sentry_dsn":"newdsn"}`)
	}))
	out, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 || out.PublicKey != "newpk" {
		t.Errorf("unexpected key: %+v", out)
	}
}

// TestCreateClientKey_Error verifies that CreateClientKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey verifies the DeleteClientKey handler.
// The mock GitLab API at /api/v4/projects/1/error_tracking/client_keys/10 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDeleteClientKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys/10" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteClientKey_Error verifies that DeleteClientKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey_InvalidKeyID verifies the DeleteClientKey_InvalidKeyID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteClientKey_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 0})
	if err == nil {
		t.Fatal("expected error for zero key_id")
	}
	if !strings.Contains(err.Error(), "key_id") {
		t.Errorf("expected error to mention key_id, got %q", err)
	}
	err = DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: -1})
	if err == nil {
		t.Fatal("expected error for negative key_id")
	}
}

// The Markdown formatters are asserted whole in markdown_test.go.

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Constants & fixtures
// ---------------------------------------------------------------------------.

// covKeyJSON identifies the cov key JSON constant used by this package.
const covKeyJSON = `{"id":1,"active":true,"public_key":"pk-abc","sentry_dsn":"https://dsn"}`

// ---------------------------------------------------------------------------
// ListClientKeys — offset pagination branch (Page > 0, PerPage > 0)
// ---------------------------------------------------------------------------.

// TestListClientKeys_WithPagination verifies that ListClientKeys forwards offset
// pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/error_tracking/client_keys (GET) responds with HTTP OK.
// It asserts the page/per_page query parameters are forwarded and the response
// metadata is propagated to the [toolutil.PaginationOutput].
func TestListClientKeys_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/error_tracking/client_keys" && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("page query = %q, want 2", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "1" {
				t.Errorf("per_page query = %q, want 1", got)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covKeyJSON+`]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{
		ProjectID: "1",
		Page:      2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.Keys))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// ListClientKeys — keyset pagination branch (order_by/sort/page_token/pagination)
// ---------------------------------------------------------------------------.

// TestListClientKeys_WithKeyset verifies that ListClientKeys forwards keyset
// pagination parameters (order_by, sort, pagination, page_token) onto the
// underlying gl.ListOptions and through to the GitLab API query string.
// The mock GitLab API at /api/v4/projects/1/error_tracking/client_keys (GET) responds with HTTP OK.
// It asserts each keyset query parameter is forwarded exactly as supplied.
func TestListClientKeys_WithKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/error_tracking/client_keys" && r.Method == http.MethodGet {
			q := r.URL.Query()
			if got := q.Get("order_by"); got != "id" {
				t.Errorf("order_by query = %q, want id", got)
			}
			if got := q.Get("sort"); got != "asc" {
				t.Errorf("sort query = %q, want asc", got)
			}
			if got := q.Get("pagination"); got != "keyset" {
				t.Errorf("pagination query = %q, want keyset", got)
			}
			if got := q.Get("page_token"); got != "tok-5" {
				t.Errorf("page_token query = %q, want tok-5", got)
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+covKeyJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok-5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.Keys))
	}
}

// ---------------------------------------------------------------------------
// The Markdown formatters are asserted whole in markdown_test.go.

// ---------------------------------------------------------------------------
// Field mapping: each published field comes from its own response field
// ---------------------------------------------------------------------------.

const (
	// pathSettings is the settings endpoint of the fixture project.
	pathSettings = "/api/v4/projects/1/error_tracking/settings"
	// pathClientKeys is the client key collection of the fixture project.
	pathClientKeys = "/api/v4/projects/1/error_tracking/client_keys"
	// sentryDashboardURL is the Sentry page a person opens.
	sentryDashboardURL = "https://sentry.example.com/org/proj"
	// sentryIngestURL is the endpoint events are posted to; deliberately
	// unlike [sentryDashboardURL], so a field carrying the other one shows.
	sentryIngestURL = "https://sentry.example.com/api/0/projects/org/proj"
)

// TestGetSettings_SentryURLs_EachLandsInItsOwnField asserts that the two URLs a
// settings response carries reach the caller under their own names.
//
// Why it matters: this pair is the only place in the package where two string
// fields of one object are interchangeable, and the settings tests asserted
// neither of them. Swapped, the card's "Sentry URL" would send a reader to the
// ingest endpoint and its "API URL" to a dashboard, and every test still
// passed — confirmed by applying the swap by hand.
func TestGetSettings_SentryURLs_EachLandsInItsOwnField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathSettings || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(
			`{"active":true,"project_name":"proj","sentry_external_url":%q,"api_url":%q,"integrated":false}`,
			sentryDashboardURL, sentryIngestURL,
		))
	}))
	out, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.SentryExternalURL != sentryDashboardURL {
		t.Errorf("SentryExternalURL = %q, want the sentry_external_url GitLab sent", out.SentryExternalURL)
	}
	if out.APIURL != sentryIngestURL {
		t.Errorf("APIURL = %q, want the api_url GitLab sent", out.APIURL)
	}
}

// TestEnableDisable_Flags_TravelUnderTheirOwnNamesBothWays asserts that active
// and integrated are sent to GitLab under their own names, and that the
// settings GitLab answers with are published under theirs.
//
// Why it matters: these are the two booleans of a mutating action, they are
// interchangeable in the options struct, and nothing but the request body can
// tell them apart. Swapped, a caller asking to move a project onto GitLab's
// integrated backend would instead switch error tracking off — and the
// response echo would still read correctly, because GitLab answers with both.
// The suite passed with the swap applied until this test existed. The
// handler builds its own copy of the response mapping, so the URL assertions
// are repeated here rather than left to the get-settings test.
func TestEnableDisable_Flags_TravelUnderTheirOwnNamesBothWays(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathSettings || r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Active     *bool `json:"active"`
			Integrated *bool `json:"integrated"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the update body: %v", err)
		}
		switch {
		case body.Active == nil:
			t.Error("active absent from the request body, want false")
		case *body.Active:
			t.Error("active in the request body = true, want the false the caller asked for")
		}
		switch {
		case body.Integrated == nil:
			t.Error("integrated absent from the request body, want true")
		case !*body.Integrated:
			t.Error("integrated in the request body = false, want the true the caller asked for")
		}
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(
			`{"active":false,"project_name":"proj","sentry_external_url":%q,"api_url":%q,"integrated":true}`,
			sentryDashboardURL, sentryIngestURL,
		))
	}))
	active, integrated := false, true
	out, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1", Active: &active, Integrated: &integrated})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Active {
		t.Error("Active = true, want the false GitLab answered with")
	}
	if !out.Integrated {
		t.Error("Integrated = false, want the true GitLab answered with")
	}
	if out.ProjectName != "proj" {
		t.Errorf("ProjectName = %q, want proj", out.ProjectName)
	}
	if out.SentryExternalURL != sentryDashboardURL {
		t.Errorf("SentryExternalURL = %q, want the sentry_external_url GitLab sent", out.SentryExternalURL)
	}
	if out.APIURL != sentryIngestURL {
		t.Errorf("APIURL = %q, want the api_url GitLab sent", out.APIURL)
	}
}

// TestClientKeyHandlers_EveryPublishedField_ComesFromItsOwnResponseField
// asserts that a client key's four fields each carry the response field of the
// same name, for the create handler and for each row of the list handler.
//
// Why it matters: the key tests asserted the public key and the id and left
// the DSN and the active flag to chance, and both handlers build the mapping
// separately. The public key and the DSN are the two values a reader copies
// into a client's configuration; a DSN filled from the public key produces a
// string that looks plausible and sends no events. The list row's flag is
// asserted on two keys that disagree, so neither a hardcoded true nor a
// hardcoded false survives.
func TestClientKeyHandlers_EveryPublishedField_ComesFromItsOwnResponseField(t *testing.T) {
	const (
		createdKeyJSON = `{"id":77,"active":false,"public_key":"pk-created","sentry_dsn":"https://dsn-created.example.com"}`
		listedKeysJSON = `[{"id":11,"active":true,"public_key":"pk-first","sentry_dsn":"https://dsn-first.example.com"},` +
			`{"id":22,"active":false,"public_key":"pk-second","sentry_dsn":"https://dsn-second.example.com"}]`
	)
	assertKey := func(t *testing.T, got ClientKeyItem, wantID int64, wantActive bool, wantKey, wantDSN string) {
		t.Helper()
		if got.ID != wantID {
			t.Errorf("ID = %d, want %d", got.ID, wantID)
		}
		if got.Active != wantActive {
			t.Errorf("Active = %t, want the %t GitLab sent", got.Active, wantActive)
		}
		if got.PublicKey != wantKey {
			t.Errorf("PublicKey = %q, want %q", got.PublicKey, wantKey)
		}
		if got.SentryDsn != wantDSN {
			t.Errorf("SentryDsn = %q, want %q", got.SentryDsn, wantDSN)
		}
	}

	t.Run("create", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != pathClientKeys || r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			testutil.RespondJSON(w, http.StatusCreated, createdKeyJSON)
		}))
		out, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
		if err != nil {
			t.Fatalf(fmtUnexpErr, err)
		}
		assertKey(t, out, 77, false, "pk-created", "https://dsn-created.example.com")
	})

	t.Run("list", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != pathClientKeys || r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			testutil.RespondJSON(w, http.StatusOK, listedKeysJSON)
		}))
		out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
		if err != nil {
			t.Fatalf(fmtUnexpErr, err)
		}
		if len(out.Keys) != 2 {
			t.Fatalf("keys = %d, want 2", len(out.Keys))
		}
		assertKey(t, out.Keys[0], 11, true, "pk-first", "https://dsn-first.example.com")
		assertKey(t, out.Keys[1], 22, false, "pk-second", "https://dsn-second.example.com")
	})
}

// TestListClientKeys_PaginationBlock_CarriesEveryPageHeaderGitLabSent asserts
// that every field of the pagination block is filled from the response
// headers, and that HasMore states whether a further page exists.
//
// Why it matters: only the page count was asserted, so the block could lose
// the page, the per-page size, the totals and both neighbors and still pass.
// That block is the only way a caller learns there is more to ask for — the
// blind spot R-PAGE was built for — and a key list that hides its successor
// leaves a maintainer believing they have seen every key their project
// publishes.
func TestListClientKeys_PaginationBlock_CarriesEveryPageHeaderGitLabSent(t *testing.T) {
	keysWithHeaders := func(t *testing.T, headers testutil.PaginationHeaders) ListClientKeysOutput {
		t.Helper()
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != pathClientKeys || r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covKeyJSON+`]`, headers)
		}))
		out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
		if err != nil {
			t.Fatalf(fmtUnexpErr, err)
		}
		return out
	}

	t.Run("a middle page reports both neighbors", func(t *testing.T) {
		got := keysWithHeaders(t, testutil.PaginationHeaders{
			Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1",
		}).Pagination
		want := toolutil.PaginationOutput{
			Page: 2, PerPage: 1, TotalItems: 3, TotalPages: 3, NextPage: 3, PrevPage: 1, HasMore: true,
		}
		if got != want {
			t.Errorf("pagination = %+v, want %+v", got, want)
		}
	})

	t.Run("the last page reports no successor", func(t *testing.T) {
		got := keysWithHeaders(t, testutil.PaginationHeaders{
			Page: "3", PerPage: "1", Total: "3", TotalPages: "3", PrevPage: "2",
		}).Pagination
		if got.NextPage != 0 {
			t.Errorf("NextPage = %d, want 0 when GitLab sent no X-Next-Page", got.NextPage)
		}
		if got.HasMore {
			t.Error("HasMore = true on the last page, which tells a caller to ask for a page that does not exist")
		}
	})
}

// ---------------------------------------------------------------------------
// Error classification: a hint reaches the caller on its own status only
// ---------------------------------------------------------------------------.

// hintMismatchStatus is a status no handler in this package attaches a hint
// to, so it is what each case is contrasted against.
const hintMismatchStatus = http.StatusNotFound

// errorTrackingClientAnswering returns a client whose GitLab answers every
// request with status.
func errorTrackingClientAnswering(t *testing.T, status int) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, `{"message":"refused"}`)
	}))
}

// TestErrorTrackingHandlers_StatusHint_ReachesTheCallerOnItsOwnStatusOnly
// asserts, for each of the five handlers, that the corrective hint it declares
// is in the error GitLab's own status produces, and is absent from an error
// with another status.
//
// Why it matters: every handler here classifies its failure through
// [toolutil.WrapErrWithStatusHint], whose whole behavior is the status it is
// given, and the error tests asserted only that some error came back. The
// status was therefore free: changing the settings handler's 403 to a 404 left
// the suite green, which means the hint telling a model it needs the Maintainer
// role could stop being attached to the refusal that needs it without anything
// noticing. Both directions are asserted, because a hint on every status is as
// wrong as a hint on none: it would advise about permissions on a project that
// does not exist.
func TestErrorTrackingHandlers_StatusHint_ReachesTheCallerOnItsOwnStatusOnly(t *testing.T) {
	cases := []struct {
		name   string
		status int
		hint   string
		call   func(context.Context, *gitlabclient.Client) error
	}{
		{
			name:   "get_settings",
			status: http.StatusForbidden,
			hint:   "must be enabled at the instance level",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := GetSettings(ctx, c, GetSettingsInput{ProjectID: "1"})
				return err
			},
		},
		{
			name:   "enable_disable",
			status: http.StatusBadRequest,
			hint:   "active=true requires error tracking to be configured",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := EnableDisable(ctx, c, EnableDisableInput{ProjectID: "1"})
				return err
			},
		},
		{
			name:   "list_client_keys",
			status: http.StatusForbidden,
			hint:   "client keys are used by SDK clients to send events",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListClientKeys(ctx, c, ListClientKeysInput{ProjectID: "1"})
				return err
			},
		},
		{
			name:   "create_client_key",
			status: http.StatusBadRequest,
			hint:   "the returned public_key is used as the Sentry DSN",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := CreateClientKey(ctx, c, CreateClientKeyInput{ProjectID: "1"})
				return err
			},
		},
		{
			name:   "delete_client_key",
			status: http.StatusForbidden,
			hint:   "deletion is irreversible",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				return DeleteClientKey(ctx, c, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status == hintMismatchStatus {
				t.Fatalf("case declares the contrast status %d, so the second half asserts nothing", hintMismatchStatus)
			}
			t.Run("its own status carries the hint", func(t *testing.T) {
				err := tc.call(t.Context(), errorTrackingClientAnswering(t, tc.status))
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if !strings.Contains(err.Error(), tc.hint) {
					t.Errorf("a %d produced %q, which does not carry the hint %q", tc.status, err, tc.hint)
				}
			})
			t.Run("another status does not", func(t *testing.T) {
				err := tc.call(t.Context(), errorTrackingClientAnswering(t, hintMismatchStatus))
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if strings.Contains(err.Error(), tc.hint) {
					t.Errorf("a %d carries the %d hint %q", hintMismatchStatus, tc.status, tc.hint)
				}
			})
		})
	}
}
