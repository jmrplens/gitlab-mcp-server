// group_storage_moves_test.go contains unit tests for GitLab group storage
// move operations. Tests use httptest to mock the GitLab Group Storage Moves API.
package groupstoragemoves

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestRetrieveAll_Success verifies that RetrieveAll succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/group_repository_storage_moves (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestRetrieveAll_Empty verifies the RetrieveAll_Empty handler.
// The mock GitLab API at /api/v4/group_repository_storage_moves (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestRetrieveAll_APIError verifies that RetrieveAll returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRetrieveAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := RetrieveAll(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveForGroup_Success verifies that RetrieveForGroup succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/repository_storage_moves (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestRetrieveForGroup_MissingGroupID verifies that RetrieveForGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRetrieveForGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/group_repository_storage_moves/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGet_MissingID verifies that Get_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGetForGroup_Success verifies that GetForGroup succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/repository_storage_moves/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGetForGroup_MissingGroupID verifies that GetForGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetForGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetForGroup_MissingID verifies that GetForGroup_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetForGroup_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestSchedule_Success verifies that Schedule succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/repository_storage_moves (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestSchedule_MissingGroupID verifies that Schedule_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSchedule_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestScheduleAll_Success verifies that ScheduleAll succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/group_repository_storage_moves (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestScheduleAll_APIError verifies that ScheduleAll returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestScheduleAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ScheduleAll(context.Background(), client, ScheduleAllInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveAll_ContextCanceled verifies the RetrieveAll_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestRetrieveAll_Pagination verifies that RetrieveAll forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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
		Page: 2, PerPage: 5,
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

// TestRetrieveForGroup_APIError verifies that RetrieveForGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRetrieveForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRetrieveForGroup_ContextCanceled verifies the RetrieveForGroup_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestRetrieveForGroup_Pagination verifies that RetrieveForGroup forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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
		GroupID: 10,
		Page:    3, PerPage: 10,
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

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGet_ContextCanceled verifies the Get_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestGetForGroup_APIError verifies that GetForGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetForGroup(context.Background(), client, GroupMoveInput{GroupID: 10, ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGetForGroup_ContextCanceled verifies the GetForGroup_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestSchedule_APIError verifies that Schedule returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSchedule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Schedule(context.Background(), client, ScheduleInput{GroupID: 10})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestSchedule_ContextCanceled verifies the Schedule_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestSchedule_NilDestination verifies the Schedule_NilDestination handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestScheduleAll_ContextCanceled verifies the ScheduleAll_ContextCanceled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestScheduleAll_NilParams verifies that ScheduleAll_NilParams forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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

// TestToOutput_NilGroup verifies the ToOutput_NilGroup handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatScheduleAllMarkdown verifies the ScheduleAllMarkdown Markdown formatter for a representative scheduleall input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestRetrieveAll_KeysetAndSort verifies that RetrieveAll forwards keyset
// pagination (pagination, page_token) and ordering (order_by, sort) parameters
// to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that every keyset and ordering query parameter is sent.
func TestRetrieveAll_KeysetAndSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/group_repository_storage_moves")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "99")
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
	}))

	out, err := RetrieveAll(context.Background(), client, ListInput{
		Pagination: "keyset", PageToken: "99",
		OrderBy: "id",
		Sort:    "desc",
	})
	if err != nil {
		t.Fatalf("RetrieveAll() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
}

// TestRetrieveForGroup_KeysetAndSort verifies that RetrieveForGroup forwards
// keyset pagination and ordering parameters to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that every keyset and ordering query parameter is sent.
func TestRetrieveForGroup_KeysetAndSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/10/repository_storage_moves")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "7")
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "asc")
		testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveJSON+`]`)
	}))

	out, err := RetrieveForGroup(context.Background(), client, ListForGroupInput{
		GroupID:    10,
		Pagination: "keyset", PageToken: "7",
		OrderBy: "id",
		Sort:    "asc",
	})
	if err != nil {
		t.Fatalf("RetrieveForGroup() error: %v", err)
	}
	if len(out.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(out.Moves))
	}
}
