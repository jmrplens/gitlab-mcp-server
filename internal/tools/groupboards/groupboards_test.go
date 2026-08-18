// groupboards_test.go contains unit tests for the group issue board MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupboards

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestCreateGroupBoard_Success verifies that CreateGroupBoard succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateGroupBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
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

// TestCreateGroupBoardList_Success verifies that CreateGroupBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
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

// TestUpdateGroupBoardList_Success verifies that UpdateGroupBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateGroupBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		// GitLab returns the single updated list object — not an array; the
		// handler bypasses the client-go wrapper that expects []*BoardList.
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"position":2,"label":{"id":5,"name":"To Do"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{GroupID: toolutil.StringOrInt("42"), BoardID: 1, ListID: 10, Position: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Position != 2 {
		t.Errorf("position = %d, want 2", out.Position)
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

// TestFormatGroupBoardMarkdown verifies the GroupBoardMarkdown Markdown formatter for a representative groupboard input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGroupBoardMarkdown(t *testing.T) {
	out := GroupBoardOutput{
		ID:        1,
		Name:      "Dev Board",
		Group:     &GroupRefOutput{ID: 42, Name: "mygroup"},
		Milestone: &MilestoneOutput{ID: 5, Title: "v1.0"},
		Labels:    []*LabelDetailsOutput{{ID: 1, Name: "bug"}, {ID: 2, Name: "feature"}},
		Lists:     []BoardListOutput{{ID: 10, Label: &LabelOutput{Name: "To Do"}, Position: 0}},
	}
	md := FormatGroupBoardMarkdown(out)
	if !strings.Contains(md, "Dev Board") {
		t.Errorf("markdown missing board name")
	}
	if !strings.Contains(md, "mygroup") {
		t.Errorf("markdown missing group name")
	}
	if !strings.Contains(md, "v1.0") {
		t.Errorf("markdown missing milestone")
	}
	if !strings.Contains(md, "bug, feature") {
		t.Errorf("markdown missing labels")
	}
	if !strings.Contains(md, "To Do") {
		t.Errorf("markdown missing list label")
	}
}

// TestFormatListGroupBoardsMarkdown verifies the ListGroupBoardsMarkdown Markdown formatter for a representative listgroupboards input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListGroupBoardsMarkdown(t *testing.T) {
	out := ListGroupBoardsOutput{
		Boards: []GroupBoardOutput{
			{ID: 1, Name: "Board A", Group: &GroupRefOutput{ID: 1, Name: "grp"}, Milestone: &MilestoneOutput{ID: 3, Title: "M1"}},
			{ID: 2, Name: "Board B", Group: &GroupRefOutput{ID: 1, Name: "grp"}},
		},
	}
	md := FormatListGroupBoardsMarkdown(out)
	if !strings.Contains(md, "Board A") || !strings.Contains(md, "Board B") {
		t.Errorf("markdown missing board names")
	}
}

// TestFormatBoardListMarkdown verifies the BoardListMarkdown Markdown formatter for a representative boardlist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardListMarkdown(t *testing.T) {
	out := BoardListOutput{
		ID:             10,
		Label:          &LabelOutput{Name: "Priority"},
		Position:       0,
		Assignee:       &BoardListAssigneeOutput{ID: 3, Username: "dev1"},
		Iteration:      &IterationOutput{ID: 9, Title: "Iteration 1"},
		Milestone:      &MilestoneOutput{ID: 7, Title: "sprint-1"},
		MaxIssueCount:  10,
		MaxIssueWeight: 50,
	}
	md := FormatBoardListMarkdown(out)
	if !strings.Contains(md, "Priority") {
		t.Errorf("markdown missing label")
	}
	if !strings.Contains(md, "dev1") {
		t.Errorf("markdown missing assignee")
	}
	if !strings.Contains(md, "Iteration 1") {
		t.Errorf("markdown missing iteration")
	}
	if !strings.Contains(md, "sprint-1") {
		t.Errorf("markdown missing milestone")
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

// TestCreateGroupBoard_ValidationAPIError verifies that CreateGroupBoard_ValidationAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoard_ValidationAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Name has already been taken"}`)
	}))

	_, err := CreateGroupBoard(context.Background(), client, CreateGroupBoardInput{GroupID: "42", Name: "board"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "unique within the group") {
		t.Fatalf("error = %q, want validation hint", err.Error())
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
// guidance about referenced assignee, milestone, label, and weight values.
func TestUpdateGroupBoard_ValidationAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"Invalid board scope"}`)
	}))

	_, err := UpdateGroupBoard(context.Background(), client, UpdateGroupBoardInput{GroupID: "42", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "referenced assignee_id") {
		t.Fatalf("error = %q, want validation hint", err.Error())
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

// TestCreateGroupBoardList_ValidationAPIError verifies that CreateGroupBoardList_ValidationAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroupBoardList_ValidationAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"List already exists"}`)
	}))

	_, err := CreateGroupBoardList(context.Background(), client, CreateGroupBoardListInput{GroupID: "42", BoardID: 1, LabelID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "same scope already exists") {
		t.Fatalf("error = %q, want duplicate scope hint", err.Error())
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

// TestFormatGroupBoardMarkdown_Minimal verifies the GroupBoardMarkdown_Minimal Markdown formatter for a representative groupboard_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGroupBoardMarkdown_Minimal(t *testing.T) {
	md := FormatGroupBoardMarkdown(GroupBoardOutput{ID: 1, Name: "Board"})
	if !strings.Contains(md, "Board") {
		t.Error("markdown missing board name")
	}
	for _, absent := range []string{"**Group**", "**Milestone**", "**Labels**", "### Lists"} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal board", absent)
		}
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatListGroupBoardsMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListGroupBoardsMarkdown_Empty verifies the ListGroupBoardsMarkdown_Empty Markdown formatter for a representative listgroupboards_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListGroupBoardsMarkdown_Empty(t *testing.T) {
	md := FormatListGroupBoardsMarkdown(ListGroupBoardsOutput{})
	if !strings.Contains(md, "## Group Issue Boards") {
		t.Error("markdown missing header")
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatBoardListMarkdown — minimal (no optional fields)
// ---------------------------------------------------------------------------.

// TestFormatBoardListMarkdown_Minimal verifies the BoardListMarkdown_Minimal Markdown formatter for a representative boardlist_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardListMarkdown_Minimal(t *testing.T) {
	md := FormatBoardListMarkdown(BoardListOutput{ID: 5, Position: 1})
	if !strings.Contains(md, "Board List (ID: 5)") {
		t.Error("markdown missing list header")
	}
	if !strings.Contains(md, "**Position**: 1") {
		t.Error("markdown missing position")
	}
	for _, absent := range []string{"**Label**", "**Max Issue Count**", "**Max Issue Weight**", "**Assignee**", "**Milestone**"} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal board list", absent)
		}
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage: FormatListBoardListsMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatListBoardListsMarkdown_WithData verifies the ListBoardListsMarkdown_WithData Markdown formatter for a representative listboardlists_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardListsMarkdown_WithData(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{
			{ID: 10, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 5, MaxIssueWeight: 20},
			{ID: 11, Label: &LabelOutput{Name: "Doing"}, Position: 1, MaxIssueCount: 3, MaxIssueWeight: 15},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListBoardListsMarkdown(out)
	for _, want := range []string{"## Board Lists", "To Do", "Doing", "| 10 |", "| 11 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListBoardListsMarkdown_Empty verifies the ListBoardListsMarkdown_Empty Markdown formatter for a representative listboardlists_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardListsMarkdown_Empty(t *testing.T) {
	md := FormatListBoardListsMarkdown(ListBoardListsOutput{})
	if !strings.Contains(md, "## Board Lists") {
		t.Error("markdown missing header")
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
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "100"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=100"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
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
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=position", "sort=asc", "pagination=keyset", "page_token=5"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
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
	if got := groupName(nil); got != "" {
		t.Errorf("groupName(nil) = %q, want empty", got)
	}
	if got := milestoneTitle(nil); got != "" {
		t.Errorf("milestoneTitle(nil) = %q, want empty", got)
	}
	// A board with a nil group/milestone and a label-less list must still render.
	md := FormatGroupBoardMarkdown(GroupBoardOutput{
		ID: 1, Name: "Plain", Labels: []*LabelDetailsOutput{nil},
		Lists: []BoardListOutput{{ID: 9, Position: 0}},
	})
	if !strings.Contains(md, "Plain") {
		t.Errorf("markdown missing board name:\n%s", md)
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

// TestBasicUserOutput_Nil verifies the assignee converter returns nil for a nil
// input (the Free-tier / no-assignee case).
func TestBasicUserOutput_Nil(t *testing.T) {
	if got := basicUserOutput(nil); got != nil {
		t.Errorf("basicUserOutput(nil) = %+v, want nil", got)
	}
}
