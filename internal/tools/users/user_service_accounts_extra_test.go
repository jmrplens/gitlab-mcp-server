// user_service_accounts_extra_test.go covers service account and PAT functions
// with API errors, cancelled contexts, all optional ListServiceAccounts fields,
// the full FormatServiceAccountListMarkdownString path, and CreateCurrentUserPAT
// with invalid date format.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestCreateServiceAccount_APIError verifies error handling on API failure.
func TestCreateServiceAccount_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := CreateServiceAccount(context.Background(), client, CreateServiceAccountInput{
		Name: "svc", Username: "svc", Email: "svc@example.com",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestListServiceAccounts_AllOptions verifies ListServiceAccounts with all optional
// parameters set (OrderBy, Sort, Page, PerPage).
func TestListServiceAccounts_AllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/service_accounts" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"svc-1","name":"Service 1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListServiceAccounts(context.Background(), client, ListServiceAccountsInput{
		OrderBy: "id",
		Sort:    "desc",
		Page:    1,
		PerPage: 20,
	})
	if err != nil {
		t.Fatalf("ListServiceAccounts() unexpected error: %v", err)
	}
	if len(out.Accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(out.Accounts))
	}
}

// TestListServiceAccounts_APIError verifies error handling on API failure.
func TestListServiceAccounts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ListServiceAccounts(context.Background(), client, ListServiceAccountsInput{})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestCreateCurrentUserPAT_InvalidDateFormat verifies that an invalid expires_at
// returns a parsing error.
func TestCreateCurrentUserPAT_InvalidDateFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{
		Name:      "test",
		Scopes:    []string{"api"},
		ExpiresAt: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_at format, got nil")
	}
}

// TestCreateCurrentUserPAT_APIError verifies error handling on API failure.
func TestCreateCurrentUserPAT_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{
		Name: "test", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestCreateCurrentUserPAT_WithDescription verifies PAT creation with description field.
func TestCreateCurrentUserPAT_WithDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":11,"name":"my-pat","active":true,"token":"glpat-desc",
				"scopes":["api"],"revoked":false,"user_id":1,
				"description":"Automation token",
				"created_at":"2026-01-15T10:00:00Z",
				"last_used_at":"2026-06-01T12:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{
		Name:        "my-pat",
		Scopes:      []string{"api"},
		Description: "Automation token",
	})
	if err != nil {
		t.Fatalf("CreateCurrentUserPAT() unexpected error: %v", err)
	}
	if out.Description != "Automation token" {
		t.Errorf("Description = %q, want %q", out.Description, "Automation token")
	}
	if out.LastUsedAt == "" {
		t.Error("expected non-empty LastUsedAt")
	}
}

// TestFormatServiceAccountListMarkdownString_WithData verifies full table rendering.
func TestFormatServiceAccountListMarkdownString_WithData(t *testing.T) {
	out := ServiceAccountListOutput{
		Accounts: []ServiceAccountOutput{
			{ID: 1, Username: "svc-1", Name: "Service 1"},
			{ID: 2, Username: "svc-2", Name: "Service 2"},
		},
	}
	md := FormatServiceAccountListMarkdownString(out)

	for _, want := range []string{
		"## Service Accounts (2)",
		"| ID | Username | Name |",
		"| 1 | svc-1 | Service 1 |",
		"| 2 | svc-2 | Service 2 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatCurrentUserPATMarkdownString_WithAllFields verifies full PAT markdown
// including token and expires_at.
func TestFormatCurrentUserPATMarkdownString_WithAllFields(t *testing.T) {
	md := FormatCurrentUserPATMarkdownString(CurrentUserPATOutput{
		ID:          10,
		Name:        "my-pat",
		Active:      true,
		Token:       "glpat-secret",
		Scopes:      []string{"api", "read_user"},
		Description: "Test token",
		ExpiresAt:   "2026-01-15",
		UserID:      1,
	})

	for _, want := range []string{
		"## Personal Access Token",
		"**Name**: my-pat",
		"**Active**: true",
		"**Scopes**: api, read_user",
		"**Description**: Test token",
		"**Expires At**: 2026-01-15",
		"`glpat-secret`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}
