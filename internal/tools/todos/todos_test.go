// todos_test.go contains unit tests for the to-do MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package todos

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// fmtUnexpPath identifies the fmt unexp path constant used by this package.
	fmtUnexpPath = "unexpected path: %s"
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// pathTodos identifies the path todos constant used by this package.
	pathTodos = "/api/v4/todos"
	// pathTodoMarkDone identifies the path todo mark done constant used by this package.
	pathTodoMarkDone = "/api/v4/todos/1/mark_as_done"
	// pathTodoMarkAll identifies the path todo mark all constant used by this package.
	pathTodoMarkAll = "/api/v4/todos/mark_as_done"
)

// todoList tests.

// TestTodoList_Success verifies TodoList when success.
func TestTodoList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodos {
			t.Errorf(fmtUnexpPath, r.URL.Path)
			http.Error(w, "assertion failed", http.StatusInternalServerError)
			return
		}
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{
				"id": 1,
				"action_name": "assigned",
				"target_type": "Issue",
				"target": {"title": "Fix bug"},
				"target_url": "https://gitlab.example.com/proj/-/issues/1",
				"body": "Fix the login bug",
				"state": "pending",
				"project": {"name": "my-project"},
				"author": {"username": "alice"},
				"created_at": "2026-01-15T10:00:00Z"
			},
			{
				"id": 2,
				"action_name": "mentioned",
				"target_type": "MergeRequest",
				"target": {"title": "Add feature"},
				"target_url": "https://gitlab.example.com/proj/-/merge_requests/5",
				"body": "@bob check this",
				"state": "pending",
				"project": {"name": "my-project"},
				"author": {"username": "charlie"},
				"created_at": "2026-01-16T12:00:00Z"
			}
		]`, testutil.PaginationHeaders{Page: "1", Total: "2", TotalPages: "1", PerPage: "20"})
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(out.Todos))
	}
	if out.Todos[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Todos[0].ID)
	}
	if out.Todos[0].ActionName != "assigned" {
		t.Errorf("expected action assigned, got %s", out.Todos[0].ActionName)
	}
	if out.Todos[0].Target == nil || out.Todos[0].Target.Title != "Fix bug" {
		t.Errorf("expected target title 'Fix bug', got %+v", out.Todos[0].Target)
	}
	if out.Todos[0].Project == nil || out.Todos[0].Project.Name != "my-project" {
		t.Errorf("expected project 'my-project', got %+v", out.Todos[0].Project)
	}
	if out.Todos[0].Author == nil || out.Todos[0].Author.Username != "alice" {
		t.Errorf("expected author 'alice', got %+v", out.Todos[0].Author)
	}
	if out.Pagination.TotalItems != 2 {
		t.Errorf("expected 2 total items, got %d", out.Pagination.TotalItems)
	}
}

// TestTodoList_WithFilters verifies TodoList when with filters.
func TestTodoList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "assigned" {
			t.Errorf("expected action filter 'assigned', got %s", r.URL.Query().Get("action"))
		}
		if r.URL.Query().Get("state") != "pending" {
			t.Errorf("expected state filter 'pending', got %s", r.URL.Query().Get("state"))
		}
		if r.URL.Query().Get("type") != "Issue" {
			t.Errorf("expected type filter 'Issue', got %s", r.URL.Query().Get("type"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", Total: "0", TotalPages: "1", PerPage: "20"})
	}))

	out, err := List(context.Background(), client, ListInput{
		Action: "assigned",
		State:  "pending",
		Type:   "Issue",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Todos) != 0 {
		t.Fatalf("expected 0 todos, got %d", len(out.Todos))
	}
}

// TestTodoListServer_Error verifies TodoListServer when error.
func TestTodoListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTodoList_CancelledContext verifies TodoList when cancelled context.
func TestTodoList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// todoMarkDone tests.

// TestTodoMarkDone_Success verifies TodoMarkDone when success.
func TestTodoMarkDone_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodoMarkDone {
			t.Errorf(fmtUnexpPath, r.URL.Path)
			http.Error(w, "assertion failed", http.StatusInternalServerError)
			return
		}
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		w.WriteHeader(http.StatusNoContent)
	}))

	out, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestTodoMark_DoneZeroID verifies TodoMark when done zero ID.
func TestTodoMark_DoneZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	_, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 0})
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
}

// TestTodoMarkDone_NotFound verifies TodoMarkDone when not found.
func TestTodoMarkDone_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Todo Not Found"}`)
	}))

	_, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

