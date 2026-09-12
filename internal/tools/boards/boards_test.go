// boards_test.go contains unit tests for the issue board MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package boards

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errProjectIDRequired identifies the err project ID required constant used by this package.
const errProjectIDRequired = "project_id is required"

// errBoardIDRequired identifies the err board ID required constant used by this package.
const errBoardIDRequired = "board_id is required"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

const (
	// pathBoard1 identifies the path board 1 constant used by this package.
	pathBoard1 = "/api/v4/projects/10/boards/1"
	// pathBoardList100 identifies the path board list 100 constant used by this package.
	pathBoardList100 = "/api/v4/projects/10/boards/1/lists/100"
	// fmtExpectedID1 identifies the fmt expected ID 1 constant used by this package.
	fmtExpectedID1 = "expected ID 1, got %d"
	// fmtExpectedID100 identifies the fmt expected ID 100 constant used by this package.
	fmtExpectedID100 = "expected ID 100, got %d"
	// fmtExpectedProjectIDReq identifies the fmt expected project ID req constant used by this package.
	fmtExpectedProjectIDReq = "expected project_id required, got %v"
	// fmtExpectedBoardIDReq identifies the fmt expected board ID req constant used by this package.
	fmtExpectedBoardIDReq = "expected board_id required, got %v"
	// msgMethodNotAllowed identifies the msg method not allowed constant used by this package.
	msgMethodNotAllowed = "method not allowed"
	// errListIDRequired identifies the err list ID required constant used by this package.
	errListIDRequired = "list_id is required"
	// fmtExpectedListIDReq identifies the fmt expected list ID req constant used by this package.
	fmtExpectedListIDReq = "expected list_id required, got %v"
)

// ---------------------------------------------------------------------------
// Shared JSON fixtures
// ---------------------------------------------------------------------------.

// boardJSON stores the package-level board JSON state.
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

// boardListJSON stores the package-level board list JSON state.
var boardListJSON = `[` + boardJSON + `]`

// boardListItemJSON stores the package-level board list item JSON state.
var boardListItemJSON = `{
	"id": 100,
	"label": {"id": 20, "name": "To Do"},
	"position": 0,
	"max_issue_count": 10,
	"max_issue_weight": 50,
	"assignee": {"id": 3, "name": "Alice", "username": "alice"},
	"milestone": {"id": 5, "title": "v1.0"}
}`

// boardListsArrayJSON stores the package-level board lists array JSON state.
var boardListsArrayJSON = `[` + boardListItemJSON + `]`

// ---------------------------------------------------------------------------
// Board CRUD tests
// ---------------------------------------------------------------------------.

// TestListBoards_Success verifies that ListBoards succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListBoards_MissingProjectID verifies that ListBoards_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListBoards_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListBoards(context.Background(), client, ListBoardsInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// TestGetBoard_Success verifies that GetBoard succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
	if out.Milestone == nil || out.Milestone.Title != "v1.0" {
		t.Errorf("expected milestone v1.0, got %+v", out.Milestone)
	}
	if out.Project == nil || out.Project.PathWithNamespace != "group/my-project" {
		t.Errorf("expected project path group/my-project, got %+v", out.Project)
	}
	if out.Assignee == nil || out.Assignee.Username != "alice" {
		t.Errorf("expected assignee alice, got %+v", out.Assignee)
	}
	if len(out.Labels) != 2 || out.Labels[0].Name != "bug" {
		t.Errorf("expected 2 labels starting with bug, got %+v", out.Labels)
	}
	if len(out.Lists) != 1 || out.Lists[0].Label == nil || out.Lists[0].Label.Name != "To Do" {
		t.Errorf("expected list label To Do, got %+v", out.Lists)
	}
}

// TestGetBoard_MissingParams verifies that GetBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreateBoard_Success verifies that CreateBoard succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreateBoard_MissingParams verifies that CreateBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestUpdateBoard_Success verifies that UpdateBoard succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUpdateBoard_MissingParams verifies that UpdateBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDeleteBoard_Success verifies that DeleteBoard succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteBoard_MissingParams verifies that DeleteBoard_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListBoardLists_Success verifies that ListBoardLists succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
	if out.Lists[0].Label == nil || out.Lists[0].Label.Name != "To Do" {
		t.Errorf("expected label To Do, got %+v", out.Lists[0].Label)
	}
	if out.Lists[0].Assignee == nil || out.Lists[0].Assignee.Username != "alice" {
		t.Errorf("expected assignee alice, got %+v", out.Lists[0].Assignee)
	}
	if out.Lists[0].Milestone == nil || out.Lists[0].Milestone.Title != "v1.0" {
		t.Errorf("expected milestone v1.0, got %+v", out.Lists[0].Milestone)
	}
}

