// epic_issues_test.go validates all epic-issue tool handlers:
// List, Assign, Remove, and UpdateOrder. Covers success paths,
// input validation (missing group_id, epic_iid, issue_id, epic_issue_id),
// API errors (404, 500), context cancellation, pagination parameters,
// empty results, MoveAfterID path, and markdown formatters.
package epicissues

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathEpicIssues  = "/api/v4/groups/mygroup/epics/1/issues"
	pathEpicIssue42 = "/api/v4/groups/mygroup/epics/1/issues/42"

	epicIssueJSON = `{
		"id": 42,
		"iid": 10,
		"project_id": 7,
		"title": "Fix login bug",
		"state": "opened",
		"web_url": "https://gitlab.example.com/mygroup/myproject/-/issues/10",
		"author": {"username": "alice"},
		"labels": ["bug", "critical"],
		"created_at": "2024-01-15T10:00:00Z",
		"updated_at": "2024-01-16T10:00:00Z"
	}`

	assignJSON = `{"id": 42, "epic": {"iid": 1}, "issue": {"id": 100}}`

	testGroupID = "mygroup"
)

// TestList validates the List handler for various scenarios including
// success, pagination, empty results, validation, API errors, and context cancellation.
func TestList(t *testing.T) {
	tests := []struct {
		name       string
		input      ListInput
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		validate   func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns issues for valid epic",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathEpicIssues)
				testutil.RespondJSON(w, http.StatusOK, "["+epicIssueJSON+"]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
				}
				if out.Issues[0].Title != "Fix login bug" {
					t.Errorf("Issues[0].Title = %q, want %q", out.Issues[0].Title, "Fix login bug")
				}
				if out.Issues[0].IID != 10 {
					t.Errorf("Issues[0].IID = %d, want 10", out.Issues[0].IID)
				}
				if out.Issues[0].State != "opened" {
					t.Errorf("Issues[0].State = %q, want %q", out.Issues[0].State, "opened")
				}
			},
		},
		{
			name: "passes pagination parameters to API",
			input: ListInput{
				GroupID:         testGroupID,
				EpicIID:         1,
				PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "5")
				testutil.RespondJSONWithPagination(w, http.StatusOK, "["+epicIssueJSON+"]", testutil.PaginationHeaders{
					Page: "2", PerPage: "5", Total: "10", TotalPages: "2",
				})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Pagination.TotalItems != 10 {
					t.Errorf("TotalItems = %d, want 10", out.Pagination.TotalItems)
				}
			},
		},
		{
			name:  "returns empty list when no issues",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, "[]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 0 {
					t.Errorf("len(Issues) = %d, want 0", len(out.Issues))
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      ListInput{EpicIID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "group_id is required",
		},
		{
			name:       "returns error when epic_iid is zero",
			input:      ListInput{GroupID: testGroupID},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when epic_iid is negative",
			input:      ListInput{GroupID: testGroupID, EpicIID: -1},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:  "returns error on API 404",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on API 500",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"internal server error"}`)
			},
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
			if tt.wantErrMsg != "" && err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrMsg)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestList_CancelledContext verifies List returns an error when the
