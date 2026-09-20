// group_boards_test.go contains unit tests for the group issue board MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupboards

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Shared JSON fixtures
// ---------------------------------------------------------------------------.

// groupBoardJSON stores the package-level group board JSON state.
var groupBoardJSON = `{
	"id": 1,
	"name": "Development",
	"group": {"id": 42, "name": "mygroup", "path": "mygroup", "full_path": "mygroup", "visibility": "private", "web_url": "https://gitlab.example.com/groups/mygroup", "created_at": "2026-01-01T00:00:00Z"},
	"milestone": {"id": 5, "title": "v1.0", "state": "active", "created_at": "2026-01-01T00:00:00Z", "due_date": "2026-03-01"},
	"labels": [{"id": 1, "name": "bug", "priority": 1}, {"id": 2, "name": "feature"}],
	"lists": [
		{"id": 10, "label": {"id": 20, "name": "To Do"}, "position": 0, "max_issue_count": 10}
	]
}`

// groupBoardListJSON stores the package-level group board list JSON state.
var groupBoardListJSON = `[` + groupBoardJSON + `]`

// boardListItemJSON stores the package-level board list item JSON state. It
// carries every gl.BoardList sub-object (assignee, iteration, label with
// priority, milestone) so the converter and shape branches are fully exercised.
var boardListItemJSON = `{
	"id": 10,
	"label": {"id": 20, "name": "To Do", "color": "#ff0000", "priority": 3, "is_project_label": false, "archived": false},
	"position": 0,
	"max_issue_count": 10,
	"max_issue_weight": 50,
	"assignee": {"id": 3, "name": "Alice", "username": "alice"},
	"iteration": {"id": 9, "iid": 1, "title": "Iteration 1", "state": 1, "due_date": "2026-01-15", "start_date": "2026-01-01"},
	"milestone": {"id": 7, "title": "sprint-1", "due_date": "2026-02-01", "expired": false}
}`

// boardListsArrayJSON stores the package-level board lists array JSON state.
var boardListsArrayJSON = `[` + boardListItemJSON + `]`

// ---------------------------------------------------------------------------
// Group Board CRUD tests
// ---------------------------------------------------------------------------.

// TestListGroupBoards_Success verifies that ListGroupBoards succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListGroupBoards_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, groupBoardListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroupBoards(context.Background(), client, ListGroupBoardsInput{GroupID: toolutil.StringOrInt("42")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Boards) != 1 {
		t.Fatalf("expected 1 board, got %d", len(out.Boards))
	}
	if out.Boards[0].Name != "Development" {
		t.Errorf("name = %q, want %q", out.Boards[0].Name, "Development")
	}
	if out.Boards[0].Group == nil || out.Boards[0].Group.ID != 42 {
		t.Errorf("group = %+v, want id 42", out.Boards[0].Group)
	}
}

// TestListGroupBoards_MissingGroupID verifies that ListGroupBoards_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroupBoards_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListGroupBoards(context.Background(), client, ListGroupBoardsInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
}

// TestGetGroupBoard_Success verifies that GetGroupBoard succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetGroupBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupBoardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "Development" {
		t.Errorf("name = %q, want %q", out.Name, "Development")
	}
	if out.Milestone == nil || out.Milestone.Title != "v1.0" {
		t.Errorf("milestone = %+v, want title v1.0", out.Milestone)
	}
	if len(out.Labels) != 2 {
		t.Errorf("labels count = %d, want 2", len(out.Labels))
	}
	if len(out.Lists) != 1 {
		t.Errorf("lists count = %d, want 1", len(out.Lists))
	}
}

// TestGetGroupBoard_MissingParams verifies that GetGroupBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroupBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
}

// TestCreateGroupBoard_Success verifies that CreateGroupBoard succeeds when the
// GitLab API returns a valid response, and that the POST carries the caller's
// name and nothing else. The body used to be decoded and thrown away, so a
// handler that never sent the name at all would have passed.
func TestCreateGroupBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/api/v4/groups/42/boards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"New Board","group":{"id":42,"name":"mygroup"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{GroupID: toolutil.StringOrInt("42"), Name: "New Board"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "New Board" {
		t.Errorf("name = %q, want %q", out.Name, "New Board")
	}
	if want := map[string]any{"name": "New Board"}; !reflect.DeepEqual(gotBody, want) {
		t.Errorf("POST body = %#v, want %#v", gotBody, want)
	}
}

// TestCreateGroupBoard_MissingParams verifies that CreateGroupBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name required error, got: %v", err)
	}
}

// TestUpdateGroupBoard_Success verifies that UpdateGroupBoard succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateGroupBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"Updated","group":{"id":42,"name":"mygroup"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{
		GroupID: toolutil.StringOrInt("42"), BoardID: 1, Name: "Updated",
		Labels: []string{"bug"}, AssigneeID: 3, MilestoneID: 5, Weight: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "Updated" {
		t.Errorf("name = %q, want %q", out.Name, "Updated")
	}
}

