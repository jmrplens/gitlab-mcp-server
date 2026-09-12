// runner_controllers_test.go contains unit tests for the runner controller MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	sampleControllerJSON = `{"id":1,"description":"ctrl-1","state":"enabled","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	sampleDetailsJSON    = `{"id":1,"description":"ctrl-1","state":"enabled","connected":true,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	errUnexpected        = "unexpected error: %v"
	errExpValidation     = "expected validation error, got nil"
	errExpAPIErr         = "expected API error, got nil"
	errExpCtxCancel      = "expected context error, got nil"
	msgNotFound          = `{"message":"404 Not Found"}`
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
}

// TestList_Success verifies that List returns controllers with pagination.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[`+sampleControllerJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Controllers) != 1 {
		t.Fatalf("expected 1 controller, got %d", len(out.Controllers))
	}
	if out.Controllers[0].ID != 1 || out.Controllers[0].Description != "ctrl-1" {
		t.Errorf("controller mismatch: %+v", out.Controllers[0])
	}
	if out.Controllers[0].State != "enabled" {
		t.Errorf("state = %q, want enabled", out.Controllers[0].State)
	}
}

// TestList_WithPagination verifies that List passes pagination parameters.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got %s", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "1", TotalPages: "1"})
	}))

	_, err := List(context.Background(), client, ListInput{
		Page: 2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_WithKeysetPagination verifies that List forwards keyset pagination parameters.
func TestList_WithKeysetPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %s", r.URL.Query().Get("pagination"))
		}
		if r.URL.Query().Get("page_token") != "cursor-42" {
			t.Errorf("expected page_token=cursor-42, got %s", r.URL.Query().Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	_, err := List(context.Background(), client, ListInput{
		Pagination: "keyset", PageToken: "cursor-42",
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_WithOrderBySort verifies that List forwards the order_by and sort
// query parameters (R-INPUT: gl.ListOptions.OrderBy/Sort).
func TestList_WithOrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("order_by") != "created_at" {
			t.Errorf("expected order_by=created_at, got %s", r.URL.Query().Get("order_by"))
		}
		if r.URL.Query().Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %s", r.URL.Query().Get("sort"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	_, err := List(context.Background(), client, ListInput{OrderBy: "created_at", Sort: "desc"})
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

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Controllers) != 0 {
		t.Errorf("expected 0 controllers, got %d", len(out.Controllers))
	}
}

// TestList_APIError verifies that List propagates API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestList_ContextCancelled verifies that List respects context cancellation.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGet_Success verifies that Get returns controller details.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleDetailsJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 || !out.Connected {
		t.Errorf("details mismatch: %+v", out)
	}
	if out.Description != "ctrl-1" {
		t.Errorf("description = %q, want ctrl-1", out.Description)
	}
}

// TestGet_MissingID verifies that Get rejects missing controller_id.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestGet_APIError verifies that Get propagates API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))

	_, err := Get(context.Background(), client, GetInput{ControllerID: 999})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestGet_ContextCancelled verifies that Get respects context cancellation.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestCreate_Success verifies that Create returns a new controller.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{Description: "ctrl-1", State: "enabled"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 || out.Description != "ctrl-1" {
		t.Errorf("output mismatch: %+v", out)
	}
}

// TestCreate_Defaults verifies that Create works with empty optional fields.
func TestCreate_Defaults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestCreate_APIError verifies that Create propagates API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{Description: "x"})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestCreate_BadRequest verifies create validation hints.
func TestCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Description: "x"})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "experimental admin-only API") {
		t.Fatalf("error = %v, want admin-only hint", err)
	}
}

// TestCreate_ContextCancelled verifies that Create respects context cancellation.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{Description: "x"})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestUpdate_Success verifies that Update returns the updated controller.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, sampleControllerJSON)
	}))

	out, err := Update(context.Background(), client, UpdateInput{ControllerID: 1, Description: "ctrl-1", State: "enabled"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestUpdate_MissingID verifies that Update rejects missing controller_id.
func TestUpdate_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestUpdate_APIError verifies that Update propagates API errors.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestUpdate_NotFound verifies update not-found hints.
func TestUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "gitlab_runner_controller_list") {
		t.Fatalf("error = %v, want list hint", err)
	}
}