// context is cancelled before the API call.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestAssign validates the Assign handler covering success, input validation,
// API errors, context cancellation, and output field assertions.
func TestAssign(t *testing.T) {
	tests := []struct {
		name       string
		input      AssignInput
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		validate   func(t *testing.T, out AssignOutput)
	}{
		{
			name:  "assigns issue to epic successfully",
			input: AssignInput{GroupID: testGroupID, EpicIID: 1, IssueID: 100},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathEpicIssues+"/100")
				testutil.RespondJSON(w, http.StatusCreated, assignJSON)
			},
			validate: func(t *testing.T, out AssignOutput) {
				t.Helper()
				if out.ID != 42 {
					t.Errorf("ID = %d, want 42", out.ID)
				}
				if out.EpicIID != 1 {
					t.Errorf("EpicIID = %d, want 1", out.EpicIID)
				}
				if out.IssueID != 100 {
					t.Errorf("IssueID = %d, want 100", out.IssueID)
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      AssignInput{EpicIID: 1, IssueID: 100},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "group_id is required",
		},
		{
			name:       "returns error when epic_iid is zero",
			input:      AssignInput{GroupID: testGroupID, IssueID: 100},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when epic_iid is negative",
			input:      AssignInput{GroupID: testGroupID, EpicIID: -5, IssueID: 100},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when issue_id is zero",
			input:      AssignInput{GroupID: testGroupID, EpicIID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "issue_id",
		},
		{
			name:  "returns error on API 404",
			input: AssignInput{GroupID: testGroupID, EpicIID: 1, IssueID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on API 409 conflict",
			input: AssignInput{GroupID: testGroupID, EpicIID: 1, IssueID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusConflict, `{"message":"Issue already assigned"}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Assign(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Assign() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrMsg != "" && err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrMsg)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestAssign_CancelledContext verifies Assign returns an error when the
// context is cancelled before the API call.
func TestAssign_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Assign(ctx, client, AssignInput{GroupID: testGroupID, EpicIID: 1, IssueID: 100})
	if err == nil {
		t.Fatal("Assign() expected context error, got nil")
	}
}

// TestAssign_NilEpicAndIssue verifies toAssignOutput handles nil Epic and Issue pointers
// gracefully by returning zero values for EpicIID and IssueID.
func TestAssign_NilEpicAndIssue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id": 55}`)
	}))
	out, err := Assign(context.Background(), client, AssignInput{GroupID: testGroupID, EpicIID: 1, IssueID: 100})
	if err != nil {
		t.Fatalf("Assign() error: %v", err)
	}
	if out.ID != 55 {
		t.Errorf("ID = %d, want 55", out.ID)
	}
	if out.EpicIID != 0 {
		t.Errorf("EpicIID = %d, want 0 when epic is nil", out.EpicIID)
	}
	if out.IssueID != 0 {
		t.Errorf("IssueID = %d, want 0 when issue is nil", out.IssueID)
	}
}

// TestRemove validates the Remove handler covering success, input validation,
// API errors, and context cancellation.
func TestRemove(t *testing.T) {
	tests := []struct {
		name       string
		input      RemoveInput
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		validate   func(t *testing.T, out AssignOutput)
	}{
		{
			name:  "removes issue from epic successfully",
			input: RemoveInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathEpicIssue42)
				testutil.RespondJSON(w, http.StatusOK, assignJSON)
			},
			validate: func(t *testing.T, out AssignOutput) {
				t.Helper()
				if out.ID != 42 {
					t.Errorf("ID = %d, want 42", out.ID)
				}
				if out.EpicIID != 1 {
					t.Errorf("EpicIID = %d, want 1", out.EpicIID)
				}
				if out.IssueID != 100 {
					t.Errorf("IssueID = %d, want 100", out.IssueID)
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      RemoveInput{EpicIID: 1, EpicIssueID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "group_id is required",
		},
		{
			name:       "returns error when epic_iid is zero",
			input:      RemoveInput{GroupID: testGroupID, EpicIssueID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when epic_iid is negative",
			input:      RemoveInput{GroupID: testGroupID, EpicIID: -3, EpicIssueID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when epic_issue_id is zero",
			input:      RemoveInput{GroupID: testGroupID, EpicIID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_issue_id",
		},
		{
			name:  "returns error on API 404",
			input: RemoveInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Remove(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrMsg != "" && err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrMsg)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestRemove_CancelledContext verifies Remove returns an error when the
// context is cancelled before the API call.
func TestRemove_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Remove(ctx, client, RemoveInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42})
	if err == nil {
		t.Fatal("Remove() expected context error, got nil")
	}
}

// TestUpdateOrder validates the UpdateOrder handler covering success with
// MoveBeforeID, MoveAfterID, both IDs, input validation, API errors,
// and context cancellation.
func TestUpdateOrder(t *testing.T) {
	beforeID := int64(10)
	afterID := int64(20)

	tests := []struct {
		name       string
		input      UpdateInput
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		validate   func(t *testing.T, out ListOutput)
	}{
		{
			name:  "reorders with move_before_id",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42, MoveBeforeID: &beforeID},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, pathEpicIssue42)
				testutil.RespondJSON(w, http.StatusOK, "["+epicIssueJSON+"]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
				}
				if out.Issues[0].Title != "Fix login bug" {
					t.Errorf("Issues[0].Title = %q, want %q", out.Issues[0].Title, "Fix login bug")
				}
			},
		},
		{
			name:  "reorders with move_after_id",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42, MoveAfterID: &afterID},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.RespondJSON(w, http.StatusOK, "["+epicIssueJSON+"]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
				}
			},
		},
		{
			name:  "reorders with both move_before_id and move_after_id",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42, MoveBeforeID: &beforeID, MoveAfterID: &afterID},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, "["+epicIssueJSON+"]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      UpdateInput{EpicIID: 1, EpicIssueID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "group_id is required",
		},
		{
			name:       "returns error when epic_iid is zero",
			input:      UpdateInput{GroupID: testGroupID, EpicIssueID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_iid",
		},
		{
			name:       "returns error when epic_issue_id is zero",
			input:      UpdateInput{GroupID: testGroupID, EpicIID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) {},
			wantErr:    true,
			wantErrMsg: "epic_issue_id",
		},
		{
			name:  "returns error on API 500",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42, MoveBeforeID: &beforeID},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := UpdateOrder(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateOrder() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrMsg != "" && err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrMsg)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestUpdateOrder_CancelledContext verifies UpdateOrder returns an error
// when the context is cancelled before the API call.
func TestUpdateOrder_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	beforeID := int64(10)
	_, err := UpdateOrder(ctx, client, UpdateInput{GroupID: testGroupID, EpicIID: 1, EpicIssueID: 42, MoveBeforeID: &beforeID})
	if err == nil {
		t.Fatal("UpdateOrder() expected context error, got nil")
	}
}

// TestFormatListMarkdown validates the markdown formatter for list output
// across scenarios: issues present, empty list, and with labels.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		excludes []string
	}{
		{
			name: "renders issues table with labels",
			input: ListOutput{
				Issues: []issues.Output{
					{IID: 10, Title: "Fix login bug", State: "opened", Author: "alice", Labels: []string{"bug", "critical"}, CreatedAt: "2024-01-15T10:00:00Z"},
					{IID: 20, Title: "Add feature", State: "closed", Author: "bob", Labels: nil, CreatedAt: "2024-02-01T12:00:00Z"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2},
			},
			contains: []string{
				"## Epic Issues (2)",
				"| IID | Title | State | Author | Labels | Created |",
				"#10", "Fix login bug", "opened", "alice", "bug, critical",
				"#20", "Add feature", "closed", "bob",
				"epic_issue_assign",
				"epic_issue_remove",
			},
		},
		{
			name: "renders empty list message",
			input: ListOutput{
				Issues:     nil,
				Pagination: toolutil.PaginationOutput{TotalItems: 0},
			},
			contains: []string{
				"## Epic Issues (0)",
				"No issues found in this epic.",
			},
			excludes: []string{"| IID |"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatAssignMarkdown validates the markdown formatter for assign/remove
// output with various field combinations and action words.
func TestFormatAssignMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		output   AssignOutput
		action   string
		contains []string
		excludes []string
	}{
		{
			name:   "renders assigned output with all fields",
			output: AssignOutput{ID: 42, EpicIID: 1, IssueID: 100},
			action: "assigned",
			contains: []string{
				"## Epic Issue assigned",
				"**Association ID**: 42",
				"**Epic IID**: &1",
				"**Issue ID**: 100",
				"gitlab_epic_issue_list",
				"gitlab_epic_issue_remove",
			},
		},
		{
			name:   "renders removed output",
			output: AssignOutput{ID: 55, EpicIID: 3, IssueID: 200},
			action: "removed",
			contains: []string{
				"## Epic Issue removed",
				"**Association ID**: 55",
			},
		},
		{
			name:   "omits epic IID and issue ID when zero",
			output: AssignOutput{ID: 10},
			action: "assigned",
			contains: []string{
				"**Association ID**: 10",
			},
			excludes: []string{
				"**Epic IID**",
				"**Issue ID**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAssignMarkdown(tt.output, tt.action)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}
