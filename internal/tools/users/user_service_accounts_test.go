// user_service_accounts_test.go contains unit tests for GitLab service account
// operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestCreateServiceAccount_Success verifies CreateServiceAccount returns the
// new service account when POST /service_accounts responds 201 Created.
func TestCreateServiceAccount_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/service_accounts" {
			testutil.RespondJSON(w, http.StatusCreated, userJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateServiceAccount(context.Background(), client, CreateServiceAccountInput{
		Name: "svc-bot", Username: "svc-bot", Email: "svc@example.com",
	})
	if err != nil {
		t.Fatalf("CreateServiceAccount() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
}

// TestListServiceAccounts_Success verifies ListServiceAccounts returns the
// account list when GET /service_accounts responds 200 with two entries.
func TestListServiceAccounts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/service_accounts" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"svc-1","name":"Service 1"},{"id":2,"username":"svc-2","name":"Service 2"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListServiceAccounts(context.Background(), client, ListServiceAccountsInput{})
	if err != nil {
		t.Fatalf("ListServiceAccounts() unexpected error: %v", err)
	}
	if len(out.Accounts) != 2 {
		t.Fatalf("len(out.Accounts) = %d, want 2", len(out.Accounts))
	}
}

// TestCreateCurrentUserPAT_Success verifies CreateCurrentUserPAT returns the
// new token (including the plaintext token field) when
// POST /user/personal_access_tokens responds 201 Created.
func TestCreateCurrentUserPAT_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":10,"name":"my-pat","active":true,"token":"glpat-xyz",
				"scopes":["api"],"revoked":false,"user_id":1,
				"created_at":"2026-01-15T10:00:00Z","expires_at":"2026-01-15"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{
		Name: "my-pat", Scopes: []string{"api"}, ExpiresAt: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("CreateCurrentUserPAT() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
	if out.Token != "glpat-xyz" {
		t.Errorf("out.Token = %q, want %q", out.Token, "glpat-xyz")
	}
}

// TestCreateCurrentUserPAT_EmptyName verifies CreateCurrentUserPAT returns a
// validation error when the name field is empty.
func TestCreateCurrentUserPAT_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{Scopes: []string{"api"}})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestCreateCurrentUserPAT_EmptyScopes verifies CreateCurrentUserPAT returns a
// validation error when the scopes slice is empty.
func TestCreateCurrentUserPAT_EmptyScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{Name: "test"})
	if err == nil {
		t.Fatal("expected error for empty scopes, got nil")
	}
}

// TestFormatServiceAccountListMarkdownString_Empty verifies the markdown
// formatter returns a non-empty string for an empty service account list.
func TestFormatServiceAccountListMarkdownString_Empty(t *testing.T) {
	md := FormatServiceAccountListMarkdownString(ServiceAccountListOutput{})
	if md == "" {
		t.Fatal("expected non-empty markdown for empty list")
	}
}

// TestFormatCurrentUserPATMarkdownString verifies FormatCurrentUserPATMarkdownString
// produces non-empty markdown for a PAT output.
func TestFormatCurrentUserPATMarkdownString(t *testing.T) {
	md := FormatCurrentUserPATMarkdownString(CurrentUserPATOutput{
		ID: 1, Name: "test", Scopes: []string{"api"}, UserID: 42,
	})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}
