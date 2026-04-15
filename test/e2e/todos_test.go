//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
)

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
