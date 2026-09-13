//go:build e2e

// b7_issue_subscriptions_test.go covers what a caller attaches to themselves
// on an issue: the notification subscription, and the to-do item. Both were
// asserted by the old CE suite and reached by nothing in the rebuilt one.
//
// Each scenario takes an issue of its own per surface, because both routes
// write state that belongs to the calling user and the same user drives all
// three surfaces: a second subscribe to one issue, and a second to-do on
// one issue, are each answered "not modified" rather than run again.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestIssueSubscription_SubscribeThenUnsubscribe subscribes the run's user to
// an issue of its own on every surface and reads the subscription off the
// answer, then unsubscribes and reads it gone.
//
// The author of an issue is subscribed to it by participation, so the
// subscribe may be GitLab's "not modified" rather than a change; the handler
// answers that with a fresh read, and the flag is what this asserts either
// way. The unsubscribe is the half that always changes something, since it
// records an explicit refusal over that implicit subscription.
func TestIssueSubscription_SubscribeThenUnsubscribe(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuesubscribe"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		issue := fixture.NewIssue(e, project, "subscription fixture for the "+string(surface)+" surface")
		params := map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID}

		subscribed := harness.Do[issues.Output](s, actionIssueSubscribe, params)
		if subscribed.IID != issue.IID || !subscribed.Subscribed {
			e.T.Errorf("subscribe answered %+v, want issue #%d carrying subscribed=true", subscribed, issue.IID)
		}

		unsubscribed := harness.Do[issues.Output](s, actionIssueUnsubscribe, params)
		if unsubscribed.IID != issue.IID || unsubscribed.Subscribed {
			e.T.Errorf("unsubscribe answered %+v, want issue #%d carrying subscribed=false", unsubscribed, issue.IID)
		}
	})
}

// TestIssueTodo_Create_AddsAPendingTodoNamingTheIssue creates a to-do on an
// issue of its own on every surface and holds the answer to the issue it was
// made from: an identifier, the pending state, and the issue's title as the
// to-do's target.
//
// It holds LockCurrentUserState for the reason todos_test.go does: a to-do
// belongs to the authenticated user, which every session of every package
// shares, and that file marks every to-do of that user done.
func TestIssueTodo_Create_AddsAPendingTodoNamingTheIssue(t *testing.T) {
	e := harness.New(t, harness.Locks(harness.LockCurrentUserState))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuetodo"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		title := "to-do fixture for the " + string(surface) + " surface"
		issue := fixture.NewIssue(e, project, title)

		todo := harness.Do[issues.TodoOutput](s, actionIssueCreateTodo,
			map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID})
		if todo.ID == 0 {
			e.T.Fatalf("create_todo answered %+v, want a to-do with an ID", todo)
		}
		if todo.State != todoStatePending {
			e.T.Errorf("the to-do created from issue #%d is %q, want %q", issue.IID, todo.State, todoStatePending)
		}
		if todo.TargetTitle != title {
			e.T.Errorf("the to-do created from issue #%d targets %q, want the issue titled %q", issue.IID, todo.TargetTitle, title)
		}
	})
}
