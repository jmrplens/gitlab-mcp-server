// impersonationtokens_test.go contains unit tests for GitLab impersonation
// token operations. Tests use httptest to mock the GitLab API.
package impersonationtokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	pathListTokens  = "/api/v4/users/42/impersonation_tokens"
	pathGetToken    = "/api/v4/users/42/impersonation_tokens/1"
	pathCreateToken = "/api/v4/users/42/impersonation_tokens"
	pathRevokeToken = "/api/v4/users/42/impersonation_tokens/1"
	pathCreatePAT   = "/api/v4/users/42/personal_access_tokens"

	tokenJSON = `{
		"id":1,
		"name":"test-token",
		"active":true,
		"token":"glpat-abc123",
		"scopes":["api","read_user"],
		"revoked":false,
		"created_at":"2026-01-15T10:00:00Z",
		"expires_at":"2026-01-15",
		"last_used_at":"2026-06-01T08:00:00Z"
	}`

	tokenListJSON = `[{
		"id":1,"name":"token-1","active":true,"scopes":["api"],"revoked":false,
		"created_at":"2026-01-15T10:00:00Z"
	},{
		"id":2,"name":"token-2","active":false,"scopes":["read_user"],"revoked":true,
		"created_at":"2026-02-20T12:00:00Z"
	}]`

	patJSON = `{
		"id":10,
		"name":"my-pat",
		"active":true,
		"token":"glpat-xyz789",
		"scopes":["api"],
		"revoked":false,
		"description":"Test PAT",
		"user_id":42,
		"created_at":"2026-01-15T10:00:00Z",
		"expires_at":"2026-01-15"
	}`
)

// TestList_Success verifies that List returns the expected output when the GitLab API responds successfully.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListTokens {
			testutil.RespondJSON(w, http.StatusOK, tokenListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{UserID: 42})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tokens) != 2 {
		t.Fatalf("len(out.Tokens) = %d, want 2", len(out.Tokens))
	}
	if out.Tokens[0].Name != "token-1" {
		t.Errorf("out.Tokens[0].Name = %q, want %q", out.Tokens[0].Name, "token-1")
	}
}

// TestList_InvalidUserID verifies that List returns a validation error when user_id is invalid.
func TestList_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestList_WithStateFilter verifies that List forwards the state filter parameters to the GitLab API.
func TestList_WithStateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListTokens {
			if r.URL.Query().Get("state") != "active" {
				t.Errorf("expected state=active query param, got %q", r.URL.Query().Get("state"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"token-1","active":true,"scopes":["api"],"revoked":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{UserID: 42, State: "active"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("len(out.Tokens) = %d, want 1", len(out.Tokens))
	}
}

// TestGet_Success verifies that Get returns the expected output when the GitLab API responds successfully.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetToken {
			testutil.RespondJSON(w, http.StatusOK, tokenJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.Name != "test-token" {
		t.Errorf("out.Name = %q, want %q", out.Name, "test-token")
	}
	if !out.Active {
		t.Error("out.Active = false, want true")
	}
}

// TestGet_InvalidUserID verifies that Get returns a validation error when user_id is invalid.
func TestGet_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{UserID: 0, TokenID: 1})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestGet_InvalidTokenID verifies that Get returns a validation error when token_id is invalid.
func TestGet_InvalidTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for invalid token_id, got nil")
	}
}

// TestCreate_Success verifies that Create returns the expected output when the GitLab API responds successfully.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathCreateToken {
			testutil.RespondJSON(w, http.StatusCreated, tokenJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		UserID: 42, Name: "test-token", Scopes: []string{"api"}, ExpiresAt: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.Token != "glpat-abc123" {
		t.Errorf("out.Token = %q, want %q", out.Token, "glpat-abc123")
	}
}

// TestCreate_EmptyName verifies that Create returns a validation error when name is empty.
func TestCreate_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{UserID: 42, Name: "", Scopes: []string{"api"}})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestCreate_EmptyScopes verifies that Create returns a validation error when scopes is empty.
func TestCreate_EmptyScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{UserID: 42, Name: "test", Scopes: nil})
	if err == nil {
		t.Fatal("expected error for empty scopes, got nil")
	}
}

