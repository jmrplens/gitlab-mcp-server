// boards_test.go contains unit tests for the issue board MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package boards

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errProjectIDRequired = "project_id is required"

const errBoardIDRequired = "board_id is required"

const fmtUnexpErr = "unexpected error: %v"

const (
	pathBoard1              = "/api/v4/projects/10/boards/1"
	pathBoardList100        = "/api/v4/projects/10/boards/1/lists/100"
	fmtExpectedID1          = "expected ID 1, got %d"
	fmtExpectedID100        = "expected ID 100, got %d"
	fmtExpectedProjectIDReq = "expected project_id required, got %v"
	fmtExpectedBoardIDReq   = "expected board_id required, got %v"
	msgMethodNotAllowed     = "method not allowed"
	errListIDRequired       = "list_id is required"
	fmtExpectedListIDReq    = "expected list_id required, got %v"
	fmtMDMissingContent     = "markdown missing expected content: %s"
)

// ---------------------------------------------------------------------------
// Shared JSON fixtures
// ---------------------------------------------------------------------------.

var boardJSON = `{
	"id": 1,
	"name": "Development",
	"project": {"id": 10, "name": "My Project", "path_with_namespace": "group/my-project"},
	"milestone": {"id": 5, "title": "v1.0"},
	"assignee": {"id": 3, "username": "alice"},
	"weight": 2,
	"labels": [{"name": "bug"}, {"name": "feature"}],
	"hide_backlog_list": false,
	"hide_closed_list": true,
	"lists": [
		{"id": 100, "label": {"id": 20, "name": "To Do"}, "position": 0, "max_issue_count": 10}
	]
}`

var boardListJSON = `[` + boardJSON + `]`

var boardListItemJSON = `{
	"id": 100,
	"label": {"id": 20, "name": "To Do"},
	"position": 0,
	"max_issue_count": 10,
	"max_issue_weight": 50,
	"assignee": {"id": 3, "name": "Alice", "username": "alice"},
	"milestone": {"id": 5, "title": "v1.0"}
}`

var boardListsArrayJSON = `[` + boardListItemJSON + `]`

// ---------------------------------------------------------------------------
// Board CRUD tests
// ---------------------------------------------------------------------------.

// TestListBoards_Success verifies the behavior of list boards success.
func TestListBoards_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, boardListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBoards(context.Background(), client, ListBoardsInput{ProjectID: toolutil.StringOrInt("10")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Boards) != 1 {
		t.Fatalf("expected 1 board, got %d", len(out.Boards))
	}
	if out.Boards[0].Name != "Development" {
		t.Errorf("expected name Development, got %s", out.Boards[0].Name)
	}
}

// TestListBoards_MissingProjectID verifies the behavior of list boards missing project i d.
func TestListBoards_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListBoards(context.Background(), client, ListBoardsInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// TestGetBoard_Success verifies the behavior of get board success.
func TestGetBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: toolutil.StringOrInt("10"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtExpectedID1, out.ID)
	}
	if out.MilestoneTitle != "v1.0" {
		t.Errorf("expected milestone v1.0, got %s", out.MilestoneTitle)
	}
}

// TestGetBoard_MissingParams verifies the behavior of get board missing params.
func TestGetBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetBoard(context.Background(), client, GetBoardInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = GetBoard(context.Background(), client, GetBoardInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
}

// TestCreateBoard_Success verifies the behavior of create board success.
func TestCreateBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, boardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateBoard(context.Background(), client, CreateBoardInput{
		ProjectID: toolutil.StringOrInt("10"), Name: "Development",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtExpectedID1, out.ID)
	}
}

// TestCreateBoard_MissingParams verifies the behavior of create board missing params.
func TestCreateBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateBoard(context.Background(), client, CreateBoardInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = CreateBoard(context.Background(), client, CreateBoardInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name required, got %v", err)
	}
}

// TestUpdateBoard_Success verifies the behavior of update board success.
func TestUpdateBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, boardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateBoard(context.Background(), client, UpdateBoardInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1, Name: "Updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtExpectedID1, out.ID)
	}
}

