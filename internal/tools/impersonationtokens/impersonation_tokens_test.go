// impersonation_tokens_test.go contains unit tests for GitLab impersonation
// token operations. Tests use httptest to mock the GitLab API, including the
// fields GitLab sends that client-go's token structs do not carry.
package impersonationtokens

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

	// capturedTokenJSON is what GitLab sends beside the fields the SDK
	// decodes: the impersonation flag and the personal access token fields
	// gl.ImpersonationToken does not model.
	capturedTokenJSON = `{"id":1,"name":"test-token","active":true,"scopes":["api"],"impersonation":true,
		"description":"acting as","user_id":42,"granular":true,"last_used_ips":["192.0.2.10"],
		"granular_scopes":[{"access":"personal_projects","permissions":["read_job"],"project_id":3}]}`

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

// TestGet_ReadsWhatTheSDKDoesNotModel verifies an impersonation token carries,
// beside what client-go decoded, the fields
// lib/api/entities/impersonation_token.rb sends and gl.ImpersonationToken does
// not: its impersonation flag, and the description, user_id, granular,
// granular_scopes and last_used_ips of the personal access token entity it
// inherits.
func TestGet_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, capturedTokenJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if !out.Impersonation || out.Description != "acting as" || out.UserID != 42 || !out.Granular ||
		len(out.GranularScopes) != 1 || out.GranularScopes[0].ProjectID != 3 || len(out.LastUsedIPs) != 1 {
		t.Errorf("Get() = %+v, want the captured fields", out)
	}
}

// TestCreatePAT_ReadsWhatTheSDKDoesNotModel verifies the personal access
// token this package creates carries the three fields
// lib/api/entities/personal_access_token.rb sends and the SDK struct lacks.
func TestCreatePAT_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"my-pat","active":true,"scopes":["api"],`+
			`"granular":true,"last_used_ips":["192.0.2.10"],"granular_scopes":[{"access":"group","permissions":["read_job"],"group_id":5}]}`)
	}))

	out, err := CreatePAT(context.Background(), client, CreatePATInput{UserID: 42, Name: "my-pat", Scopes: []string{"api"}})
	if err != nil {
		t.Fatalf("CreatePAT() unexpected error: %v", err)
	}
	if !out.Granular || len(out.GranularScopes) != 1 || out.GranularScopes[0].GroupID != 5 || len(out.LastUsedIPs) != 1 {
		t.Errorf("CreatePAT() = %+v, want the captured fields", out)
	}
}

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler that presents a token:
// GitLab's answer decodes for the SDK and not for the fields read beside it,
// and the handler reports it rather than swallowing it, on a token alone and
// on a list of them.
func TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"id":1,"name":"test-token","granular":"not-a-bool"}`
		if r.Method == http.MethodGet && r.URL.Path == pathListTokens {
			body = "[" + body + "]"
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error { _, err := List(context.Background(), client, ListInput{UserID: 42}); return err }},
		{Name: "get", Call: func() error {
			_, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 1})
			return err
		}},
		{Name: "create", Call: func() error {
			_, err := Create(context.Background(), client, CreateInput{UserID: 42, Name: "t", Scopes: []string{"api"}})
			return err
		}},
		{Name: "create personal access token", Call: func() error {
			_, err := CreatePAT(context.Background(), client, CreatePATInput{UserID: 42, Name: "t", Scopes: []string{"api"}})
			return err
		}},
	})
}

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_InvalidUserID asserts that List refuses a user_id of zero with an
// error. It reaches no GitLab path, the guard running before the request; that
// no request is made is asserted by
// TestHandlers_AnIdentifierOfZero_IsRefusedBeforeAnyRequest.
func TestList_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestList_WithStateFilter asserts that a state the caller gave reaches the
// listing request's query, and that the single token GitLab answers with comes
// back.
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

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_InvalidUserID asserts that Get refuses a user_id of zero with an
// error, before any request.
func TestGet_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{UserID: 0, TokenID: 1})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestGet_InvalidTokenID asserts that Get refuses a token_id of zero with an
// error, before any request.
func TestGet_InvalidTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{UserID: 42, TokenID: 0})
	if err == nil {
		t.Fatal("expected error for invalid token_id, got nil")
	}
}

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreate_EmptyName asserts that Create refuses a token with no name with
// an error, before any request.
func TestCreate_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{UserID: 42, Name: "", Scopes: []string{"api"}})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestCreate_EmptyScopes asserts that Create refuses a token with no scopes
// with an error, before any request.
func TestCreate_EmptyScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{UserID: 42, Name: "test", Scopes: nil})
	if err == nil {
		t.Fatal("expected error for empty scopes, got nil")
	}
}

