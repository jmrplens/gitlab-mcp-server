// pages_test.go contains unit tests for the GitLab Pages MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pages

import (
	"context"
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestGetPages_Success verifies GetPages when success.
func TestGetPages_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pages" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"url":"https://myproject.pages.io",
			"is_unique_domain_enabled":true,
			"force_https":true,
			"deployments":[{"created_at":"2026-01-15T10:00:00Z","url":"https://myproject.pages.io","path_prefix":"","root_directory":"public"}],
			"primary_domain":"myproject.pages.io"
		}`)
	}))

	out, err := GetPages(context.Background(), client, GetPagesInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.URL != "https://myproject.pages.io" {
		t.Errorf("got URL %q, want %q", out.URL, "https://myproject.pages.io")
	}
	if !out.IsUniqueDomainEnabled {
		t.Error("expected IsUniqueDomainEnabled=true")
	}
	if !out.ForceHTTPS {
		t.Error("expected ForceHTTPS=true")
	}
	if len(out.Deployments) != 1 {
		t.Fatalf("got %d deployments, want 1", len(out.Deployments))
	}
}

// TestGetPages_ValidationError verifies GetPages when validation error.
func TestGetPages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetPages(context.Background(), client, GetPagesInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestUpdatePages_Success verifies UpdatePages when success.
func TestUpdatePages_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"url":"https://myproject.pages.io",
			"is_unique_domain_enabled":false,
			"force_https":true,
			"primary_domain":"custom.example.com"
		}`)
	}))

	httpsOnly := true
	out, err := UpdatePages(context.Background(), client, UpdatePagesInput{
		ProjectID:      "42",
		PagesHTTPSOnly: &httpsOnly,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.ForceHTTPS {
		t.Error("expected ForceHTTPS=true")
	}
}

// TestUnpublishPages_Success verifies UnpublishPages when success.
func TestUnpublishPages_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := UnpublishPages(context.Background(), client, UnpublishPagesInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUnpublishPages_ValidationError verifies UnpublishPages when validation error.
func TestUnpublishPages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := UnpublishPages(context.Background(), client, UnpublishPagesInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListAllDomains_Success verifies ListAllDomains when success.
func TestListAllDomains_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/pages/domains" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"domain":"example.com","auto_ssl_enabled":true,"url":"https://example.com","project_id":1,"verified":true},
			{"domain":"test.io","auto_ssl_enabled":false,"url":"https://test.io","project_id":2,"verified":false}
		]`)
	}))

	out, err := ListAllDomains(context.Background(), client, ListAllDomainsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(out.Domains))
	}
	if out.Domains[0].Domain != "example.com" {
		t.Errorf("got domain %q, want %q", out.Domains[0].Domain, "example.com")
	}
}

// TestListDomains_Success verifies ListDomains when success.
func TestListDomains_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pages/domains" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"domain":"custom.example.com","auto_ssl_enabled":true,"url":"https://custom.example.com","project_id":42,"verified":true}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListDomains(context.Background(), client, ListDomainsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Domains) != 1 {
		t.Fatalf("got %d domains, want 1", len(out.Domains))
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("got total %d, want 1", out.Pagination.TotalItems)
	}
}

// TestListDomains_KeysetAndOrdering verifies ListDomains forwards keyset
// pagination plus order_by/sort query parameters to the GitLab API.
func TestListDomains_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "tok42" {
			t.Errorf("page_token = %q, want tok42", q.Get("page_token"))
		}
		if q.Get("order_by") != "domain" {
			t.Errorf("order_by = %q, want domain", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("sort = %q, want asc", q.Get("sort"))
		}
		if q.Get("per_page") != "50" {
			t.Errorf("per_page = %q, want 50", q.Get("per_page"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := ListDomains(context.Background(), client, ListDomainsInput{
		ProjectID:  "42",
		OrderBy:    "domain",
		Sort:       "asc",
		PerPage:    50,
		Pagination: "keyset", PageToken: "tok42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestToDomainOutput_FullCertificate verifies toDomainOutput mirrors every
// PagesDomainCertificate field, including certificate_text.
func TestToDomainOutput_FullCertificate(t *testing.T) {
	out := toDomainOutput(&gl.PagesDomain{
		Domain:    testDomain,
		ProjectID: 42,
		Certificate: gl.PagesDomainCertificate{
			Subject:         testDomain,
			Expired:         true,
			Certificate:     "-----BEGIN CERTIFICATE-----",
			CertificateText: "Certificate:\n    Data:",
		},
	}, toolutil.PagesDomainExtra{})
	if out.Certificate.Certificate != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("Certificate = %q, want PEM body", out.Certificate.Certificate)
	}
	if out.Certificate.CertificateText != "Certificate:\n    Data:" {
		t.Errorf("CertificateText = %q, want decoded text", out.Certificate.CertificateText)
	}
	if !out.Certificate.Expired {
		t.Error("expected Expired=true")
	}
}

// TestGetDomain_Success verifies GetDomain when success.
func TestGetDomain_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pages/domains/example.com" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"domain":"example.com","auto_ssl_enabled":true,"url":"https://example.com","project_id":42,"verified":true,
			"verification_code":"abc123","certificate":{"subject":"example.com","expired":false}
		}`)
	}))

	out, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "42", Domain: "example.com"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Domain != "example.com" {
		t.Errorf("got domain %q, want %q", out.Domain, "example.com")
	}
	if out.VerificationCode != "abc123" {
		t.Errorf("got verification code %q, want %q", out.VerificationCode, "abc123")
	}
}

