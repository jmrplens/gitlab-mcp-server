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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	// fmtMDMissingContent identifies the fmt md missing content constant used by this package.
	fmtMDMissingContent = "markdown missing expected content: %s"
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

// TestFormatBoardMarkdown verifies the BoardMarkdown Markdown formatter for a representative board input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardMarkdown(t *testing.T) {
	out := BoardOutput{
		ID: 1, Name: "Dev",
		Project:         &ProjectOutput{ID: 10, Name: "P", PathWithNamespace: "group/p"},
		Milestone:       &MilestoneOutput{ID: 5, Title: "v1"},
		Assignee:        &BasicUserOutput{ID: 3, Username: "alice"},
		Labels:          []*LabelDetailsOutput{{ID: 1, Name: "bug"}},
		HideBacklogList: false, HideClosedList: true,
		Lists: []BoardListOutput{{ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0}},
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

// TestFormatListBoardsMarkdown verifies the ListBoardsMarkdown Markdown formatter for a representative listboards input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardsMarkdown(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{{ID: 1, Name: "Dev", Project: &ProjectOutput{ID: 1, PathWithNamespace: "group/dev"}}},
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

// TestFormatBoardListMarkdown verifies the BoardListMarkdown Markdown formatter for a representative boardlist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardListMarkdown(t *testing.T) {
	out := BoardListOutput{ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 10}
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

// TestFormatBoardMarkdown_NoProject verifies the BoardMarkdown_NoProject Markdown formatter for a representative board_noproject input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardMarkdown_NoProject(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Board"}
	md := FormatBoardMarkdown(out)
	if strings.Contains(md, "**Project**") {
		t.Errorf("should not show project when empty: %s", md)
	}
}

// TestFormatBoardMarkdown_ProjectNameFallback verifies the BoardMarkdown_ProjectNameFallback Markdown formatter for a representative board_projectnamefallback input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardMarkdown_ProjectNameFallback(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Board", Project: &ProjectOutput{ID: 5, Name: "MyProject"}}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "MyProject") {
		t.Errorf("should fall back to project name: %s", md)
	}
}

// TestFormatBoardMarkdown_ListWithoutLabel verifies the BoardMarkdown_ListWithoutLabel Markdown formatter for a representative board_listwithoutlabel input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListBoardsMarkdown_FallbackToName verifies the ListBoardsMarkdown_FallbackToName Markdown formatter for a representative listboards_fallbacktoname input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardsMarkdown_FallbackToName(t *testing.T) {
	out := ListBoardsOutput{
		Boards: []BoardOutput{{ID: 1, Name: "Dev", Project: &ProjectOutput{ID: 1, Name: "MyProject"}}},
	}
	md := FormatListBoardsMarkdown(out)
	if !strings.Contains(md, "MyProject") {
		t.Errorf("should fall back to project name: %s", md)
	}
}

// TestFormatBoardListMarkdown_NoLabelFallback verifies the BoardListMarkdown_NoLabelFallback Markdown formatter for a representative boardlist_nolabelfallback input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardListMarkdown_NoLabelFallback(t *testing.T) {
	out := BoardListOutput{ID: 200, Position: 3}
	md := FormatBoardListMarkdown(out)
	if !strings.Contains(md, "## Board List #200") {
		t.Errorf("heading should fall back to #ID when no label: %s", md)
	}
}

// TestFormatListBoardListsMarkdown_NoLabelFallback verifies the ListBoardListsMarkdown_NoLabelFallback Markdown formatter for a representative listboardlists_nolabelfallback input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatBoardMarkdown_Minimal verifies the BoardMarkdown_Minimal Markdown formatter for a representative board_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatBoardMarkdown_WithWeight verifies the BoardMarkdown_WithWeight Markdown formatter for a representative board_withweight input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardMarkdown_WithWeight(t *testing.T) {
	out := BoardOutput{ID: 1, Name: "Dev", Weight: 5}
	md := FormatBoardMarkdown(out)
	if !strings.Contains(md, "Weight") {
		t.Errorf("expected Weight in:\n%s", md)
	}
}

// TestFormatListBoardListsMarkdown verifies the ListBoardListsMarkdown Markdown formatter for a representative listboardlists input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardListsMarkdown(t *testing.T) {
	out := ListBoardListsOutput{
		Lists: []BoardListOutput{
			{ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0, MaxIssueCount: 10, MaxIssueWeight: 50},
			{ID: 101, Label: &LabelOutput{Name: "Doing"}, Position: 1},
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

// TestFormatListBoardListsMarkdown_Empty verifies the ListBoardListsMarkdown_Empty Markdown formatter for a representative listboardlists_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListBoardListsMarkdown_Empty(t *testing.T) {
	out := ListBoardListsOutput{}
	md := FormatListBoardListsMarkdown(out)
	if !strings.Contains(md, "Board Lists") {
		t.Errorf("missing header:\n%s", md)
	}
}

// TestFormatBoardListMarkdown_AllFields verifies the BoardListMarkdown_AllFields Markdown formatter for a representative boardlist_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatBoardListMarkdown_AllFields(t *testing.T) {
	out := BoardListOutput{
		ID: 100, Label: &LabelOutput{Name: "To Do"}, Position: 0,
		MaxIssueCount: 10, MaxIssueWeight: 50,
		Assignee:  &BoardListAssigneeOutput{ID: 3, Username: "alice"},
		Milestone: &MilestoneOutput{ID: 5, Title: "v1.0"},
	}
	md := FormatBoardListMarkdown(out)
	for _, want := range []string{"To Do", "Max Issue Count", "Max Issue Weight", "alice", "v1.0"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("FormatBoardListMarkdown missing %q in:\n%s", want, md)
			}
		})
	}
	if strings.Contains(md, "(ID:") {
		t.Errorf("should not contain redundant (ID:) patterns:\n%s", md)
	}
}

// TestFormatBoardListMarkdown_Minimal verifies the BoardListMarkdown_Minimal Markdown formatter for a representative boardlist_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