// TestUpdateBoard_MissingParams verifies the behavior of update board missing params.
func TestUpdateBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateBoard(context.Background(), client, UpdateBoardInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = UpdateBoard(context.Background(), client, UpdateBoardInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
}

// TestDeleteBoard_Success verifies the behavior of delete board success.
func TestDeleteBoard_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteBoard(context.Background(), client, DeleteBoardInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteBoard_MissingParams verifies the behavior of delete board missing params.
func TestDeleteBoard_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteBoard(context.Background(), client, DeleteBoardInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	err = DeleteBoard(context.Background(), client, DeleteBoardInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
}

// ---------------------------------------------------------------------------
// Board List CRUD tests
// ---------------------------------------------------------------------------.

// TestListBoardLists_Success verifies the behavior of list board lists success.
func TestListBoardLists_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, boardListsArrayJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBoardLists(context.Background(), client, ListBoardListsInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Lists) != 1 {
		t.Fatalf("expected 1 list, got %d", len(out.Lists))
	}
	if out.Lists[0].LabelName != "To Do" {
		t.Errorf("expected label To Do, got %s", out.Lists[0].LabelName)
	}
}

// TestListBoardLists_MissingParams verifies the behavior of list board lists missing params.
func TestListBoardLists_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListBoardLists(context.Background(), client, ListBoardListsInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = ListBoardLists(context.Background(), client, ListBoardListsInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
}

// TestGetBoardList_Success verifies the behavior of get board list success.
func TestGetBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoardList100, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardListItemJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetBoardList(context.Background(), client, GetBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1, ListID: 100,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf(fmtExpectedID100, out.ID)
	}
}

// TestGetBoardList_MissingParams verifies the behavior of get board list missing params.
func TestGetBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetBoardList(context.Background(), client, GetBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = GetBoardList(context.Background(), client, GetBoardListInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
	_, err = GetBoardList(context.Background(), client, GetBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), errListIDRequired) {
		t.Fatalf(fmtExpectedListIDReq, err)
	}
}

// TestCreateBoardList_Success verifies the behavior of create board list success.
func TestCreateBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, boardListItemJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateBoardList(context.Background(), client, CreateBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1, LabelID: 20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf(fmtExpectedID100, out.ID)
	}
}

// TestCreateBoardList_MissingParams verifies the behavior of create board list missing params.
func TestCreateBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateBoardList(context.Background(), client, CreateBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = CreateBoardList(context.Background(), client, CreateBoardListInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
}

// TestUpdateBoardList_Success verifies the behavior of update board list success.
func TestUpdateBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoardList100, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, boardListItemJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateBoardList(context.Background(), client, UpdateBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1, ListID: 100, Position: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf(fmtExpectedID100, out.ID)
	}
}

// TestUpdateBoardList_MissingParams verifies the behavior of update board list missing params.
func TestUpdateBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateBoardList(context.Background(), client, UpdateBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	_, err = UpdateBoardList(context.Background(), client, UpdateBoardListInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
	_, err = UpdateBoardList(context.Background(), client, UpdateBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), errListIDRequired) {
		t.Fatalf(fmtExpectedListIDReq, err)
	}
}

// TestDeleteBoardList_Success verifies the behavior of delete board list success.
func TestDeleteBoardList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoardList100, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteBoardList(context.Background(), client, DeleteBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1, ListID: 100,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteBoardList_MissingParams verifies the behavior of delete board list missing params.
func TestDeleteBoardList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteBoardList(context.Background(), client, DeleteBoardListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDReq, err)
	}
	err = DeleteBoardList(context.Background(), client, DeleteBoardListInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errBoardIDRequired) {
		t.Fatalf(fmtExpectedBoardIDReq, err)
	}
	err = DeleteBoardList(context.Background(), client, DeleteBoardListInput{
		ProjectID: toolutil.StringOrInt("10"), BoardID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), errListIDRequired) {
		t.Fatalf(fmtExpectedListIDReq, err)
	}
}