// TestGetDomain_ValidationError verifies GetDomain when validation error.
func TestGetDomain_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected validation error for missing domain")
	}
}

// TestCreateDomain_Success verifies CreateDomain when success.
func TestCreateDomain_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{
			"domain":"new.example.com","auto_ssl_enabled":true,"url":"https://new.example.com","project_id":42,"verified":false
		}`)
	}))

	autoSSL := true
	out, err := CreateDomain(context.Background(), client, CreateDomainInput{
		ProjectID:      "42",
		Domain:         "new.example.com",
		AutoSslEnabled: &autoSSL,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Domain != "new.example.com" {
		t.Errorf("got domain %q, want %q", out.Domain, "new.example.com")
	}
}

// TestUpdateDomain_Success verifies UpdateDomain when success.
func TestUpdateDomain_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"domain":"example.com","auto_ssl_enabled":false,"url":"https://example.com","project_id":42,"verified":true
		}`)
	}))

	autoSSL := false
	out, err := UpdateDomain(context.Background(), client, UpdateDomainInput{
		ProjectID:      "42",
		Domain:         "example.com",
		AutoSslEnabled: &autoSSL,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AutoSslEnabled {
		t.Error("expected AutoSslEnabled=false")
	}
}

// TestDeleteDomain_Success verifies DeleteDomain when success.
func TestDeleteDomain_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := DeleteDomain(context.Background(), client, DeleteDomainInput{
		ProjectID: "42",
		Domain:    "example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteDomain_APIError verifies DeleteDomain when API error.
func TestDeleteDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	err := DeleteDomain(context.Background(), client, DeleteDomainInput{
		ProjectID: "42",
		Domain:    "nonexistent.com",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	// argProjectID identifies the arg project ID constant used by this package.
	argProjectID = "project_id"
	// argDomain identifies the arg domain constant used by this package.
	argDomain = "domain"
	// testDomain identifies the test domain constant used by this package.
	testDomain = "example.com"
	// testPagesURL identifies the test pages URL constant used by this package.
	testPagesURL = "https://p.io"
	// testExampleURL identifies the test example URL constant used by this package.
	testExampleURL = "https://example.com"
	// testDomainA identifies the test domain a constant used by this package.
	testDomainA = "a.com"
	// testMyGroupProject identifies the test my group project constant used by this package.
	testMyGroupProject = "mygroup/myproject"
	// errExpectedAPI identifies the err expected API constant used by this package.
	errExpectedAPI = "expected API error, got nil"
	// errEmptyProjID identifies the err empty proj ID constant used by this package.
	errEmptyProjID = "expected validation error for empty project_id"
	// errEmptyDomain identifies the err empty domain constant used by this package.
	errEmptyDomain = "expected validation error for empty domain"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// testDomainAURL identifies the test domain aurl constant used by this package.
	testDomainAURL = "https://a.com"
)

// ---------------------------------------------------------------------------
// UpdatePages -- validation error, API error
// ---------------------------------------------------------------------------.

// TestUpdatePages_ValidationError verifies UpdatePages when validation error.
func TestUpdatePages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := UpdatePages(context.Background(), client, UpdatePagesInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdatePages_APIError verifies UpdatePages when API error.
func TestUpdatePages_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdatePages(context.Background(), client, UpdatePagesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdatePages_AllOptionalFields verifies UpdatePages when all optional fields.
func TestUpdatePages_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			testutil.RespondJSON(w, http.StatusOK, `{"url":"https://p.io","is_unique_domain_enabled":true,"force_https":false,"primary_domain":"custom.io"}`)
			return
		}
		http.NotFound(w, r)
	}))
	uniqueDomain := true
	httpsOnly := false
	out, err := UpdatePages(context.Background(), client, UpdatePagesInput{
		ProjectID:                "42",
		PagesUniqueDomainEnabled: &uniqueDomain,
		PagesHTTPSOnly:           &httpsOnly,
		PagesPrimaryDomain:       "custom.io",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.IsUniqueDomainEnabled {
		t.Error("expected IsUniqueDomainEnabled=true")
	}
}

// ---------------------------------------------------------------------------
// GetPages -- API error
// ---------------------------------------------------------------------------.

// TestGetPages_APIError verifies GetPages when API error.
func TestGetPages_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetPages(context.Background(), client, GetPagesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// UnpublishPages -- API error
// ---------------------------------------------------------------------------.

// TestUnpublishPages_APIError verifies UnpublishPages when API error.
func TestUnpublishPages_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := UnpublishPages(context.Background(), client, UnpublishPagesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListAllDomains -- API error
// ---------------------------------------------------------------------------.

// TestListAllDomains_APIError verifies ListAllDomains when API error.
func TestListAllDomains_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListAllDomains(context.Background(), client, ListAllDomainsInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListDomains -- validation error, API error
// ---------------------------------------------------------------------------.

// TestListDomains_ValidationError verifies ListDomains when validation error.
func TestListDomains_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListDomains(context.Background(), client, ListDomainsInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestListDomains_APIError verifies ListDomains when API error.
func TestListDomains_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListDomains(context.Background(), client, ListDomainsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetDomain -- validation (missing project_id)
// ---------------------------------------------------------------------------.

// TestGetDomain_ValidationMissingProjectID verifies GetDomain when validation missing project ID.
func TestGetDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetDomain(context.Background(), client, GetDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetDomain_APIError verifies GetDomain when API error.
func TestGetDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "42", Domain: testDomain})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// CreateDomain -- validation errors, API error, with optional fields
// ---------------------------------------------------------------------------.

// TestCreateDomain_ValidationMissingProjectID verifies CreateDomain when validation missing project ID.
func TestCreateDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestCreateDomain_ValidationMissingDomain verifies CreateDomain when validation missing domain.
func TestCreateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// TestCreateDomain_APIError verifies CreateDomain when API error.
func TestCreateDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{ProjectID: "42", Domain: "bad.com"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateDomain_WithCert verifies CreateDomain when with cert.
func TestCreateDomain_WithCert(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"domain":"cert.example.com","auto_ssl_enabled":false,"url":"https://cert.example.com","project_id":42,"verified":false,"certificate":{"subject":"cert.example.com","expired":false}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateDomain(context.Background(), client, CreateDomainInput{
		ProjectID:   "42",
		Domain:      "cert.example.com",
		Certificate: "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
		Key:         "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Domain != "cert.example.com" {
		t.Errorf("expected cert.example.com, got %s", out.Domain)
	}
}

// ---------------------------------------------------------------------------
// UpdateDomain -- validation errors, API error, with optional fields
// ---------------------------------------------------------------------------.

// TestUpdateDomain_ValidationMissingProjectID verifies UpdateDomain when validation missing project ID.
func TestUpdateDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdateDomain_ValidationMissingDomain verifies UpdateDomain when validation missing domain.
func TestUpdateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// TestUpdateDomain_APIError verifies UpdateDomain when API error.
func TestUpdateDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{ProjectID: "42", Domain: testDomain})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateDomain_WithCert verifies UpdateDomain when with cert.
func TestUpdateDomain_WithCert(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"domain":"example.com","auto_ssl_enabled":false,"url":"https://example.com","project_id":42,"verified":true,"certificate":{"subject":"example.com","expired":false}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := UpdateDomain(context.Background(), client, UpdateDomainInput{
		ProjectID:   "42",
		Domain:      testDomain,
		Certificate: "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
		Key:         "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Domain != testDomain {
		t.Errorf("expected example.com, got %s", out.Domain)
	}
}

// ---------------------------------------------------------------------------
// DeleteDomain -- validation errors
// ---------------------------------------------------------------------------.

// TestDeleteDomain_ValidationMissingProjectID verifies DeleteDomain when validation missing project ID.
func TestDeleteDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteDomain(context.Background(), client, DeleteDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestDeleteDomain_ValidationMissingDomain verifies DeleteDomain when validation missing domain.
func TestDeleteDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteDomain(context.Background(), client, DeleteDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// assertPagesMarkdown compares a whole rendered response with what the
// formatter is meant to write, byte for byte.
func assertPagesMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// The guidance sections the Pages formatters close with.
const (
	pagesSettingsHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.pages_domain_list' to see the project's custom Pages domains\n"
	pagesDomainHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.pages_domain_update' to change this domain's auto-SSL flag or certificate\n" +
		"- Use action 'project.pages_get' to read the project's Pages settings\n"
	pagesDomainTableHead = "| Domain | URL | Verified | Auto SSL | Project ID |\n" +
		"| --- | --- | --- | --- | --- |\n"
)

// TestFormatPagesMarkdown verifies the whole card a project's Pages settings
// render, the deployments as a nested collection under a heading of their own.
func TestFormatPagesMarkdown(t *testing.T) {
	md := FormatPagesMarkdown(Output{
		URL:        testPagesURL,
		ForceHTTPS: true,
		Deployments: []DeploymentOutput{
			{URL: testPagesURL, CreatedAt: "2026-01-15T10:00:00Z", PathPrefix: "", RootDirectory: "public"},
		},
	})

	assertPagesMarkdown(t, md, "## Pages Settings\n\n"+
		"- **URL**: ["+testPagesURL+"]("+testPagesURL+")\n"+
		"- **Unique Domain**: ❌\n"+
		"- **Force HTTPS**: ✅\n"+
		"\n### Deployments\n\n"+
		"| URL | Created | Path Prefix | Root Dir |\n"+
		"| --- | --- | --- | --- |\n"+
		"| ["+testPagesURL+"]("+testPagesURL+") | 15 Jan 2026 10:00 UTC |  | public |\n"+
		pagesSettingsHints)
}

// TestFormatPagesMarkdown_NoDeployments verifies that a project with no
// deployments opens no collection heading.
func TestFormatPagesMarkdown_NoDeployments(t *testing.T) {
	assertPagesMarkdown(t, FormatPagesMarkdown(Output{URL: testPagesURL}), "## Pages Settings\n\n"+
		"- **URL**: ["+testPagesURL+"]("+testPagesURL+")\n"+
		"- **Unique Domain**: ❌\n"+
		"- **Force HTTPS**: ❌\n"+
		pagesSettingsHints)
}

// TestFormatDomainMarkdown_WithOptionalFields verifies the whole card a
// verified domain with a certificate renders: the certificate is a nested
// object, and a verified domain shows no verification code.
func TestFormatDomainMarkdown_WithOptionalFields(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:           testDomain,
		URL:              testExampleURL,
		Verified:         true,
		VerificationCode: "abc123",
		EnabledUntil:     "2026-01-01T00:00:00Z",
		Certificate:      CertificateOutput{Subject: testDomain, Expired: false},
	})

	assertPagesMarkdown(t, md, "## Pages Domain: "+testDomain+"\n\n"+
		"- **URL**: ["+testExampleURL+"]("+testExampleURL+")\n"+
		"- **Verified**: ✅\n"+
		"- **Auto SSL**: ❌\n"+
		"- **Enabled Until**: 1 Jan 2026 00:00 UTC\n"+
		"- **Certificate**:\n"+
		"  - **Subject**: "+testDomain+"\n"+
		pagesDomainHints)
}

// TestFormatDomainMarkdown_Unverified verifies that the verification code is
// shown while the domain is unverified, which is the one moment a reader needs
// it, and that an expired certificate is marked with a warning rather than
// with the tick a true flag would print.
func TestFormatDomainMarkdown_Unverified(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:           testDomain,
		URL:              testExampleURL,
		Verified:         false,
		VerificationCode: "abc123",
		Certificate:      CertificateOutput{Subject: testDomain, Expired: true},
	})

	assertPagesMarkdown(t, md, "## Pages Domain: "+testDomain+"\n\n"+
		"- **URL**: ["+testExampleURL+"]("+testExampleURL+")\n"+
		"- **Verified**: ❌\n"+
		"- **Verification Code**: `abc123`\n"+
		"- **Auto SSL**: ❌\n"+
		"- **Certificate**:\n"+
		"  - **Subject**: "+testDomain+"\n"+
		"  - ⚠️ **Expired**\n"+
		pagesDomainHints)
}

// TestFormatDomainListMarkdown_Empty verifies that an empty page renders the
// one sentence and nothing else.
func TestFormatDomainListMarkdown_Empty(t *testing.T) {
	assertPagesMarkdown(t, FormatDomainListMarkdown(ListDomainsOutput{}), "No Pages domains found.\n")
}

// TestFormatAllDomainsMarkdown_Empty verifies that an empty instance-wide page
// renders the one sentence and nothing else.
func TestFormatAllDomainsMarkdown_Empty(t *testing.T) {
	assertPagesMarkdown(t, FormatAllDomainsMarkdown(ListAllDomainsOutput{}), "No Pages domains found.\n")
}

// TestFormatAllDomainsMarkdown_NonEmpty verifies the whole table the
// instance-wide listing renders.
func TestFormatAllDomainsMarkdown_NonEmpty(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{
		Domains: []DomainOutput{{Domain: testDomainA, URL: testDomainAURL, ProjectID: 1}},
	})

	assertPagesMarkdown(t, md, "## All Pages Domains (1)\n\n"+
		pagesDomainTableHead+
		"| "+testDomainA+" | ["+testDomainAURL+"]("+testDomainAURL+") | ❌ | ❌ | 1 |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- "+toolutil.HintPreserveLinks+"\n"+
		"- Use action 'project.pages_domain_get' to read one domain in full\n"+
		"- Use action 'project.pages_domain_list_all' to list every Pages domain on the instance again\n")
}

// ---------------------------------------------------------------------------
// Markdown formatters -- project display
// ---------------------------------------------------------------------------.

// TestProjectDisplay covers projectDisplay with table-driven subtests. The
// zero case is every project-scoped call: GitLab answers those with the domain
// alone and no project, and "#0" named a project that does not exist.
func TestProjectDisplay(t *testing.T) {
	tests := []struct {
		name string
		id   int64
		want string
	}{
		{"numeric id", 42, "42"},
		{"zero id renders nothing", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectDisplay(tt.id)
			if got != tt.want {
				t.Errorf("projectDisplay(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestFormatDomainMarkdown_NumericProject verifies the whole card an
// instance-wide domain renders, its project named by its ID.
func TestFormatDomainMarkdown_NumericProject(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:    testDomain,
		URL:       testExampleURL,
		ProjectID: 99,
		Verified:  true,
	})

	assertPagesMarkdown(t, md, "## Pages Domain: "+testDomain+"\n\n"+
		"- **URL**: ["+testExampleURL+"]("+testExampleURL+")\n"+
		"- **Project ID**: 99\n"+
		"- **Verified**: ✅\n"+
		"- **Auto SSL**: ❌\n"+
		pagesDomainHints)
}

// TestFormatDomainMarkdown_ProjectScoped verifies that a domain GitLab
// answered a project-scoped call with, which carries no project object, writes
// no project row at all.
func TestFormatDomainMarkdown_ProjectScoped(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:   testDomain,
		URL:      testExampleURL,
		Verified: true,
	})

	assertPagesMarkdown(t, md, "## Pages Domain: "+testDomain+"\n\n"+
		"- **URL**: ["+testExampleURL+"]("+testExampleURL+")\n"+
		"- **Verified**: ✅\n"+
		"- **Auto SSL**: ❌\n"+
		pagesDomainHints)
}

// TestFormatDomainListMarkdown_NumericProject verifies the whole table a
// project's domain listing renders, each project named by its ID.
func TestFormatDomainListMarkdown_NumericProject(t *testing.T) {
	md := FormatDomainListMarkdown(ListDomainsOutput{
		Domains: []DomainOutput{
			{Domain: testDomainA, URL: testDomainAURL, ProjectID: 1},
			{Domain: "b.com", URL: "https://b.com", ProjectID: 2},
		},
	})

	assertPagesMarkdown(t, md, "## Pages Domains (2)\n\n"+
		pagesDomainTableHead+
		"| "+testDomainA+" | ["+testDomainAURL+"]("+testDomainAURL+") | ❌ | ❌ | 1 |\n"+
		"| b.com | [https://b.com](https://b.com) | ❌ | ❌ | 2 |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- "+toolutil.HintPreserveLinks+"\n"+
		"- Use action 'project.pages_domain_get' to read one domain in full\n")
}

// TestConverters_EdgeCases verifies Pages converter nil and optional date branches.
func TestConverters_EdgeCases(t *testing.T) {
	if out := toPagesOutput(nil); out.URL != "" || len(out.Deployments) != 0 {
		t.Fatalf("toPagesOutput(nil) = %+v, want zero output", out)
	}

	if out := toDomainOutput(nil, toolutil.PagesDomainExtra{}); out.Domain != "" || out.ProjectID != 0 {
		t.Fatalf("toDomainOutput(nil) = %+v, want zero output", out)
	}

	enabledUntil := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expiration := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	out := toDomainOutput(&gl.PagesDomain{
		Domain:       testDomain,
		URL:          testExampleURL,
		ProjectID:    42,
		EnabledUntil: &enabledUntil,
		Certificate: gl.PagesDomainCertificate{
			Subject:    testDomain,
			Expiration: &expiration,
		},
	}, toolutil.PagesDomainExtra{
		CertificateExpiration: &toolutil.PagesCertificateExpirationOutput{Expired: true, Expiration: &expiration},
	})
	if out.EnabledUntil == "" {
		t.Fatal("expected EnabledUntil to be formatted")
	}
	if out.Certificate.Expiration == "" {
		t.Fatal("expected certificate expiration to be formatted")
	}
	if out.CertificateExpiration == nil || !out.CertificateExpiration.Expired {
		t.Fatal("expected the certificate_expiration object read beside the decode")
	}
}

// TestPagesDomains_UnreadableCapturedCertificateExpiration verifies that every
// Pages domain handler returns an error rather than a half-filled domain when
// GitLab sends certificate_expiration as something that is not an object. The
// SDK ignores the key its own PagesDomain does not model, so the read of the
// captured response is the only thing that can notice, and a certificate whose
// expiry silently disappears is the one fact this field is read for.
func TestPagesDomains_UnreadableCapturedCertificateExpiration(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list_all", Call: func() error {
			client := poisoned(`[{"domain":"example.com","certificate_expiration":"soon"}]`)
			_, err := ListAllDomains(context.Background(), client, ListAllDomainsInput{})
			return err
		}},
		{Name: "list", Call: func() error {
			client := poisoned(`[{"domain":"example.com","certificate_expiration":"soon"}]`)
			_, err := ListDomains(context.Background(), client, ListDomainsInput{ProjectID: "42"})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"domain":"example.com","certificate_expiration":"soon"}`)
			_, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "42", Domain: "example.com"})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"domain":"example.com","certificate_expiration":"soon"}`)
			_, err := CreateDomain(context.Background(), client, CreateDomainInput{ProjectID: "42", Domain: "example.com"})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"domain":"example.com","certificate_expiration":"soon"}`)
			_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{ProjectID: "42", Domain: "example.com"})
			return err
		}},
	})
}

// TestGetDomain_ReturnsProjectID verifies GetDomain surfaces the numeric project ID from the API.
func TestGetDomain_ReturnsProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"domain":"d.com","auto_ssl_enabled":false,"url":"https://d.com","project_id":7,"verified":true,
			"verification_code":"x","certificate":{"subject":"","expired":false}
		}`)
	}))
	out, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: testMyGroupProject, Domain: "d.com"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ProjectID != 7 {
		t.Errorf("got ProjectID %d, want 7", out.ProjectID)
	}
}
