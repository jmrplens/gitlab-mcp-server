// errortracking_test.go contains unit tests for the error tracking MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package errortracking

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestGetSettings verifies GetSettings.
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

// TestGetSettings_Error verifies GetSettings when error.
func TestGetSettings_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEnableDisable verifies EnableDisable.
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

// TestEnableDisable_Error verifies EnableDisable when error.
func TestEnableDisable_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListClientKeys verifies ListClientKeys.
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

// TestListClientKeys_Error verifies ListClientKeys when error.
func TestListClientKeys_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateClientKey verifies CreateClientKey.
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

// TestCreateClientKey_Error verifies CreateClientKey when error.
func TestCreateClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey verifies DeleteClientKey.
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

// TestDeleteClientKey_Error verifies DeleteClientKey when error.
func TestDeleteClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey_InvalidKeyID verifies DeleteClientKey when invalid key ID.
func TestDeleteClientKey_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
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

// TestFormatSettingsMarkdown verifies FormatSettingsMarkdown.
func TestFormatSettingsMarkdown(t *testing.T) {
	out := SettingsOutput{Active: true, ProjectName: "test", SentryExternalURL: "https://sentry.io", Integrated: false}
	md := FormatSettingsMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatListKeysMarkdown verifies FormatListKeysMarkdown.
func TestFormatListKeysMarkdown(t *testing.T) {
	out := ListClientKeysOutput{Keys: []ClientKeyItem{{ID: 1, Active: true, PublicKey: "pk", SentryDsn: "dsn"}}}
	md := FormatListKeysMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Constants & fixtures
// ---------------------------------------------------------------------------.

const (
	// covSettingsJSON identifies the cov settings JSON constant used by this package.
	covSettingsJSON = `{"active":true,"project_name":"proj","sentry_external_url":"https://sentry.io","api_url":"https://sentry.io/api","integrated":true}`
	// covKeyJSON identifies the cov key JSON constant used by this package.
	covKeyJSON = `{"id":1,"active":true,"public_key":"pk-abc","sentry_dsn":"https://dsn"}`
)

// ---------------------------------------------------------------------------
// ListClientKeys — pagination branch (Page > 0, PerPage > 0)
// ---------------------------------------------------------------------------.

// TestListClientKeys_WithPagination verifies ListClientKeys when with pagination.
func TestListClientKeys_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/error_tracking/client_keys" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covKeyJSON+`]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1", Page: 2, PerPage: 1})
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
// FormatKeyMarkdown
// ---------------------------------------------------------------------------.

// TestFormatKeyMarkdown verifies FormatKeyMarkdown.
func TestFormatKeyMarkdown(t *testing.T) {
	md := FormatKeyMarkdown(ClientKeyItem{ID: 42, Active: true, PublicKey: "pk-123", SentryDsn: "https://dsn.example.com"})
	for _, want := range []string{
		"## Error Tracking Client Key",
		"**ID**: 42",
		"**Active**: true",
		"**Public Key**: pk-123",
		"**Sentry DSN**: https://dsn.example.com",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatKeyMarkdown_Inactive verifies FormatKeyMarkdown when inactive.
func TestFormatKeyMarkdown_Inactive(t *testing.T) {
	md := FormatKeyMarkdown(ClientKeyItem{ID: 7, Active: false, PublicKey: "pk-xyz", SentryDsn: "dsn2"})
	if !strings.Contains(md, "**Active**: false") {
		t.Errorf("expected Active=false in markdown:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListKeysMarkdown — empty keys branch
// ---------------------------------------------------------------------------.

// TestFormatListKeysMarkdown_Empty verifies FormatListKeysMarkdown when empty.
func TestFormatListKeysMarkdown_Empty(t *testing.T) {
	md := FormatListKeysMarkdown(ListClientKeysOutput{Keys: []ClientKeyItem{}})
	if !strings.Contains(md, "No client keys found") {
		t.Errorf("expected empty-keys message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListKeysMarkdown_NilKeys verifies FormatListKeysMarkdown when nil keys.
func TestFormatListKeysMarkdown_NilKeys(t *testing.T) {
	md := FormatListKeysMarkdown(ListClientKeysOutput{})
	if !strings.Contains(md, "No client keys found") {
		t.Errorf("expected empty-keys message:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatSettingsMarkdown — minimal fields (no ProjectName, no SentryExternalURL)
// ---------------------------------------------------------------------------.

// TestFormatSettingsMarkdown_MinimalFields verifies FormatSettingsMarkdown when minimal fields.
func TestFormatSettingsMarkdown_MinimalFields(t *testing.T) {
	md := FormatSettingsMarkdown(SettingsOutput{Active: false, Integrated: true})
	if !strings.Contains(md, "**Active**: false") {
		t.Errorf("missing Active:\n%s", md)
	}
	if strings.Contains(md, "**Project Name**") {
		t.Error("should not contain Project Name when empty")
	}
	if strings.Contains(md, "**Sentry URL**") {
		t.Error("should not contain Sentry URL when empty")
	}
}