// TestTodoMarkDone_CancelledContext verifies TodoMarkDone when cancelled context.
func TestTodoMarkDone_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := MarkDone(ctx, client, MarkDoneInput{ID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// todoMarkAllDone tests.

// TestTodoMarkAllDone_Success verifies TodoMarkAllDone when success.
func TestTodoMarkAllDone_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodoMarkAll {
			t.Errorf(fmtUnexpPath, r.URL.Path)
			http.Error(w, "assertion failed", http.StatusInternalServerError)
			return
		}
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		w.WriteHeader(http.StatusNoContent)
	}))

	out, err := MarkAllDone(context.Background(), client, MarkAllDoneInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestTodoMarkAllDoneServer_Error verifies TodoMarkAllDoneServer when error.
func TestTodoMarkAllDoneServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := MarkAllDone(context.Background(), client, MarkAllDoneInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTodoMarkAllDone_CancelledContext verifies TodoMarkAllDone when cancelled context.
func TestTodoMarkAllDone_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := MarkAllDone(ctx, client, MarkAllDoneInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedNonNilResult identifies the err expected non nil result constant used by this package.
const errExpectedNonNilResult = "expected non-nil result"

// ---------------------------------------------------------------------------
// List with all filter params
// ---------------------------------------------------------------------------.

// TestTodoList_AllFilters verifies TodoList when all filters.
func TestTodoList_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("author_id") != "5" {
			t.Errorf("expected author_id=5, got %q", q.Get("author_id"))
		}
		if q.Get("project_id") != "10" {
			t.Errorf("expected project_id=10, got %q", q.Get("project_id"))
		}
		if q.Get("group_id") != "3" {
			t.Errorf("expected group_id=3, got %q", q.Get("group_id"))
		}
		if q.Get("page") != "2" {
			t.Errorf("expected page=2, got %q", q.Get("page"))
		}
		if q.Get("per_page") != "30" {
			t.Errorf("expected per_page=30, got %q", q.Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", Total: "0", TotalPages: "1", PerPage: "20"})
	}))

	_, err := List(context.Background(), client, ListInput{
		Action:    "assigned",
		AuthorID:  5,
		ProjectID: 10,
		GroupID:   3,
		State:     "pending",
		Type:      "Issue",
		Page:      2, PerPage: 30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// toOutput with nil fields
// ---------------------------------------------------------------------------.

// TestToOutput_NilTargetProjectAuthorCreatedAt verifies ToOutput when nil target project author created at.
func TestToOutput_NilTargetProjectAuthorCreatedAt(t *testing.T) {
	todo := todoWithNils()
	out := toOutput(&todo, toolutil.TodoExtra{})
	if out.Target != nil {
		t.Errorf("expected nil Target, got %+v", out.Target)
	}
	if out.Project != nil {
		t.Errorf("expected nil Project, got %+v", out.Project)
	}
	if out.Author != nil {
		t.Errorf("expected nil Author, got %+v", out.Author)
	}
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
}

// todoWithNils converts the GitLab API response to the tool output format.
func todoWithNils() gl.Todo {
	return gl.Todo{
		ID:         99,
		ActionName: "assigned",
		TargetType: "Issue",
		TargetURL:  "https://x",
		Body:       "body",
		State:      "pending",
		Target:     nil,
		Project:    nil,
		Author:     nil,
		CreatedAt:  nil,
	}
}

// ---------------------------------------------------------------------------
// Markdown formatter tests
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
func TestFormatOutputMarkdown(t *testing.T) {
	r := FormatOutputMarkdown(Output{ID: 1})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	r := FormatListMarkdown(ListOutput{})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatMarkDoneMarkdown verifies FormatMarkDoneMarkdown.
func TestFormatMarkDoneMarkdown(t *testing.T) {
	r := FormatMarkDoneMarkdown(MarkDoneOutput{Message: "done"})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatMarkAllDoneMarkdown verifies FormatMarkAllDoneMarkdown.
func TestFormatMarkAllDoneMarkdown(t *testing.T) {
	r := FormatMarkAllDoneMarkdown(MarkAllDoneOutput{Message: "done"})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route tests
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Full nested target object mapping (1:1 audit policy)
// ---------------------------------------------------------------------------.

// TestToOutput_EveryField_ComesFromItsOwnSource verifies the whole converted
// to-do against one expected value, for a to-do in which no two values agree.
//
// A converter is straight-line assignment, so no mutant and no uncovered
// condition can report one that reads the field beside it; only a fixture in
// which every value is distinct can. The assertions this replaces set eight of
// the target's strings and read four of them back, so swapping the description
// for the title, or the notes link for the award-emoji link, would have passed.
func TestToOutput_EveryField_ComesFromItsOwnSource(t *testing.T) {
	todoCreated := mustTime(t, "2026-01-15T10:00:00Z")
	projectCreated := mustTime(t, "2026-01-02T02:00:00Z")
	authorCreated := mustTime(t, "2026-01-03T03:00:00Z")
	targetCreated := mustTime(t, "2026-01-04T04:00:00Z")
	milestoneCreated := mustTime(t, "2026-01-05T05:00:00Z")
	milestoneUpdated := mustTime(t, "2026-01-06T06:00:00Z")
	targetUpdated := mustTime(t, "2026-01-07T07:00:00Z")
	merged := mustTime(t, "2026-01-08T08:00:00Z")
	todoUpdated := mustTime(t, "2026-01-09T09:00:00Z")
	expired := true

	todo := &gl.Todo{
		ID:         7,
		ActionName: gl.TodoAssigned,
		TargetType: gl.TodoTargetIssue,
		TargetURL:  "https://gitlab.example.com/org/project-path/-/issues/42",
		Body:       "todo body",
		State:      "pending",
		CreatedAt:  &todoCreated,
		Project: &gl.BasicProject{
			ID: 3, Description: "project description", Name: "project name",
			NameWithNamespace: "Org / project name", Path: "project-path",
			PathWithNamespace: "org/project-path", CreatedAt: &projectCreated,
		},
		Author: &gl.BasicUser{
			ID: 11, Username: "author-username", Name: "Author Name", State: "active",
			AvatarURL: "https://gitlab.example.com/avatar/author.png",
			WebURL:    "https://gitlab.example.com/author-username", CreatedAt: &authorCreated,
		},
		Target: &gl.TodoTarget{
			// The nil element is deliberate: GitLab has sent one, and the
			// converter drops it rather than publishing a null.
			Assignees:   []*gl.BasicUser{{ID: 21, Username: "assignee-username"}, nil},
			Assignee:    &gl.BasicUser{ID: 22, Username: "single-assignee-username"},
			Author:      &gl.BasicUser{ID: 23, Username: "target-author-username"},
			CreatedAt:   &targetCreated,
			Description: "target description",
			Downvotes:   31,
			ID:          float64(41),
			IID:         42,
			Labels:      []string{"label-one", "label-two"},
			Milestone: &gl.Milestone{
				ID: 51, IID: 52, GroupID: 53, ProjectID: 54,
				Title: "milestone title", Description: "milestone description",
				State: "active", WebURL: "https://gitlab.example.com/milestone",
				StartDate: mustISO(t, "2026-02-01"), DueDate: mustISO(t, "2026-03-02"),
				CreatedAt: &milestoneCreated, UpdatedAt: &milestoneUpdated, Expired: &expired,
			},
			ProjectID:            61,
			State:                "opened",
			Subscribed:           true,
			TaskCompletionStatus: &gl.TasksCompletionStatus{Count: 7, CompletedCount: 3},
			Title:                "target title",
			UpdatedAt:            &targetUpdated,
			Upvotes:              33,
			UserNotesCount:       34,
			WebURL:               "https://gitlab.example.com/org/project-path/-/issues/42#note",
			Confidential:         true,
			DueDate:              "2026-04-05",
			HasTasks:             true,
			Links: &gl.IssueLinks{
				Self: "https://gitlab.example.com/links/self", Notes: "https://gitlab.example.com/links/notes",
				AwardEmoji: "https://gitlab.example.com/links/award_emoji", Project: "https://gitlab.example.com/links/project",
			},
			MovedToID: 71,
			TimeStats: &gl.TimeStats{
				HumanTimeEstimate: "2h", HumanTotalTimeSpent: "45m",
				TimeEstimate: 7200, TotalTimeSpent: 2700,
			},
			Weight:                    8,
			MergedAt:                  &merged,
			ApprovalsBeforeMerge:      9,
			ForceRemoveSourceBranch:   true,
			MergeCommitSHA:            "merge-commit-sha",
			MergeWhenPipelineSucceeds: true,
			MergeStatus:               "can_be_merged",
			Reference:                 "!81",
			Reviewers:                 []*gl.BasicUser{{ID: 24, Username: "reviewer-username"}, nil},
			SHA:                       "head-sha",
			ShouldRemoveSourceBranch:  true,
			SourceBranch:              "source-branch",
			SourceProjectID:           82,
			Squash:                    true,
			TargetBranch:              "target-branch",
			TargetProjectID:           83,
			WorkInProgress:            true,
			FileName:                  "design.png",
			ImageURL:                  "https://gitlab.example.com/design.png",
		},
	}
	extra := toolutil.TodoExtra{
		UpdatedAt: &todoUpdated,
		Group: &toolutil.NamespaceBasicOutput{
			ID: 91, Name: "group name", Path: "group-path", Kind: "group",
			FullPath: "org/group-path", ParentID: 92,
			AvatarURL: "https://gitlab.example.com/avatar/group.png",
			WebURL:    "https://gitlab.example.com/org/group-path",
		},
	}

	want := Output{
		ID: 7,
		Project: &BasicProjectOut{
			ID: 3, Description: "project description", Name: "project name",
			NameWithNamespace: "Org / project name", Path: "project-path",
			PathWithNamespace: "org/project-path", CreatedAt: "2026-01-02T02:00:00Z",
		},
		Author: &BasicUserOut{
			ID: 11, Username: "author-username", Name: "Author Name", State: "active",
			AvatarURL: "https://gitlab.example.com/avatar/author.png",
			WebURL:    "https://gitlab.example.com/author-username", CreatedAt: "2026-01-03T03:00:00Z",
		},
		ActionName: gl.TodoAssigned,
		TargetType: gl.TodoTargetIssue,
		Target: &TodoTargetOut{
			Assignees:   []*BasicUserOut{{ID: 21, Username: "assignee-username"}},
			Assignee:    &BasicUserOut{ID: 22, Username: "single-assignee-username"},
			Author:      &BasicUserOut{ID: 23, Username: "target-author-username"},
			CreatedAt:   "2026-01-04T04:00:00Z",
			Description: "target description",
			Downvotes:   31,
			ID:          float64(41),
			IID:         42,
			Labels:      []string{"label-one", "label-two"},
			Milestone: &MilestoneOut{
				ID: 51, IID: 52, GroupID: 53, ProjectID: 54,
				Title: "milestone title", Description: "milestone description",
				State: "active", WebURL: "https://gitlab.example.com/milestone",
				StartDate: "2026-02-01", DueDate: "2026-03-02",
				CreatedAt: "2026-01-05T05:00:00Z", UpdatedAt: "2026-01-06T06:00:00Z", Expired: &expired,
			},
			ProjectID:            61,
			State:                "opened",
			Subscribed:           true,
			TaskCompletionStatus: &TaskCompletionStatusOut{Count: 7, CompletedCount: 3},
			Title:                "target title",
			UpdatedAt:            "2026-01-07T07:00:00Z",
			Upvotes:              33,
			UserNotesCount:       34,
			WebURL:               "https://gitlab.example.com/org/project-path/-/issues/42#note",
			Confidential:         true,
			DueDate:              "2026-04-05",
			HasTasks:             true,
			Links: &IssueLinksOut{
				Self: "https://gitlab.example.com/links/self", Notes: "https://gitlab.example.com/links/notes",
				AwardEmoji: "https://gitlab.example.com/links/award_emoji", Project: "https://gitlab.example.com/links/project",
			},
			MovedToID: 71,
			TimeStats: &TimeStatsOut{
				HumanTimeEstimate: "2h", HumanTotalTimeSpent: "45m",
				TimeEstimate: 7200, TotalTimeSpent: 2700,
			},
			Weight:                    8,
			MergedAt:                  "2026-01-08T08:00:00Z",
			ApprovalsBeforeMerge:      9,
			ForceRemoveSourceBranch:   true,
			MergeCommitSHA:            "merge-commit-sha",
			MergeWhenPipelineSucceeds: true,
			MergeStatus:               "can_be_merged",
			Reference:                 "!81",
			Reviewers:                 []*BasicUserOut{{ID: 24, Username: "reviewer-username"}},
			SHA:                       "head-sha",
			ShouldRemoveSourceBranch:  true,
			SourceBranch:              "source-branch",
			SourceProjectID:           82,
			Squash:                    true,
			TargetBranch:              "target-branch",
			TargetProjectID:           83,
			WorkInProgress:            true,
			FileName:                  "design.png",
			ImageURL:                  "https://gitlab.example.com/design.png",
		},
		TargetURL: "https://gitlab.example.com/org/project-path/-/issues/42",
		Body:      "todo body",
		State:     "pending",
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-09T09:00:00Z",
		Group:     extra.Group,
	}

	if got := toOutput(todo, extra); !reflect.DeepEqual(got, want) {
		t.Errorf("toOutput():\n got %+v\nwant %+v", got, want)
	}
}

// TestTodoTargetOut_EachFlag_ComesFromItsOwnField verifies that every boolean a
// to-do target publishes is read from the SDK field of the same name.
//
// A block of flags has no fixture in which no two values agree, so the test
// above cannot distinguish them: with all eight set, a converter reading
// Squash for WorkInProgress passes. Each is therefore driven alone and the
// whole converted target compared with one carrying only that flag.
func TestTodoTargetOut_EachFlag_ComesFromItsOwnField(t *testing.T) {
	tests := []struct {
		name string
		set  func(*gl.TodoTarget)
		want TodoTargetOut
	}{
		{"subscribed", func(tt *gl.TodoTarget) { tt.Subscribed = true }, TodoTargetOut{Subscribed: true}},
		{"confidential", func(tt *gl.TodoTarget) { tt.Confidential = true }, TodoTargetOut{Confidential: true}},
		{"has_tasks", func(tt *gl.TodoTarget) { tt.HasTasks = true }, TodoTargetOut{HasTasks: true}},
		{"force_remove_source_branch", func(tt *gl.TodoTarget) { tt.ForceRemoveSourceBranch = true }, TodoTargetOut{ForceRemoveSourceBranch: true}},
		{"merge_when_pipeline_succeeds", func(tt *gl.TodoTarget) { tt.MergeWhenPipelineSucceeds = true }, TodoTargetOut{MergeWhenPipelineSucceeds: true}},
		{"should_remove_source_branch", func(tt *gl.TodoTarget) { tt.ShouldRemoveSourceBranch = true }, TodoTargetOut{ShouldRemoveSourceBranch: true}},
		{"squash", func(tt *gl.TodoTarget) { tt.Squash = true }, TodoTargetOut{Squash: true}},
		{"work_in_progress", func(tt *gl.TodoTarget) { tt.WorkInProgress = true }, TodoTargetOut{WorkInProgress: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var target gl.TodoTarget
			tc.set(&target)
			got := todoTargetOut(&target)
			if got == nil {
				t.Fatal("todoTargetOut() = nil, want a target")
			}
			if !reflect.DeepEqual(*got, tc.want) {
				t.Errorf("todoTargetOut() with only %s set:\n got %+v\nwant %+v", tc.name, *got, tc.want)
			}
		})
	}
}

// TestMilestoneOut_NilDates verifies milestoneOut renders empty ISO dates when
// the SDK milestone has nil start/due dates (formatISOTimePtr nil branch).
func TestMilestoneOut_NilDates(t *testing.T) {
	out := toOutput(&gl.Todo{
		Target: &gl.TodoTarget{Milestone: &gl.Milestone{ID: 1, Title: "M"}},
	}, toolutil.TodoExtra{})
	if out.Target == nil || out.Target.Milestone == nil {
		t.Fatal("expected milestone")
	}
	if out.Target.Milestone.StartDate != "" || out.Target.Milestone.DueDate != "" {
		t.Errorf("expected empty dates, got start=%q due=%q", out.Target.Milestone.StartDate, out.Target.Milestone.DueDate)
	}
}

// TestList_OrderBySort verifies that order_by and sort are forwarded as query
// parameters on the list request.
func TestList_OrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "created_at" {
			t.Errorf("expected order_by=created_at, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %q", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", Total: "0", TotalPages: "1", PerPage: "20"})
	}))
	_, err := List(context.Background(), client, ListInput{
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDecorateTodoMeta_UnknownTool verifies the no-op path for an unmapped tool.
func TestDecorateTodoMeta_UnknownTool(t *testing.T) {
	opts := userTodoOptions("gitlab_unknown")
	before := opts.Usage
	decorateTodoMeta(&opts, "gitlab_unknown")
	if opts.Usage != before {
		t.Errorf("expected unchanged usage for unknown tool, got %q", opts.Usage)
	}
}

// mustTime parses an RFC 3339 timestamp or fails the test.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return parsed
}

// mustISO parses a YYYY-MM-DD date into a gl.ISOTime pointer or fails the test.
func mustISO(t *testing.T, s string) *gl.ISOTime {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s+"T00:00:00Z")
	if err != nil {
		t.Fatalf("parse iso %q: %v", s, err)
	}
	iso := gl.ISOTime(parsed)
	return &iso
}

// TestActionSpecs_Metadata verifies that this package declares three specs and
// that each names its owner package and an individual tool. What each spec
// carries for a model to read is asserted by
// TestActionSpecs_DiscoveryMetadata_ReachesEverySpec below.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "todos" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// specsByTool builds the ActionSpecs of this package indexed by individual
// tool name, against a client no test here lets a handler reach.
func specsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestActionSpecs_DiscoveryMetadata_ReachesEverySpec verifies that the usage,
// aliases, related actions and individual-tool description the metadata table
// holds for each to-do tool are the ones its spec carries, and that none of
// them is left at the generic placeholder the options start with.
//
// It asserts the wiring rather than the wording: the table is what a reader
// edits, and repeating its sentences here would freeze prose without holding
// anything. Nothing asserted this before, so a decoration that never ran served
// "Use to execute todos domain action." as the usage of all three actions and
// attached no related actions at all, which is R-META's whole subject.
func TestActionSpecs_DiscoveryMetadata_ReachesEverySpec(t *testing.T) {
	byTool := specsByTool(t)
	for tool, meta := range todoActionMeta {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("no ActionSpec projects %s", tool)
			}
			undecorated := userTodoOptions(tool)
			if spec.Usage != meta.usage {
				t.Errorf("Usage = %q, want the table's %q", spec.Usage, meta.usage)
			}
			if spec.Usage == undecorated.Usage {
				t.Errorf("Usage is still the generic placeholder %q", undecorated.Usage)
			}
			if !reflect.DeepEqual(spec.Aliases, meta.aliases) {
				t.Errorf("Aliases = %v, want the table's %v", spec.Aliases, meta.aliases)
			}
			if !reflect.DeepEqual(spec.RelatedActions, meta.related) {
				t.Errorf("RelatedActions = %v, want the table's %v", spec.RelatedActions, meta.related)
			}
			if spec.IndividualTool.Description != meta.description {
				t.Errorf("IndividualTool.Description = %q, want the table's %q", spec.IndividualTool.Description, meta.description)
			}
		})
	}
}

// TestActionSpecs_FilterVocabularies_AreAttachedToTheListActionAlone verifies
// that the fixed action, state and type enums reach the input schema of
// gitlab_todo_list and of nothing else. The two mark-done actions take an ID
// and no filter, so an override landing on them would publish an enum for a
// parameter they do not have, and the guard that decides this was answerable
// by no assertion in the package.
func TestActionSpecs_FilterVocabularies_AreAttachedToTheListActionAlone(t *testing.T) {
	byTool := specsByTool(t)
	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			if tool != "gitlab_todo_list" {
				if len(spec.InputSchemaOverrides) != 0 {
					t.Errorf("%s carries %d input-schema override(s), want none", tool, len(spec.InputSchemaOverrides))
				}
				return
			}
			var paths []string
			for _, override := range spec.InputSchemaOverrides {
				paths = append(paths, override.PropertyPath)
				if values, ok := override.Values["enum"].([]any); !ok || len(values) == 0 {
					t.Errorf("override for %q carries no enum values: %+v", override.PropertyPath, override.Values)
				}
			}
			if want := []string{"action", "state", "type"}; !reflect.DeepEqual(paths, want) {
				t.Errorf("overridden properties = %v, want %v", paths, want)
			}
		})
	}
}