// TestCreate_InvalidExpiresAt asserts that Create refuses an expiry it cannot
// parse as YYYY-MM-DD with an error, before any request.
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

// TestRevoke_Success verifies that Revoke succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestRevoke_InvalidUserID asserts that Revoke refuses a user_id of zero with
// an error, before any request.
func TestRevoke_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Revoke(context.Background(), client, RevokeInput{UserID: 0, TokenID: 1})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestCreatePAT_Success verifies that CreatePAT succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreatePAT_EmptyName asserts that CreatePAT refuses a token with no name
// with an error, before any request.
func TestCreatePAT_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreatePAT(context.Background(), client, CreatePATInput{UserID: 42, Scopes: []string{"api"}})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// The guidance sections the impersonation-token formatters close with.
const (
	tokHintsOpening = "\n---\n\U0001F4A1 **Next steps:**\n"

	tokCardHints = tokHintsOpening +
		"- Use action 'user.revoke_impersonation_token' to revoke this token\n"

	tokListHints = tokHintsOpening +
		"- Use action 'user.get_impersonation_token' to read one of these tokens in full\n" +
		"- Use action 'user.revoke_impersonation_token' to revoke one of these tokens\n"

	tokRevokeHints = tokHintsOpening +
		"- Use action 'user.list_impersonation_tokens' to list the tokens this user has left\n"
)

// TestFormatListMarkdownString_Empty pins the whole response of a list with no
// tokens: the one sentence, where the warning sign under a heading counting
// nothing used to say the same thing twice.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	if got, want := FormatListMarkdownString(ListOutput{}), "No impersonation tokens found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString pins the whole card of an ordinary impersonation
// token: no secret row, so no store-it hint.
func TestFormatMarkdownString(t *testing.T) {
	got := FormatMarkdownString(Output{ID: 1, Name: "test", Scopes: []string{"api"}, Active: true})

	want := "## Impersonation Token #1\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: test\n" +
		"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Scopes**: api\n" +
		tokCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatPATMarkdownString pins the whole card of a personal access token
// created for another user.
func TestFormatPATMarkdownString(t *testing.T) {
	got := FormatPATMarkdownString(PATOutput{ID: 1, Name: "test", Scopes: []string{"api"}, UserID: 42})

	want := "## Personal Access Token #1\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: test\n" +
		"- **Active**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Scopes**: api\n" +
		"- **User ID**: 42\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestList_PaginationParams asserts that the page and per_page a caller gave
// reach the listing request's query.
//
// It stops there because there is nothing further to assert: ListOutput carries
// no [toolutil.PaginationOutput], so the page, total and next-page headers
// GitLab answers a paged listing with are read by nothing. That is the R-PAGE
// finding this action is the worked example of, and closing it moves the
// published surface rather than the tests.
func TestList_PaginationParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathListTokens)
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "50")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		UserID: 42,
		Page:   2, PerPage: 50,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tokens) != 0 {
		t.Errorf("len(out.Tokens) = %d, want 0", len(out.Tokens))
	}
}

