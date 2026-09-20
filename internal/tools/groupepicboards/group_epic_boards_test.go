// group_epic_boards_test.go validates the List and Get handlers for GitLab
// group epic board operations, covering success paths, input validation
// (missing group_id, missing/zero board_id), API error responses and which of
// them carry the epic-board hint, context cancellation, pagination parameter
// forwarding, empty results, the escaping that makes a group path one request
// segment, and edge cases in toOutput (nil labels, nil list entries, lists
// without labels). The Markdown formatters are pinned here too, whole output
// at a time.
package groupepicboards

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathBoards = "/api/v4/groups/mygroup/epic_boards"
	pathBoard1 = "/api/v4/groups/mygroup/epic_boards/1"

	// The two label timestamps differ on purpose: GitLab sends them under
	// separate keys, and a fixture that gives both the same instant cannot
	// tell a converter reading the wrong one from a converter reading the
	// right one.
	labelCreatedAt = "2023-01-27T10:40:59.738Z"
	labelUpdatedAt = "2024-06-03T08:15:22.101Z"

	boardJSON = `{
		"id": 1,
		"name": "Epic Board",
		"hide_backlog_list": true,
		"hide_closed_list": false,
		"group": {"id": 7, "name": "My Group", "web_url": "http://example.com/groups/my-group"},
		"labels": [{"id": 10, "title": "Priority", "name": "Priority", "color": "#FF0000", "text_color": "#FFFFFF", "description": "P", "description_html": "<p>P</p>", "group_id": 7, "project_id": null, "template": false, "created_at": "` + labelCreatedAt + `", "updated_at": "` + labelUpdatedAt + `"}],
		"lists": [
			{"id": 100, "label": {"id": 10, "name": "Priority", "color": "#F0AD4E", "description": null}, "position": 0, "list_type": "label", "collapsed": false}
		]
	}`

	testGroupID = "mygroup"
)

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		validate  func(t *testing.T, out ListOutput)
	}{
		{
			name:      "returns boards on success",
			input:     ListInput{GroupID: testGroupID},
			handler:   listBoardsSuccessHandler(t),
			wantCount: 1,
			validate:  assertListBoardDetails,
		},
		{
			name:  "returns empty boards for empty array",
			input: ListInput{GroupID: testGroupID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, "[]")
			}),
			wantCount: 0,
		},
		{
			name:  "forwards pagination parameters",
			input: ListInput{GroupID: testGroupID, PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5}},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "5")
				testutil.RespondJSONWithPagination(w, http.StatusOK, "[]", testutil.PaginationHeaders{
					Page: "2", PerPage: "5", Total: "10", TotalPages: "2",
				})
			}),
			wantCount: 0,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Pagination.TotalItems != 10 {
					t.Errorf("TotalItems = %d, want 10", out.Pagination.TotalItems)
				}
			},
		},
		{
			name: "forwards order_by, sort, and keyset params",
			input: ListInput{
				GroupID:               testGroupID,
				OrderBy:               "name",
				Sort:                  "desc",
				KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "order_by", "name")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.AssertQueryParam(t, r, "pagination", "keyset")
				testutil.AssertQueryParam(t, r, "page_token", "tok")
				testutil.RespondJSON(w, http.StatusOK, "[]")
			}),
			wantCount: 0,
		},
		{
			name:  "returns error for missing group_id",
			input: ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for missing group_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: testGroupID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(out.Boards) != tt.wantCount {
					t.Fatalf("len(Boards) = %d, want %d", len(out.Boards), tt.wantCount)
				}
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

func listBoardsSuccessHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathBoards)
		testutil.RespondJSON(w, http.StatusOK, "["+boardJSON+"]")
	}
}

func assertListBoardDetails(t *testing.T, out ListOutput) {
	t.Helper()
	board := out.Boards[0]
	if board.ID != 1 {
		t.Errorf("Boards[0].ID = %d, want 1", board.ID)
	}
	if board.Name != "Epic Board" {
		t.Errorf("Boards[0].Name = %q, want %q", board.Name, "Epic Board")
	}
	assertListBoardLabels(t, board)
	assertListBoardLists(t, board)
}

