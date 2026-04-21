//go:build e2e

// todos_test.go tests the GitLab todo MCP tools against a live GitLab instance.
// Covers listing todos and marking all as done via both individual tools and
// the gitlab_user meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
)

// TestIndividual_Todos exercises the todo tools via individual MCP tools:
// list all todos, then mark all as done.
func TestIndividual_Todos(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[todos.ListOutput](ctx, sess.individual, "gitlab_todo_list", todos.ListInput{})
		requireNoError(t, err, "list todos")
		t.Logf("Listed %d todos", len(out.Todos))
	})

	t.Run("MarkAllDone", func(t *testing.T) {
		out, err := callToolOn[todos.MarkAllDoneOutput](ctx, sess.individual, "gitlab_todo_mark_all_done", todos.MarkAllDoneInput{})
		requireNoError(t, err, "mark all todos done")
		t.Logf("Marked all done: %s", out.Message)
	})
}

// TestMeta_Todos exercises the same todo lifecycle via the gitlab_user meta-tool:
// todo_list and todo_mark_all_done actions.
func TestMeta_Todos(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[todos.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "todo_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta list todos")
		t.Logf("Listed %d todos via meta-tool", len(out.Todos))
	})

	t.Run("MarkAllDone", func(t *testing.T) {
		out, err := callToolOn[todos.MarkAllDoneOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "todo_mark_all_done",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta mark all done")
		t.Logf("Marked all done via meta-tool: %s", out.Message)
	})
}