// TestList_KeysetAndSortParams verifies that List forwards keyset pagination
// (pagination, page_token) and ordering (order_by, sort) query parameters to
// the GitLab API.
//
// The test exercises the GET path of the underlying GitLab API call.
// It asserts each keyset and ordering parameter reaches the request query.
func TestList_KeysetAndSortParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathListTokens)
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "cursor-123")
		testutil.AssertQueryParam(t, r, "order_by", "created_at")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		UserID:     42,
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "cursor-123",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tokens) != 0 {
		t.Errorf("len(out.Tokens) = %d, want 0", len(out.Tokens))
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab
// API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the wrapped error names the operation; the hint it carries is
// asserted by TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith.
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

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab
// API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the wrapped error names the operation; the hint it carries is
// asserted by TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith.
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

// TestCreate_InvalidUserID asserts that Create refuses a user_id of zero with
// an error naming the field, before any request.
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

// TestCreate_APIError verifies that Create returns a wrapped error when the
// GitLab API responds with an error status.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts that the wrapped error names the operation; the hint it carries is
// asserted by TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith.
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

// TestRevoke_InvalidTokenID asserts that Revoke refuses a token_id of zero
// with an error naming the field, before any request.
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

// TestRevoke_APIError verifies that Revoke returns a wrapped error when the
// GitLab API responds with an error status.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the wrapped error names the operation; the hint it carries is
// asserted by TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith.
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

// TestCreatePAT_InvalidUserID asserts that CreatePAT refuses a negative
// user_id with an error naming the field, before any request.
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

// TestCreatePAT_EmptyScopes asserts that CreatePAT refuses a token with no
// scopes with an error naming the field, before any request.
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

// TestCreatePAT_InvalidExpiresAt asserts that CreatePAT refuses an expiry it
// cannot parse as YYYY-MM-DD with an error naming the field, before any
// request.
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

// TestCreatePAT_APIError verifies that CreatePAT returns a wrapped error when
// the GitLab API responds with an error status.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts that the wrapped error names the operation; the hint it carries is
// asserted by TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith.
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

// TestCreatePAT_MinimalInput asserts what a personal access token created with
// neither description nor expiry carries back: the id and the secret GitLab
// answered with, and both optional fields empty.
// The test exercises the POST path of the underlying GitLab API call.
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

// TestToPATOutput_WithLastUsedAt asserts that a last_used_at GitLab sent on a
// personal access token reaches the output rather than being dropped with the
// pointer it arrived behind.
// The test exercises the POST path of the underlying GitLab API call.
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

// TestFormatListMarkdownString_WithTokens pins the whole table, the Revoked
// column included: a list of tokens that never said which of them are dead is
// what a reader of an audit cannot act on.
func TestFormatListMarkdownString_WithTokens(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 1, Name: "token-a", Active: true, Scopes: []string{"api", "read_user"}, ExpiresAt: "2026-12-31"},
			{ID: 2, Name: "token-b", Active: false, Revoked: true, Scopes: []string{"read_api"}, ExpiresAt: ""},
		},
	}

	yes, no := toolutil.BoolEmoji(true), toolutil.BoolEmoji(false)
	want := "## Impersonation Tokens (2)\n\n" +
		"| ID | Name | Active | Revoked | Scopes | Expires At |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | token-a | " + yes + " | " + no + " | api, read_user | 31 Dec 2026 |\n" +
		"| 2 | token-b | " + no + " | " + yes + " | read_api | never |\n" +
		tokListHints

	if got := FormatListMarkdownString(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_AllOptionalFields pins the whole card of a token
// with every optional field set, the revocation among them.
func TestFormatMarkdownString_AllOptionalFields(t *testing.T) {
	out := Output{
		ID: 5, Name: "full-token", Active: true, Revoked: true,
		Scopes: []string{"api"}, ExpiresAt: "2026-06-15", Token: "glpat-secret",
	}

	want := "## Impersonation Token #5\n\n" +
		"- **ID**: 5\n" +
		"- **Name**: full-token\n" +
		"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
		"- " + toolutil.EmojiWarning + " **Revoked**\n" +
		"- **Scopes**: api\n" +
		"- **Expires At**: 15 Jun 2026\n" +
		"- **Token**: `glpat-secret`\n" +
		tokHintsOpening +
		"- Store the token securely. It cannot be retrieved later\n" +
		"- Use action 'user.revoke_impersonation_token' to revoke this token\n"

	if got := FormatMarkdownString(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_MinimalFields pins the card of a token GitLab sent
// no expiry and no secret for: neither row is written, and no store-it hint.
func TestFormatMarkdownString_MinimalFields(t *testing.T) {
	got := FormatMarkdownString(Output{ID: 6, Name: "basic", Active: false, Scopes: []string{"read_user"}})

	want := "## Impersonation Token #6\n\n" +
		"- **ID**: 6\n" +
		"- **Name**: basic\n" +
		"- **Active**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Scopes**: read_user\n" +
		tokCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatPATMarkdownString_AllOptionalFields pins the whole card of a
// personal access token with description, expiry and secret.
func TestFormatPATMarkdownString_AllOptionalFields(t *testing.T) {
	out := PATOutput{
		ID: 10, Name: "full-pat", Active: true,
		Scopes: []string{"api"}, UserID: 42,
		Description: "My important PAT",
		ExpiresAt:   "2026-12-01",
		Token:       "glpat-fullpat",
	}

	want := "## Personal Access Token #10\n\n" +
		"- **ID**: 10\n" +
		"- **Name**: full-pat\n" +
		"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Scopes**: api\n" +
		"- **Description**: My important PAT\n" +
		"- **User ID**: 42\n" +
		"- **Expires At**: 1 Dec 2026\n" +
		"- **Token**: `glpat-fullpat`\n" +
		tokHintsOpening +
		"- Store the token securely. It cannot be retrieved later\n"

	if got := FormatPATMarkdownString(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatPATMarkdownString_MinimalFields pins the card of a personal access
// token GitLab sent nothing optional on.
func TestFormatPATMarkdownString_MinimalFields(t *testing.T) {
	got := FormatPATMarkdownString(PATOutput{ID: 11, Name: "bare", Active: false, Scopes: []string{"read_api"}, UserID: 99})

	want := "## Personal Access Token #11\n\n" +
		"- **ID**: 11\n" +
		"- **Name**: bare\n" +
		"- **Active**: " + toolutil.BoolEmoji(false) + "\n" +
		"- **Scopes**: read_api\n" +
		"- **User ID**: 99\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestHandlers_AnIdentifierOfZero_IsRefusedBeforeAnyRequest holds all seven
// identifier guards to refusing zero here, without spending a request on it.
// Asserting only that an error came back cannot see this: with the guard
// loosened to `< 0`, zero flows on to GitLab, which answers 404, and the hint
// that refusal carries names the very field the local refusal names ("verify
// token_id with gitlab_list_impersonation_tokens"), so a message assertion
// passes on either path. What separates them is whether GitLab was asked at
// all, which is also the behavior that matters: `/users/0/impersonation_tokens`
// is a round trip that cannot succeed.
func TestHandlers_AnIdentifierOfZero_IsRefusedBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name  string
		field string
		call  func(context.Context, *gitlabclient.Client) error
	}{
		{"list rejects user_id", "user_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := List(ctx, c, ListInput{UserID: 0})
			return err
		}},
		{"get rejects user_id", "user_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Get(ctx, c, GetInput{UserID: 0, TokenID: 1})
			return err
		}},
		{"get rejects token_id", "token_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Get(ctx, c, GetInput{UserID: 42, TokenID: 0})
			return err
		}},
		{"create rejects user_id", "user_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Create(ctx, c, CreateInput{UserID: 0, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{"revoke rejects user_id", "user_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Revoke(ctx, c, RevokeInput{UserID: 0, TokenID: 1})
			return err
		}},
		{"revoke rejects token_id", "token_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Revoke(ctx, c, RevokeInput{UserID: 42, TokenID: 0})
			return err
		}},
		{"create_personal_access_token rejects user_id", "user_id", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := CreatePAT(ctx, c, CreatePATInput{UserID: 0, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var asked atomic.Bool
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				asked.Store(true)
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}))

			err := tc.call(t.Context(), client)
			if err == nil {
				t.Fatalf("expected an error for %s = 0, got nil", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error = %q, want it to name %q", err.Error(), tc.field)
			}
			if asked.Load() {
				t.Errorf("%s = 0 reached GitLab; the handler must refuse it without a request", tc.field)
			}
		})
	}
}

