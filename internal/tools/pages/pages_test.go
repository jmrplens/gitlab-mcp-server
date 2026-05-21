// pages_test.go contains unit tests for the GitLab Pages MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pages

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
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
	// testGroupProject identifies the test group project constant used by this package.
	testGroupProject = "group/project"
	// testMyGroupProject identifies the test my group project constant used by this package.
	testMyGroupProject = "mygroup/myproject"
	// errNoHandler identifies the err no handler constant used by this package.
	errNoHandler = "handler should not be called"
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestCreateDomain_ValidationMissingDomain verifies CreateDomain when validation missing domain.
func TestCreateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdateDomain_ValidationMissingDomain verifies UpdateDomain when validation missing domain.
func TestUpdateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	err := DeleteDomain(context.Background(), client, DeleteDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestDeleteDomain_ValidationMissingDomain verifies DeleteDomain when validation missing domain.
func TestDeleteDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	err := DeleteDomain(context.Background(), client, DeleteDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// TestFormatPagesMarkdown verifies FormatPagesMarkdown.
func TestFormatPagesMarkdown(t *testing.T) {
	md := FormatPagesMarkdown(Output{
		URL:        testPagesURL,
		ForceHTTPS: true,
		Deployments: []DeploymentOutput{
			{URL: testPagesURL, CreatedAt: "2026-01-15T10:00:00Z", PathPrefix: "", RootDirectory: "public"},
		},
	})
	if !strings.Contains(md, testPagesURL) {
		t.Error("expected URL in output")
	}
	if !strings.Contains(md, "Deployments") {
		t.Error("expected Deployments section")
	}
}

// TestFormatPagesMarkdown_NoDeployments verifies FormatPagesMarkdown when no deployments.
func TestFormatPagesMarkdown_NoDeployments(t *testing.T) {
	md := FormatPagesMarkdown(Output{URL: testPagesURL})
	if strings.Contains(md, "Deployments") {
		t.Error("should not contain Deployments section when empty")
	}
}

// TestFormatDomainMarkdown_WithOptionalFields verifies FormatDomainMarkdown when with optional fields.
func TestFormatDomainMarkdown_WithOptionalFields(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:       testDomain,
		URL:          testExampleURL,
		Verified:     true,
		EnabledUntil: "2026-01-01T00:00:00Z",
		Certificate:  CertificateOutput{Subject: testDomain, Expired: false},
	})
	if !strings.Contains(md, "Enabled Until") {
		t.Error("expected EnabledUntil in output")
	}
	if !strings.Contains(md, "Cert Subject") {
		t.Error("expected certificate subject in output")
	}
}

// TestFormatDomainListMarkdown_Empty verifies FormatDomainListMarkdown when empty.
func TestFormatDomainListMarkdown_Empty(t *testing.T) {
	md := FormatDomainListMarkdown(ListDomainsOutput{})
	if !strings.Contains(md, "No Pages domains found") {
		t.Error("expected empty message")
	}
}

// TestFormatAllDomainsMarkdown_Empty verifies FormatAllDomainsMarkdown when empty.
func TestFormatAllDomainsMarkdown_Empty(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{})
	if !strings.Contains(md, "No Pages domains found") {
		t.Error("expected empty message")
	}
}

// TestFormatAllDomainsMarkdown_NonEmpty verifies FormatAllDomainsMarkdown when non empty.
func TestFormatAllDomainsMarkdown_NonEmpty(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{
		Domains: []DomainOutput{{Domain: testDomainA, URL: testDomainAURL, ProjectID: 1}},
	})
	if !strings.Contains(md, testDomainA) {
		t.Error("expected domain in output")
	}
}

// TestFormatDeleteMarkdown verifies FormatDeleteMarkdown.
func TestFormatDeleteMarkdown(t *testing.T) {
	md := FormatDeleteMarkdown(testDomain)
	if !strings.Contains(md, testDomain) {
		t.Error("expected domain in delete message")
	}
}

