//go:build e2e

// todos_test.go covers the run user's to-do list: a pending item listed,
// one marked done by ID, and the whole list marked done. The items are
// raised on an issue of the test's own project, since a to-do belongs to
// the account every session runs on and nothing else in the run should see
// its list change; the test holds the current-user lock for the same
// reason.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The to-do states GitLab lists by.
const (
	todoStatePending = "pending"
	todoStateDone    = "done"
)

// TestTodos_OwnIssue_ListMarkDoneAndMarkAllDone raises two to-dos on an
// issue of its own per surface, finds the first pending, marks it done by
// ID, marks everything done and checks the second left the pending list
// for the done one.
//
// Replaces: TestIndividual_Todos, TestMeta_Todos, TestMeta_UserTodosEvents, TestIndividual_UserTodoMarkDone
func TestTodos_OwnIssue_ListMarkDoneAndMarkAllDone(t *testing.T) {
	e := harness.New(t, harness.Locks(harness.LockCurrentUserState))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("todo"))
		first := fixture.NewIssue(e, project, "to-do target one")
		second := fixture.NewIssue(e, project, "to-do target two")
		firstTodo := raiseTodo(e, project, first)

		pending := harness.Do[todos.ListOutput](s, actionUserTodoList, map[string]any{"state": todoStatePending, "project_id": project.ID})
		if !containsID(todoIDs(pending.Todos), firstTodo) {
			e.T.Errorf("the pending to-dos of project %d do not hold %d: %+v", project.ID, firstTodo, pending.Todos)
		}

		done := harness.Do[todos.MarkDoneOutput](s, actionUserTodoMarkDone, map[string]any{"id": firstTodo})
		if done.ID != firstTodo || done.Message == "" {
			e.T.Errorf("todo_mark_done answered %+v, want a confirmation naming to-do %d", done, firstTodo)
		}

		secondTodo := raiseTodo(e, project, second)
		all := harness.Do[todos.MarkAllDoneOutput](s, actionUserTodoMarkAllDone, nil)
		if all.Message == "" {
			e.T.Errorf("todo_mark_all_done answered %+v, want a confirmation", all)
		}

		stillPending := harness.Do[todos.ListOutput](s, actionUserTodoList, map[string]any{"state": todoStatePending, "project_id": project.ID})
		if ids := todoIDs(stillPending.Todos); containsID(ids, firstTodo) || containsID(ids, secondTodo) {
			e.T.Errorf("to-dos %d and %d are still pending after being marked done: %+v", firstTodo, secondTodo, stillPending.Todos)
		}
		finished := harness.Do[todos.ListOutput](s, actionUserTodoList, map[string]any{"state": todoStateDone, "project_id": project.ID})
		if ids := todoIDs(finished.Todos); !containsID(ids, firstTodo) || !containsID(ids, secondTodo) {
			e.T.Errorf("the done to-dos of project %d do not hold both %d and %d: %+v", project.ID, firstTodo, secondTodo, finished.Todos)
		}
	})
}

// raiseTodo creates a pending to-do for the run user on an issue through
// client-go, the one deterministic way to get one: assigning oneself an
// issue raises none.
func raiseTodo(e *harness.Env, project fixture.Project, issue fixture.Issue) int64 {
	e.T.Helper()

	todo, _, err := e.Client().GL().Issues.CreateTodo(project.ID, issue.IID, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("raising a to-do on issue #%d of project %d: %v", issue.IID, project.ID, err)
	}
	if todo.ID == 0 {
		e.T.Fatalf("the to-do raised on issue #%d has no ID", issue.IID)
	}
	return todo.ID
}

// todoIDs collects the IDs of listed to-dos.
func todoIDs(listed []todos.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, todo := range listed {
		ids = append(ids, todo.ID)
	}
	return ids
}
