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
	"group": {"id": 42, "name": "mygroup"},
	"milestone": {"id": 5, "title": "v1.0"},
	"labels": [{"name": "bug"}, {"name": "feature"}],
	"lists": [
		{"id": 10, "label": {"id": 20, "name": "To Do"}, "position": 0, "max_issue_count": 10}
	]
}`

// groupBoardListJSON stores the package-level group board list JSON state.
var groupBoardListJSON = `[` + groupBoardJSON + `]`

// boardListItemJSON stores the package-level board list item JSON state.
var boardListItemJSON = `{
	"id": 10,
	"label": {"id": 20, "name": "To Do"},
	"position": 0,
	"max_issue_count": 10,
	"max_issue_weight": 50,
	"assignee": {"id": 3, "username": "alice"},
	"milestone": {"id": 7, "title": "sprint-1"}
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
	if out.Boards[0].GroupID != 42 {
		t.Errorf("group_id = %d, want 42", out.Boards[0].GroupID)
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
	if out.MilestoneTitle != "v1.0" {
		t.Errorf("milestone = %q, want %q", out.MilestoneTitle, "v1.0")
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
		Labels: "bug", AssigneeID: 3, MilestoneID: 5, Weight: 2,
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
	if out.Lists[0].LabelName != "To Do" {
		t.Errorf("label = %q, want %q", out.Lists[0].LabelName, "To Do")
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
	if out.LabelName != "To Do" {
		t.Errorf("label = %q, want %q", out.LabelName, "To Do")
	}
	if out.AssigneeUser != "alice" {
		t.Errorf("assignee = %q, want %q", out.AssigneeUser, "alice")
	}
	if out.MilestoneTitle != "sprint-1" {
		t.Errorf("milestone = %q, want %q", out.MilestoneTitle, "sprint-1")
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
	if out.LabelName != "Priority" {
		t.Errorf("label = %q, want %q", out.LabelName, "Priority")
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
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"position":2,"label":{"id":5,"name":"To Do"}}]`)
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
		ID:             1,
		Name:           "Dev Board",
		GroupName:      "mygroup",
		GroupID:        42,
		MilestoneTitle: "v1.0",
		MilestoneID:    5,
		Labels:         []string{"bug", "feature"},
		Lists:          []BoardListOutput{{ID: 10, LabelName: "To Do", Position: 0}},
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
			{ID: 1, Name: "Board A", GroupName: "grp"},
			{ID: 2, Name: "Board B", GroupName: "grp"},
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
		LabelName:      "Priority",
		LabelID:        5,
		Position:       0,
		AssigneeUser:   "dev1",
		AssigneeID:     3,
		MilestoneTitle: "sprint-1",
		MilestoneID:    7,
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

// TestUpdateGroupBoardList_FallbackFirstElement verifies the UpdateGroupBoardList_FallbackFirstElement handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateGroupBoardList_FallbackFirstElement(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":99,"position":3,"label":{"id":7,"name":"Fallback"}}]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{
		GroupID: "42", BoardID: 1, ListID: 10, Position: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 99 {
		t.Errorf("ID = %d, want 99 (fallback to first element)", out.ID)
	}
	if out.LabelName != "Fallback" {
		t.Errorf("LabelName = %q, want %q", out.LabelName, "Fallback")
	}
}

// TestUpdateGroupBoardList_EmptyResult verifies the UpdateGroupBoardList_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateGroupBoardList_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := UpdateGroupBoardList(context.Background(), client, UpdateGroupBoardListInput{
		GroupID: "42", BoardID: 1, ListID: 10, Position: 2,
	})
	if err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
	if !strings.Contains(err.Error(), "no board list returned") {
		t.Errorf("error = %q, want 'no board list returned'", err.Error())
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
			{ID: 10, LabelName: "To Do", Position: 0, MaxIssueCount: 5, MaxIssueWeight: 20},
			{ID: 11, LabelName: "Doing", Position: 1, MaxIssueCount: 3, MaxIssueWeight: 15},
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