// ---------------------------------------------------------------------------
// Formatter tests
// ---------------------------------------------------------------------------.

// TestFormatBoardMarkdown verifies the behavior of format board markdown.
func TestFormatBoardMarkdown(t *testing.T) {
	out := BoardOutput{
		ID: 1, Name: "Dev", ProjectName: "P", ProjectPath: "group/p", ProjectID: 10,
		MilestoneTitle: "v1", MilestoneID: 5,
		AssigneeUser: "alice", AssigneeID: 3,
		Labels: []string{"bug"}, HideBacklogList: false, HideClosedList: true,
		Lists: []BoardListOutput{{ID: 100, LabelName: "To Do", Position: 0}},
	}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "Dev") || !strings.Contains(md, "To Do") {
		t.Errorf(fmtMDMissingContent, md)
	}
	// No redundant numeric IDs in prose
	if strings.Contains(md, "(ID:") {
		t.Errorf("markdown should not contain redundant (ID:) patterns: %s", md)
	}
	// Project path used instead of name
	if !strings.Contains(md, "group/p") {
		t.Errorf("expected project path in markdown: %s", md)
	}
}

// TestFormatListBoardsMarkdown verifies the behavior of format list boards markdown.
func TestFormatListBoardsMarkdown(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{{ID: 1, Name: "Dev", ProjectPath: "group/dev"}},
	}
	md := FormatListBoardsMarkdown(out)
	if !strings.Contains(md, "Dev") {
		t.Errorf(fmtMDMissingContent, md)
	}
	// Table should show project path, not numeric ID
	if !strings.Contains(md, "group/dev") {
		t.Errorf("expected project path in table: %s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Errorf("table should not have ID column: %s", md)
	}
}

// TestFormatBoardListMarkdown verifies the behavior of format board list markdown.
func TestFormatBoardListMarkdown(t *testing.T) {
	out := BoardListOutput{ID: 100, LabelName: "To Do", Position: 0, MaxIssueCount: 10}
	md := FormatBoardListMarkdown(out)
	if !strings.Contains(md, "To Do") {
		t.Errorf(fmtMDMissingContent, md)
	}
	// Heading uses label name, not (ID: N)
	if strings.Contains(md, "(ID:") {
		t.Errorf("markdown should not contain redundant (ID:) patterns: %s", md)
	}
	if !strings.Contains(md, "## Board List: To Do") {
		t.Errorf("heading should use label name: %s", md)
	}
}

// ---------------------------------------------------------------------------
// Comprehensive markdown formatter tests
// ---------------------------------------------------------------------------.

// TestFormatBoardMarkdown_NoProject verifies the behavior of format board markdown no project.
func TestFormatBoardMarkdown_NoProject(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Board"}
	md := FormatBoardMarkdown(out)
	if strings.Contains(md, "**Project**") {
		t.Errorf("should not show project when empty: %s", md)
	}
}

// TestFormatBoardMarkdown_ProjectNameFallback verifies the behavior of format board markdown project name fallback.
func TestFormatBoardMarkdown_ProjectNameFallback(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Board", ProjectName: "MyProject", ProjectID: 5}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "MyProject") {
		t.Errorf("should fall back to project name: %s", md)
	}
}

// TestFormatBoardMarkdown_ListWithoutLabel verifies the behavior of format board markdown list without label.
func TestFormatBoardMarkdown_ListWithoutLabel(t *testing.T) {
	out := BoardOutput{
		ID: 1, Name: "Board",
		Lists: []BoardListOutput{{ID: 50, Position: 1}},
	}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "#50") {
		t.Errorf("list without label should show #ID fallback: %s", md)
	}
}

// TestFormatListBoardsMarkdown_FallbackToName verifies the behavior of format list boards markdown fallback to name.
func TestFormatListBoardsMarkdown_FallbackToName(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{{ID: 1, Name: "Dev", ProjectName: "MyProject"}},
	}
	md := FormatListBoardsMarkdown(out)
	if !strings.Contains(md, "MyProject") {
		t.Errorf("should fall back to project name: %s", md)
	}
}

