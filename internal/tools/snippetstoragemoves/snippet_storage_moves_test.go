// snippet_storage_moves_test.go contains unit tests for GitLab snippet storage
// move operations. Tests use httptest to mock the GitLab Snippet Storage Moves API.
package snippetstoragemoves

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const storageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"snippet": {
		"id": 55,
		"title": "my-snippet",
		"web_url": "https://gitlab.example.com/snippets/55"
	}
}`

// TestRetrieveAll_Success verifies that RetrieveAll returns the expected output when the GitLab API responds successfully.
func TestRetrieveAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/snippet_repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RetrieveAll(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("RetrieveAll() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
	if out.Moves[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Moves[0].ID)
	}
	if out.Moves[0].State != "finished" {
		t.Errorf("expected state finished, got %s", out.Moves[0].State)
	}
}

// TestRetrieveAll_Empty verifies that RetrieveAll handles an empty API response and returns a non-nil empty result.
func TestRetrieveAll_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/snippet_repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RetrieveAll(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("RetrieveAll() error: %v", err)
	}
	if len(out.Moves) != 0 {
		t.Fatalf("expected 0 moves, got %d", len(out.Moves))
	}
}

// TestRetrieveAll_APIError verifies that RetrieveAll returns an error when the GitLab API responds with a failure status.
func TestRetrieveAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := RetrieveAll(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveForSnippet_Success verifies that RetrieveForSnippet returns the expected output when the GitLab API responds successfully.
func TestRetrieveForSnippet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/snippets/55/repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RetrieveForSnippet(context.Background(), client, ListForSnippetInput{SnippetID: 55})
	if err != nil {
		t.Fatalf("RetrieveForSnippet() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
	if out.Moves[0].Snippet != nil && out.Moves[0].Snippet.ID != 55 {
		t.Errorf("expected snippet ID 55, got %d", out.Moves[0].Snippet.ID)
	}
}

// TestRetrieveForSnippet_MissingSnippetID verifies that RetrieveForSnippet returns a validation error when snippet_id is missing.
func TestRetrieveForSnippet_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := RetrieveForSnippet(context.Background(), client, ListForSnippetInput{})
	if err == nil {
		t.Fatal("expected error for missing snippet_id")
	}
}

// TestGet_Success verifies that Get returns the expected output when the GitLab API responds successfully.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/snippet_repository_storage_moves/1" {
			testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.SourceStorageName != "default" {
		t.Errorf("expected source default, got %s", out.SourceStorageName)
	}
	if out.DestinationStorageName != "storage2" {
		t.Errorf("expected destination storage2, got %s", out.DestinationStorageName)
	}
}

// TestGet_MissingID verifies that Get returns a validation error when id is missing.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGetForSnippet_Success verifies that GetForSnippet returns the expected output when the GitLab API responds successfully.
func TestGetForSnippet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/snippets/55/repository_storage_moves/1" {
			testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetForSnippet(context.Background(), client, SnippetMoveInput{SnippetID: 55, ID: 1})
	if err != nil {
		t.Fatalf("GetForSnippet() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestGetForSnippet_MissingSnippetID verifies that GetForSnippet returns a validation error when snippet_id is missing.
func TestGetForSnippet_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForSnippet(context.Background(), client, SnippetMoveInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for missing snippet_id")
	}
}

// TestGetForSnippet_MissingID verifies that GetForSnippet returns a validation error when id is missing.
func TestGetForSnippet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForSnippet(context.Background(), client, SnippetMoveInput{SnippetID: 55})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestSchedule_Success verifies that Schedule returns the expected output when the GitLab API responds successfully.
func TestSchedule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/snippets/55/repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	dest := "storage2"
	out, err := Schedule(context.Background(), client, ScheduleInput{SnippetID: 55, DestinationStorageName: &dest})
	if err != nil {
		t.Fatalf("Schedule() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.DestinationStorageName != "storage2" {
		t.Errorf("expected destination storage2, got %s", out.DestinationStorageName)
	}
}

// TestSchedule_MissingSnippetID verifies that Schedule returns a validation error when snippet_id is missing.
func TestSchedule_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{})
	if err == nil {
		t.Fatal("expected error for missing snippet_id")
	}
}

// TestScheduleAll_Success verifies that ScheduleAll returns the expected output when the GitLab API responds successfully.
func TestScheduleAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/snippet_repository_storage_moves" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	src := "default"
	dest := "storage2"
	out, err := ScheduleAll(context.Background(), client, ScheduleAllInput{SourceStorageName: &src, DestinationStorageName: &dest})
	if err != nil {
		t.Fatalf("ScheduleAll() error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestScheduleAll_APIError verifies that ScheduleAll returns an error when the GitLab API responds with a failure status.
func TestScheduleAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ScheduleAll(context.Background(), client, ScheduleAllInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveAll_ContextCanceled verifies that RetrieveAll returns an error when the context is already cancelled.
func TestRetrieveAll_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := RetrieveAll(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// storageMoveNoSnippetJSON is a storage move response without an embedded snippet.
const storageMoveNoSnippetJSON = `{
	"id": 2,
	"state": "started",
	"source_storage_name": "default",
	"destination_storage_name": "storage3"
}`

// ---------------------------------------------------------------------------
// RetrieveForSnippet — additional tests
// ---------------------------------------------------------------------------

// TestRetrieveForSnippet_APIError verifies that RetrieveForSnippet wraps API
// errors returned by the GitLab client.
func TestRetrieveForSnippet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := RetrieveForSnippet(context.Background(), client, ListForSnippetInput{SnippetID: 55})
	if err == nil {
		t.Fatal("expected error on API failure, got nil")
	}
}

// TestRetrieveForSnippet_ContextCanceled verifies that RetrieveForSnippet
// returns an error when the context is already cancelled.
func TestRetrieveForSnippet_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := RetrieveForSnippet(ctx, client, ListForSnippetInput{SnippetID: 55})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestRetrieveForSnippet_Pagination verifies that pagination parameters are
// forwarded to the GitLab API.
func TestRetrieveForSnippet_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "5")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+storageMoveJSON+`]`, testutil.PaginationHeaders{
			Page:       "2",
			PerPage:    "5",
			Total:      "10",
			TotalPages: "2",
		})
	}))

	in := ListForSnippetInput{
		SnippetID: 55,
		Page:      2,
		PerPage:   5,
	}
	out, err := RetrieveForSnippet(context.Background(), client, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("pagination page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("pagination total pages = %d, want 2", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// RetrieveAll — pagination test
// ---------------------------------------------------------------------------

// TestRetrieveAll_Pagination verifies that pagination parameters are forwarded
// to the GitLab API and parsed from response headers.
func TestRetrieveAll_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertQueryParam(t, r, "page", "3")
		testutil.AssertQueryParam(t, r, "per_page", "10")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
			Page:       "3",
			PerPage:    "10",
			Total:      "25",
			TotalPages: "3",
		})
	}))

	in := ListInput{
		Page:    3,
		PerPage: 10,
	}
	out, err := RetrieveAll(context.Background(), client, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("pagination page = %d, want 3", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 25 {
		t.Errorf("pagination total items = %d, want 25", out.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// Get — additional tests
// ---------------------------------------------------------------------------

// TestGet_APIError verifies that Get wraps API errors returned by the GitLab
// client for a non-existent storage move ID.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error on API 404, got nil")
	}
}

// TestGet_ContextCanceled verifies that Get returns an error when the context
// is already cancelled.
func TestGet_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetForSnippet — additional tests
// ---------------------------------------------------------------------------

// TestGetForSnippet_APIError verifies that GetForSnippet wraps API errors.
func TestGetForSnippet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetForSnippet(context.Background(), client, SnippetMoveInput{SnippetID: 55, ID: 999})
	if err == nil {
		t.Fatal("expected error on API 404, got nil")
	}
}

// TestGetForSnippet_ContextCanceled verifies that GetForSnippet returns an
// error when the context is already cancelled.
func TestGetForSnippet_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetForSnippet(ctx, client, SnippetMoveInput{SnippetID: 55, ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// Schedule — additional tests
// ---------------------------------------------------------------------------

// TestSchedule_APIError verifies that Schedule wraps API errors.
func TestSchedule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{SnippetID: 55})
	if err == nil {
		t.Fatal("expected error on API 403, got nil")
	}
}

// TestSchedule_ContextCanceled verifies that Schedule returns an error when
// the context is already cancelled.
func TestSchedule_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Schedule(ctx, client, ScheduleInput{SnippetID: 55})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestSchedule_WithoutDestination verifies that Schedule works when
// DestinationStorageName is nil (GitLab will auto-select).
func TestSchedule_WithoutDestination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/snippets/77/repository_storage_moves")
		testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
	}))

	out, err := Schedule(context.Background(), client, ScheduleInput{SnippetID: 77})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// ---------------------------------------------------------------------------
// ScheduleAll — additional tests
// ---------------------------------------------------------------------------

// TestScheduleAll_ContextCanceled verifies that ScheduleAll returns an error
// when the context is already cancelled.
func TestScheduleAll_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ScheduleAll(ctx, client, ScheduleAllInput{})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestScheduleAll_WithoutOptions verifies that ScheduleAll works when both
// SourceStorageName and DestinationStorageName are nil.
func TestScheduleAll_WithoutOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/snippet_repository_storage_moves")
		w.WriteHeader(http.StatusAccepted)
	}))

	out, err := ScheduleAll(context.Background(), client, ScheduleAllInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// ---------------------------------------------------------------------------
// toOutput edge cases
// ---------------------------------------------------------------------------

// TestToOutput_NilSnippet verifies that toOutput correctly handles a response
// with no embedded snippet (Snippet is nil in the output).
func TestToOutput_NilSnippet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storageMoveNoSnippetJSON)
	}))

	out, err := Get(context.Background(), client, IDInput{ID: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Snippet != nil {
		t.Error("expected nil Snippet for response without snippet field")
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.State != "started" {
		t.Errorf("State = %q, want %q", out.State, "started")
	}
	if !out.CreatedAt.IsZero() {
		t.Error("expected zero CreatedAt for response without created_at")
	}
}

// TestRetrieveAll_MultipleMovesWithMixedSnippets verifies that toOutput
// handles a mix of moves with and without snippet data.
func TestRetrieveAll_MultipleMovesWithMixedSnippets(t *testing.T) {
	body := `[` + storageMoveJSON + `,` + storageMoveNoSnippetJSON + `]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))

	out, err := RetrieveAll(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Moves) != 2 {
		t.Fatalf("got %d moves, want 2", len(out.Moves))
	}
	if out.Moves[0].Snippet == nil {
		t.Error("first move should have snippet data")
	}
	if out.Moves[1].Snippet != nil {
		t.Error("second move should have nil snippet")
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------

// TestFormatOutputMarkdown_WithSnippet verifies that FormatOutputMarkdown
// renders a complete table with snippet link when snippet data is present.
func TestFormatOutputMarkdown_WithSnippet(t *testing.T) {
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	o := Output{
		ID:                     1,
		State:                  "finished",
		SourceStorageName:      "default",
		DestinationStorageName: "storage2",
		CreatedAt:              ts,
		Snippet: &SnippetOutput{
			ID:     55,
			Title:  "my-snippet",
			WebURL: "https://gitlab.example.com/snippets/55",
		},
	}

	md := FormatOutputMarkdown(o)
	want := "## Snippet Storage Move #1\n\n" +
		"- **ID**: 1\n" +
		"- **State**: finished\n" +
		"- **Source**: default\n" +
		"- **Destination**: storage2\n" +
		"- **Created**: 15 Jan 2026 10:30 UTC\n" +
		"- **Snippet**: [my-snippet](https://gitlab.example.com/snippets/55) (ID: 55)\n" +
		snippetMoveCardHints
	if md != want {
		t.Errorf("storage move card:\n got %q\nwant %q", md, want)
	}
}

// snippetMoveCardHints is the guidance section every snippet storage move
// card ends with, the same two actions its project and group siblings name.
const snippetMoveCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'storage_move.retrieve_all_snippet' to see every snippet storage move on the instance\n" +
	"- Use action 'storage_move.schedule_snippet' to schedule another move for this snippet\n"

// TestFormatOutputMarkdown_WithoutSnippet verifies that FormatOutputMarkdown
// renders the card without the Snippet row when snippet data is nil.
func TestFormatOutputMarkdown_WithoutSnippet(t *testing.T) {
	o := Output{
		ID:                     2,
		State:                  "started",
		SourceStorageName:      "default",
		DestinationStorageName: "storage3",
	}

	md := FormatOutputMarkdown(o)
	want := "## Snippet Storage Move #2\n\n" +
		"- **ID**: 2\n" +
		"- **State**: started\n" +
		"- **Source**: default\n" +
		"- **Destination**: storage3\n" +
		snippetMoveCardHints
	if md != want {
		t.Errorf("storage move card:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdown_Empty verifies that an empty page renders the one
// sentence and nothing else: no heading counting zero, no table header.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if want := "No snippet storage moves found.\n"; md != want {
		t.Errorf("empty list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdown_WithMoves verifies that FormatListMarkdown renders
// a table with the correct columns and snippet links.
func TestFormatListMarkdown_WithMoves(t *testing.T) {
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	o := ListOutput{
		Moves: []Output{
			{
				ID:                     1,
				State:                  "finished",
				SourceStorageName:      "default",
				DestinationStorageName: "storage2",
				CreatedAt:              ts,
				Snippet: &SnippetOutput{
					ID:     55,
					Title:  "my-snippet",
					WebURL: "https://gitlab.example.com/snippets/55",
				},
			},
			{
				ID:                     2,
				State:                  "started",
				SourceStorageName:      "default",
				DestinationStorageName: "storage3",
				CreatedAt:              ts,
			},
		},
		Pagination: toolutil.PaginationOutput{
			Page: 1,
		},
	}

	md := FormatListMarkdown(o)
	want := "## Snippet Storage Moves (2)\n\n" +
		"| ID | State | Source | Destination | Snippet | Created |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | finished | default | storage2 | [my-snippet](https://gitlab.example.com/snippets/55) | 1 Jun 2026 12:00 UTC |\n" +
		"| 2 | started | default | storage3 |  | 1 Jun 2026 12:00 UTC |\n" +
		"\nPage 1 | no more pages\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- " + toolutil.HintPreserveLinks + "\n"
	if md != want {
		t.Errorf("storage move list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdown_NoPagination verifies the whole table a page with no
// pagination metadata renders: no footer line, and no link hint over a table
// whose only linkable column is empty.
func TestFormatListMarkdown_NoPagination(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Moves: []Output{{ID: 3, State: "scheduled"}},
	})
	want := "## Snippet Storage Moves (1)\n\n" +
		"| ID | State | Source | Destination | Snippet | Created |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 3 | scheduled |  |  |  |  |\n"
	if md != want {
		t.Errorf("storage move list:\n got %q\nwant %q", md, want)
	}
}

// fullSnippetMoveJSON is a storage move whose embedded snippet populates every
// field of gl.RepositorySnippet so the 1:1 field mapping in toOutput is exercised.
const fullSnippetMoveJSON = `{
	"id": 7,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"snippet": {
		"id": 55,
		"title": "my-snippet",
		"description": "a snippet",
		"visibility": "private",
		"updated_at": "2026-02-01T08:00:00Z",
		"created_at": "2026-01-01T08:00:00Z",
		"project_id": 12,
		"web_url": "https://gitlab.example.com/snippets/55",
		"raw_url": "https://gitlab.example.com/snippets/55/raw",
		"ssh_url_to_repo": "git@gitlab.example.com:snippets/55.git",
		"http_url_to_repo": "https://gitlab.example.com/snippets/55.git"
	}
}`

// TestToOutput_FullSnippetFields_EachValueComesFromItsOwnKey verifies that
// toOutput maps every field of the embedded gl.RepositorySnippet onto the
// SnippetOutput field of the same name (1:1 audit), by holding each to the one
// distinct value the fixture gives it.
//
// The four repository URLs and the two timestamps are the reason this is exact
// rather than a non-empty check: they are same-typed neighbors in the same
// literal, so any permutation among them still leaves every one populated and
// every mutant alive. Both gates score branches and a straight-line assignment
// has none, so nothing else here can see a swap.
func TestToOutput_FullSnippetFields_EachValueComesFromItsOwnKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fullSnippetMoveJSON)
	}))

	out, err := Get(context.Background(), client, IDInput{ID: 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Snippet
	if s == nil {
		t.Fatal("expected snippet, got nil")
	}
	strFields := []struct{ key, got, want string }{
		{"title", s.Title, "my-snippet"},
		{"description", s.Description, "a snippet"},
		{"visibility", s.Visibility, "private"},
		{"web_url", s.WebURL, "https://gitlab.example.com/snippets/55"},
		{"raw_url", s.RawURL, "https://gitlab.example.com/snippets/55/raw"},
		{"ssh_url_to_repo", s.SSHURLToRepo, "git@gitlab.example.com:snippets/55.git"},
		{"http_url_to_repo", s.HTTPURLToRepo, "https://gitlab.example.com/snippets/55.git"},
	}
	for _, f := range strFields {
		t.Run(f.key, func(t *testing.T) {
			if f.got != f.want {
				t.Errorf("%s = %q, want %q", f.key, f.got, f.want)
			}
		})
	}
	if s.ID != 55 {
		t.Errorf("ID = %d, want 55", s.ID)
	}
	if s.ProjectID != 12 {
		t.Errorf("ProjectID = %d, want 12", s.ProjectID)
	}
	wantUpdated := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)
	if s.UpdatedAt == nil || !s.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v", s.UpdatedAt, wantUpdated)
	}
	wantCreated := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	if s.CreatedAt == nil || !s.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt = %v, want %v", s.CreatedAt, wantCreated)
	}
}

