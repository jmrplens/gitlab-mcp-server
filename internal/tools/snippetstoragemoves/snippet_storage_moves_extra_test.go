// Package snippetstoragemoves — additional tests for snippet repository storage
// move handlers and markdown formatters. Covers context cancellation for all
// handlers, API error responses, toOutput edge cases (nil CreatedAt, nil Snippet),
// pagination parameter passthrough, and all three markdown formatters with
// various inputs (empty lists, populated lists, with/without snippet info).
package snippetstoragemoves

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
		w.WriteHeader(http.StatusInternalServerError)
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

	in := ListForSnippetInput{SnippetID: 55}
	in.Page = 2
	in.PerPage = 5
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

	in := ListInput{}
	in.Page = 3
	in.PerPage = 10
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
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
	for _, want := range []string{
		"## Snippet Storage Move #1",
		"| **ID** | 1 |",
		"| **State** | finished |",
		"| **Source** | default |",
		"| **Destination** | storage2 |",
		"| **Created** | 2024-01-15 10:30:00 |",
		"[my-snippet](https://gitlab.example.com/snippets/55)",
		"(ID: 55)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_WithoutSnippet verifies that FormatOutputMarkdown
// renders a table without the Snippet row when snippet data is nil.
func TestFormatOutputMarkdown_WithoutSnippet(t *testing.T) {
	o := Output{
		ID:                     2,
		State:                  "started",
		SourceStorageName:      "default",
		DestinationStorageName: "storage3",
	}

	md := FormatOutputMarkdown(o)
	if !strings.Contains(md, "## Snippet Storage Move #2") {
		t.Errorf("missing header in output:\n%s", md)
	}
	if strings.Contains(md, "| **Snippet**") {
		t.Errorf("should not contain Snippet row when snippet is nil:\n%s", md)
	}
}

// TestFormatListMarkdown_Empty verifies that FormatListMarkdown renders
// a "no moves found" message when the list is empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No snippet storage moves found.") {
		t.Errorf("expected empty message, got:\n%s", md)
	}
}

// TestFormatListMarkdown_WithMoves verifies that FormatListMarkdown renders
// a table with the correct columns and snippet links.
func TestFormatListMarkdown_WithMoves(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
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
	for _, want := range []string{
		"## Snippet Storage Moves",
		"| ID | State | Source | Destination | Snippet | Created |",
		"| 1 | finished | default | storage2 |",
		"[my-snippet](https://gitlab.example.com/snippets/55)",
		"| 2 | started | default | storage3 |",
		"_Page 1, 2 moves shown._",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_NoPagination verifies that FormatListMarkdown omits
// the pagination footer when Page is zero.
func TestFormatListMarkdown_NoPagination(t *testing.T) {
	o := ListOutput{
		Moves: []Output{
			{
				ID:    3,
				State: "scheduled",
			},
		},
	}
	md := FormatListMarkdown(o)
	if strings.Contains(md, "_Page") {
		t.Errorf("should not contain pagination footer when page=0:\n%s", md)
	}
}

// TestFormatScheduleAllMarkdown verifies the schedule-all confirmation message
// rendering.
func TestFormatScheduleAllMarkdown(t *testing.T) {
	o := ScheduleAllOutput{Message: "All snippet repository storage moves have been scheduled"}
	md := FormatScheduleAllMarkdown(o)
	for _, want := range []string{
		"## Schedule All Snippet Storage Moves",
		"All snippet repository storage moves have been scheduled",
		"gitlab_retrieve_all_snippet_storage_moves",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, md)
		}
	}
}
