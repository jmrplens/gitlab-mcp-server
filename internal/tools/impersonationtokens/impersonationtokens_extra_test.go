package impersonationtokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

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
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
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