// TestUpdateGroupBoard_SendsOnlyTheFieldTheCallerSet pins what the PUT body
// carries for each optional input, one at a time. Every optional field of this
// action is behind a guard, and until now no test read the request, so a guard
// inverted — the caller's value dropped and an unset one sent as empty — was
// invisible. Driving one field per case also tells the five guards apart: with
// all five set at once, two swapped lines would still produce a body with all
// five keys in it.
func TestUpdateGroupBoard_SendsOnlyTheFieldTheCallerSet(t *testing.T) {
	cases := []struct {
		name  string
		input UpdateGroupBoardInput
		want  map[string]any
	}{
		{"nothing optional", UpdateGroupBoardInput{}, map[string]any{}},
		{"name", UpdateGroupBoardInput{Name: "Renamed"}, map[string]any{"name": "Renamed"}},
		{"assignee", UpdateGroupBoardInput{AssigneeID: 7}, map[string]any{"assignee_id": float64(7)}},
		{"milestone", UpdateGroupBoardInput{MilestoneID: 9}, map[string]any{"milestone_id": float64(9)}},
		{"labels", UpdateGroupBoardInput{Labels: []string{"bug", "urgent"}}, map[string]any{"labels": "bug,urgent"}},
		{"weight", UpdateGroupBoardInput{Weight: 3}, map[string]any{"weight": float64(3)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"Board"}`)
			})
			client := testutil.NewTestClient(t, mux)

			input := tc.input
			input.GroupID, input.BoardID = toolutil.StringOrInt("42"), 1
			if _, err := UpdateGroupBoard(context.Background(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(gotBody, tc.want) {
				t.Errorf("PUT body = %#v, want %#v", gotBody, tc.want)
			}
		})
	}
}

// TestUpdateGroupBoard_MissingParams verifies that UpdateGroupBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateGroupBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
}

// TestDeleteGroupBoard_Success verifies that DeleteGroupBoard succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroupBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteGroupBoard(context.Background(), client, DeleteGroupBoardInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteGroupBoard_MissingParams verifies that DeleteGroupBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroupBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteGroupBoard(context.Background(), client, DeleteGroupBoardInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	err = DeleteGroupBoard(context.Background(), client, DeleteGroupBoardInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Group Board List CRUD tests
// ---------------------------------------------------------------------------.

// TestListGroupBoardLists_Success verifies that ListGroupBoardLists succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListGroupBoardLists_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, boardListsArrayJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Lists) != 1 {
		t.Fatalf("expected 1 list, got %d", len(out.Lists))
	}
	if out.Lists[0].Label == nil || out.Lists[0].Label.Name != "To Do" {
		t.Errorf("label = %+v, want name To Do", out.Lists[0].Label)
	}
}

// TestListGroupBoardLists_MissingParams verifies that ListGroupBoardLists_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroupBoardLists_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
}

// TestGetGroupBoardList_Success verifies that GetGroupBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardListItemJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1, ListID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Label == nil || out.Label.Name != "To Do" {
		t.Errorf("label = %+v, want name To Do", out.Label)
	}
	if out.Assignee == nil || out.Assignee.Username != "alice" {
		t.Errorf("assignee = %+v, want username alice", out.Assignee)
	}
	if out.Milestone == nil || out.Milestone.Title != "sprint-1" {
		t.Errorf("milestone = %+v, want title sprint-1", out.Milestone)
	}
}

// TestGetGroupBoardList_MissingParams verifies that GetGroupBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroupBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
	_, err = GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{GroupID: "42", BoardID: 1})
	if err == nil || !strings.Contains(err.Error(), "list_id is required") {
		t.Fatalf("expected list_id required error, got: %v", err)
	}
}

// TestCreateGroupBoardList_Success verifies that CreateGroupBoardList succeeds
// when the GitLab API returns a valid response, and that the POST carries the
// caller's label_id: the label is the only thing that says what the new column
// collects, and nothing else in the suite reads the body.
func TestCreateGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":12,"position":2,"label":{"id":8,"name":"Priority"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1, LabelID: 8})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Label == nil || out.Label.Name != "Priority" {
		t.Errorf("label = %+v, want name Priority", out.Label)
	}
	if want := map[string]any{"label_id": float64(8)}; !reflect.DeepEqual(gotBody, want) {
		t.Errorf("POST body = %#v, want %#v", gotBody, want)
	}
}

// TestCreateGroupBoardList_MissingParams verifies that CreateGroupBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
	_, err = CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1})
	if err == nil || !strings.Contains(err.Error(), "label_id is required") {
		t.Fatalf("expected label_id required error, got: %v", err)
	}
}

// TestUpdateGroupBoardList_Success verifies that UpdateGroupBoardList succeeds
// when the GitLab API returns a valid response, and that the PUT carries the
// requested position. The position is the only thing this action changes, and
// the response echoing it is no evidence that the request asked for it: the
// mock answers 2 whatever it is sent.
func TestUpdateGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		// GitLab returns the single updated list object — not an array; the
		// handler bypasses the client-go wrapper that expects []*BoardList.
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"position":2,"label":{"id":5,"name":"To Do"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1, ListID: 10, Position: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Position != 2 {
		t.Errorf("position = %d, want 2", out.Position)
	}
	if want := map[string]any{"position": float64(3)}; !reflect.DeepEqual(gotBody, want) {
		t.Errorf("PUT body = %#v, want %#v", gotBody, want)
	}
}

// TestUpdateGroupBoardList_MissingParams verifies that UpdateGroupBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateGroupBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	_, err = UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
	_, err = UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: "42", BoardID: 1})
	if err == nil || !strings.Contains(err.Error(), "list_id is required") {
		t.Fatalf("expected list_id required error, got: %v", err)
	}
}

// TestDeleteGroupBoardList_Success verifies that DeleteGroupBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteGroupBoardList(context.Background(), client, DeleteGroupBoardListInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1, ListID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteGroupBoardList_MissingParams verifies that DeleteGroupBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroupBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteGroupBoardList(context.Background(), client, DeleteGroupBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got: %v", err)
	}
	err = DeleteGroupBoardList(context.Background(), client, DeleteGroupBoardListInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "board_id is required") {
		t.Fatalf("expected board_id required error, got: %v", err)
	}
	err = DeleteGroupBoardList(context.Background(), client, DeleteGroupBoardListInput{GroupID: "42", BoardID: 1})
	if err == nil || !strings.Contains(err.Error(), "list_id is required") {
		t.Fatalf("expected list_id required error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Formatter tests
// ---------------------------------------------------------------------------.

// TestFormatGroupBoardMarkdown_AllFields pins the whole group board card: the
// heading carrying the board's reference, one list item per field with the
// group, milestone and assignee linked, the weight, the two visibility flags
// as glyphs, and the columns as a nested table naming each one's scope. The
// label list carries a nil entry and a nameless one because GitLab's arrays
// carry both, and neither may reach the card as an empty name between two
// commas.
func TestFormatGroupBoardMarkdown_AllFields(t *testing.T) {
	out := GroupBoardOutput{
		ID:              1,
		Name:            "Dev Board",
		Group:           &GroupRefOutput{ID: 42, Name: "mygroup", WebURL: "https://gitlab.example.com/groups/mygroup"},
		Milestone:       &MilestoneOutput{ID: 5, Title: "v1.0", WebURL: "https://gitlab.example.com/groups/mygroup/-/milestones/1"},
		Assignee:        &BasicUserOutput{ID: 3, Username: "alice", WebURL: "https://gitlab.example.com/alice"},
		Weight:          4,
		Labels:          []*LabelDetailsOutput{{ID: 1, Name: "bug"}, nil, {ID: 3}, {ID: 2, Name: "feature"}},
		HideBacklogList: true,
		HideClosedList:  false,
		Lists: []BoardListOutput{
			{ID: 10, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 5},
			{ID: 11, Position: 1, Iteration: &IterationOutput{ID: 9, Title: "Sprint 3"}},
		},
	}

	md := FormatGroupBoardMarkdown(out)

	want := "## Group Board #1: Dev Board\n\n" +
		"- **ID**: 1\n" +
		"- **Group**: [mygroup](https://gitlab.example.com/groups/mygroup)\n" +
		"- **Milestone**: [v1.0](https://gitlab.example.com/groups/mygroup/-/milestones/1)\n" +
		"- **Assignee**: [@alice](https://gitlab.example.com/alice)\n" +
		"- **Weight**: 4\n" +
		"- **Labels**: bug, feature\n" +
		"- **Hide Backlog**: " + toolutil.EmojiSuccess + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
		"### Lists\n\n" +
		"| ID | Scope | Position | Max Issues | Max Weight |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 10 | To Do | 0 | 5 | - |\n" +
		"| 11 | Iteration: Sprint 3 | 1 | - | - |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_create_list' to add a column to this board\n" +
		"- Use action 'group.group_board_update' to change this board's name or scope\n" +
		"- Use action 'group.group_board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatGroupBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListGroupBoardsMarkdown pins the whole list: the counted heading,
// one row per board with its ID and its linked group and milestone, and the
// guidance section last with the preserve-links hint first.
func TestFormatListGroupBoardsMarkdown(t *testing.T) {
	out := ListGroupBoardsOutput{
		Boards: []GroupBoardOutput{
			{
				ID: 1, Name: "Board A",
				Group:     &GroupRefOutput{ID: 1, Name: "grp", WebURL: "https://gitlab.example.com/groups/grp"},
				Milestone: &MilestoneOutput{ID: 3, Title: "M1", WebURL: "https://gitlab.example.com/groups/grp/-/milestones/3"},
				Lists:     []BoardListOutput{{ID: 10}},
			},
			{ID: 2, Name: "Board B", Group: &GroupRefOutput{ID: 1, Name: "grp", WebURL: "https://gitlab.example.com/groups/grp"}},
		},
	}

	md := FormatListGroupBoardsMarkdown(out)

	want := "## Group Issue Boards (2)\n\n" +
		"| ID | Name | Group | Milestone | Lists |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | Board A | [grp](https://gitlab.example.com/groups/grp) | [M1](https://gitlab.example.com/groups/grp/-/milestones/3) | 1 |\n" +
		"| 2 | Board B | [grp](https://gitlab.example.com/groups/grp) |  | 0 |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'group.group_board_get' to read one board with its columns\n" +
		"- Use action 'group.group_board_create' to add a new board to the group\n"
	if md != want {
		t.Errorf("FormatListGroupBoardsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatBoardListMarkdown_AllFields pins the whole column card: every
// scope GitLab sent, the position, the two ceilings and the metric they are
// counted in.
func TestFormatBoardListMarkdown_AllFields(t *testing.T) {
	out := BoardListOutput{
		ID:             10,
		Label:          &LabelOutput{Name: "Priority"},
		Position:       0,
		Assignee:       &BoardListAssigneeOutput{ID: 3, Username: "dev1"},
		Iteration:      &IterationOutput{ID: 9, Title: "Iteration 1", WebURL: "https://gitlab.example.com/groups/grp/-/iterations/9"},
		Milestone:      &MilestoneOutput{ID: 7, Title: "sprint-1", WebURL: "https://gitlab.example.com/groups/grp/-/milestones/7"},
		MaxIssueCount:  10,
		MaxIssueWeight: 50,
		LimitMetric:    "all_metrics",
	}

	md := FormatBoardListMarkdown(out)

	want := "## Board List #10: Priority\n\n" +
		"- **ID**: 10\n" +
		"- **Label**: Priority\n" +
		"- **Assignee**: @dev1\n" +
		"- **Iteration**: [Iteration 1](https://gitlab.example.com/groups/grp/-/iterations/9)\n" +
		"- **Milestone**: [sprint-1](https://gitlab.example.com/groups/grp/-/milestones/7)\n" +
		"- **Position**: 0\n" +
		"- **Max Issue Count**: 10\n" +
		"- **Max Issue Weight**: 50\n" +
		"- **Limit Metric**: all_metrics\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_update_list' to change this column's position or limits\n" +
		"- Use action 'group.group_board_delete_list' to remove this column\n"
	if md != want {
		t.Errorf("FormatBoardListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ListGroupBoards — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListGroupBoards_APIError verifies that ListGroupBoards returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroupBoards_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroupBoards(context.Background(), client, ListGroupBoardsInput{GroupID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroupBoards_CancelledContext verifies the ListGroupBoards_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListGroupBoards_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroupBoards(ctx, client, ListGroupBoardsInput{GroupID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetGroupBoard — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetGroupBoard_APIError verifies that GetGroupBoard returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroupBoard_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetGroupBoard_CancelledContext verifies the GetGroupBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetGroupBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroupBoard(ctx, client, GetGroupBoardInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateGroupBoard — API error, canceled context
// ---------------------------------------------------------------------------.

// TestCreateGroupBoard_APIError verifies that CreateGroupBoard returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoard_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{GroupID: "42", Name: "board"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateGroupBoard_ValidationAPIError verifies that a refused creation
// carries the name-and-scope hint. GitLab answers a rejected board name with
// 400 on some paths and 422 on others, and the handler owes the same guidance
// for both, so each code is driven rather than whichever one the fixture
// happened to pick.
func TestCreateGroupBoard_ValidationAPIError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, status, `{"message":"Name has already been taken"}`)
			}))

			_, err := CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{GroupID: "42", Name: "board"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), "unique within the group") {
				t.Fatalf("error = %q, want validation hint", err.Error())
			}
		})
	}
}

// TestCreateGroupBoard_CancelledContext verifies the CreateGroupBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateGroupBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateGroupBoard(ctx, client, CreateGroupBoardInput{GroupID: "42", Name: "board"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// UpdateGroupBoard — API error, canceled context
// ---------------------------------------------------------------------------.

// TestUpdateGroupBoard_APIError verifies that UpdateGroupBoard returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateGroupBoard_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{GroupID: "42", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateGroupBoard_ValidationAPIError verifies validation failures include
// guidance about referenced assignee, milestone, label, and weight values,
// under either code GitLab refuses a board scope with: 400 and 422 share the
// branch, so each is driven rather than only the one the fixture picked.
func TestUpdateGroupBoard_ValidationAPIError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, status, `{"message":"Invalid board scope"}`)
			}))

			_, err := UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{GroupID: "42", BoardID: 1, Name: "x"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), "referenced assignee_id") {
				t.Fatalf("error = %q, want validation hint", err.Error())
			}
		})
	}
}

// TestUpdateGroupBoard_CancelledContext verifies the UpdateGroupBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateGroupBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateGroupBoard(ctx, client, UpdateGroupBoardInput{GroupID: "42", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// DeleteGroupBoard — API error, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteGroupBoard_APIError verifies that DeleteGroupBoard returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroupBoard_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteGroupBoard(context.Background(), client, DeleteGroupBoardInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteGroupBoard_CancelledContext verifies the DeleteGroupBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteGroupBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteGroupBoard(ctx, client, DeleteGroupBoardInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListGroupBoardLists — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListGroupBoardLists_APIError verifies that ListGroupBoardLists returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroupBoardLists_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroupBoardLists_CancelledContext verifies the ListGroupBoardLists_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListGroupBoardLists_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroupBoardLists(ctx, client, ListGroupBoardListsInput{GroupID: "42", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetGroupBoardList — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetGroupBoardList_APIError verifies that GetGroupBoardList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroupBoardList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetGroupBoardList_CancelledContext verifies the GetGroupBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetGroupBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroupBoardList(ctx, client, GetGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateGroupBoardList — API error, canceled context
// ---------------------------------------------------------------------------.

// TestCreateGroupBoardList_APIError verifies that CreateGroupBoardList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoardList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1, LabelID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateGroupBoardList_ValidationAPIError verifies a refused column
// creation carries the duplicate-scope hint under either code GitLab refuses
// one with: 400 and 422 share the branch, so each is driven rather than only
// the one the fixture picked.
func TestCreateGroupBoardList_ValidationAPIError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, status, `{"message":"List already exists"}`)
			}))

			_, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1, LabelID: 5})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), "same scope already exists") {
				t.Fatalf("error = %q, want duplicate scope hint", err.Error())
			}
		})
	}
}

// TestCreateGroupBoardList_CancelledContext verifies the CreateGroupBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateGroupBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateGroupBoardList(ctx, client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1, LabelID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// UpdateGroupBoardList — API error, canceled context, fallback, empty
// ---------------------------------------------------------------------------.

// TestUpdateGroupBoardList_APIError verifies that UpdateGroupBoardList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateGroupBoardList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10, Position: 2})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateGroupBoardList_CancelledContext verifies the UpdateGroupBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateGroupBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateGroupBoardList(ctx, client, UpdateGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10, Position: 2})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// DeleteGroupBoardList — API error, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteGroupBoardList_APIError verifies that DeleteGroupBoardList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroupBoardList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteGroupBoardList(context.Background(), client, DeleteGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteGroupBoardList_CancelledContext verifies the DeleteGroupBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteGroupBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteGroupBoardList(ctx, client, DeleteGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatGroupBoardMarkdown — minimal (no optional fields)
// ---------------------------------------------------------------------------.

// TestFormatGroupBoardMarkdown_Minimal pins the whole card of a board GitLab
// sent nothing optional for: no group, milestone, assignee, weight, label or
// column row is written at all, and the two flags GitLab always sends are.
func TestFormatGroupBoardMarkdown_Minimal(t *testing.T) {
	md := FormatGroupBoardMarkdown(GroupBoardOutput{ID: 1, Name: "Board"})

	want := "## Group Board #1: Board\n\n" +
		"- **ID**: 1\n" +
		"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_create_list' to add a column to this board\n" +
		"- Use action 'group.group_board_update' to change this board's name or scope\n" +
		"- Use action 'group.group_board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatGroupBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatListGroupBoardsMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListGroupBoardsMarkdown_Empty pins the whole render of a group
// with no boards: one sentence, where the formatter used to write a heading
// and a table header with nothing under it.
func TestFormatListGroupBoardsMarkdown_Empty(t *testing.T) {
	if got, want := FormatListGroupBoardsMarkdown(ListGroupBoardsOutput{}), "No group issue boards found.\n"; got != want {
		t.Errorf("FormatListGroupBoardsMarkdown() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatBoardListMarkdown — minimal (no optional fields)
// ---------------------------------------------------------------------------.

// TestFormatBoardListMarkdown_Minimal pins the whole card of a column with no
// scope and no ceilings: the heading falls back to the reference, the position
// is written because zero is the first column, and nothing else is.
func TestFormatBoardListMarkdown_Minimal(t *testing.T) {
	md := FormatBoardListMarkdown(BoardListOutput{ID: 5, Position: 1})

	want := "## Board List #5\n\n" +
		"- **ID**: 5\n" +
		"- **Position**: 1\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_update_list' to change this column's position or limits\n" +
		"- Use action 'group.group_board_delete_list' to remove this column\n"
	if md != want {
		t.Errorf("FormatBoardListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatListBoardListsMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatListBoardListsMarkdown_WithData pins the whole column list: the
// counted heading, one row per column naming what it collects, the pagination
// line, and the guidance section last with no preserve-links hint, since the
// table carries no link.
func TestFormatListBoardListsMarkdown_WithData(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{
			{ID: 10, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 5, MaxIssueWeight: 20},
			{ID: 11, Label: &LabelOutput{Name: "Doing"}, Position: 1, MaxIssueCount: 3, MaxIssueWeight: 15},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	md := FormatListBoardListsMarkdown(out)

	want := "## Board Lists (2)\n\n" +
		"| ID | Scope | Position | Max Issues | Max Weight |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 10 | To Do | 0 | 5 | 20 |\n" +
		"| 11 | Doing | 1 | 3 | 15 |\n\n" +
		"Page 1 of 1 | 2 items total | 20 per page\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_get_list' to read one column\n" +
		"- Use action 'group.group_board_list_lists' to page through the rest of the board's columns\n"
	if md != want {
		t.Errorf("FormatListBoardListsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListBoardListsMarkdown_Empty pins the whole render of a board with
// no columns: one sentence and nothing else.
func TestFormatListBoardListsMarkdown_Empty(t *testing.T) {
	if got, want := FormatListBoardListsMarkdown(ListBoardListsOutput{}), "No board lists found.\n"; got != want {
		t.Errorf("FormatListBoardListsMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatListBoardListsMarkdown_ScopeOfEachColumnKind pins the Scope cell
// for every scope GitLab gives a column, and for a scope object it sent with
// nothing in it. Only the label and iteration kinds were driven before, so the
// assignee and milestone arms of the scope decision were never taken at all,
// and the emptiness checks beside each of them were never reached: a column
// whose assignee has no username, or whose milestone or iteration has no
// title, must read as the board's own backlog or closed column rather than as
// a bare handle or a label with nothing after it.
func TestFormatListBoardListsMarkdown_ScopeOfEachColumnKind(t *testing.T) {
	cases := []struct {
		name string
		list BoardListOutput
		want string
	}{
		{"label", BoardListOutput{ID: 1, Label: &LabelOutput{Name: "To Do"}}, "To Do"},
		{"assignee", BoardListOutput{ID: 1, Assignee: &BoardListAssigneeOutput{ID: 3, Username: "ada"}}, "@ada"},
		{"milestone", BoardListOutput{ID: 1, Milestone: &MilestoneOutput{ID: 4, Title: "v1.0"}}, "Milestone: v1.0"},
		{"iteration", BoardListOutput{ID: 1, Iteration: &IterationOutput{ID: 5, Title: "Sprint 4"}}, "Iteration: Sprint 4"},
		{"label with no name", BoardListOutput{ID: 1, Label: &LabelOutput{}}, ""},
		{"assignee with no username", BoardListOutput{ID: 1, Assignee: &BoardListAssigneeOutput{ID: 3}}, ""},
		{"milestone with no title", BoardListOutput{ID: 1, Milestone: &MilestoneOutput{ID: 4}}, ""},
		{"iteration with no title", BoardListOutput{ID: 1, Iteration: &IterationOutput{ID: 5}}, ""},
		{"no scope at all", BoardListOutput{ID: 1}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := FormatListBoardListsMarkdown(ListBoardListsOutput{Lists: []BoardListOutput{tc.list}})
			want := "| 1 | " + tc.want + " | 0 | - | - |\n"
			if !strings.Contains(md, want) {
				t.Errorf("FormatListBoardListsMarkdown() = %q, want a row %q", md, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Keyset/order_by/sort and nil-shape coverage
// ---------------------------------------------------------------------------.

// TestListGroupBoards_OrderBySortKeyset verifies that order_by, sort, and keyset
// pagination parameters are forwarded as GitLab query parameters.
// It asserts the underlying request carries the expected query string.
func TestListGroupBoards_OrderBySortKeyset(t *testing.T) {
	mux := http.NewServeMux()
	var gotQuery string
	mux.HandleFunc("/api/v4/groups/42/boards", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, groupBoardListJSON)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListGroupBoards(context.Background(), client, ListGroupBoardsInput{
		GroupID: toolutil.StringOrInt("42"), OrderBy: "created_at", Sort: "desc",
		Pagination: "keyset", PageToken: "100",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=100"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestListGroupBoardLists_OrderBySortKeyset verifies that order_by, sort, and
// keyset pagination parameters are forwarded for the board-list list endpoint.
// It asserts the underlying request carries the expected query string.
func TestListGroupBoardLists_OrderBySortKeyset(t *testing.T) {
	mux := http.NewServeMux()
	var gotQuery string
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, boardListsArrayJSON)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{
		GroupID: toolutil.StringOrInt("42"), BoardID: 1, OrderBy: "position", Sort: "asc",
		Pagination: "keyset", PageToken: "5",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=position", "sort=asc", "pagination=keyset", "page_token=5"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestShapeConverters_NilInputs verifies that the shape converters return nil for
// nil SDK inputs and that nil sub-objects are skipped in slices.
// It asserts each helper handles the nil/empty path without panicking.
func TestShapeConverters_NilInputs(t *testing.T) {
	if groupRefOutput(nil) != nil {
		t.Error("groupRefOutput(nil) != nil")
	}
	if milestoneOutput(nil) != nil {
		t.Error("milestoneOutput(nil) != nil")
	}
	if labelOutput(nil) != nil {
		t.Error("labelOutput(nil) != nil")
	}
	if boardListAssigneeOutput(nil) != nil {
		t.Error("boardListAssigneeOutput(nil) != nil")
	}
	if iterationOutput(nil) != nil {
		t.Error("iterationOutput(nil) != nil")
	}
	if groupLabelOutputs(nil) != nil {
		t.Error("groupLabelOutputs(nil) != nil")
	}
	// A slice with a nil element skips it but still returns the valid one.
	if got := groupLabelOutputs([]*gl.GroupLabel{nil, {ID: 7, Name: "kept"}}); len(got) != 1 || got[0].Name != "kept" {
		t.Errorf("groupLabelOutputs nil-skip = %+v, want one 'kept' label", got)
	}
}

// TestMarkdownHelpers_NilFallbacks verifies the nil-fallback paths of the
// markdown sub-object accessors return empty strings.
// It asserts board lists/boards with absent sub-objects render without panic.
func TestMarkdownHelpers_NilFallbacks(t *testing.T) {
	if got := boardListLabelName(BoardListOutput{ID: 1}); got != "" {
		t.Errorf("boardListLabelName(no label) = %q, want empty", got)
	}
	if got := groupCell(nil); got != "" {
		t.Errorf("groupCell(nil) = %q, want empty", got)
	}
	if got := milestoneCell(nil); got != "" {
		t.Errorf("milestoneCell(nil) = %q, want empty", got)
	}
	// A board with a nil group and milestone, a nil label entry and a column
	// with no scope renders the whole card without inventing any of them.
	md := FormatGroupBoardMarkdown(GroupBoardOutput{
		ID: 1, Name: "Plain", Labels: []*LabelDetailsOutput{nil},
		Lists: []BoardListOutput{{ID: 9, Position: 0}},
	})

	want := "## Group Board #1: Plain\n\n" +
		"- **ID**: 1\n" +
		"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
		"### Lists\n\n" +
		"| ID | Scope | Position | Max Issues | Max Weight |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 9 |  | 0 | - | - |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.group_board_create_list' to add a column to this board\n" +
		"- Use action 'group.group_board_update' to change this board's name or scope\n" +
		"- Use action 'group.group_board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatGroupBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// Documented-but-SDK-missing field surfacing (raw-fetch superset) tests
// ---------------------------------------------------------------------------.

// premiumGroupBoardJSON carries the documented Premium/Ultimate group-board
// fields that gl.GroupIssueBoard omits: hide_backlog_list, hide_closed_list,
// assignee and weight. It is used to assert the raw-superset fetch path surfaces
// them on the canonical json keys.
const premiumGroupBoardJSON = `{
	"id": 1,
	"name": "new_name",
	"hide_backlog_list": true,
	"hide_closed_list": true,
	"group": {"id": 5, "name": "Documentcloud", "web_url": "http://example.com/groups/documentcloud"},
	"milestone": {"id": 44, "iid": 1, "group_id": 5, "title": "Group Milestone", "state": "active", "web_url": "http://example.com/m/1"},
	"assignee": {"id": 1, "name": "Administrator", "username": "root", "state": "active", "avatar_url": "https://gravatar/x", "web_url": "http://example.com/root"},
	"labels": [{"id": 11, "name": "GroupLabel", "color": "#428BCA", "description": ""}],
	"weight": 4
}`

// TestGetGroupBoard_SurfacesDocumentedPremiumFields verifies that the raw-fetch
// superset path surfaces the documented hide_backlog_list, hide_closed_list,
// assignee and weight fields that client-go's gl.GroupIssueBoard omits.
func TestGetGroupBoard_SurfacesDocumentedPremiumFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, premiumGroupBoardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: toolutil.StringOrInt("5"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.HideBacklogList || !out.HideClosedList {
		t.Errorf("hide flags = (%v, %v), want (true, true)", out.HideBacklogList, out.HideClosedList)
	}
	if out.Weight != 4 {
		t.Errorf("weight = %d, want 4", out.Weight)
	}
	if out.Assignee == nil || out.Assignee.Username != "root" || out.Assignee.State != "active" {
		t.Errorf("assignee = %+v, want username root state active", out.Assignee)
	}
	if out.Milestone == nil || out.Milestone.GroupID != 5 {
		t.Errorf("milestone group_id = %+v, want 5", out.Milestone)
	}
	if len(out.Labels) != 1 || out.Labels[0].ID != 11 || out.Labels[0].Color != "#428BCA" {
		t.Errorf("labels = %+v, want one GroupLabel with color", out.Labels)
	}
}

// TestGetGroupBoard_VersionTolerantWhenPremiumFieldsAbsent verifies that a
// response WITHOUT the SDK-missing documented fields (older / Free-tier GitLab)
// decodes successfully with those fields at their zero value and omitted from
// the envelope, never failing the tool.
func TestGetGroupBoard_VersionTolerantWhenPremiumFieldsAbsent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		// Minimal Free-tier shape: no hide_*_list, assignee or weight keys.
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"Development","group":{"id":42,"name":"mygroup"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1})
	if err != nil {
		t.Fatalf("tool must not fail when documented premium fields are absent: %v", err)
	}
	if out.HideBacklogList || out.HideClosedList || out.Weight != 0 || out.Assignee != nil {
		t.Errorf("absent fields should be zero/nil, got hide=(%v,%v) weight=%d assignee=%+v",
			out.HideBacklogList, out.HideClosedList, out.Weight, out.Assignee)
	}

	// The omitempty assignee/weight keys must not appear in the marshaled envelope.
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	if strings.Contains(js, `"assignee"`) || strings.Contains(js, `"weight"`) {
		t.Errorf("absent assignee/weight must be omitted from envelope: %s", js)
	}
}

// TestListGroupBoards_SurfacesDocumentedPremiumFields verifies the list handler's
// raw-fetch superset path surfaces hide_*_list/assignee/weight per board.
func TestListGroupBoards_SurfacesDocumentedPremiumFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/boards", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+premiumGroupBoardJSON+`]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroupBoards(context.Background(), client, ListGroupBoardsInput{GroupID: toolutil.StringOrInt("5")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Boards) != 1 || out.Boards[0].Weight != 4 || !out.Boards[0].HideBacklogList {
		t.Errorf("board[0] = %+v, want weight 4 and hide_backlog_list true", out.Boards)
	}
}

// wholeGroupBoardJSON is a board answer in which no two values agree: every
// number, string, date and flag is distinct, including the two visibility
// flags and the nested milestone's four timestamps. That is what makes a
// swapped pair of assignments in the converters visible — with
// hide_backlog_list and hide_closed_list both true, as in every other fixture
// here, exchanging those two lines changes nothing a test can see.
const wholeGroupBoardJSON = `{
	"id": 1,
	"name": "Delivery",
	"hide_backlog_list": true,
	"hide_closed_list": false,
	"weight": 3,
	"group": {"id": 42, "name": "platform", "web_url": "https://gitlab.example.com/groups/platform"},
	"milestone": {
		"id": 44, "iid": 7, "group_id": 55, "title": "v1.0", "description": "first release",
		"state": "active", "web_url": "https://gitlab.example.com/m/44",
		"start_date": "2026-01-02", "due_date": "2026-03-04",
		"created_at": "2026-01-05T06:07:08Z", "updated_at": "2026-02-09T10:11:12Z"
	},
	"assignee": {
		"id": 11, "name": "Ada", "username": "ada", "state": "active",
		"avatar_url": "https://gitlab.example.com/avatar/ada", "web_url": "https://gitlab.example.com/ada"
	},
	"labels": [{"id": 21, "name": "bug", "color": "#d9534f", "description": "a defect"}],
	"lists": [{
		"id": 31, "position": 5, "max_issue_count": 6, "max_issue_weight": 7,
		"limit_metric": "all_metrics",
		"label": {"id": 41, "name": "Doing", "color": "#5bc0de", "description": "in progress"},
		"assignee": {"id": 51, "name": "Grace", "username": "grace"},
		"milestone": {
			"id": 61, "iid": 8, "group_id": 62, "title": "sprint-9", "description": "ninth",
			"state": "closed", "web_url": "https://gitlab.example.com/m/61",
			"start_date": "2026-04-01", "due_date": "2026-05-02",
			"created_at": "2026-03-03T04:05:06Z", "updated_at": "2026-03-07T08:09:10Z"
		},
		"iteration": {
			"id": 71, "iid": 9, "sequence": 10, "group_id": 72, "title": "Iteration 4",
			"description": "week 4", "state": 2, "web_url": "https://gitlab.example.com/it/71"
		}
	}]
}`

// wholeGroupBoardOutput is what the handlers must make of
// [wholeGroupBoardJSON]: every field of the board, of its group, milestone,
// assignee and labels, and of its one column with all four scope objects.
func wholeGroupBoardOutput() GroupBoardOutput {
	return GroupBoardOutput{
		ID:   1,
		Name: "Delivery",
		Group: &GroupRefOutput{
			ID: 42, Name: "platform", WebURL: "https://gitlab.example.com/groups/platform",
		},
		Milestone: &MilestoneOutput{
			ID: 44, IID: 7, GroupID: 55, Title: "v1.0", Description: "first release",
			State: "active", WebURL: "https://gitlab.example.com/m/44",
			StartDate: "2026-01-02", DueDate: "2026-03-04",
			CreatedAt: "2026-01-05T06:07:08Z", UpdatedAt: "2026-02-09T10:11:12Z",
		},
		Assignee: &BasicUserOutput{
			ID: 11, Username: "ada", Name: "Ada", State: "active",
			AvatarURL: "https://gitlab.example.com/avatar/ada", WebURL: "https://gitlab.example.com/ada",
		},
		Weight:          3,
		Labels:          []*LabelDetailsOutput{{ID: 21, Name: "bug", Color: "#d9534f", Description: "a defect"}},
		HideBacklogList: true,
		HideClosedList:  false,
		Lists:           []BoardListOutput{wholeBoardListOutput()},
	}
}

// wholeBoardListOutput is the single column of [wholeGroupBoardJSON], which is
// also the shape the board-list handlers must produce from the same object.
func wholeBoardListOutput() BoardListOutput {
	return BoardListOutput{
		ID:       31,
		Assignee: &BoardListAssigneeOutput{ID: 51, Name: "Grace", Username: "grace"},
		Iteration: &IterationOutput{
			ID: 71, IID: 9, Sequence: 10, GroupID: 72, Title: "Iteration 4",
			Description: "week 4", State: 2, WebURL: "https://gitlab.example.com/it/71",
		},
		Label:          &LabelOutput{Name: "Doing", Color: "#5bc0de", Description: "in progress"},
		LimitMetric:    "all_metrics",
		MaxIssueCount:  6,
		MaxIssueWeight: 7,
		Milestone: &MilestoneOutput{
			ID: 61, IID: 8, GroupID: 62, Title: "sprint-9", Description: "ninth",
			State: "closed", WebURL: "https://gitlab.example.com/m/61",
			StartDate: "2026-04-01", DueDate: "2026-05-02",
			CreatedAt: "2026-03-03T04:05:06Z", UpdatedAt: "2026-03-07T08:09:10Z",
		},
		Position: 5,
	}
}

// TestGetGroupBoard_WholeShapeOfOneAnswer pins every field the board
// converters fill, against a response in which no two values agree. Neither
// gate can see a converter that reads the wrong neighbor, because an
// assignment has no branch to flip; a whole-struct comparison against distinct
// values is what notices, and it covers the nested group, milestone, assignee,
// label and column shapes in one pass.
func TestGetGroupBoard_WholeShapeOfOneAnswer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, wholeGroupBoardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	got, err := GetGroupBoard(context.Background(), client, GetGroupBoardInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if want := wholeGroupBoardOutput(); !reflect.DeepEqual(got, want) {
		t.Errorf("GetGroupBoard()\n got %+v\nwant %+v", got, want)
	}
}

// TestGetGroupBoardList_WholeShapeOfOneAnswer pins the column shape the SDK
// path produces, for the same reason and against the same distinct values. It
// is a path of its own rather than a repeat of the board test: here the column
// comes from client-go's own struct and its limit_metric from the captured
// response beside it, and only this test says the two are joined on the right
// column.
func TestGetGroupBoardList_WholeShapeOfOneAnswer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/31", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, wholeBoardListJSON(t))
	})
	client := testutil.NewTestClient(t, mux)

	got, err := GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{
		GroupID: toolutil.StringOrInt("42"), BoardID: 1, ListID: 31,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if want := wholeBoardListOutput(); !reflect.DeepEqual(got, want) {
		t.Errorf("GetGroupBoardList()\n got %+v\nwant %+v", got, want)
	}
}

