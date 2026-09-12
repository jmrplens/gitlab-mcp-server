// runner_controller_tokens_test.go contains unit tests for the runner controller token MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollertokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	sampleTokenJSON = `{"id":10,"runner_controller_id":1,"description":"my-token","token":"glrt-abc123","last_used_at":"2026-01-15T10:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`
	errUnexpected   = "unexpected error: %v"
	errExpValid     = "expected validation error, got nil"
	errExpAPIErr    = "expected API error, got nil"
	errExpCtxCancel = "expected context error, got nil"
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
}

// TestList_Success verifies that List returns tokens with pagination.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[`+sampleTokenJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	if out.Tokens[0].ID != 10 || out.Tokens[0].Token != "glrt-abc123" {
		t.Errorf("token mismatch: %+v", out.Tokens[0])
	}
}

// TestList_WithPagination verifies List passes pagination parameters.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		ControllerID: 1,
		Page:         2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_Empty verifies that List handles empty results.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(out.Tokens))
	}
}

// TestList_WithKeyset verifies List forwards keyset pagination parameters.
func TestList_WithKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		if q.Get("page_token") != "abc123" {
			t.Errorf("expected page_token=abc123, got %q", q.Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		ControllerID: 1,
		Pagination:   "keyset", PageToken: "abc123",
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_WithOrderBySort verifies that List forwards the order_by and sort
// query parameters (R-INPUT: gl.ListOptions.OrderBy/Sort).
func TestList_WithOrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %q", q.Get("sort"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		ControllerID: 1,
		OrderBy:      "id",
		Sort:         "asc",
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_MissingControllerID verifies that List rejects missing controller_id.
func TestList_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestList_APIError verifies that List propagates API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestList_NotFound verifies non-forbidden list errors use the controller lookup hint.
func TestList_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "gitlab_runner_controller_list") {
		t.Fatalf("error = %v, want controller list hint", err)
	}
}

// TestList_ContextCancelled verifies that List respects context cancellation.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGet_Success verifies that Get returns token details.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 || out.Token != "glrt-abc123" {
		t.Errorf("token mismatch: %+v", out)
	}
	if out.RunnerControllerID != 1 {
		t.Errorf("controller ID = %d, want 1", out.RunnerControllerID)
	}
}

// TestGet_MissingControllerID verifies that Get rejects missing controller_id.
func TestGet_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestGet_MissingTokenID verifies that Get rejects missing token_id.
func TestGet_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestGet_APIError verifies that Get propagates API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ControllerID: 1, TokenID: 999})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestGet_ContextCancelled verifies that Get respects context cancellation.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestCreate_Success verifies that Create returns the new token.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{ControllerID: 1, Description: "my-token"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 || out.Description != "my-token" {
		t.Errorf("output mismatch: %+v", out)
	}
}

// TestCreate_DefaultDescription verifies Create with empty description.
func TestCreate_DefaultDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
}

// TestCreate_MissingControllerID verifies Create rejects missing controller_id.
func TestCreate_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestCreate_APIError verifies that Create propagates API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestCreate_NotFound verifies non-forbidden create errors use the controller lookup hint.
func TestCreate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "gitlab_runner_controller_list") {
		t.Fatalf("error = %v, want controller list hint", err)
	}
}

// TestCreate_ContextCancelled verifies that Create respects context cancellation.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRotate_Success verifies that Rotate returns the rotated token.
func TestRotate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	}))

	out, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
}

