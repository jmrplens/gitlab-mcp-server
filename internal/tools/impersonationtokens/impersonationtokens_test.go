// impersonationtokens_test.go contains unit tests for GitLab impersonation
// token operations. Tests use httptest to mock the GitLab API.

package impersonationtokens

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