// TestCreate_InvalidExpiresAt verifies that Create returns a validation error when expires_at is invalid.
func TestCreate_InvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		UserID: 42, Name: "test", Scopes: []string{"api"}, ExpiresAt: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_at, got nil")
	}
}

// TestRevoke_Success verifies that Revoke returns the expected output when the GitLab API responds successfully.
func TestRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathRevokeToken {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Revoke(context.Background(), client, RevokeInput{UserID: 42, TokenID: 1})
	if err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}
	if !out.Revoked {
		t.Error("out.Revoked = false, want true")
	}
}

// TestRevoke_InvalidUserID verifies that Revoke returns a validation error when user_id is invalid.
func TestRevoke_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Revoke(context.Background(), client, RevokeInput{UserID: 0, TokenID: 1})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestCreatePAT_Success verifies that CreatePAT returns the expected output when the GitLab API responds successfully.
func TestCreatePAT_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathCreatePAT {
			testutil.RespondJSON(w, http.StatusCreated, patJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "my-pat", Scopes: []string{"api"}, Description: "Test PAT", ExpiresAt: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("CreatePAT() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
	if out.Description != "Test PAT" {
		t.Errorf("out.Description = %q, want %q", out.Description, "Test PAT")
	}
}

// TestCreatePAT_EmptyName verifies that CreatePAT returns a validation error when name is empty.
func TestCreatePAT_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreatePAT(context.Background(), client, CreatePATInput{UserID: 42, Scopes: []string{"api"}})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestFormatListMarkdownString_Empty verifies that FormatListMarkdownString returns a non-empty markdown string for an empty list.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md == "" {
		t.Fatal("expected non-empty markdown for empty list")
	}
}

// TestFormatMarkdownString verifies that FormatMarkdownString returns a non-empty markdown rendering of a token output.
func TestFormatMarkdownString(t *testing.T) {
	md := FormatMarkdownString(Output{ID: 1, Name: "test", Scopes: []string{"api"}, Active: true})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}

// TestFormatPATMarkdownString verifies that FormatPATMarkdownString returns a non-empty markdown rendering of a PAT output.
func TestFormatPATMarkdownString(t *testing.T) {
	md := FormatPATMarkdownString(PATOutput{ID: 1, Name: "test", Scopes: []string{"api"}, UserID: 42})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}

// TestList_PaginationParams verifies that Page and PerPage options are passed
// as query parameters to the GitLab API when provided.
func TestList_PaginationParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathListTokens)
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "50")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		UserID:  42,
		Page:    2,
		PerPage: 50,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tokens) != 0 {
		t.Errorf("len(out.Tokens) = %d, want 0", len(out.Tokens))
	}
}

// TestList_APIError verifies that the handler wraps GitLab API errors for List.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "list_impersonation_tokens") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "list_impersonation_tokens")
	}
}

// TestGet_APIError verifies that the handler wraps GitLab API errors for Get.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "get_impersonation_token") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "get_impersonation_token")
	}
}

// TestCreate_InvalidUserID verifies that Create rejects user_id <= 0.
func TestCreate_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		UserID: 0, Name: "test", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
	if !strings.Contains(err.Error(), "user_id") {
		t.Errorf("error = %q, want it to mention user_id", err.Error())
	}
}

// TestCreate_APIError verifies that the handler wraps GitLab API errors for Create.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		UserID: 42, Name: "test", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "create_impersonation_token") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "create_impersonation_token")
	}
}

// TestRevoke_InvalidTokenID verifies that Revoke rejects token_id <= 0.
func TestRevoke_InvalidTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Revoke(context.Background(), client, RevokeInput{UserID: 42, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for invalid token_id, got nil")
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error = %q, want it to mention token_id", err.Error())
	}
}