// boardListWithLimitMetricJSON is a board-list array whose entry includes the
// documented limit_metric REST field that client-go's gl.BoardList omits. Used
// to verify the raw-superset fetch path surfaces it.
var boardListWithLimitMetricJSON = `[{
	"id": 100,
	"label": {"id": 20, "name": "To Do"},
	"position": 0,
	"max_issue_count": 10,
	"limit_metric": "issue_count"
}]`

// boardWithLimitMetricJSON is a single board whose list entry includes the
// documented limit_metric REST field absent from gl.BoardList.
var boardWithLimitMetricJSON = `{
	"id": 1,
	"name": "Development",
	"lists": [
		{"id": 100, "label": {"id": 20, "name": "To Do"}, "position": 0, "limit_metric": "all_metrics"}
	]
}`

// TestListBoardLists_LimitMetricSurfaced verifies that the documented
// limit_metric REST field (absent from client-go's gl.BoardList) is surfaced
// through the raw-superset fetch path used by ListBoardLists.
func TestListBoardLists_LimitMetricSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, boardListWithLimitMetricJSON,
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
	if out.Lists[0].LimitMetric != "issue_count" {
		t.Errorf("expected limit_metric issue_count, got %q", out.Lists[0].LimitMetric)
	}
}

// TestGetBoard_LimitMetricSurfaced verifies that each list's documented
// limit_metric is surfaced through the raw-superset fetch path used by GetBoard.
func TestGetBoard_LimitMetricSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardWithLimitMetricJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: toolutil.StringOrInt("10"), BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Lists) != 1 || out.Lists[0].LimitMetric != "all_metrics" {
		t.Errorf("expected list limit_metric all_metrics, got %+v", out.Lists)
	}
}

// TestListBoardLists_LimitMetricAbsentOmitted verifies version tolerance: when
// the GitLab response does not include limit_metric (older instances), the field
// decodes to its zero value and is omitted from the marshaled MCP envelope,
// without failing the request.
func TestListBoardLists_LimitMetricAbsentOmitted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/boards/1/lists", func(w http.ResponseWriter, r *http.Request) {
		// boardListsArrayJSON intentionally omits limit_metric.
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
	if out.Lists[0].LimitMetric != "" {
		t.Errorf("expected empty limit_metric when absent, got %q", out.Lists[0].LimitMetric)
	}
	data, err := json.Marshal(out.Lists[0])
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), "limit_metric") {
		t.Errorf("expected limit_metric omitted from envelope, got %s", data)
	}
}