// TestCreatePAT_Description_ReachesTheBodyOnlyWhenGiven pins which body GitLab
// is sent. The guard around the optional description decides between a key
// carrying the caller's text and no key at all, and an inverted guard sends
// `"description": ""` for every call that named none: GitLab would record an
// empty description on the token as though the caller had asked for one, and
// nothing in the response this handler reads back would contradict it.
func TestCreatePAT_Description_ReachesTheBodyOnlyWhenGiven(t *testing.T) {
	cases := []struct {
		name  string
		input CreatePATInput
		want  any // nil means the key must be absent
	}{
		{
			name:  "a description the caller gave is sent",
			input: CreatePATInput{UserID: 42, Name: "my-pat", Scopes: []string{"api"}, Description: "for the nightly job"},
			want:  "for the nightly job",
		},
		{
			name:  "no description leaves the key off entirely",
			input: CreatePATInput{UserID: 42, Name: "my-pat", Scopes: []string{"api"}},
			want:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding the request body: %v", err)
					testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unreadable request body"}`)
					return
				}
				testutil.RespondJSON(w, http.StatusCreated, patJSON)
			}))

			if _, err := CreatePAT(t.Context(), client, tc.input); err != nil {
				t.Fatalf("CreatePAT() unexpected error: %v", err)
			}
			got, present := body["description"]
			if tc.want == nil {
				if present {
					t.Errorf("body carries description = %v, want the key to be absent", got)
				}
				return
			}
			if !present {
				t.Fatalf("body = %v, want it to carry a description", body)
			}
			if got != tc.want {
				t.Errorf("description = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGet_TheTokenIsAssembledFieldByField pins the whole token one Get
// produces from a body in which no two values agree.
//
// The converter is straight-line assignment, which neither gate can reach: no
// operator to flip and no condition to evaluate both ways. Two crossings it
// leaves invisible are already in reach here, because scopes and last_used_ips
// are both lists of strings and created_at and last_used_at are both RFC3339
// instants, so a fixture giving either pair one value apiece publishes GitLab's
// answer under the wrong name and every field-at-a-time assertion still passes.
// The body also names a user the request did not, so publishing the request's
// user_id in place of the response's is visible too.
func TestGet_TheTokenIsAssembledFieldByField(t *testing.T) {
	const body = `{
		"id":77,"name":"nightly-audit","active":true,"token":"glpat-7QcRz",
		"scopes":["read_api","read_user"],"revoked":false,
		"created_at":"2026-01-15T10:00:00Z","expires_at":"2026-09-30",
		"last_used_at":"2026-06-01T08:00:00Z",
		"impersonation":true,"description":"raised for the quarterly audit","user_id":91,
		"granular":true,"last_used_ips":["192.0.2.10","198.51.100.7"],
		"granular_scopes":[{"access":"personal_projects","permissions":["read_job"],"project_id":3}]
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathGetToken)
		testutil.RespondJSON(w, http.StatusOK, body)
	}))

	got, err := Get(t.Context(), client, GetInput{UserID: 42, TokenID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}

	want := Output{
		ID: 77, Name: "nightly-audit", Active: true, Token: "glpat-7QcRz",
		Scopes:   []string{"read_api", "read_user"},
		Granular: true,
		GranularScopes: []toolutil.TokenGranularScopeOutput{
			{Access: "personal_projects", Permissions: []string{"read_job"}, ProjectID: 3},
		},
		Description:   "raised for the quarterly audit",
		UserID:        91,
		Impersonation: true,
		CreatedAt:     "2026-01-15T10:00:00Z",
		ExpiresAt:     "2026-09-30",
		LastUsedAt:    "2026-06-01T08:00:00Z",
		LastUsedIPs:   []string{"192.0.2.10", "198.51.100.7"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Get() = %+v,\nwant %+v", got, want)
	}
}

// TestGet_EachFlagIsReadOntoItsOwnField drives one boolean at a time, which is
// the only fixture that tells four flags apart: a body setting several of them
// true is one where a crossed assignment publishes exactly the same token. Each
// case compares the whole token, so a flag that landed on a neighbour's field
// fails on both fields at once.
func TestGet_EachFlagIsReadOntoItsOwnField(t *testing.T) {
	cases := []struct {
		name string // the one flag the body sets, and the subtest's name
		want Output
	}{
		{name: "active", want: Output{ID: 1, Name: "flagged", Active: true}},
		{name: "revoked", want: Output{ID: 1, Name: "flagged", Revoked: true}},
		{name: "granular", want: Output{ID: 1, Name: "flagged", Granular: true}},
		{name: "impersonation", want: Output{ID: 1, Name: "flagged", Impersonation: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"flagged","`+tc.name+`":true}`)
			}))

			got, err := Get(t.Context(), client, GetInput{UserID: 42, TokenID: 1})
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("with %s alone set, Get() = %+v,\nwant %+v", tc.name, got, tc.want)
			}
		})
	}
}