// TestRevoke_APIError verifies that the handler wraps GitLab API errors for Revoke.
func TestRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Revoke(context.Background(), client, RevokeInput{UserID: 42, TokenID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "revoke_impersonation_token") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "revoke_impersonation_token")
	}
}

// TestCreatePAT_InvalidUserID verifies that CreatePAT rejects user_id <= 0.
func TestCreatePAT_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: -1, Name: "test", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
	if !strings.Contains(err.Error(), "user_id") {
		t.Errorf("error = %q, want it to mention user_id", err.Error())
	}
}

// TestCreatePAT_EmptyScopes verifies that CreatePAT rejects empty scopes.
func TestCreatePAT_EmptyScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "test", Scopes: nil,
	})
	if err == nil {
		t.Fatal("expected error for empty scopes, got nil")
	}
	if !strings.Contains(err.Error(), "scopes") {
		t.Errorf("error = %q, want it to mention scopes", err.Error())
	}
}

// TestCreatePAT_InvalidExpiresAt verifies that CreatePAT rejects invalid date format.
func TestCreatePAT_InvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "test", Scopes: []string{"api"}, ExpiresAt: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_at, got nil")
	}
	if !strings.Contains(err.Error(), "expires_at") {
		t.Errorf("error = %q, want it to mention expires_at", err.Error())
	}
}

// TestCreatePAT_APIError verifies that the handler wraps GitLab API errors for CreatePAT.
func TestCreatePAT_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "test", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "create_personal_access_token") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "create_personal_access_token")
	}
}

// TestCreatePAT_MinimalInput verifies CreatePAT succeeds with no optional fields
// (no description, no expires_at).
func TestCreatePAT_MinimalInput(t *testing.T) {
	const minimalPATJSON = `{
		"id":20,"name":"bare-pat","active":true,"token":"glpat-min123",
		"scopes":["read_user"],"revoked":false,"user_id":42,
		"created_at":"2026-03-01T10:00:00Z"
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, pathCreatePAT)
		testutil.RespondJSON(w, http.StatusCreated, minimalPATJSON)
	}))

	out, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "bare-pat", Scopes: []string{"read_user"},
	})
	if err != nil {
		t.Fatalf("CreatePAT() unexpected error: %v", err)
	}
	if out.ID != 20 {
		t.Errorf("out.ID = %d, want 20", out.ID)
	}
	if out.Token != "glpat-min123" {
		t.Errorf("out.Token = %q, want %q", out.Token, "glpat-min123")
	}
	if out.Description != "" {
		t.Errorf("out.Description = %q, want empty", out.Description)
	}
	if out.ExpiresAt != "" {
		t.Errorf("out.ExpiresAt = %q, want empty", out.ExpiresAt)
	}
}

// TestToPATOutput_WithLastUsedAt verifies that toPATOutput formats LastUsedAt
// when the field is non-nil in the GitLab response.
func TestToPATOutput_WithLastUsedAt(t *testing.T) {
	const patWithLastUsed = `{
		"id":30,"name":"used-pat","active":true,"token":"glpat-used",
		"scopes":["api"],"revoked":false,"user_id":42,
		"created_at":"2026-01-01T00:00:00Z",
		"expires_at":"2026-06-01",
		"last_used_at":"2026-12-01T15:30:00Z"
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, patWithLastUsed)
	}))

	out, err := CreatePAT(context.Background(), client, CreatePATInput{
		UserID: 42, Name: "used-pat", Scopes: []string{"api"}, ExpiresAt: "2026-06-01",
	})
	if err != nil {
		t.Fatalf("CreatePAT() unexpected error: %v", err)
	}
	if out.LastUsedAt == "" {
		t.Error("out.LastUsedAt is empty, want non-empty when API returns last_used_at")
	}
}

