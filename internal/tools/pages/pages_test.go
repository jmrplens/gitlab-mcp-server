// pages_test.go contains unit tests for the GitLab Pages MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package pages

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetPages_Success verifies that GetPages handles the success scenario correctly.
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

// TestGetPages_ValidationError verifies that GetPages handles the validation error scenario correctly.
func TestGetPages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetPages(context.Background(), client, GetPagesInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestUpdatePages_Success verifies that UpdatePages handles the success scenario correctly.
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

// TestUnpublishPages_Success verifies that UnpublishPages handles the success scenario correctly.
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

// TestUnpublishPages_ValidationError verifies that UnpublishPages handles the validation error scenario correctly.
func TestUnpublishPages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	err := UnpublishPages(context.Background(), client, UnpublishPagesInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListAllDomains_Success verifies that ListAllDomains handles the success scenario correctly.
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

// TestListDomains_Success verifies that ListDomains handles the success scenario correctly.
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

// TestGetDomain_Success verifies that GetDomain handles the success scenario correctly.
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

// TestGetDomain_ValidationError verifies that GetDomain handles the validation error scenario correctly.
func TestGetDomain_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetDomain(context.Background(), client, GetDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected validation error for missing domain")
	}
}

// TestCreateDomain_Success verifies that CreateDomain handles the success scenario correctly.
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

// TestUpdateDomain_Success verifies that UpdateDomain handles the success scenario correctly.
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

// TestDeleteDomain_Success verifies that DeleteDomain handles the success scenario correctly.
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

// TestDeleteDomain_APIError verifies that DeleteDomain handles the a p i error scenario correctly.
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
	argProjectID       = "project_id"
	argDomain          = "domain"
	msgBadRequest      = "bad request"
	testDomain         = "example.com"
	testPagesURL       = "https://p.io"
	testExampleURL     = "https://example.com"
	testDomainA        = "a.com"
	testGroupProject   = "group/project"
	testMyGroupProject = "mygroup/myproject"
	errNoHandler       = "handler should not be called"
	errExpectedAPI     = "expected API error, got nil"
	errEmptyProjID     = "expected validation error for empty project_id"
	errEmptyDomain     = "expected validation error for empty domain"
	fmtUnexpErr        = "unexpected error: %v"
	testDomainAURL     = "https://a.com"
)

// ---------------------------------------------------------------------------
// UpdatePages -- validation error, API error
// ---------------------------------------------------------------------------.

// TestUpdatePages_ValidationError verifies the behavior of update pages validation error.
func TestUpdatePages_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := UpdatePages(context.Background(), client, UpdatePagesInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdatePages_APIError verifies the behavior of update pages a p i error.
func TestUpdatePages_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdatePages(context.Background(), client, UpdatePagesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdatePages_AllOptionalFields verifies the behavior of update pages all optional fields.
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

// TestGetPages_APIError verifies the behavior of get pages a p i error.
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

// TestUnpublishPages_APIError verifies the behavior of unpublish pages a p i error.
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

// TestListAllDomains_APIError verifies the behavior of list all domains a p i error.
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

// TestListDomains_ValidationError verifies the behavior of list domains validation error.
func TestListDomains_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := ListDomains(context.Background(), client, ListDomainsInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestListDomains_APIError verifies the behavior of list domains a p i error.
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

// TestGetDomain_ValidationMissingProjectID verifies the behavior of get domain validation missing project i d.
func TestGetDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := GetDomain(context.Background(), client, GetDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetDomain_APIError verifies the behavior of get domain a p i error.
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

// TestCreateDomain_ValidationMissingProjectID verifies the behavior of create domain validation missing project i d.
func TestCreateDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestCreateDomain_ValidationMissingDomain verifies the behavior of create domain validation missing domain.
func TestCreateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// TestCreateDomain_APIError verifies the behavior of create domain a p i error.
func TestCreateDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateDomain(context.Background(), client, CreateDomainInput{ProjectID: "42", Domain: "bad.com"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateDomain_WithCert verifies the behavior of create domain with cert.
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

// TestUpdateDomain_ValidationMissingProjectID verifies the behavior of update domain validation missing project i d.
func TestUpdateDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdateDomain_ValidationMissingDomain verifies the behavior of update domain validation missing domain.
func TestUpdateDomain_ValidationMissingDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyDomain)
	}
}

// TestUpdateDomain_APIError verifies the behavior of update domain a p i error.
func TestUpdateDomain_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateDomain(context.Background(), client, UpdateDomainInput{ProjectID: "42", Domain: testDomain})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateDomain_WithCert verifies the behavior of update domain with cert.
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

// TestDeleteDomain_ValidationMissingProjectID verifies the behavior of delete domain validation missing project i d.
func TestDeleteDomain_ValidationMissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoHandler)
	}))
	err := DeleteDomain(context.Background(), client, DeleteDomainInput{Domain: testDomain})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestDeleteDomain_ValidationMissingDomain verifies the behavior of delete domain validation missing domain.
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

// TestFormatPagesMarkdown verifies the behavior of format pages markdown.
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

