// user_service_accounts_test.go contains unit tests for GitLab service account
// operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestCreateCurrentUserPAT_ReadsWhatTheSDKDoesNotModel verifies the token
// carries, beside what client-go decoded, the three fields
// lib/api/entities/personal_access_token.rb sends and the SDK struct does not.
func TestCreateCurrentUserPAT_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"name":"mine","active":true,"scopes":["api"],`+
			`"granular":true,"last_used_ips":["192.0.2.10"],"granular_scopes":[{"access":"personal_projects","permissions":["read_job"],"project_id":3}]}`)
	}))

	out, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{Name: "mine", Scopes: []string{"api"}})
	if err != nil {
		t.Fatalf("CreateCurrentUserPAT() unexpected error: %v", err)
	}
	if !out.Granular || len(out.GranularScopes) != 1 || out.GranularScopes[0].ProjectID != 3 || len(out.LastUsedIPs) != 1 {
		t.Errorf("CreateCurrentUserPAT() = %+v, want the captured fields", out)
	}
}

// TestCreateCurrentUserPAT_ACapturedFieldTheTypeCannotHold_IsReported
// verifies the one failure the captured response adds: GitLab's answer
// decodes for the SDK and not for the fields read beside it, and the handler
// reports it rather than swallowing it.
func TestCreateCurrentUserPAT_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"name":"mine","granular":"not-a-bool"}`)
	}))

	_, err := CreateCurrentUserPAT(context.Background(), client, CreateCurrentUserPATInput{Name: "mine", Scopes: []string{"api"}})

	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("CreateCurrentUserPAT() error = %v, want the capture's decode failure", err)
	}
}

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
	// The state condition's absent side: this account has no address change
	// pending, so lib/api/entities/service_account.rb exposes no
	// unconfirmed_email and the output must not invent one.
	if out.UnconfirmedEmail != "" {
		t.Errorf("out.UnconfirmedEmail = %q, want empty when nothing is pending", out.UnconfirmedEmail)
	}
}

// TestCreateServiceAccount_ReadsThePendingAddress verifies the one key
// lib/api/entities/service_account.rb sends beyond what client-go's User
// models, on the side of its condition where the account does have an address
// change waiting to be confirmed. POST /service_accounts is the only route
// reaching this output type that presents a service account rather than a user.
func TestCreateServiceAccount_ReadsThePendingAddress(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/service_accounts" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":42,"username":"svc-bot","name":"Service","email":"svc@example.com","unconfirmed_email":"new@example.com"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateServiceAccount(context.Background(), client, CreateServiceAccountInput{Username: "svc-bot"})
	if err != nil {
		t.Fatalf("CreateServiceAccount() unexpected error: %v", err)
	}
	if out.UnconfirmedEmail != "new@example.com" {
		t.Errorf("out.UnconfirmedEmail = %q, want the pending address", out.UnconfirmedEmail)
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

// TestFormatServiceAccountListMarkdownString_Empty verifies that a list with
// nothing in it is the one sentence and nothing else: it used to be a heading
// counting nothing above a warning sign, which reads as a failure rather than
// as an instance with no service accounts on it.
func TestFormatServiceAccountListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatServiceAccountListMarkdownString(ServiceAccountListOutput{}), "No service accounts found.\n")
}

