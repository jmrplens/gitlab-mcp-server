// group_storage_moves_test.go contains unit tests for GitLab group storage
// move operations. Tests use httptest to mock the GitLab Group Storage Moves API.

package groupstoragemoves

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const storageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"group": {
		"id": 10,
		"name": "my-group",
		"web_url": "https://gitlab.example.com/groups/my-group"
	}
}`

// TestRetrieveAll_Success verifies that RetrieveAll returns the expected output when the GitLab API responds successfully.
func TestRetrieveAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/group_repository_storage_moves" {
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
		if r.URL.Path == "/api/v4/group_repository_storage_moves" {
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

// TestRetrieveForGroup_Success verifies that RetrieveForGroup returns the expected output when the GitLab API responds successfully.
func TestRetrieveForGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10/repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{GroupID: 10})
	if err != nil {
		t.Fatalf("RetrieveForGroup() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
	if out.Moves[0].Group != nil && out.Moves[0].Group.ID != 10 {
		t.Errorf("expected group ID 10, got %d", out.Moves[0].Group.ID)
	}
}

// TestRetrieveForGroup_MissingGroupID verifies that RetrieveForGroup returns a validation error when group_id is missing.
func TestRetrieveForGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGet_Success verifies that Get returns the expected output when the GitLab API responds successfully.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/group_repository_storage_moves/1" {
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

// TestGetForGroup_Success verifies that GetForGroup returns the expected output when the GitLab API responds successfully.
func TestGetForGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10/repository_storage_moves/1" {
			testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetForGroup(context.Background(), client, GroupMoveInput{GroupID: 10, ID: 1})
	if err != nil {
		t.Fatalf("GetForGroup() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestGetForGroup_MissingGroupID verifies that GetForGroup returns a validation error when group_id is missing.
func TestGetForGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetForGroup_MissingID verifies that GetForGroup returns a validation error when id is missing.
func TestGetForGroup_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestSchedule_Success verifies that Schedule returns the expected output when the GitLab API responds successfully.
func TestSchedule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/10/repository_storage_moves" {
			testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	dest := "storage2"
	out, err := Schedule(context.Background(), client, ScheduleInput{GroupID: 10, DestinationStorageName: &dest})
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

// TestSchedule_MissingGroupID verifies that Schedule returns a validation error when group_id is missing.
func TestSchedule_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestScheduleAll_Success verifies that ScheduleAll returns the expected output when the GitLab API responds successfully.
func TestScheduleAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/group_repository_storage_moves" {
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

// TestRetrieveAll_Pagination verifies that pagination parameters are forwarded
// and pagination metadata is returned in the output.
func TestRetrieveAll_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/group_repository_storage_moves")
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "5")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+storageMoveJSON+`]`, testutil.PaginationHeaders{
			Page:       "2",
			PerPage:    "5",
			Total:      "8",
			TotalPages: "2",
			NextPage:   "",
			PrevPage:   "1",
		})
	}))

	out, err := RetrieveAll(context.Background(), client, ListInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("RetrieveAll() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("expected total_pages 2, got %d", out.Pagination.TotalPages)
	}
}

// TestRetrieveForGroup_APIError verifies that API errors from the GitLab
// server are propagated when listing storage moves for a group.
func TestRetrieveForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveForGroup_ContextCanceled verifies that RetrieveForGroup returns
// an error when the context is cancelled before the API call.
func TestRetrieveForGroup_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := RetrieveForGroup(ctx, client, ListForGroupInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestRetrieveForGroup_Pagination verifies that pagination parameters are
// forwarded correctly for the group-specific list endpoint.
func TestRetrieveForGroup_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "3")
		testutil.AssertQueryParam(t, r, "per_page", "10")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
			Page:       "3",
			PerPage:    "10",
			Total:      "20",
			TotalPages: "2",
		})
	}))

	out, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{
		GroupID:         10,
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("RetrieveForGroup() error: %v", err)
	}
	if len(out.Moves) != 0 {
		t.Fatalf("expected 0 moves, got %d", len(out.Moves))
	}
	if out.Pagination.TotalItems != 20 {
		t.Errorf("expected total 20, got %d", out.Pagination.TotalItems)
	}
}

// TestGet_APIError verifies that API errors are propagated when getting
// a single storage move.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGet_ContextCanceled verifies that Get returns an error when the
// context is cancelled before the API call.
func TestGet_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestGetForGroup_APIError verifies that API errors are propagated when
// getting a specific storage move for a group.
func TestGetForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{GroupID: 10, ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGetForGroup_ContextCanceled verifies that GetForGroup returns an error
// when the context is cancelled before the API call.
func TestGetForGroup_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetForGroup(ctx, client, GroupMoveInput{GroupID: 10, ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestSchedule_APIError verifies that API errors are propagated when
// scheduling a storage move.
func TestSchedule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestSchedule_ContextCanceled verifies that Schedule returns an error when
// the context is cancelled before the API call.
func TestSchedule_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Schedule(ctx, client, ScheduleInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestSchedule_NilDestination verifies that Schedule works without
// a destination storage name (server picks default).
func TestSchedule_NilDestination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/10/repository_storage_moves")
		testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
	}))

	out, err := Schedule(context.Background(), client, ScheduleInput{GroupID: 10})
	if err != nil {
		t.Fatalf("Schedule() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestScheduleAll_ContextCanceled verifies that ScheduleAll returns an error
// when the context is cancelled before the API call.
func TestScheduleAll_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ScheduleAll(ctx, client, ScheduleAllInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestScheduleAll_NilParams verifies that ScheduleAll works without
// source or destination storage name parameters.
func TestScheduleAll_NilParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/group_repository_storage_moves")
		w.WriteHeader(http.StatusAccepted)
	}))

	out, err := ScheduleAll(context.Background(), client, ScheduleAllInput{})
	if err != nil {
		t.Fatalf("ScheduleAll() error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestToOutput_NilGroup verifies that toOutput handles a storage move
// with no associated group (Group field is nil).
func TestToOutput_NilGroup(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 5,
			"state": "started",
			"source_storage_name": "default",
			"destination_storage_name": "storage2"
		}`)
	}))

	out, err := Get(context.Background(), client, IDInput{ID: 5})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.Group != nil {
		t.Errorf("expected nil group, got %+v", out.Group)
	}
	if out.State != "started" {
		t.Errorf("expected state started, got %s", out.State)
	}
}