// TestFormatPagesMarkdown_NoDeployments verifies the behavior of format pages markdown no deployments.
func TestFormatPagesMarkdown_NoDeployments(t *testing.T) {
	md := FormatPagesMarkdown(Output{URL: testPagesURL})
	if strings.Contains(md, "Deployments") {
		t.Error("should not contain Deployments section when empty")
	}
}

// TestFormatDomainMarkdown_WithOptionalFields verifies the behavior of format domain markdown with optional fields.
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

// TestFormatDomainListMarkdown_Empty verifies the behavior of format domain list markdown empty.
func TestFormatDomainListMarkdown_Empty(t *testing.T) {
	md := FormatDomainListMarkdown(ListDomainsOutput{})
	if !strings.Contains(md, "No Pages domains found") {
		t.Error("expected empty message")
	}
}

// TestFormatAllDomainsMarkdown_Empty verifies the behavior of format all domains markdown empty.
func TestFormatAllDomainsMarkdown_Empty(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{})
	if !strings.Contains(md, "No Pages domains found") {
		t.Error("expected empty message")
	}
}

// TestFormatAllDomainsMarkdown_NonEmpty verifies the behavior of format all domains markdown non empty.
func TestFormatAllDomainsMarkdown_NonEmpty(t *testing.T) {
	md := FormatAllDomainsMarkdown(ListAllDomainsOutput{
		Domains: []DomainOutput{{Domain: testDomainA, URL: testDomainAURL, ProjectID: 1}},
	})
	if !strings.Contains(md, testDomainA) {
		t.Error("expected domain in output")
	}
}

// TestFormatDeleteMarkdown verifies the behavior of format delete markdown.
func TestFormatDeleteMarkdown(t *testing.T) {
	md := FormatDeleteMarkdown(testDomain)
	if !strings.Contains(md, testDomain) {
		t.Error("expected domain in delete message")
	}
}

// TestFormatUnpublishMarkdown verifies the behavior of format unpublish markdown.
func TestFormatUnpublishMarkdown(t *testing.T) {
	md := FormatUnpublishMarkdown()
	if !strings.Contains(md, "unpublished") {
		t.Error("expected unpublished in message")
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters -- project display
// ---------------------------------------------------------------------------.

// TestProjectDisplay validates project display across multiple scenarios using table-driven subtests.
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

// TestSetProjectPathFromInput validates set project path from input across multiple scenarios using table-driven subtests.
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

// TestFormatDomainMarkdown_WithProjectPath verifies the behavior of format domain markdown with project path.
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

// TestFormatDomainMarkdown_NumericFallback verifies the behavior of format domain markdown numeric fallback.
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

// TestFormatDomainListMarkdown_WithProjectPath verifies the behavior of format domain list markdown with project path.
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

// TestFormatAllDomainsMarkdown_WithProjectPath verifies the behavior of format all domains markdown with project path.
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

// TestGetDomain_PropagatesProjectPath verifies the behavior of get domain propagates project path.
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

// TestGetDomain_NumericInputNoPath verifies the behavior of get domain numeric input no path.
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

// ---------------------------------------------------------------------------
// RegisterTools -- no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta -- no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newPagesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"pages_get", "gitlab_pages_get", map[string]any{argProjectID: "42"}},
		{"pages_update", "gitlab_pages_update", map[string]any{argProjectID: "42"}},
		{"pages_unpublish", "gitlab_pages_unpublish", map[string]any{argProjectID: "42"}},
		{"domain_list_all", "gitlab_pages_domain_list_all", map[string]any{}},
		{"domain_list", "gitlab_pages_domain_list", map[string]any{argProjectID: "42"}},
		{"domain_get", "gitlab_pages_domain_get", map[string]any{argProjectID: "42", argDomain: testDomain}},
		{"domain_create", "gitlab_pages_domain_create", map[string]any{argProjectID: "42", argDomain: "new.com"}},
		{"domain_update", "gitlab_pages_domain_update", map[string]any{argProjectID: "42", argDomain: testDomain}},
		{"domain_delete", "gitlab_pages_domain_delete", map[string]any{argProjectID: "42", argDomain: testDomain}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallSuccess(t, session, ctx, tt.tool, tt.args)
		})
	}
}

// assertToolCallSuccess calls an MCP tool and fails the test if it returns an error.
func assertToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, tool string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", tool, err)
	}
	if result.IsError {
		t.Fatalf("CallTool(%s) returned error: %s", tool, extractErrorText(result))
	}
}

// extractErrorText returns the first text content from an MCP error result.
func extractErrorText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "IsError=true (no text content)"
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newPagesMCPSession is an internal helper for the pages package.
func newPagesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	pagesJSON := `{"url":"https://p.io","is_unique_domain_enabled":true,"force_https":true,"primary_domain":"p.io"}`
	domainJSON := `{"domain":"example.com","auto_ssl_enabled":true,"url":"https://example.com","project_id":42,"verified":true,"verification_code":"abc","certificate":{"subject":"example.com","expired":false}}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesJSON)
	})

	handler.HandleFunc("PATCH /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler.HandleFunc("GET /api/v4/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+domainJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/42/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+domainJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, domainJSON)
	})

	handler.HandleFunc("POST /api/v4/projects/42/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, domainJSON)
	})

	handler.HandleFunc("PUT /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, domainJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