// TestFormatCurrentUserPATMarkdownString verifies the whole card for a token
// GitLab returned without its secret, which is every read of an existing one.
func TestFormatCurrentUserPATMarkdownString(t *testing.T) {
	assertMarkdown(t, FormatCurrentUserPATMarkdownString(CurrentUserPATOutput{
		ID: 1, Name: "test", Scopes: []string{"api"}, UserID: 42,
	}),
		"## Personal Access Token\n\n"+
			"- **ID**: 1\n"+
			"- **Name**: test\n"+
			"- **Active**: ❌\n"+
			"- **Scopes**: api\n"+
			"- **Granular**: ❌\n")
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
		Page:    1, PerPage: 20,
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

// TestFormatServiceAccountListMarkdownString_WithData verifies the whole list
// render.
func TestFormatServiceAccountListMarkdownString_WithData(t *testing.T) {
	out := ServiceAccountListOutput{
		Accounts: []ServiceAccountOutput{
			{ID: 1, Username: "svc-1", Name: "Service 1"},
			{ID: 2, Username: "svc-2", Name: "Service 2"},
		},
	}

	assertMarkdown(t, FormatServiceAccountListMarkdownString(out),
		"## Service Accounts (2)\n\n"+
			"| ID | Username | Name | Email |\n"+
			"| --- | --- | --- | --- |\n"+
			"| 1 | svc-1 | Service 1 |  |\n"+
			"| 2 | svc-2 | Service 2 |  |\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.create_service_account' to add a service account\n")
}

// TestFormatCurrentUserPATMarkdownString_WithAllFields verifies the whole card
// for a token GitLab minted: the secret in a code span, the expiry in the
// display form, and the store-securely sentence the secret row adds.
func TestFormatCurrentUserPATMarkdownString_WithAllFields(t *testing.T) {
	assertMarkdown(t, FormatCurrentUserPATMarkdownString(CurrentUserPATOutput{
		ID:          10,
		Name:        "my-pat",
		Active:      true,
		Token:       "glpat-secret",
		Scopes:      []string{"api", "read_user"},
		Description: "Test token",
		ExpiresAt:   "2026-01-15",
		UserID:      1,
	}),
		"## Personal Access Token\n\n"+
			"- **ID**: 10\n"+
			"- **Name**: my-pat\n"+
			"- **Active**: ✅\n"+
			"- **Scopes**: api, read_user\n"+
			"- **Granular**: ❌\n"+
			"- **Description**: Test token\n"+
			"- **Expires At**: 15 Jan 2026\n"+
			"- **Token**: `glpat-secret`\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Store the token securely. It cannot be retrieved later\n")
}

// TestFormatCurrentUserPATMarkdownString_Granular verifies the granular token:
// a token scoped by permissions rather than by the scopes list carried neither
// the flag nor the scopes, so the card said nothing about what it could reach.
func TestFormatCurrentUserPATMarkdownString_Granular(t *testing.T) {
	assertMarkdown(t, FormatCurrentUserPATMarkdownString(CurrentUserPATOutput{
		ID:       11,
		Name:     "granular-pat",
		Active:   true,
		Scopes:   []string{},
		Granular: true,
		GranularScopes: []toolutil.TokenGranularScopeOutput{
			{Access: "personal_projects", Permissions: []string{"read_job", "read_code"}, ProjectID: 3},
			{Access: "group", Permissions: []string{"read_group"}, GroupID: 9},
		},
	}),
		"## Personal Access Token\n\n"+
			"- **ID**: 11\n"+
			"- **Name**: granular-pat\n"+
			"- **Active**: ✅\n"+
			"- **Granular**: ✅\n"+
			"\n### Granular Scopes\n\n"+
			"| Access | Project ID | Group ID | Permissions |\n"+
			"| --- | --- | --- | --- |\n"+
			"| personal_projects | 3 |  | read_job, read_code |\n"+
			"| group |  | 9 | read_group |\n")
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
	assertMarkdown(t, FormatServiceAccountMarkdownString(out),
		"## Service Account\n\n"+
			"- **ID**: 7\n"+
			"- **Username**: svc-7\n"+
			"- **Name**: Service Seven\n"+
			"- **Email**: svc7@example.com\n"+
			"- **Unconfirmed Email**: pending@example.com\n")
}

// TestFormatServiceAccountMarkdownString_NoEmail verifies that the two absent
// addresses write no row rather than a label with nothing after it.
func TestFormatServiceAccountMarkdownString_NoEmail(t *testing.T) {
	out := ServiceAccountOutput{
		ID:       8,
		Username: "svc-8",
		Name:     "Service Eight",
	}

	assertMarkdown(t, FormatServiceAccountMarkdownString(out),
		"## Service Account\n\n"+
			"- **ID**: 8\n"+
			"- **Username**: svc-8\n"+
			"- **Name**: Service Eight\n")
}