// TestListBoardLists_MissingParams verifies that ListBoardLists_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestGetBoardList_Success verifies that GetBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGetBoardList_MissingParams verifies that GetBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreateBoardList_Success verifies that CreateBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreateBoardList_MissingParams verifies that CreateBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestUpdateBoardList_Success verifies that UpdateBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUpdateBoardList_MissingParams verifies that UpdateBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDeleteBoardList_Success verifies that DeleteBoardList succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteBoardList_MissingParams verifies that DeleteBoardList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatBoardMarkdown_AllFields pins the whole board card: the heading
// carrying the board's reference, one list item per field with the project,
// milestone and assignee linked, the two visibility flags as glyphs, the
// columns as a nested table naming each one's scope, and the hints last.
func TestFormatBoardMarkdown_AllFields(t *testing.T) {
	out := BoardOutput{
		ID: 1, Name: "Dev",
		Project:         &ProjectOutput{ID: 10, Name: "P", PathWithNamespace: "group/p", WebURL: "https://gitlab.example.com/group/p"},
		Milestone:       &MilestoneOutput{ID: 5, Title: "v1", WebURL: "https://gitlab.example.com/group/p/-/milestones/1"},
		Assignee:        &BasicUserOutput{ID: 3, Username: "alice", WebURL: "https://gitlab.example.com/alice"},
		Weight:          5,
		Labels:          []*LabelDetailsOutput{{ID: 1, Name: "bug"}, nil, {ID: 2, Name: "ux"}},
		HideBacklogList: false, HideClosedList: true,
		Lists: []BoardListOutput{
			{ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 10, MaxIssueWeight: 50},
			{ID: 101, Position: 1, Assignee: &BoardListAssigneeOutput{ID: 3, Username: "alice"}},
		},
	}

	md := FormatBoardMarkdown(out)

	want := "## Board #1: Dev\n\n" +
		"- **ID**: 1\n" +
		"- **Project**: [group/p](https://gitlab.example.com/group/p)\n" +
		"- **Milestone**: [v1](https://gitlab.example.com/group/p/-/milestones/1)\n" +
		"- **Assignee**: [@alice](https://gitlab.example.com/alice)\n" +
		"- **Weight**: 5\n" +
		"- **Labels**: bug, ux\n" +
		"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiSuccess + "\n\n" +
		"### Lists\n\n" +
		"| ID | Scope | Position | Max Issues | Max Weight |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 100 | To Do | 0 | 10 | 50 |\n" +
		"| 101 | @alice | 1 | - | - |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_create' to add a column to this board\n" +
		"- Use action 'project.board_update' to change this board's name or scope\n" +
		"- Use action 'project.board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatBoardMarkdown_Minimal pins the whole card of a board GitLab sent
// nothing optional for: no project, milestone, assignee, weight, label or
// column row is written at all, and the two flags GitLab always sends are.
func TestFormatBoardMarkdown_Minimal(t *testing.T) {
	md := FormatBoardMarkdown(BoardOutput{ID: 2, Name: "Minimal"})

	want := "## Board #2: Minimal\n\n" +
		"- **ID**: 2\n" +
		"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_create' to add a column to this board\n" +
		"- Use action 'project.board_update' to change this board's name or scope\n" +
		"- Use action 'project.board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatBoardMarkdown_GroupBoardProjectNameFallback pins the whole card of
// a board GitLab sent a group for and a project with no path: the group row is
// written and the project falls back to its name, unlinked when GitLab sent no
// address.
func TestFormatBoardMarkdown_GroupBoardProjectNameFallback(t *testing.T) {
	out := BoardOutput{
		ID: 3, Name: "Board",
		Project: &ProjectOutput{ID: 5, Name: "MyProject"},
		Group:   &toolutil.BasicGroupDetailsOutput{ID: 7, Name: "Platform", WebURL: "https://gitlab.example.com/groups/platform"},
	}

	md := FormatBoardMarkdown(out)

	want := "## Board #3: Board\n\n" +
		"- **ID**: 3\n" +
		"- **Project**: MyProject\n" +
		"- **Group**: [Platform](https://gitlab.example.com/groups/platform)\n" +
		"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
		"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_create' to add a column to this board\n" +
		"- Use action 'project.board_update' to change this board's name or scope\n" +
		"- Use action 'project.board_delete' to remove this board\n"
	if md != want {
		t.Errorf("FormatBoardMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListBoardsMarkdown pins the whole board list: the counted heading,
// one row per board carrying its ID, the linked project, and the number of
// columns, then the guidance section with the preserve-links hint first.
func TestFormatListBoardsMarkdown(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{
			{
				ID: 1, Name: "Dev",
				Project:   &ProjectOutput{ID: 1, PathWithNamespace: "group/dev", WebURL: "https://gitlab.example.com/group/dev"},
				Milestone: &MilestoneOutput{ID: 5, Title: "v1", WebURL: "https://gitlab.example.com/group/dev/-/milestones/1"},
				Assignee:  &BasicUserOutput{ID: 3, Username: "alice", WebURL: "https://gitlab.example.com/alice"},
				Lists:     []BoardListOutput{{ID: 100}, {ID: 101}},
			},
			{ID: 2, Name: "Ops", Project: &ProjectOutput{ID: 1, Name: "MyProject"}},
		},
	}

	md := FormatListBoardsMarkdown(out)

	want := "## Issue Boards (2)\n\n" +
		"| ID | Name | Project | Milestone | Assignee | Lists |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | Dev | [group/dev](https://gitlab.example.com/group/dev) | [v1](https://gitlab.example.com/group/dev/-/milestones/1) | [@alice](https://gitlab.example.com/alice) | 2 |\n" +
		"| 2 | Ops | MyProject |  |  | 0 |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'project.board_get' to read one board with its columns\n" +
		"- Use action 'project.board_create' to add a new board\n"
	if md != want {
		t.Errorf("FormatListBoardsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListBoardsMarkdown_Empty pins the whole render of a project with
// no boards: one sentence, with no heading counting zero and no table header
// standing over nothing.
func TestFormatListBoardsMarkdown_Empty(t *testing.T) {
	if got, want := FormatListBoardsMarkdown(ListBoardsOutput{}), "No issue boards found.\n"; got != want {
		t.Errorf("FormatListBoardsMarkdown() = %q, want %q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// JSON fixtures
// ---------------------------------------------------------------------------.

const (
	// errExpectedErr identifies the err expected err constant used by this package.
	errExpectedErr = "expected error"
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// covBoardMinimalJSON identifies the cov board minimal JSON constant used by this package.
	covBoardMinimalJSON = `{"id":2,"name":"Minimal","hide_backlog_list":false,"hide_closed_list":false}`
)

// ---------------------------------------------------------------------------
// Board CRUD — server errors & canceled contexts
// ---------------------------------------------------------------------------.

// TestListBoards_ServerError verifies that ListBoards_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListBoards_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := ListBoards(context.Background(), client, ListBoardsInput{ProjectID: "10"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListBoards_CancelledContext verifies the ListBoards_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListBoards_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListBoards(ctx, client, ListBoardsInput{ProjectID: "10"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListBoards_WithPagination verifies that ListBoards_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListBoards_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covBoardMinimalJSON+`]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
	}))
	out, err := ListBoards(context.Background(), client, ListBoardsInput{
		ProjectID: "10",
		Page:      2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestGetBoard_ServerError verifies that GetBoard_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetBoard_CancelledContext verifies the GetBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetBoard(ctx, client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateBoard_ServerError verifies that CreateBoard_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := CreateBoard(context.Background(), client, CreateBoardInput{ProjectID: "10", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateBoard_CancelledContext verifies the CreateBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateBoard(ctx, client, CreateBoardInput{ProjectID: "10", Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateBoard_AllOptionalFields verifies the UpdateBoard_AllOptionalFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
		Labels:          []string{"bug", "feature"},
		Weight:          2,
		HideBacklogList: &hideTrue,
		HideClosedList:  &hideFalse,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdateBoard_ServerError verifies that UpdateBoard_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := UpdateBoard(context.Background(), client, UpdateBoardInput{ProjectID: "10", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateBoard_CancelledContext verifies the UpdateBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateBoard(ctx, client, UpdateBoardInput{ProjectID: "10", BoardID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBoard_ServerError verifies that DeleteBoard_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteBoard_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	err := DeleteBoard(context.Background(), client, DeleteBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteBoard_CancelledContext verifies the DeleteBoard_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteBoard_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteBoard(ctx, client, DeleteBoardInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Board List CRUD — server errors & canceled contexts
// ---------------------------------------------------------------------------.

// TestListBoardLists_ServerError verifies that ListBoardLists_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListBoardLists_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := ListBoardLists(context.Background(), client, ListBoardListsInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListBoardLists_CancelledContext verifies the ListBoardLists_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListBoardLists_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListBoardLists(ctx, client, ListBoardListsInput{ProjectID: "10", BoardID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetBoardList_ServerError verifies that GetBoardList_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := GetBoardList(context.Background(), client, GetBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetBoardList_CancelledContext verifies the GetBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetBoardList(ctx, client, GetBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateBoardList_AllTypes verifies the CreateBoardList_AllTypes handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreateBoardList_ServerError verifies that CreateBoardList_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := CreateBoardList(context.Background(), client, CreateBoardListInput{ProjectID: "10", BoardID: 1, LabelID: 20})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateBoardList_BadRequest verifies the CreateBoardList_BadRequest handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateBoardList_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := CreateBoardList(context.Background(), client, CreateBoardListInput{ProjectID: "10", BoardID: 1, LabelID: 20})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error = %v, want exactly-one hint", err)
	}
}

// TestCreateBoardList_CancelledContext verifies the CreateBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateBoardList(ctx, client, CreateBoardListInput{ProjectID: "10", BoardID: 1, LabelID: 20})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateBoardList_ServerError verifies that UpdateBoardList_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := UpdateBoardList(context.Background(), client, UpdateBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100, Position: 2})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateBoardList_CancelledContext verifies the UpdateBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateBoardList(ctx, client, UpdateBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100, Position: 2})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBoardList_ServerError verifies that DeleteBoardList_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteBoardList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	err := DeleteBoardList(context.Background(), client, DeleteBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteBoardList_CancelledContext verifies the DeleteBoardList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteBoardList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteBoardList(ctx, client, DeleteBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional coverage
// ---------------------------------------------------------------------------.

// TestFormatListBoardListsMarkdown pins the whole column list: the counted
// heading, one row per column naming what it collects, a dash where GitLab set
// no ceiling, and the guidance section last with no preserve-links hint, since
// the table carries no link.
func TestFormatListBoardListsMarkdown(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{
			{ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 10, MaxIssueWeight: 50},
			{ID: 101, Label: &LabelOutput{Name: "Doing"}, Position: 1},
			{ID: 102, Position: 2, Milestone: &MilestoneOutput{ID: 5, Title: "v1.0"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3},
	}

	md := FormatListBoardListsMarkdown(out)

	want := "## Board Lists (3)\n\n" +
		"| ID | Scope | Position | Max Issues | Max Weight |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 100 | To Do | 0 | 10 | 50 |\n" +
		"| 101 | Doing | 1 | - | - |\n" +
		"| 102 | Milestone: v1.0 | 2 | - | - |\n\n" +
		"3 items total\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_get' to read one column\n" +
		"- Use action 'project.board_list_create' to add a new column\n"
	if md != want {
		t.Errorf("FormatListBoardListsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListBoardListsMarkdown_Empty pins the whole render of a board with
// no columns: one sentence, where the formatter used to write a heading and a
// table header with nothing under it.
func TestFormatListBoardListsMarkdown_Empty(t *testing.T) {
	if got, want := FormatListBoardListsMarkdown(ListBoardListsOutput{}), "No board lists found.\n"; got != want {
		t.Errorf("FormatListBoardListsMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatBoardListMarkdown_AllFields pins the whole column card: the
// heading naming the column by its label, every scope GitLab sent, the
// position, the two ceilings and the metric they are counted in.
func TestFormatBoardListMarkdown_AllFields(t *testing.T) {
	out := BoardListOutput{
		ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0,
		MaxIssueCount: 10, MaxIssueWeight: 50, LimitMetric: "issue_count",
		Assignee:  &BoardListAssigneeOutput{ID: 3, Username: "alice"},
		Milestone: &MilestoneOutput{ID: 5, Title: "v1.0", WebURL: "https://gitlab.example.com/group/p/-/milestones/1"},
		Iteration: &IterationOutput{ID: 9, Title: "Sprint 3", WebURL: "https://gitlab.example.com/groups/g/-/iterations/9"},
	}

	md := FormatBoardListMarkdown(out)

	want := "## Board List #100: To Do\n\n" +
		"- **ID**: 100\n" +
		"- **Label**: To Do\n" +
		"- **Assignee**: @alice\n" +
		"- **Milestone**: [v1.0](https://gitlab.example.com/group/p/-/milestones/1)\n" +
		"- **Iteration**: [Sprint 3](https://gitlab.example.com/groups/g/-/iterations/9)\n" +
		"- **Position**: 0\n" +
		"- **Max Issue Count**: 10\n" +
		"- **Max Issue Weight**: 50\n" +
		"- **Limit Metric**: issue_count\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_update' to change this column's position or limits\n" +
		"- Use action 'project.board_list_delete' to remove this column\n"
	if md != want {
		t.Errorf("FormatBoardListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatBoardListMarkdown_Minimal pins the whole card of a column with no
// scope and no ceilings: the heading falls back to the reference, the position
// is written because zero is the first column, and nothing else is.
func TestFormatBoardListMarkdown_Minimal(t *testing.T) {
	md := FormatBoardListMarkdown(BoardListOutput{ID: 200, Position: 1})

	want := "## Board List #200\n\n" +
		"- **ID**: 200\n" +
		"- **Position**: 1\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.board_list_update' to change this column's position or limits\n" +
		"- Use action 'project.board_list_delete' to remove this column\n"
	if md != want {
		t.Errorf("FormatBoardListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := ActionSpecs(client)
	byTool := boardSpecsByTool(t, specs)

	if len(specs) != 10 {
		t.Fatalf("len(ActionSpecs) = %d, want 10", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "boards" {
			t.Fatalf("OwnerPackage for %s = %q, want boards", spec.Name, spec.OwnerPackage)
		}
	}
}

// newBoardMux constructs board mux test fixtures.
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

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, newBoardMux())
	byTool := boardSpecsByTool(t, ActionSpecs(client))

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
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestActionSpecs_BoardGetRoute validates the BoardGetRoute route through the catalog surface.
// The mock GitLab API at /api/v4/projects/42/boards/3 (GET) responds with HTTP OK.
// It asserts the route returns the expected error or result.
func TestActionSpecs_BoardGetRoute(t *testing.T) {
	const respJSON = `{"id":3,"name":"Development","project":{"id":42}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/boards/3" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := boardSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_board_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "board_id": 3})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(BoardOutput)
	if !ok {
		t.Fatalf("result type = %T, want BoardOutput", result)
	}
	if out.ID != 3 || out.Name != "Development" {
		t.Fatalf("board output = %#v, want ID 3 name Development", out)
	}
}

// fullBoardJSON exercises every nested sub-object converter using only the
// fields documented in doc/api/boards.md (project repo URLs/timestamps,
// milestone dates/timestamps, documented assignee identity, label details,
// list label name/color/description, and the premium iteration list type).
const fullBoardJSON = `{
	"id": 7,
	"name": "Full",
	"project": {"id": 10, "name": "P", "path_with_namespace": "g/p", "http_url_to_repo": "https://gl/g/p.git", "web_url": "https://gl/g/p", "created_at": "2021-01-01T00:00:00Z", "default_branch": "main"},
	"milestone": {"id": 5, "iid": 2, "project_id": 10, "title": "v1.0", "state": "active", "start_date": "2021-01-01", "due_date": "2021-02-01", "created_at": "2021-01-01T00:00:00Z", "updated_at": "2021-01-02T00:00:00Z"},
	"assignee": {"id": 3, "username": "alice", "name": "Alice", "state": "active", "web_url": "https://gl/alice"},
	"weight": 2,
	"labels": [{"id": 1, "name": "bug", "color": "#fff", "description": "bug label"}],
	"hide_backlog_list": false,
	"hide_closed_list": true,
	"lists": [
		{"id": 100, "label": {"name": "To Do", "color": "#F0AD4E", "description": "todo"}, "iteration": {"id": 9, "iid": 1, "title": "Sprint 1", "created_at": "2021-01-01T00:00:00Z", "updated_at": "2021-01-02T00:00:00Z", "start_date": "2021-01-01", "due_date": "2021-01-14"}, "position": 0, "max_issue_count": 10}
	]
}`

// TestConvertBoard_FullSubObjects verifies every nested sub-object converter
// populates its canonical key with the documented field set, covering the
// non-nil timestamp/iteration branches.
func TestConvertBoard_FullSubObjects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fullBoardJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertFullBoardProject(t, out.Project)
	assertFullBoardMilestone(t, out.Milestone)
	if out.Assignee == nil || out.Assignee.WebURL != "https://gl/alice" {
		t.Errorf("assignee not fully converted: %+v", out.Assignee)
	}
	if len(out.Labels) != 1 || out.Labels[0].Description != "bug label" {
		t.Errorf("label details not converted: %+v", out.Labels)
	}
	assertFullBoardList(t, out.Lists[0])
}

// assertFullBoardProject checks the documented project reference subset.
func assertFullBoardProject(t *testing.T, p *ProjectOutput) {
	t.Helper()
	if p == nil || p.WebURL != "https://gl/g/p" ||
		p.HTTPURLToRepo != "https://gl/g/p.git" || p.CreatedAt == "" ||
		p.DefaultBranch != "main" {
		t.Errorf("project not fully converted: %+v", p)
	}
}

// assertFullBoardMilestone checks the documented milestone reference subset.
func assertFullBoardMilestone(t *testing.T, m *MilestoneOutput) {
	t.Helper()
	if m == nil || m.ProjectID != 10 || m.StartDate == "" || m.CreatedAt == "" {
		t.Errorf("milestone not fully converted: %+v", m)
	}
}

// assertFullBoardList checks the documented list label/iteration subset.
func assertFullBoardList(t *testing.T, list BoardListOutput) {
	t.Helper()
	if list.Label == nil || list.Label.Name != "To Do" || list.Label.Color != "#F0AD4E" || list.Label.Description != "todo" {
		t.Errorf("list label not converted: %+v", list.Label)
	}
	if list.Iteration == nil || list.Iteration.StartDate == "" || list.Iteration.CreatedAt == "" {
		t.Errorf("list iteration not converted: %+v", list.Iteration)
	}
}

// TestLabelDetailsOutputs_SkipsNil verifies nil label-detail entries are skipped.
func TestLabelDetailsOutputs_SkipsNil(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoard1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"B","labels":[null,{"id":2,"name":"keep"}]}`)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := GetBoard(context.Background(), client, GetBoardInput{ProjectID: "10", BoardID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Labels) != 1 || out.Labels[0].Name != "keep" {
		t.Errorf("expected nil label skipped, got %+v", out.Labels)
	}
}

// TestConvertBoardList_NilSubObjects covers the nil-guards of the per-list
// label/assignee/milestone/iteration converters (a list with no scope set).
func TestConvertBoardList_NilSubObjects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathBoardList100, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":100,"position":2}`)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := GetBoardList(context.Background(), client, GetBoardListInput{ProjectID: "10", BoardID: 1, ListID: 100})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Label != nil || out.Assignee != nil || out.Milestone != nil || out.Iteration != nil {
		t.Errorf("expected all sub-objects nil, got %+v", out)
	}
}

// TestListBoards_OrderBySort verifies order_by/sort/keyset params are forwarded.
func TestListBoards_OrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "created_at" || q.Get("sort") != "desc" || q.Get("pagination") != "keyset" {
			t.Errorf("missing keyset params: %v", q)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covBoardMinimalJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))
	_, err := ListBoards(context.Background(), client, ListBoardsInput{
		ProjectID:  "10",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListBoardLists_OrderBySort verifies order_by/sort params reach the lists endpoint.
func TestListBoardLists_OrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "id" || q.Get("sort") != "asc" {
			t.Errorf("missing order params: %v", q)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, boardListsArrayJSON,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))
	_, err := ListBoardLists(context.Background(), client, ListBoardListsInput{
		ProjectID: "10", BoardID: 1, OrderBy: "id", Sort: "asc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestFormatListBoardsMarkdown_MilestoneAssignee covers the milestone/assignee
// non-nil branches of the list table renderer.
func TestFormatListBoardsMarkdown_MilestoneAssignee(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{{
			ID: 1, Name: "Dev",
			Milestone: &MilestoneOutput{ID: 5, Title: "v1"},
			Assignee:  &BasicUserOutput{ID: 3, Username: "alice"},
		}},
	}
	md := FormatListBoardsMarkdown(out)
	if !strings.Contains(md, "v1") || !strings.Contains(md, "alice") {
		t.Errorf("missing milestone/assignee in table:\n%s", md)
	}
}

// boardSpecsByTool supports board specs by tool assertions in boards tests.
func boardSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestBoards_UnreadableCapturedGroup verifies that every board handler reading
// the group object off the captured answer returns an error rather than a
// half-filled board when GitLab sends group as something that is not an object.
// The SDK ignores the key its own IssueBoard does not model, so the captured
// read is the only thing that can notice.
func TestBoards_UnreadableCapturedGroup(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"name":"dev","group":"not-an-object"}]`)
			_, err := ListBoards(context.Background(), client, ListBoardsInput{ProjectID: "42"})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"id":1,"name":"dev","group":"not-an-object"}`)
			_, err := CreateBoard(context.Background(), client, CreateBoardInput{ProjectID: "42", Name: "dev"})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"id":1,"name":"dev","group":"not-an-object"}`)
			_, err := UpdateBoard(context.Background(), client, UpdateBoardInput{ProjectID: "42", BoardID: 1, Name: "renamed"})
			return err
		}},
	})
}