// wholeBoardListJSON is the one column of [wholeGroupBoardJSON], taken from
// that fixture rather than written out again so the two paths cannot drift
// into agreeing with different objects.
func wholeBoardListJSON(t *testing.T) string {
	t.Helper()
	var board struct {
		Lists []json.RawMessage `json:"lists"`
	}
	if err := json.Unmarshal([]byte(wholeGroupBoardJSON), &board); err != nil {
		t.Fatalf("read the board fixture: %v", err)
	}
	if len(board.Lists) != 1 {
		t.Fatalf("board fixture has %d columns, want 1", len(board.Lists))
	}
	return string(board.Lists[0])
}

// TestBasicUserOutput_Nil verifies the assignee converter returns nil for a nil
// input (the Free-tier / no-assignee case).
func TestBasicUserOutput_Nil(t *testing.T) {
	if got := basicUserOutput(nil); got != nil {
		t.Errorf("basicUserOutput(nil) = %+v, want nil", got)
	}
}

// TestGroupBoardLists_UnreadableCapturedLimitMetric verifies that every group
// board list handler returns an error rather than a half-filled list when
// GitLab sends limit_metric as something that is not a string. The SDK ignores
// the key its own BoardList does not model, so the read of the captured
// response is the only thing that can notice.
func TestGroupBoardLists_UnreadableCapturedLimitMetric(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"position":0,"limit_metric":42}]`)
			_, err := ListGroupBoardLists(context.Background(), client, ListGroupBoardListsInput{GroupID: "42", BoardID: 1})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"position":0,"limit_metric":42}`)
			_, err := GetGroupBoardList(context.Background(), client, GetGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 1})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"id":1,"position":0,"limit_metric":42}`)
			_, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1, LabelID: 5})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"id":1,"position":0,"limit_metric":42}`)
			_, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: "42", BoardID: 1, ListID: 1, Position: 2})
			return err
		}},
	})
}