// TestDecorateTodoMeta_AnEntryThatFillsNothing_KeepsTheGenericMetadata
// verifies that each half of the decoration applies only when the table has
// something to put there, so an entry filling one field cannot blank the
// others.
//
// Every entry in the real table fills all four, so the skipping branch of each
// guard is reachable only through an entry that does not, which is what this
// installs. Without it those guards could be inverted or widened and no test
// would notice.
func TestDecorateTodoMeta_AnEntryThatFillsNothing_KeepsTheGenericMetadata(t *testing.T) {
	const tool = "gitlab_todo_nothing_filled"
	todoActionMeta[tool] = todoActionMetaEntry{}
	t.Cleanup(func() { delete(todoActionMeta, tool) })

	options := userTodoOptions(tool)
	decorateTodoMeta(&options, tool)
	if want := userTodoOptions(tool); !reflect.DeepEqual(options, want) {
		t.Errorf("decorateTodoMeta() with an empty entry:\n got %+v\nwant %+v", options, want)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestList_UnreadableCapturedGroup verifies that the todo list handler returns
// an error rather than a half-filled list when GitLab sends the group object as
// something that is not an object. The SDK ignores the key its own Todo does
// not model, so the read of the captured response is the only thing that can
// notice.
func TestList_UnreadableCapturedGroup(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"action_name":"assigned","state":"pending","group":"not-an-object"}]`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
	})
}

// TestList_CapturedFields_ArePairedWithTheirOwnTodo verifies that the two
// fields client-go's Todo does not model are read off the captured answer per
// item and in order: each to-do's own updated_at, and the group object GitLab
// renders only on a to-do raised in a group rather than a project.
//
// Nothing drove the success path of that read before, only its failure, so a
// handler pairing item i with another item's capture published one to-do's
// group and timestamp on another and no assertion could see it.
func TestList_CapturedFields_ArePairedWithTheirOwnTodo(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{
				"id": 1, "action_name": "assigned", "target_type": "Issue", "state": "pending",
				"project": {"id": 3, "path_with_namespace": "org/project-path"},
				"updated_at": "2026-01-10T10:00:00Z"
			},
			{
				"id": 2, "action_name": "mentioned", "target_type": "Epic", "state": "pending",
				"group": {"id": 91, "name": "group name", "path": "group-path", "kind": "group", "full_path": "org/group-path"},
				"updated_at": "2026-01-11T11:00:00Z"
			}
		]`, testutil.PaginationHeaders{Page: "1", Total: "2", TotalPages: "1", PerPage: "20"})
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(out.Todos))
	}
	if out.Todos[0].Group != nil {
		t.Errorf("the project-scoped to-do carries a group: %+v", out.Todos[0].Group)
	}
	if out.Todos[0].UpdatedAt != "2026-01-10T10:00:00Z" {
		t.Errorf("UpdatedAt of the first to-do = %q, want its own", out.Todos[0].UpdatedAt)
	}
	want := &toolutil.NamespaceBasicOutput{ID: 91, Name: "group name", Path: "group-path", Kind: "group", FullPath: "org/group-path"}
	if !reflect.DeepEqual(out.Todos[1].Group, want) {
		t.Errorf("group of the second to-do:\n got %+v\nwant %+v", out.Todos[1].Group, want)
	}
	if out.Todos[1].UpdatedAt != "2026-01-11T11:00:00Z" {
		t.Errorf("UpdatedAt of the second to-do = %q, want its own", out.Todos[1].UpdatedAt)
	}
}

// TestActionSpecs_CallRoutes covers ActionSpecs with table-driven subtests for call routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == pathTodos && r.Method == http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"action_name":"assigned","target_type":"Issue","target":{"title":"T"},"state":"pending","project":{"name":"p"},"author":{"username":"u"},"created_at":"2026-01-01T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", Total: "1", TotalPages: "1", PerPage: "20"})
		case r.URL.Path == pathTodoMarkDone && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == pathTodoMarkAll && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_todo_list", map[string]any{}},
		{"gitlab_todo_mark_done", map[string]any{"id": float64(1)}},
		{"gitlab_todo_mark_all_done", map[string]any{}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			result, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler %s: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}