// TestFormatBoardListMarkdown_NoLabelFallback verifies the behavior of format board list markdown no label fallback.
func TestFormatBoardListMarkdown_NoLabelFallback(t *testing.T) {
	out := BoardListOutput{ID: 200, Position: 3}
	md := FormatBoardListMarkdown(out)
	if !strings.Contains(md, "## Board List #200") {
		t.Errorf("heading should fall back to #ID when no label: %s", md)
	}
}

// TestFormatListBoardListsMarkdown_NoLabelFallback verifies the behavior of format list board lists markdown no label fallback.
func TestFormatListBoardListsMarkdown_NoLabelFallback(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{{ID: 300, Position: 0}},
	}
	md := FormatListBoardListsMarkdown(out)
	if !strings.Contains(md, "#300") {
		t.Errorf("list row without label should show #ID fallback: %s", md)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// JSON fixtures
// ---------------------------------------------------------------------------.

const (
	errExpectedErr          = "expected error"
	errExpCancelledCtx      = "expected error for canceled context"
	covBoardMinimalJSON     = `{"id":2,"name":"Minimal","hide_backlog_list":false,"hide_closed_list":false}`
	covBoardListMinimalJSON = `{"id":200,"position":1}`
)

// ---------------------------------------------------------------------------
// Board CRUD — server errors & canceled contexts
// ---------------------------------------------------------------------------.

// TestListBoards_ServerError verifies the behavior of cov list boards server error.
func TestListBoards_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := ListBoards(context.Background(), client, ListBoardsInput{ProjectID: "10"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListBoards_CancelledContext verifies the behavior of cov list boards cancelled context.
func TestListBoards_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListBoards(ctx, client, ListBoardsInput{ProjectID: "10"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListBoards_WithPagination verifies the behavior of cov list boards with pagination.
func TestListBoards_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covBoardMinimalJSON+`]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
	}))
	out, err := ListBoards(context.Background(), client, ListBoardsInput{
		ProjectID:       "10",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestGetBoard_ServerError verifies the behavior of cov get board server error.
func TestGetBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetBoard_CancelledContext verifies the behavior of cov get board cancelled context.
func TestGetBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetBoard(ctx, client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateBoard_ServerError verifies the behavior of cov create board server error.
func TestCreateBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := CreateBoard(context.Background(), client, CreateBoardInput{ProjectID: "10", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateBoard_CancelledContext verifies the behavior of cov create board cancelled context.
func TestCreateBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateBoard(ctx, client, CreateBoardInput{ProjectID: "10", Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateBoard_AllOptionalFields verifies the behavior of cov update board all optional fields.
func TestUpdateBoard_AllOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	hideTrue := true
	hideFalse := false
	_, err := UpdateBoard(context.Background(), client, UpdateBoardInput{
		ProjectID:       "10",
		BoardID:         1,
		Name:            "Updated",
		AssigneeID:      3,
		MilestoneID:     5,
		Labels:          "bug,feature",
		Weight:          2,
		HideBacklogList: &hideTrue,
		HideClosedList:  &hideFalse,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdateBoard_ServerError verifies the behavior of cov update board server error.
func TestUpdateBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := UpdateBoard(context.Background(), client, UpdateBoardInput{ProjectID: "10", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateBoard_CancelledContext verifies the behavior of cov update board cancelled context.
func TestUpdateBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := UpdateBoard(ctx, client, UpdateBoardInput{ProjectID: "10", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBoard_ServerError verifies the behavior of cov delete board server error.
func TestDeleteBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	err := DeleteBoard(context.Background(), client, DeleteBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteBoard_CancelledContext verifies the behavior of cov delete board cancelled context.
func TestDeleteBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteBoard(ctx, client, DeleteBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Board List CRUD — server errors & canceled contexts
// ---------------------------------------------------------------------------.

// TestListBoardLists_ServerError verifies the behavior of cov list board lists server error.
func TestListBoardLists_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := ListBoardLists(context.Background(), client, ListBoardListsInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListBoardLists_CancelledContext verifies the behavior of cov list board lists cancelled context.
func TestListBoardLists_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListBoardLists(ctx, client, ListBoardListsInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetBoardList_ServerError verifies the behavior of cov get board list server error.
func TestGetBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := GetBoardList(context.Background(), client, GetBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetBoardList_CancelledContext verifies the behavior of cov get board list cancelled context.
func TestGetBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetBoardList(ctx, client, GetBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateBoardList_AllTypes verifies the behavior of cov create board list all types.
func TestCreateBoardList_AllTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, boardListItemJSON)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := CreateBoardList(context.Background(), client, CreateBoardListInput{
		ProjectID:   "10",
		BoardID:     1,
		AssigneeID:  3,
		MilestoneID: 5,
		IterationID: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreateBoardList_ServerError verifies the behavior of cov create board list server error.
func TestCreateBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := CreateBoardList(context.Background(), client, CreateBoardListInput{ProjectID: "10", BoardID: 1, LabelID: 20})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateBoardList_CancelledContext verifies the behavior of cov create board list cancelled context.
func TestCreateBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateBoardList(ctx, client, CreateBoardListInput{ProjectID: "10", BoardID: 1, LabelID: 20})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateBoardList_ServerError verifies the behavior of cov update board list server error.
func TestUpdateBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := UpdateBoardList(context.Background(), client, UpdateBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100, Position: 2})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateBoardList_CancelledContext verifies the behavior of cov update board list cancelled context.
func TestUpdateBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := UpdateBoardList(ctx, client, UpdateBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100, Position: 2})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBoardList_ServerError verifies the behavior of cov delete board list server error.
func TestDeleteBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	err := DeleteBoardList(context.Background(), client, DeleteBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteBoardList_CancelledContext verifies the behavior of cov delete board list cancelled context.
func TestDeleteBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteBoardList(ctx, client, DeleteBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional coverage
// ---------------------------------------------------------------------------.

// TestFormatBoardMarkdown_Minimal verifies the behavior of cov format board markdown minimal.
func TestFormatBoardMarkdown_Minimal(t *testing.T) {
	out := BoardOutput{ID: 2, Name: "Minimal"}
	md := FormatBoardMarkdown(out)
	if strings.Contains(md, "**Project**") {
		t.Error("minimal board should not show Project")
	}
	if strings.Contains(md, "**Milestone**") {
		t.Error("minimal board should not show Milestone")
	}
	if strings.Contains(md, "**Assignee**") {
		t.Error("minimal board should not show Assignee")
	}
	if strings.Contains(md, "Weight") {
		t.Error("minimal board should not show Weight")
	}
	if strings.Contains(md, "Labels") {
		t.Error("minimal board should not show Labels")
	}
	if strings.Contains(md, "### Lists") {
		t.Error("minimal board should not show Lists section")
	}
	if !strings.Contains(md, "Minimal") {
		t.Error("missing board name")
	}
	if strings.Contains(md, "(ID:") {
		t.Error("should not contain redundant (ID:) patterns")
	}
}

// TestFormatBoardMarkdown_WithWeight verifies the behavior of cov format board markdown with weight.
func TestFormatBoardMarkdown_WithWeight(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Dev", Weight: 5}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "Weight") {
		t.Errorf("expected Weight in:\n%s", md)
	}
}

// TestFormatListBoardListsMarkdown verifies the behavior of cov format list board lists markdown.
func TestFormatListBoardListsMarkdown(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{
			{ID: 100, LabelName: "To Do", Position: 0, MaxIssueCount: 10, MaxIssueWeight: 50},
			{ID: 101, LabelName: "Doing", Position: 1},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatListBoardListsMarkdown(out)
	if !strings.Contains(md, "To Do") {
		t.Errorf("missing list label:\n%s", md)
	}
	if !strings.Contains(md, "Doing") {
		t.Errorf("missing second list:\n%s", md)
	}
	if !strings.Contains(md, "| Label |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatListBoardListsMarkdown_Empty verifies the behavior of cov format list board lists markdown empty.
func TestFormatListBoardListsMarkdown_Empty(t *testing.T) {
	out := ListBoardListsOutput{}
	md := FormatListBoardListsMarkdown(out)
	if !strings.Contains(md, "Board Lists") {
		t.Errorf("missing header:\n%s", md)
	}
}

// TestFormatBoardListMarkdown_AllFields verifies the behavior of cov format board list markdown all fields.
func TestFormatBoardListMarkdown_AllFields(t *testing.T) {
	out := BoardListOutput{
		ID: 100, LabelName: "To Do", LabelID: 20, Position: 0,
		MaxIssueCount: 10, MaxIssueWeight: 50,
		AssigneeUser: "alice", AssigneeID: 3,
		MilestoneTitle: "v1.0", MilestoneID: 5,
	}
	md := FormatBoardListMarkdown(out)
	for _, want := range []string{"To Do", "Max Issue Count", "Max Issue Weight", "alice", "v1.0"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatBoardListMarkdown missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "(ID:") {
		t.Errorf("should not contain redundant (ID:) patterns:\n%s", md)
	}
}

// TestFormatBoardListMarkdown_Minimal verifies the behavior of cov format board list markdown minimal.
func TestFormatBoardListMarkdown_Minimal(t *testing.T) {
	out := BoardListOutput{ID: 200, Position: 1}
	md := FormatBoardListMarkdown(out)
	if strings.Contains(md, "**Label**") {
		t.Error("minimal list should not show Label")
	}
	if strings.Contains(md, "Max Issue") {
		t.Error("minimal list should not show Max Issue")
	}
	if strings.Contains(md, "Assignee") {
		t.Error("minimal list should not show Assignee")
	}
	if strings.Contains(md, "Milestone") {
		t.Error("minimal list should not show Milestone")
	}
	if !strings.Contains(md, "#200") {
		t.Error("minimal list should show #ID fallback in heading")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools & RegisterMeta — no panic + MCP round-trip
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// newBoardMux is an internal helper for the boards package.
func newBoardMux() *http.ServeMux {
	const boardPath = "/api/v4/projects/10/boards"
	mux := http.NewServeMux()
	mux.HandleFunc(boardPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+boardJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, boardJSON)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(boardPath+"/1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, boardJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, boardJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(boardPath+"/1/lists", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, boardListsArrayJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, boardListItemJSON)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(boardPath+"/1/lists/100", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, boardListItemJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, boardListItemJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	client := testutil.NewTestClient(t, newBoardMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_board_list", map[string]any{"project_id": "10"}},
		{"gitlab_board_get", map[string]any{"project_id": "10", "board_id": float64(1)}},
		{"gitlab_board_create", map[string]any{"project_id": "10", "name": "Test"}},
		{"gitlab_board_update", map[string]any{"project_id": "10", "board_id": float64(1), "name": "Updated"}},
		{"gitlab_board_delete", map[string]any{"project_id": "10", "board_id": float64(1)}},
		{"gitlab_board_list_lists", map[string]any{"project_id": "10", "board_id": float64(1)}},
		{"gitlab_board_list_get", map[string]any{"project_id": "10", "board_id": float64(1), "list_id": float64(100)}},
		{"gitlab_board_list_create", map[string]any{"project_id": "10", "board_id": float64(1), "label_id": float64(20)}},
		{"gitlab_board_list_update", map[string]any{"project_id": "10", "board_id": float64(1), "list_id": float64(100), "position": float64(2)}},
		{"gitlab_board_list_delete", map[string]any{"project_id": "10", "board_id": float64(1), "list_id": float64(100)}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s): nil result", tc.name)
			}
		})
	}
}