// --- Markdown formatter tests ---

func mustParseTime(s string) time.Time {
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return tt
}

// TestFormatOutputMarkdown validates that FormatOutputMarkdown produces
// correct Markdown for moves with and without group data.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		wantAll  []string
		wantNone []string
	}{
		{
			name: "full output with group",
			input: Output{
				ID:                     1,
				State:                  "finished",
				SourceStorageName:      "default",
				DestinationStorageName: "storage2",
				CreatedAt:              mustParseTime("2026-01-15T10:30:00Z"),
				Group: &GroupOutput{
					ID:     10,
					Name:   "my-group",
					WebURL: "https://gitlab.example.com/groups/my-group",
				},
			},
			wantAll: []string{
				"## Group Storage Move #1",
				"| **ID** | 1 |",
				"| **State** | finished |",
				"| **Source** | default |",
				"| **Destination** | storage2 |",
				"2026-01-15",
				"[my-group](https://gitlab.example.com/groups/my-group)",
				"(ID: 10)",
			},
		},
		{
			name: "output without group",
			input: Output{
				ID:                     2,
				State:                  "scheduled",
				SourceStorageName:      "default",
				DestinationStorageName: "storage3",
			},
			wantAll: []string{
				"## Group Storage Move #2",
				"| **State** | scheduled |",
			},
			wantNone: []string{
				"| **Group** |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantNone {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates that FormatListMarkdown produces correct
// Markdown tables for lists with moves, empty lists, and pagination info.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		wantAll  []string
		wantNone []string
	}{
		{
			name: "list with moves and pagination",
			input: ListOutput{
				Moves: []Output{
					{
						ID:                     1,
						State:                  "finished",
						SourceStorageName:      "default",
						DestinationStorageName: "storage2",
						Group: &GroupOutput{
							ID:     10,
							Name:   "my-group",
							WebURL: "https://gitlab.example.com/groups/my-group",
						},
					},
					{
						ID:                     2,
						State:                  "scheduled",
						SourceStorageName:      "default",
						DestinationStorageName: "storage3",
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1},
			},
			wantAll: []string{
				"## Group Storage Moves",
				"| ID | State | Source | Destination | Group | Created |",
				"| 1 | finished | default | storage2 |",
				"[my-group](https://gitlab.example.com/groups/my-group)",
				"| 2 | scheduled | default | storage3 |",
				"_Page 1, 2 moves shown._",
			},
		},
		{
			name: "empty list shows no-moves message",
			input: ListOutput{
				Moves: []Output{},
			},
			wantAll: []string{
				"## Group Storage Moves",
				"No group storage moves found.",
			},
			wantNone: []string{
				"_Page",
				"| ID |",
			},
		},
		{
			name: "list without pagination does not show page line",
			input: ListOutput{
				Moves: []Output{
					{
						ID:                     3,
						State:                  "started",
						SourceStorageName:      "default",
						DestinationStorageName: "storage4",
					},
				},
			},
			wantAll: []string{
				"| 3 | started |",
			},
			wantNone: []string{
				"_Page",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantNone {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatScheduleAllMarkdown validates that FormatScheduleAllMarkdown
// produces correct Markdown with the confirmation message.
func TestFormatScheduleAllMarkdown(t *testing.T) {
	out := ScheduleAllOutput{Message: "All group repository storage moves have been scheduled"}
	got := FormatScheduleAllMarkdown(out)

	wantAll := []string{
		"## Schedule All Group Storage Moves",
		"All group repository storage moves have been scheduled",
	}
	for _, want := range wantAll {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, got)
		}
	}
}