func assertListBoardLabels(t *testing.T, board Output) {
	t.Helper()
	if len(board.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1", len(board.Labels))
	}
	if board.Labels[0].Name != "Priority" {
		t.Errorf("Labels[0].Name = %q, want %q", board.Labels[0].Name, "Priority")
	}
	if board.Labels[0].Color != "#FF0000" {
		t.Errorf("Labels[0].Color = %q, want %q", board.Labels[0].Color, "#FF0000")
	}
	if board.Labels[0].DescriptionHTML != "<p>P</p>" {
		t.Errorf("Labels[0].DescriptionHTML = %q, want %q", board.Labels[0].DescriptionHTML, "<p>P</p>")
	}
}

func assertListBoardLists(t *testing.T, board Output) {
	t.Helper()
	if board.Group == nil || board.Group.Name != "My Group" {
		t.Errorf("Group = %+v, want name My Group", board.Group)
	}
	if !board.HideBacklogList || board.HideClosedList {
		t.Errorf("hide flags = %v/%v, want true/false", board.HideBacklogList, board.HideClosedList)
	}
	if len(board.Lists) != 1 {
		t.Fatalf("len(Lists) = %d, want 1", len(board.Lists))
	}
	if board.Lists[0].Label == nil || board.Lists[0].Label.Name != "Priority" {
		t.Errorf("Lists[0].Label = %+v, want name Priority", board.Lists[0].Label)
	}
	if board.Lists[0].ListType != "label" {
		t.Errorf("Lists[0].ListType = %q, want label", board.Lists[0].ListType)
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestList_NotFoundIncludesActionableHint verifies that List_NotFoundIncludesActionableHint returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_NotFoundIncludesActionableHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
	for _, want := range []string{"Premium/Ultimate", "can be empty", "configured for the group"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("List() error missing %q: %v", want, err)
			}
		})
	}
}

// TestBoardPathsEscapeTheGroupIntoOneSegment pins what makes both handlers'
// request-building error arms unreachable: the group a caller names is escaped
// before it is interpolated, so a path with a slash, a space or a percent in
// it still reaches GitLab as one segment and the request is always built
// rather than refused. Without the escaping the slash would split the path and
// address another endpoint entirely.
func TestBoardPathsEscapeTheGroupIntoOneSegment(t *testing.T) {
	const (
		groupPath   = "my group/sub-group%x"
		escapedPath = "/api/v4/groups/my%20group%2Fsub-group%25x/epic_boards"
	)
	tests := []struct {
		name     string
		wantPath string
		body     string
		call     func(t *testing.T, client *gitlabclient.Client) error
	}{
		{
			name:     "list",
			wantPath: escapedPath,
			body:     "[]",
			call: func(t *testing.T, client *gitlabclient.Client) error {
				t.Helper()
				_, err := List(context.Background(), client, ListInput{GroupID: toolutil.StringOrInt(groupPath)})
				return err
			},
		},
		{
			name:     "get",
			wantPath: escapedPath + "/7",
			body:     `{"id":7,"name":"B"}`,
			call: func(t *testing.T, client *gitlabclient.Client) error {
				t.Helper()
				_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt(groupPath), BoardID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.EscapedPath(); got != tt.wantPath {
					t.Errorf("escaped path = %q, want %q", got, tt.wantPath)
				}
				testutil.RespondJSON(w, http.StatusOK, tt.body)
			}))
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call with group %q error = %v, want nil", groupPath, err)
			}
		})
	}
}