// TestFormatListMarkdownString_WithTokens verifies proper Markdown table rendering
// for a non-empty token list, including tokens with and without expiration dates.
func TestFormatListMarkdownString_WithTokens(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 1, Name: "token-a", Active: true, Scopes: []string{"api", "read_user"}, ExpiresAt: "2026-12-31"},
			{ID: 2, Name: "token-b", Active: false, Scopes: []string{"read_api"}, ExpiresAt: ""},
		},
	}
	md := FormatListMarkdownString(out)

	checks := []string{
		"## Impersonation Tokens (2)",
		"| ID | Name | Active | Scopes | Expires At |",
		"| 1 | token-a | true | api, read_user | 2026-12-31 |",
		"| 2 | token-b | false | read_api | - |",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\ngot:\n%s", want, md)
		}
	}
}

// TestFormatMarkdownString_AllOptionalFields verifies that FormatMarkdownString
// renders ExpiresAt and Token fields when they are present.
func TestFormatMarkdownString_AllOptionalFields(t *testing.T) {
	out := Output{
		ID: 5, Name: "full-token", Active: true,
		Scopes: []string{"api"}, ExpiresAt: "2026-06-15", Token: "glpat-secret",
	}
	md := FormatMarkdownString(out)

	checks := []string{
		"## Impersonation Token",
		"**Name**: full-token",
		"**Active**: true",
		"**Scopes**: api",
		"**Expires At**: 2026-06-15",
		"**Token**: `glpat-secret`",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\ngot:\n%s", want, md)
		}
	}
}

// TestFormatMarkdownString_MinimalFields verifies that FormatMarkdownString omits
// ExpiresAt and Token when they are empty.
func TestFormatMarkdownString_MinimalFields(t *testing.T) {
	out := Output{ID: 6, Name: "basic", Active: false, Scopes: []string{"read_user"}}
	md := FormatMarkdownString(out)

	if strings.Contains(md, "**Expires At**") {
		t.Error("markdown should not contain '**Expires At**' for empty ExpiresAt")
	}
	if strings.Contains(md, "**Token**") {
		t.Error("markdown should not contain '**Token**' for empty Token")
	}
}

// TestFormatPATMarkdownString_AllOptionalFields verifies that FormatPATMarkdownString
// renders Description, ExpiresAt, and Token fields when present.
func TestFormatPATMarkdownString_AllOptionalFields(t *testing.T) {
	out := PATOutput{
		ID: 10, Name: "full-pat", Active: true,
		Scopes: []string{"api"}, UserID: 42,
		Description: "My important PAT",
		ExpiresAt:   "2026-12-01",
		Token:       "glpat-fullpat",
	}
	md := FormatPATMarkdownString(out)

	checks := []string{
		"## Personal Access Token",
		"**Name**: full-pat",
		"**Description**: My important PAT",
		"**User ID**: 42",
		"**Expires At**: 2026-12-01",
		"**Token**: `glpat-fullpat`",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\ngot:\n%s", want, md)
		}
	}
}

// TestFormatPATMarkdownString_MinimalFields verifies that FormatPATMarkdownString
// omits Description, ExpiresAt, and Token when empty.
func TestFormatPATMarkdownString_MinimalFields(t *testing.T) {
	out := PATOutput{ID: 11, Name: "bare", Active: false, Scopes: []string{"read_api"}, UserID: 99}
	md := FormatPATMarkdownString(out)

	if strings.Contains(md, "**Description**") {
		t.Error("markdown should not contain '**Description**' when empty")
	}
	if strings.Contains(md, "**Expires At**") {
		t.Error("markdown should not contain '**Expires At**' when empty")
	}
	if strings.Contains(md, "**Token**") {
		t.Error("markdown should not contain '**Token**' when empty")
	}
}

// TestFormatRevokeMarkdownString verifies the revocation confirmation markdown output.
func TestFormatRevokeMarkdownString(t *testing.T) {
	out := RevokeOutput{UserID: 42, TokenID: 7, Revoked: true}
	md := FormatRevokeMarkdownString(out)

	checks := []string{
		"## Token Revoked",
		"**User ID**: 42",
		"**Token ID**: 7",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\ngot:\n%s", want, md)
		}
	}
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}