// TestRetrieveAll_OrderingAndKeyset verifies that order_by, sort, and keyset
// pagination parameters are forwarded to the GitLab API.
func TestRetrieveAll_OrderingAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "100")
		testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
	}))

	in := ListInput{
		OrderBy: "id", Sort: "desc",
		Pagination: "keyset",
		PageToken:  "100",
	}
	out, err := RetrieveAll(context.Background(), client, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
}

// TestRetrieveForSnippet_OrderingAndKeyset verifies that order_by, sort, and
// keyset pagination parameters are forwarded for the per-snippet listing.
func TestRetrieveForSnippet_OrderingAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "asc")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "42")
		testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
	}))

	in := ListForSnippetInput{
		SnippetID: 55, OrderBy: "id", Sort: "asc",
		Pagination: "keyset",
		PageToken:  "42",
	}
	out, err := RetrieveForSnippet(context.Background(), client, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
}

// TestSnippetStorageMoves_UnreadableCapturedErrorMessage verifies that every
// snippet storage move handler returns an error rather than a half-filled move
// when GitLab sends error_message as something that is not a string. The SDK
// ignores the key its own SnippetRepositoryStorageMove does not model, so the
// read of the captured response is the only thing that can notice, and a failed
// move published without its message reads as one that failed for no reason.
func TestSnippetStorageMoves_UnreadableCapturedErrorMessage(t *testing.T) {
	// A retrieve answers with an array and the rest with an object, so each
	// case drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "retrieve_all", Call: func() error {
			client := poisoned(`[{"id":1,"state":"failed","error_message":42}]`)
			_, err := RetrieveAll(context.Background(), client, ListInput{})
			return err
		}},
		{Name: "retrieve_for_snippet", Call: func() error {
			client := poisoned(`[{"id":1,"state":"failed","error_message":42}]`)
			_, err := RetrieveForSnippet(context.Background(), client, ListForSnippetInput{SnippetID: 3})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"state":"failed","error_message":42}`)
			_, err := Get(context.Background(), client, IDInput{ID: 1})
			return err
		}},
		{Name: "get_for_snippet", Call: func() error {
			client := poisoned(`{"id":1,"state":"failed","error_message":42}`)
			_, err := GetForSnippet(context.Background(), client, SnippetMoveInput{SnippetID: 3, ID: 1})
			return err
		}},
		{Name: "schedule", Call: func() error {
			client := poisoned(`{"id":1,"state":"scheduled","error_message":42}`)
			_, err := Schedule(context.Background(), client, ScheduleInput{SnippetID: 3})
			return err
		}},
	})
}

// ---------------------------------------------------------------------------
// The captured error message, and what a schedule actually sends
// ---------------------------------------------------------------------------

// failedSnippetMovesJSON is a page of two failed moves whose error messages
// differ, so a handler that read one move's message onto another shows.
const failedSnippetMovesJSON = `[
	{"id": 11, "state": "failed", "source_storage_name": "default", "destination_storage_name": "storage2", "error_message": "destination storage is full"},
	{"id": 12, "state": "failed", "source_storage_name": "default", "destination_storage_name": "storage3", "error_message": "source shard is unreachable"}
]`

// TestSnippetStorageMoves_ErrorMessage_IsCarriedPerMoveOnEveryListing asserts
// that each move on a page reaches the caller with the error_message GitLab
// sent for that move.
//
// client-go's SnippetRepositoryStorageMove does not model the field, so the
// captured response is the only source and the extras are matched to the moves
// by position (ADR-0021). Nothing asserted either half: dropping the
// assignment, or reading extras[0] for every move, left the whole suite green,
// and a failed move published without its message reads as one that failed for
// no reason.
func TestSnippetStorageMoves_ErrorMessage_IsCarriedPerMoveOnEveryListing(t *testing.T) {
	cases := []struct {
		name string
		call func(*gitlabclient.Client) (ListOutput, error)
	}{
		{"retrieve_all", func(c *gitlabclient.Client) (ListOutput, error) {
			return RetrieveAll(context.Background(), c, ListInput{})
		}},
		{"retrieve_for_snippet", func(c *gitlabclient.Client) (ListOutput, error) {
			return RetrieveForSnippet(context.Background(), c, ListForSnippetInput{SnippetID: 55})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, failedSnippetMovesJSON)
			}))

			out, err := tc.call(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out.Moves) != 2 {
				t.Fatalf("got %d moves, want 2", len(out.Moves))
			}
			if out.Moves[0].ErrorMessage != "destination storage is full" {
				t.Errorf("Moves[0].ErrorMessage = %q, want why the first move failed", out.Moves[0].ErrorMessage)
			}
			if out.Moves[1].ErrorMessage != "source shard is unreachable" {
				t.Errorf("Moves[1].ErrorMessage = %q, want why the second move failed", out.Moves[1].ErrorMessage)
			}
		})
	}
}

// TestSnippetStorageMoves_ErrorMessage_IsCarriedOnEverySingleMoveRead asserts
// the same for the three handlers that answer with one move, so a failure
// reason is not lost on the call a caller makes to find out why.
func TestSnippetStorageMoves_ErrorMessage_IsCarriedOnEverySingleMoveRead(t *testing.T) {
	const failed = `{"id": 11, "state": "failed", "source_storage_name": "default", "destination_storage_name": "storage2", "error_message": "destination storage is full"}`
	cases := []struct {
		name string
		call func(*gitlabclient.Client) (Output, error)
	}{
		{"get", func(c *gitlabclient.Client) (Output, error) {
			return Get(context.Background(), c, IDInput{ID: 11})
		}},
		{"get_for_snippet", func(c *gitlabclient.Client) (Output, error) {
			return GetForSnippet(context.Background(), c, SnippetMoveInput{SnippetID: 55, ID: 11})
		}},
		{"schedule", func(c *gitlabclient.Client) (Output, error) {
			return Schedule(context.Background(), c, ScheduleInput{SnippetID: 55})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, failed)
			}))

			out, err := tc.call(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.ErrorMessage != "destination storage is full" {
				t.Errorf("ErrorMessage = %q, want why the move failed", out.ErrorMessage)
			}
		})
	}
}

// decodeSnippetMoveBody decodes a POST body into a map so a test can state
// which keys a handler sent. It reports rather than aborts, since it runs on
// the httptest server's goroutine.
func decodeSnippetMoveBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return nil
	}
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Errorf("decode request body %q: %v", raw, err)
		return nil
	}
	return body
}

// TestSchedule_DestinationShard_ReachesTheBodyOnlyWhenNamed asserts that the
// destination shard a caller names is the one GitLab is asked for, and that a
// caller naming none sends no key at all.
//
// Only the path and the method were asserted, so dropping the field from the
// options left the suite green while GitLab picked a shard by weight instead of
// the one the operator chose: a repository lands somewhere nobody asked for and
// the response looks like a success.
func TestSchedule_DestinationShard_ReachesTheBodyOnlyWhenNamed(t *testing.T) {
	dest := "storage9"
	cases := []struct {
		name  string
		input ScheduleInput
		want  map[string]any
	}{
		{"named", ScheduleInput{SnippetID: 77, DestinationStorageName: &dest}, map[string]any{"destination_storage_name": "storage9"}},
		{"omitted", ScheduleInput{SnippetID: 77}, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent = decodeSnippetMoveBody(t, r)
				testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
			}))

			if _, err := Schedule(context.Background(), client, tc.input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(sent, tc.want) {
				t.Errorf("request body = %v, want %v", sent, tc.want)
			}
		})
	}
}

// TestScheduleAll_EachShard_ReachesTheBodyUnderItsOwnKey asserts that the
// source and destination shards of a bulk move arrive under the keys GitLab
// reads them from, and that a call naming neither sends neither.
//
// The two are same-typed neighbors assigned one after the other, so a swap is
// a straight-line defect no mutation or condition gate can see: it would drain
// the shard the operator was migrating onto, back onto the one being
// evacuated, for every snippet on the instance.
func TestScheduleAll_EachShard_ReachesTheBodyUnderItsOwnKey(t *testing.T) {
	src, dest := "old-shard", "new-shard"
	cases := []struct {
		name  string
		input ScheduleAllInput
		want  map[string]any
	}{
		{
			name:  "both named",
			input: ScheduleAllInput{SourceStorageName: &src, DestinationStorageName: &dest},
			want:  map[string]any{"source_storage_name": "old-shard", "destination_storage_name": "new-shard"},
		},
		{"neither named", ScheduleAllInput{}, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent = decodeSnippetMoveBody(t, r)
				w.WriteHeader(http.StatusAccepted)
			}))

			if _, err := ScheduleAll(context.Background(), client, tc.input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(sent, tc.want) {
				t.Errorf("request body = %v, want %v", sent, tc.want)
			}
		})
	}
}

// TestSnippetStorageMoves_StatusHint_IsOfferedOnlyAtTheStatusItExplains
// verifies, for every handler here, that its corrective hint reaches the caller
// at the HTTP status that handler classifies, and at no other status.
//
// These six endpoints are admin, self-managed surfaces that refuse with a bare
// 403, 404 or 400 and no body, so the hint is the only thing that tells a model
// whether it lacked admin, named an id that does not exist, or asked for a
// shard the instance has not got. The status each hint is written for was
// asserted nowhere, and the error tests answer 403 to handlers that classify
// 404 and 400, so a hint attached to a status GitLab never sends for that call
// would have failed nothing. The negative half keeps the hint from being handed
// out for an unrelated failure.
func TestSnippetStorageMoves_StatusHint_IsOfferedOnlyAtTheStatusItExplains(t *testing.T) {
	cases := []struct {
		name   string
		status int
		hint   string
		call   func(*gitlabclient.Client) error
	}{
		{
			name:   "retrieve_all",
			status: http.StatusForbidden,
			hint:   "requires administrator access; self-managed only; storage moves are repository shard migrations between Gitaly nodes",
			call: func(c *gitlabclient.Client) error {
				_, err := RetrieveAll(context.Background(), c, ListInput{})
				return err
			},
		},
		{
			name:   "retrieve_for_snippet",
			status: http.StatusNotFound,
			hint:   "requires admin; verify snippet_id exists; only storage moves for the given snippet are returned",
			call: func(c *gitlabclient.Client) error {
				_, err := RetrieveForSnippet(context.Background(), c, ListForSnippetInput{SnippetID: 55})
				return err
			},
		},
		{
			name:   "get",
			status: http.StatusNotFound,
			hint:   "requires admin; verify id with storage_move.retrieve_all_snippet; the move record may have been pruned after completion",
			call: func(c *gitlabclient.Client) error {
				_, err := Get(context.Background(), c, IDInput{ID: 1})
				return err
			},
		},
		{
			name:   "get_for_snippet",
			status: http.StatusNotFound,
			hint:   "requires admin; verify the snippet_id + id pair with storage_move.retrieve_snippet, which lists this snippet's own moves; a move id belonging to another snippet answers 404 here, and the record may have been pruned after completion",
			call: func(c *gitlabclient.Client) error {
				_, err := GetForSnippet(context.Background(), c, SnippetMoveInput{SnippetID: 55, ID: 1})
				return err
			},
		},
		{
			name:   "schedule",
			status: http.StatusBadRequest,
			hint:   "requires admin; destination_storage_name must reference an existing Gitaly storage shard configured on the instance; cannot move to the same shard the snippet is already on",
			call: func(c *gitlabclient.Client) error {
				_, err := Schedule(context.Background(), c, ScheduleInput{SnippetID: 55})
				return err
			},
		},
		{
			name:   "schedule_all",
			status: http.StatusBadRequest,
			hint:   "requires admin; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation. May schedule many concurrent moves",
			call: func(c *gitlabclient.Client) error {
				_, err := ScheduleAll(context.Background(), c, ScheduleAllInput{})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refusing := func(status int) *gitlabclient.Client {
				return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
				}))
			}

			err := tc.call(refusing(tc.status))
			if err == nil {
				t.Fatalf("expected an error for status %d", tc.status)
			}
			if !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("status %d does not reach the caller with this action's guidance\n got: %v\nwant it to contain: %q", tc.status, err, tc.hint)
			}

			err = tc.call(refusing(http.StatusInternalServerError))
			if err == nil {
				t.Fatal("expected an error for status 500")
			}
			if strings.Contains(err.Error(), tc.hint) {
				t.Errorf("the status-%d guidance is offered for a 500 too, which advises about a refusal that did not happen: %v", tc.status, err)
			}
		})
	}
}

// TestFormatScheduleAllMarkdown verifies the whole card the bulk schedule
// confirmation writes: the message reaches the reader through a card row, so a
// value carrying markup renders as the text it is rather than as structure.
func TestFormatScheduleAllMarkdown(t *testing.T) {
	md := FormatScheduleAllMarkdown(ScheduleAllOutput{Message: "All snippet repository storage moves have been scheduled"})
	want := "## Schedule All Snippet Storage Moves\n\n" +
		"- **Result**: All snippet repository storage moves have been scheduled\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'storage_move.retrieve_all_snippet' to watch the scheduled moves progress\n"
	if md != want {
		t.Errorf("schedule-all card:\n got %q\nwant %q", md, want)
	}
}