// TestBoardHintIsBoundToNotFound verifies that the epic-board hint both
// handlers attach is attached on 404 alone: another refusal carries GitLab's
// own message instead, so a permission failure is never reported as a group
// whose boards are merely unconfigured.
func TestBoardHintIsBoundToNotFound(t *testing.T) {
	tests := []struct {
		name     string
		unwanted string
		call     func(t *testing.T, client *gitlabclient.Client) error
	}{
		{
			name:     "list",
			unwanted: groupEpicBoardHint,
			call: func(t *testing.T, client *gitlabclient.Client) error {
				t.Helper()
				_, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
				return err
			},
		},
		{
			name:     "get",
			unwanted: "configure an epic board",
			call: func(t *testing.T, client *gitlabclient.Client) error {
				t.Helper()
				_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, BoardID: 1})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			}))
			err := tt.call(t, client)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "403 Forbidden") {
				t.Errorf("error = %v, want GitLab's own message", err)
			}
			if strings.Contains(err.Error(), tt.unwanted) {
				t.Errorf("error = %v, want no 404 hint on a 403", err)
			}
		})
	}
}

// TestGet verifies the Get handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	tests := []getCase{
		{
			name:  "returns board on success",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathBoard1)
				testutil.RespondJSON(w, http.StatusOK, boardJSON)
			}),
			validate: assertEpicBoardDetails,
		},
		{
			name:  "returns board with no labels and no lists",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"Empty Board"}`)
			}),
			validate: assertEmptyEpicBoard,
		},
		{
			name:  "handles list entry without label",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"B","lists":[{"id":50,"position":2}]}`)
			}),
			validate: assertBoardListWithoutLabel,
		},
		{
			name:  "handles null label entry in labels array",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"B","labels":[null,{"id":1,"name":"Bug"}]}`)
			}),
			validate: assertNullLabelFiltered,
		},
		{
			name:  "returns error for missing group_id",
			input: GetInput{BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for missing group_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error for zero board_id",
			input: GetInput{GroupID: testGroupID, BoardID: 0},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for zero board_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error for negative board_id",
			input: GetInput{GroupID: testGroupID, BoardID: -1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for negative board_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 404 response",
			input: GetInput{GroupID: testGroupID, BoardID: 999},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 500 response",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runGetCase(t, tt)
		})
	}
}

func assertEpicBoardDetails(t *testing.T, out Output) {
	t.Helper()
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.Name != "Epic Board" {
		t.Errorf("Name = %q, want %q", out.Name, "Epic Board")
	}
	if len(out.Labels) != 1 || out.Labels[0].Name != "Priority" {
		t.Errorf("Labels = %v, want [Priority]", out.Labels)
	}
	if out.Labels[0].Title != "Priority" || out.Labels[0].GroupID != 7 {
		t.Errorf("Labels[0] superset = %+v, want title/group_id populated", out.Labels[0])
	}
	// Each timestamp is held to its own key: the two carry different instants
	// in the fixture, so reading one in the other's place fails here.
	if out.Labels[0].CreatedAt != labelCreatedAt || out.Labels[0].UpdatedAt != labelUpdatedAt {
		t.Errorf("Labels[0] timestamps = %q/%q, want %q/%q",
			out.Labels[0].CreatedAt, out.Labels[0].UpdatedAt, labelCreatedAt, labelUpdatedAt)
	}
	if out.Group == nil || out.Group.ID != 7 || out.Group.WebURL == "" {
		t.Errorf("Group = %+v, want id 7 with web_url", out.Group)
	}
	if len(out.Lists) != 1 {
		t.Fatalf("len(Lists) = %d, want 1", len(out.Lists))
	}
	if out.Lists[0].Label == nil || out.Lists[0].Label.ID != 10 {
		t.Errorf("Lists[0].Label = %+v, want id 10", out.Lists[0].Label)
	}
	if out.Lists[0].Position != 0 {
		t.Errorf("Lists[0].Position = %d, want 0", out.Lists[0].Position)
	}
}

func assertEmptyEpicBoard(t *testing.T, out Output) {
	t.Helper()
	if out.Name != "Empty Board" {
		t.Errorf("Name = %q, want %q", out.Name, "Empty Board")
	}
	if len(out.Labels) != 0 {
		t.Errorf("len(Labels) = %d, want 0", len(out.Labels))
	}
	if len(out.Lists) != 0 {
		t.Errorf("len(Lists) = %d, want 0", len(out.Lists))
	}
}

func assertBoardListWithoutLabel(t *testing.T, out Output) {
	t.Helper()
	if len(out.Lists) != 1 {
		t.Fatalf("len(Lists) = %d, want 1", len(out.Lists))
	}
	if out.Lists[0].Label != nil {
		t.Errorf("Lists[0].Label = %+v, want nil", out.Lists[0].Label)
	}
	if out.Lists[0].Position != 2 {
		t.Errorf("Lists[0].Position = %d, want 2", out.Lists[0].Position)
	}
}

func assertNullLabelFiltered(t *testing.T, out Output) {
	t.Helper()
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1", len(out.Labels))
	}
	if out.Labels[0].Name != "Bug" {
		t.Errorf("Labels[0].Name = %q, want %q", out.Labels[0].Name, "Bug")
	}
}

type getCase struct {
	name     string
	input    GetInput
	handler  http.HandlerFunc
	wantErr  bool
	validate func(t *testing.T, out Output)
}

func runGetCase(t *testing.T, tt getCase) {
	t.Helper()
	client := testutil.NewTestClient(t, tt.handler)
	out, err := Get(context.Background(), client, tt.input)
	if (err != nil) != tt.wantErr {
		t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
	}
	if tt.validate != nil {
		tt.validate(t, out)
	}
}

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: testGroupID, BoardID: 1})
	if err == nil {
		t.Fatal("Get() expected context error, got nil")
	}
}

// TestGet_NotFoundIncludesActionableHint verifies that Get_NotFoundIncludesActionableHint returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_NotFoundIncludesActionableHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Board Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, BoardID: 999})
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
	for _, want := range []string{"epic_board_list", "gitlab_group", "configure an epic board"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Get() error missing %q: %v", want, err)
			}
		})
	}
}

// TestGet_VersionTolerantOmittedFields verifies that an older GitLab instance
// response which omits the raw-superset keys (hide_*_list, the label
// title/group_id/template/created_at/updated_at, and each list's
// list_type/collapsed) decodes without error: the missing fields stay at their
// zero value and are omitted from the envelope rather than hard-failing.
func TestGet_VersionTolerantOmittedFields(t *testing.T) {
	const legacyJSON = `{
		"id": 1,
		"name": "Legacy Board",
		"group": {"id": 7, "name": "My Group"},
		"labels": [{"id": 10, "name": "Priority", "color": "#FF0000"}],
		"lists": [{"id": 100, "label": {"id": 10, "name": "Priority"}, "position": 0}]
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, legacyJSON)
	}))
	out, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, BoardID: 1})
	if err != nil {
		t.Fatalf("Get() legacy response error = %v, want nil", err)
	}
	if out.HideBacklogList || out.HideClosedList {
		t.Errorf("hide flags = %v/%v, want false/false when omitted", out.HideBacklogList, out.HideClosedList)
	}
	if len(out.Labels) != 1 || out.Labels[0].Title != "" || out.Labels[0].CreatedAt != "" {
		t.Errorf("label superset = %+v, want zero when omitted", out.Labels)
	}
	if len(out.Lists) != 1 || out.Lists[0].ListType != "" || out.Lists[0].Collapsed != nil {
		t.Errorf("list superset = %+v, want zero/nil when omitted", out.Lists)
	}
}