// TestRotate_MissingControllerID verifies Rotate rejects missing controller_id.
func TestRotate_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Rotate(context.Background(), client, RotateInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRotate_MissingTokenID verifies Rotate rejects missing token_id.
func TestRotate_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestRotate_APIError verifies that Rotate propagates API errors.
func TestRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRotate_Unauthorized verifies revoked or expired token hints on unauthorized responses.
func TestRotate_Unauthorized(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"unauthorized"}`)
	}))
	_, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "already be revoked or expired") {
		t.Fatalf("error = %v, want revoked/expired hint", err)
	}
}

// TestRotate_ContextCancelled verifies that Rotate respects context cancellation.
func TestRotate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Rotate(ctx, client, RotateInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRevoke_Success verifies that Revoke succeeds.
func TestRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestRevoke_MissingControllerID verifies Revoke rejects missing controller_id.
func TestRevoke_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Revoke(context.Background(), client, RevokeInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRevoke_MissingTokenID verifies Revoke rejects missing token_id.
func TestRevoke_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestRevoke_APIError verifies that Revoke propagates API errors.
func TestRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRevoke_NotFound verifies non-forbidden revoke errors use the token lookup hint.
func TestRevoke_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "gitlab_runner_controller_token_list") {
		t.Fatalf("error = %v, want token list hint", err)
	}
}

// TestRevoke_ContextCancelled verifies that Revoke respects context cancellation.
func TestRevoke_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	err := Revoke(ctx, client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// rctStoreHints is the guidance section a card that showed a token ends with.
const rctStoreHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Store the token securely. It cannot be retrieved later\n"

// TestFormatOutputMarkdown pins the whole card in both states: the minted
// token with its storage advice, and the same type answering a get with
// neither the token nor the timestamps GitLab did not send.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		ID: 10, RunnerControllerID: 1, Description: "my-token",
		Token: "glrt-abc123", LastUsedAt: "2026-01-15T10:00:00Z",
		CreatedAt: "2026-01-01T00:00:00Z",
	}

	withToken := "## Runner Controller Token #10\n\n" +
		"- **ID**: 10\n" +
		"- **Controller ID**: 1\n" +
		"- **Description**: my-token\n" +
		"- **Token**: `glrt-abc123`\n" +
		"- **Last Used At**: 15 Jan 2026 10:00 UTC\n" +
		"- **Created At**: 1 Jan 2026 00:00 UTC\n" +
		rctStoreHints
	if got := FormatOutputMarkdown(out); got != withToken {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, withToken)
	}

	out.Token = ""
	out.LastUsedAt = ""
	out.CreatedAt = ""
	bare := "## Runner Controller Token #10\n\n" +
		"- **ID**: 10\n" +
		"- **Controller ID**: 1\n" +
		"- **Description**: my-token\n"
	if got := FormatOutputMarkdown(out); got != bare {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, bare)
	}
}

// TestFormatOutputMarkdown_TokenShapes_CodeSpannedAndHintedOnlyWhenPresent
// verifies that the token is written inside a code span, and that the advice
// to store it securely is written only on a card that carries one.
//
// Both halves matter. A token written as bare Markdown is read as Markdown, so
// an underscore pair in it is eaten as emphasis and what the reader copies
// back is not what GitLab minted. And the same Go type answers a get, which
// carries no token: telling that reader to store a value the card does not
// hold sends them hunting for a credential that was never shown.
func TestFormatOutputMarkdown_TokenShapes_CodeSpannedAndHintedOnlyWhenPresent(t *testing.T) {
	t.Run("token is code spanned and hinted", func(t *testing.T) {
		want := "## Runner Controller Token #10\n\n" +
			"- **ID**: 10\n" +
			"- **Controller ID**: 1\n" +
			"- **Token**: `glrt-a_b_c`\n" +
			rctStoreHints
		if got := FormatOutputMarkdown(Output{ID: 10, RunnerControllerID: 1, Token: "glrt-a_b_c"}); got != want {
			t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("no token means no storage hint", func(t *testing.T) {
		want := "## Runner Controller Token #10\n\n" +
			"- **ID**: 10\n" +
			"- **Controller ID**: 1\n" +
			"- **Description**: my-token\n"
		if got := FormatOutputMarkdown(Output{ID: 10, RunnerControllerID: 1, Description: "my-token"}); got != want {
			t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})
}

// TestFormatListMarkdown pins the whole list document. The heading counts the
// total GitLab reported, and falls back to the rows shown when it reported
// none, where it used to print a zero above a table of rows.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 10, RunnerControllerID: 1, Description: "tok-1", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 11, RunnerControllerID: 1, Description: "tok-2"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}

	listHints := "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runnercontrollertokens.controller_token_get' to read one of these tokens in full\n"
	want := "## Runner Controller Tokens (2)\n\n" +
		"| ID | Controller | Description | Last Used | Created At |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 10 | 1 | tok-1 |  | 1 Jan 2026 00:00 UTC |\n" +
		"| 11 | 1 | tok-2 |  |  |\n\n" +
		"2 items total\n" +
		listHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	wantEmpty := "No runner controller tokens found.\n"
	if got := FormatListMarkdown(ListOutput{}); got != wantEmpty {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, wantEmpty)
	}
}

// TestFormatListMarkdown_Keyset pins the heading of a keyset page, where
// GitLab sends no total at all: the count is what the page shows with "more
// available" beside it, never a zero above two rows.
func TestFormatListMarkdown_Keyset(t *testing.T) {
	out := ListOutput{
		Tokens:     []Output{{ID: 10, RunnerControllerID: 1, Description: "tok-1"}},
		Pagination: toolutil.PaginationOutput{HasMore: true},
	}

	listHints := "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runnercontrollertokens.controller_token_get' to read one of these tokens in full\n"
	want := "## Runner Controller Tokens (1 shown, more available)\n\n" +
		"| ID | Controller | Description | Last Used | Created At |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 10 | 1 | tok-1 |  |  |\n" +
		listHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown returns a non-nil result.
func TestFormatGetMarkdown(t *testing.T) {
	out := Output{ID: 10, RunnerControllerID: 1, Description: "my-token"}
	result := FormatGetMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