// TestUpdate_ContextCancelled verifies that Update respects context cancellation.
func TestUpdate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestDelete_Success verifies that Delete succeeds.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestDelete_MissingID verifies that Delete rejects missing controller_id.
func TestDelete_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestDelete_APIError verifies that Delete propagates API errors.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestDelete_NotFound verifies already-deleted controller hints.
func TestDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))
	err := Delete(context.Background(), client, DeleteInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "already be deleted") {
		t.Fatalf("error = %v, want deletion hint", err)
	}
}

// TestDelete_ContextCancelled verifies that Delete respects context cancellation.
func TestDelete_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// summaryHints, detailHints and listHints are the guidance sections the three
// controller renderings close with.
const (
	summaryHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_get' to see this controller's full details\n" +
		"- Use action 'runner.controller_token_list' to manage the tokens it authenticates with\n"
	detailHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_scope_list' to see the runners this controller is scoped to\n" +
		"- Use action 'runner.controller_update' to change its description or state\n"
	listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_get' to see one controller in full\n" +
		"- Use action 'runner.controller_list' to page through the rest of the controllers\n"
)

// TestFormatOutputMarkdown pins the whole card of a runner controller, with
// and without the timestamps GitLab may omit.
func TestFormatOutputMarkdown(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID: 1, Description: "ctrl-1", State: "enabled",
		CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z",
	})

	want := "## Runner Controller #1\n\n" +
		"- **ID**: 1\n" +
		"- **Description**: ctrl-1\n" +
		"- **State**: enabled\n" +
		"- **Created**: 15 Jan 2026 10:00 UTC\n" +
		"- **Updated**: 15 Jan 2026 12:00 UTC\n" +
		summaryHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	bare := FormatOutputMarkdown(Output{ID: 1, Description: "ctrl-1", State: "enabled"})

	wantBare := "## Runner Controller #1\n\n" +
		"- **ID**: 1\n" +
		"- **Description**: ctrl-1\n" +
		"- **State**: enabled\n" +
		summaryHints

	if bare != wantBare {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", bare, wantBare)
	}
}

// TestFormatDetailsMarkdown pins the whole detail card: the same fields with
// the connection flag as a glyph rather than as the word "true".
func TestFormatDetailsMarkdown(t *testing.T) {
	got := FormatDetailsMarkdown(DetailsOutput{
		ID: 1, Description: "ctrl-1", State: "enabled",
		CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z",
		Connected: true,
	})

	want := "## Runner Controller #1: Details\n\n" +
		"- **ID**: 1\n" +
		"- **Description**: ctrl-1\n" +
		"- **State**: enabled\n" +
		"- **Created**: 15 Jan 2026 10:00 UTC\n" +
		"- **Updated**: 15 Jan 2026 12:00 UTC\n" +
		"- **Connected**: ✅\n" +
		detailHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	disconnected := FormatDetailsMarkdown(DetailsOutput{
		ID: 2, Description: "ctrl-2", State: "disabled",
	})

	wantDisconnected := "## Runner Controller #2: Details\n\n" +
		"- **ID**: 2\n" +
		"- **Description**: ctrl-2\n" +
		"- **State**: disabled\n" +
		"- **Connected**: ❌\n" +
		detailHints

	if disconnected != wantDisconnected {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", disconnected, wantDisconnected)
	}
}

// TestFormatListMarkdown pins the whole listing: the heading counting what
// GitLab reported, the created column in the display form rather than the wire
// one, and the empty answer as one sentence.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Controllers: []Output{
			{ID: 1, Description: "ctrl-1", State: "enabled", CreatedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, Description: "ctrl-2", State: "disabled"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})

	want := "## Runner Controllers (2)\n\n" +
		"| ID | Description | State | Created |\n| --- | --- | --- | --- |\n" +
		"| 1 | ctrl-1 | enabled | 15 Jan 2026 10:00 UTC |\n" +
		"| 2 | ctrl-2 | disabled |  |\n" +
		"\n2 items total\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	if empty := FormatListMarkdown(ListOutput{}); empty != "No runner controllers found.\n" {
		t.Errorf("empty list mismatch:\ngot:\n%s", empty)
	}
}