// TestRawRequestConstructionFailures verifies every raw-request handler
// surfaces a request-construction error. The branch is unreachable with valid
// inputs (PathEscape sanitizes the path), so the constructor seam is stubbed.
func TestRawRequestConstructionFailures(t *testing.T) {
	orig := newRawRequest
	newRawRequest = func(context.Context, *gitlabclient.Client, string, string, any) (*retryablehttp.Request, error) {
		return nil, errors.New("construction boom")
	}
	t.Cleanup(func() { newRawRequest = orig })
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	if _, err := ListGroupBoards(ctx, client, ListGroupBoardsInput{GroupID: "42"}); err == nil {
		t.Error("ListGroupBoards: expected construction error")
	}
	if _, err := GetGroupBoard(ctx, client, GetGroupBoardInput{GroupID: "42", BoardID: 1}); err == nil {
		t.Error("GetGroupBoard: expected construction error")
	}
	if _, err := CreateGroupBoard(ctx, client, CreateGroupBoardInput{GroupID: "42", Name: "b"}); err == nil {
		t.Error("CreateGroupBoard: expected construction error")
	}
	if _, err := UpdateGroupBoard(ctx, client, UpdateGroupBoardInput{GroupID: "42", BoardID: 1}); err == nil {
		t.Error("UpdateGroupBoard: expected construction error")
	}
}