// TestFormatOutputMarkdown pins the whole epic board card in three states:
// a populated board with its columns as a nested table, a board GitLab sent
// nothing optional for, and a board whose name carries a pipe, which must not
// reach the page as table syntax.
func TestFormatOutputMarkdown(t *testing.T) {
	collapsed := true
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name: "renders board with labels and lists",
			input: Output{
				ID:    1,
				Name:  "Sprint Board",
				Group: &GroupRefOutput{ID: 7, Name: "My Group", WebURL: "https://gitlab.example.com/groups/my-group"},
				// The nil entry and the one GitLab sent without a name are
				// both dropped from the Labels line.
				Labels:          []*LabelDetailsOutput{{ID: 10, Name: "Priority"}, nil, {ID: 12}, {ID: 11, Name: "Bug"}},
				HideBacklogList: true,
				Lists: []BoardListOutput{
					{ID: 100, Label: &ListLabelOutput{ID: 10, Name: "Priority"}, Position: 0, ListType: "label"},
					{ID: 101, Position: 1, ListType: "backlog", Collapsed: &collapsed},
				},
			},
			want: "## Epic Board #1: Sprint Board\n\n" +
				"- **ID**: 1\n" +
				"- **Group**: [My Group](https://gitlab.example.com/groups/my-group)\n" +
				"- **Labels**: Priority, Bug\n" +
				"- **Hide Backlog**: " + toolutil.EmojiSuccess + "\n" +
				"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
				"### Board Lists\n\n" +
				"| ID | Scope | Type | Position | Collapsed |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 100 | Priority | label | 0 |  |\n" +
				"| 101 | backlog | backlog | 1 | " + toolutil.EmojiSuccess + " |\n\n" +
				"---\n\U0001F4A1 **Next steps:**\n" +
				"- Use action 'group.epic_board_list' to see every epic board in the group\n",
		},
		{
			name:  "renders board without labels or lists",
			input: Output{ID: 2, Name: "Empty Board"},
			want: "## Epic Board #2: Empty Board\n\n" +
				"- **ID**: 2\n" +
				"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
				"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
				"---\n\U0001F4A1 **Next steps:**\n" +
				"- Use action 'group.epic_board_list' to see every epic board in the group\n",
		},
		{
			// A pipe is text in a heading, which is not a table row; the card
			// escaper neutralizes it wherever it would be one.
			name:  "keeps a pipe in the name as text",
			input: Output{ID: 3, Name: "Foo | Bar"},
			want: "## Epic Board #3: Foo | Bar\n\n" +
				"- **ID**: 3\n" +
				"- **Hide Backlog**: " + toolutil.EmojiCross + "\n" +
				"- **Hide Closed**: " + toolutil.EmojiCross + "\n\n" +
				"---\n\U0001F4A1 **Next steps:**\n" +
				"- Use action 'group.epic_board_list' to see every epic board in the group\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatOutputMarkdown()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown pins the whole epic board list in three states: a
// page of boards, a group with none, and a page under a larger total.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name: "renders board list table",
			input: ListOutput{
				Boards: []Output{
					{
						ID: 1, Name: "Sprint",
						Group:  &GroupRefOutput{ID: 7, Name: "My Group", WebURL: "https://gitlab.example.com/groups/my-group"},
						Labels: []*LabelDetailsOutput{{ID: 1, Name: "P1"}},
						Lists:  []BoardListOutput{{ID: 10}},
					},
					{ID: 2, Name: "Backlog"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
			},
			want: "## Group Epic Boards (2)\n\n" +
				"| ID | Name | Group | Labels | Lists |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | Sprint | [My Group](https://gitlab.example.com/groups/my-group) | P1 | 1 |\n" +
				"| 2 | Backlog |  |  | 0 |\n\n" +
				"Page 1 of 1 | 2 items total | 20 per page\n\n" +
				"---\n\U0001F4A1 **Next steps:**\n" +
				"- " + toolutil.HintPreserveLinks + "\n" +
				"- Use action 'group.epic_board_get' to read one board with its columns\n",
		},
		{
			name:  "renders empty state",
			input: ListOutput{Pagination: toolutil.PaginationOutput{TotalItems: 0}},
			want:  "No epic boards found.\n",
		},
		{
			name: "shows pagination when multiple pages",
			input: ListOutput{
				Boards: []Output{{ID: 1, Name: "B"}},
				Pagination: toolutil.PaginationOutput{
					TotalItems: 50, Page: 1, PerPage: 20, TotalPages: 3, NextPage: 2,
				},
			},
			want: "## Group Epic Boards (50)\n\n" +
				"Showing 1 of 50 results (page 1 of 3)\n\n" +
				"| ID | Name | Group | Labels | Lists |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | B |  |  | 0 |\n\n" +
				"Page 1 of 3 | 50 items total | 20 per page\n\n" +
				"---\n\U0001F4A1 **Next steps:**\n" +
				"- " + toolutil.HintPreserveLinks + "\n" +
				"- Use action 'group.epic_board_get' to read one board with its columns\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatListMarkdown()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}