// TestList_EachTokenKeepsTheCapturedFieldsOfItsOwnRow pins the join between
// what the SDK decoded and what the capture read beside it. They arrive as two
// slices paired by index, so handing every token the first row's extra is a
// straight-line change no gate can see, and a fixture whose rows carry the same
// captured values cannot see it either.
func TestList_EachTokenKeepsTheCapturedFieldsOfItsOwnRow(t *testing.T) {
	const rows = `[
		{"id":1,"name":"first","active":true,"scopes":["api"],"revoked":false,
		 "impersonation":true,"description":"the first","user_id":42,"granular":true,
		 "last_used_ips":["192.0.2.10"]},
		{"id":2,"name":"second","active":false,"scopes":["read_user"],"revoked":true,
		 "impersonation":false,"description":"the second","user_id":43,"granular":false}
	]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, pathListTokens)
		testutil.RespondJSON(w, http.StatusOK, rows)
	}))

	out, err := List(t.Context(), client, ListInput{UserID: 42})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}

	want := []Output{
		{
			ID: 1, Name: "first", Active: true, Scopes: []string{"api"},
			Granular: true, Description: "the first", UserID: 42,
			Impersonation: true, LastUsedIPs: []string{"192.0.2.10"},
		},
		{
			ID: 2, Name: "second", Revoked: true, Scopes: []string{"read_user"},
			Description: "the second", UserID: 43,
		},
	}
	if !reflect.DeepEqual(out.Tokens, want) {
		t.Errorf("List() tokens = %+v,\nwant %+v", out.Tokens, want)
	}
}

// TestRevoke_TheConfirmationNamesTheUserAndTheTokenApart pins both identifiers
// of the confirmation a revocation answers with. Both are int64 copied straight
// off the input, so crossing them is invisible to both gates, and the success
// test beside this one reads only the revoked flag: a model told token 42 of
// user 7 is gone would go looking for the wrong token next.
func TestRevoke_TheConfirmationNamesTheUserAndTheTokenApart(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		testutil.AssertRequestPath(t, r, "/api/v4/users/42/impersonation_tokens/7")
		w.WriteHeader(http.StatusNoContent)
	}))

	got, err := Revoke(t.Context(), client, RevokeInput{UserID: 42, TokenID: 7})
	if err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}
	want := RevokeOutput{UserID: 42, TokenID: 7, Revoked: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Revoke() = %+v, want %+v", got, want)
	}
}

// TestCreateHandlers_TheBodyGitLabReceives_CarriesWhatTheCallerGave pins the
// two request bodies this package builds. Nothing else reads them: the mock
// answers with its fixture whatever it was sent, so an option left unset, or
// filled from the wrong input field, reaches GitLab unnoticed by every
// assertion the package makes about the response. The two expiry encodings
// differ on purpose and are the SDK option types' own: an impersonation token's
// expires_at is a timestamp and a personal access token's is a date.
func TestCreateHandlers_TheBodyGitLabReceives_CarriesWhatTheCallerGave(t *testing.T) {
	cases := []struct {
		name string
		path string
		call func(context.Context, *gitlabclient.Client) error
		want map[string]any
	}{
		{
			name: "create_impersonation_token",
			path: pathCreateToken,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Create(ctx, c, CreateInput{
					UserID: 42, Name: "nightly-audit",
					Scopes: []string{"read_api", "read_user"}, ExpiresAt: "2026-09-30",
				})
				return err
			},
			want: map[string]any{
				"name":       "nightly-audit",
				"scopes":     []any{"read_api", "read_user"},
				"expires_at": "2026-09-30T00:00:00Z",
			},
		},
		{
			name: "create_personal_access_token",
			path: pathCreatePAT,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := CreatePAT(ctx, c, CreatePATInput{
					UserID: 42, Name: "release-bot", Scopes: []string{"api"},
					Description: "for the release job", ExpiresAt: "2026-09-30",
				})
				return err
			},
			want: map[string]any{
				"name":        "release-bot",
				"scopes":      []any{"api"},
				"description": "for the release job",
				"expires_at":  "2026-09-30",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, tc.path)
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding the request body: %v", err)
					testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unreadable request body"}`)
					return
				}
				testutil.RespondJSON(w, http.StatusCreated, tokenJSON)
			}))

			if err := tc.call(t.Context(), client); err != nil {
				t.Fatalf("%s unexpected error: %v", tc.name, err)
			}
			if !reflect.DeepEqual(body, tc.want) {
				t.Errorf("body = %v, want %v", body, tc.want)
			}
		})
	}
}

// TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith holds each
// handler's corrective hint to the status GitLab really answers with. The
// status is an argument rather than a branch of this package, so a handler
// hinting on 404 where its endpoint refuses with 403 keeps returning an error
// and the error tests beside this one, which read only the operation name, keep
// passing — while the model loses the one sentence saying what to do next.
func TestHandlers_TheHintedStatus_IsTheOneItsEndpointRefusesWith(t *testing.T) {
	cases := []struct {
		name   string
		status int
		hint   string
		call   func(context.Context, *gitlabclient.Client) error
	}{
		{
			name: "list", status: http.StatusForbidden,
			hint: "impersonation tokens require admin token",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := List(ctx, c, ListInput{UserID: 42})
				return err
			},
		},
		{
			name: "get", status: http.StatusNotFound,
			hint: "the token may have been revoked",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Get(ctx, c, GetInput{UserID: 42, TokenID: 1})
				return err
			},
		},
		{
			name: "create", status: http.StatusForbidden,
			hint: "creating impersonation tokens requires admin token",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Create(ctx, c, CreateInput{UserID: 42, Name: "tok", Scopes: []string{"api"}})
				return err
			},
		},
		{
			name: "revoke", status: http.StatusNotFound,
			hint: "the token may already be revoked",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Revoke(ctx, c, RevokeInput{UserID: 42, TokenID: 1})
				return err
			},
		},
		{
			name: "create_personal_access_token", status: http.StatusForbidden,
			hint: "creating PAT for another user requires admin token",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := CreatePAT(ctx, c, CreatePATInput{UserID: 42, Name: "tok", Scopes: []string{"api"}})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"refused"}`)
			}))

			err := tc.call(t.Context(), client)
			if err == nil {
				t.Fatalf("expected an error for status %d, got nil", tc.status)
			}
			if !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("error = %q, want it to carry the hint %q", err.Error(), tc.hint)
			}
		})
	}
}

// TestFormatRevokeMarkdownString pins the whole revocation confirmation.
func TestFormatRevokeMarkdownString(t *testing.T) {
	got := FormatRevokeMarkdownString(RevokeOutput{UserID: 42, TokenID: 7, Revoked: true})

	want := "## Token Revoked\n\n" +
		"- **User ID**: 42\n" +
		"- **Token ID**: 7\n" +
		"- **Revoked**: " + toolutil.BoolEmoji(true) + "\n" +
		tokRevokeHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
