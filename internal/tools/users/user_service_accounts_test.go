// user_service_accounts_test.go contains unit tests for GitLab service account
// operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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

// --- UpdateInstanceServiceAccount tests ---.

// TestUpdateInstanceServiceAccount_Success verifies UpdateInstanceServiceAccount
// returns the updated service account when PATCH /service_accounts/:id responds 200 OK.
func TestUpdateInstanceServiceAccount_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v4/service_accounts/5" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":5,
				"username":"svc-updated",
				"name":"Updated Service",
				"email":"updated@example.com",
				"unconfirmed_email":"new@example.com"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateInstanceServiceAccount(context.Background(), client, UpdateServiceAccountInput{
		ServiceAccountID: 5,
		Name:             "Updated Service",
		Username:         "svc-updated",
		Email:            "updated@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateInstanceServiceAccount() unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("out.ID = %d, want 5", out.ID)
	}
	if out.Username != "svc-updated" {
		t.Errorf("out.Username = %q, want svc-updated", out.Username)
	}
	if out.Name != "Updated Service" {
		t.Errorf("out.Name = %q, want 'Updated Service'", out.Name)
	}
	if out.Email != "updated@example.com" {
		t.Errorf("out.Email = %q, want updated@example.com", out.Email)
	}
	if out.UnconfirmedEmail != "new@example.com" {
		t.Errorf("out.UnconfirmedEmail = %q, want new@example.com", out.UnconfirmedEmail)
	}
}

// TestUpdateInstanceServiceAccount_MissingID verifies UpdateInstanceServiceAccount
// returns a validation error when ServiceAccountID is 0.
func TestUpdateInstanceServiceAccount_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("API should not be called when service_account_id is zero")
	}))

	_, err := UpdateInstanceServiceAccount(context.Background(), client, UpdateServiceAccountInput{
		ServiceAccountID: 0,
		Name:             "test",
	})
	if err == nil {
		t.Fatal("expected error for missing service_account_id, got nil")
	}
	if !strings.Contains(err.Error(), "service_account_id") {
		t.Errorf("expected error to mention service_account_id, got: %v", err)
	}
}

// TestUpdateInstanceServiceAccount_Forbidden verifies UpdateInstanceServiceAccount
// returns an error with admin hint when the API responds 403 Forbidden.
func TestUpdateInstanceServiceAccount_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := UpdateInstanceServiceAccount(context.Background(), client, UpdateServiceAccountInput{
		ServiceAccountID: 5,
		Name:             "test",
	})
	if err == nil {
		t.Fatal("expected error for 403 Forbidden, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected error to mention admin token requirement, got: %v", err)
	}
}

// TestUpdateInstanceServiceAccount_NilResponse verifies UpdateInstanceServiceAccount
// returns an error when the GitLab API returns a nil service account body.
func TestUpdateInstanceServiceAccount_NilResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `null`)
	}))

	_, err := UpdateInstanceServiceAccount(context.Background(), client, UpdateServiceAccountInput{
		ServiceAccountID: 5,
		Name:             "test",
	})
	if err == nil {
		t.Fatal("expected error for nil API response, got nil")
	}
	if !strings.Contains(err.Error(), "nil account") {
		t.Errorf("expected error to mention nil account, got: %v", err)
	}
}

// TestUpdateInstanceServiceAccount_GenericError verifies UpdateInstanceServiceAccount
// returns an error wrapped with the generic message (not the admin hint) when
// the API responds with a non-403 error such as 500 Internal Server Error.
// This covers the fallthrough error branch in the handler.
func TestUpdateInstanceServiceAccount_GenericError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	}))

	_, err := UpdateInstanceServiceAccount(context.Background(), client, UpdateServiceAccountInput{
		ServiceAccountID: 5,
		Name:             "test",
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "update_instance_service_account") {
		t.Errorf("expected error to contain operation name, got: %v", err)
	}
	// Should NOT include the admin-token hint (that's reserved for 403)
	if strings.Contains(err.Error(), "admin token") {
		t.Errorf("expected generic error message for 500, got admin hint: %v", err)
	}
}

// TestFormatServiceAccountMarkdownString_WithEmail verifies FormatServiceAccountMarkdownString
// includes Email and UnconfirmedEmail fields when both are set.
func TestFormatServiceAccountMarkdownString_WithEmail(t *testing.T) {
	out := ServiceAccountOutput{
		ID:               7,
		Username:         "svc-7",
		Name:             "Service Seven",
		Email:            "svc7@example.com",
		UnconfirmedEmail: "pending@example.com",
	}
	md := FormatServiceAccountMarkdownString(out)

	for _, want := range []string{
		"## Service Account",
		"**Username**: svc-7",
		"**Name**: Service Seven",
		"**Email**: svc7@example.com",
		"**Unconfirmed Email**: pending@example.com",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatServiceAccountMarkdownString_NoEmail verifies FormatServiceAccountMarkdownString
// omits the Email and UnconfirmedEmail lines when those fields are empty.
func TestFormatServiceAccountMarkdownString_NoEmail(t *testing.T) {
	out := ServiceAccountOutput{
		ID:       8,
		Username: "svc-8",
		Name:     "Service Eight",
	}
	md := FormatServiceAccountMarkdownString(out)

	if !strings.Contains(md, "**Username**: svc-8") {
		t.Errorf("markdown missing Username:\n%s", md)
	}
	if !strings.Contains(md, "**Name**: Service Eight") {
		t.Errorf("markdown missing Name:\n%s", md)
	}
	if strings.Contains(md, "**Email**") {
		t.Errorf("markdown should not contain Email when empty:\n%s", md)
	}
	if strings.Contains(md, "**Unconfirmed Email**") {
		t.Errorf("markdown should not contain Unconfirmed Email when empty:\n%s", md)
	}
}