// TestFormatUnpublishMarkdown verifies FormatUnpublishMarkdown.
func TestFormatUnpublishMarkdown(t *testing.T) {
	md := FormatUnpublishMarkdown()
	if !strings.Contains(md, "unpublished") {
		t.Error("expected unpublished in message")
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters -- project display
// ---------------------------------------------------------------------------.

// TestProjectDisplay covers ProjectDisplay with table-driven subtests.
func TestProjectDisplay(t *testing.T) {
	tests := []struct {
		name string
		path string
		id   int64
		want string
	}{
		{"path preferred", testGroupProject, 42, testGroupProject},
		{"numeric fallback", "", 42, "#42"},
		{"zero id", "", 0, "#0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectDisplay(tt.path, tt.id)
			if got != tt.want {
				t.Errorf("projectDisplay(%q, %d) = %q, want %q", tt.path, tt.id, got, tt.want)
			}
		})
	}
}

// TestSetProjectPathFromInput covers SetProjectPathFromInput with table-driven subtests.
func TestSetProjectPathFromInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPath string
	}{
		{"path input", testGroupProject, testGroupProject},
		{"numeric input", "42", ""},
		{"nested path", "org/sub/project", "org/sub/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := DomainOutput{ProjectID: 42}
			setProjectPathFromInput(&out, toolutil.StringOrInt(tt.input))
			if out.ProjectPath != tt.wantPath {
				t.Errorf("setProjectPathFromInput(%q) -> ProjectPath=%q, want %q", tt.input, out.ProjectPath, tt.wantPath)
			}
		})
	}
}

// TestFormatDomainMarkdown_WithProjectPath verifies FormatDomainMarkdown when with project path.
func TestFormatDomainMarkdown_WithProjectPath(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:      testDomain,
		URL:         testExampleURL,
		ProjectID:   42,
		ProjectPath: testMyGroupProject,
		Verified:    true,
	})
	if !strings.Contains(md, testMyGroupProject) {
		t.Error("expected project path in output")
	}
	if strings.Contains(md, "42") {
		t.Error("should not contain numeric project ID when path is set")
	}
}

// TestFormatDomainMarkdown_NumericFallback verifies FormatDomainMarkdown when numeric fallback.
func TestFormatDomainMarkdown_NumericFallback(t *testing.T) {
	md := FormatDomainMarkdown(DomainOutput{
		Domain:    testDomain,
		URL:       testExampleURL,
		ProjectID: 99,
		Verified:  true,
	})
	if !strings.Contains(md, "#99") {
		t.Error("expected #99 numeric fallback in output")
	}
}

// TestFormatDomainListMarkdown_WithProjectPath verifies FormatDomainListMarkdown when with project path.
func TestFormatDomainListMarkdown_WithProjectPath(t *testing.T) {
	md := FormatDomainListMarkdown(ListDomainsOutput{
		Domains: []DomainOutput{
			{Domain: testDomainA, URL: testDomainAURL, ProjectID: 1, ProjectPath: "team/web"},
			{Domain: "b.com", URL: "https://b.com", ProjectID: 2},
		},
	})
	if !strings.Contains(md, "team/web") {
		t.Error("expected project path for first domain")
	}
	if !strings.Contains(md, "#2") {
		t.Error("expected numeric fallback for second domain")
	}
}

// TestFormatAllDomainsMarkdown_WithProjectPath verifies FormatAllDomainsMarkdown when with project path.
func TestFormatAllDomainsMarkdown_WithProjectPath(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{
		Domains: []DomainOutput{
			{Domain: testDomainA, URL: testDomainAURL, ProjectID: 10, ProjectPath: "org/repo"},
		},
	})
	if !strings.Contains(md, "org/repo") {
		t.Error("expected project path in all-domains output")
	}
	if strings.Contains(md, "#10") {
		t.Error("should not contain numeric ID when path is set")
	}
}

// TestConverters_EdgeCases verifies Pages converter nil and optional date branches.
func TestConverters_EdgeCases(t *testing.T) {
	if out := toPagesOutput(nil); out.URL != "" || len(out.Deployments) != 0 {
		t.Fatalf("toPagesOutput(nil) = %+v, want zero output", out)
	}

	if out := toDomainOutput(nil); out.Domain != "" || out.ProjectID != 0 {
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
	})
	if out.EnabledUntil == "" {
		t.Fatal("expected EnabledUntil to be formatted")
	}
	if out.Certificate.Expiration == "" {
		t.Fatal("expected certificate expiration to be formatted")
	}
}

// TestGetDomain_PropagatesProjectPath verifies GetDomain when propagates project path.
func TestGetDomain_PropagatesProjectPath(t *testing.T) {
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
	if out.ProjectPath != testMyGroupProject {
		t.Errorf("got ProjectPath %q, want %q", out.ProjectPath, testMyGroupProject)
	}
}

// TestGetDomain_NumericInputNoPath verifies GetDomain when numeric input no path.
func TestGetDomain_NumericInputNoPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"domain":"d.com","auto_ssl_enabled":false,"url":"https://d.com","project_id":7,"verified":true,
			"verification_code":"x","certificate":{"subject":"","expired":false}
		}`)
	}))
	out, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "7", Domain: "d.com"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ProjectPath != "" {
		t.Errorf("expected empty ProjectPath for numeric input, got %q", out.ProjectPath)
	}
}
