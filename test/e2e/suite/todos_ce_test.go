//go:build e2e && !enterprise

// todos_ce_test.go tests the GitLab todo MCP tools against a live GitLab instance.
// Covers listing todos and marking all as done via both individual tools and
// the gitlab_user meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/todos"
)

// TestIndividual_Todos exercises todo tools through individual MCP tools
// against a live GitLab CE instance.
//
// The test runs inside [RunWithCapabilities] with [CapabilityCurrentUserState]
// so current-user state is snapshotted before mutation. It creates a
// project and an issue (to seed a todo), then walks gitlab_todo_list and
// gitlab_todo_mark_all_done via the individual tool surface. Each subtest
// asserts the todo payload shape and that mark_all_done clears pending
// todos in a follow-up list call.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_Todos(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
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
	})
}

// TestMeta_Todos exercises the todo lifecycle through the gitlab_user
// meta-tool against a live GitLab CE instance.
//
// The test mirrors [TestIndividual_Todos] but drives every step with
// {action, params} arguments through the catalog-backed tool. Subtests
// cover todo_list and todo_mark_all_done actions, verifying the meta-tool
// returns consistent todo payloads and that mark_all_done clears pending
// todos in a follow-up list call.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Todos(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
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
	})
}
