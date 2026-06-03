//go:build e2e && enterprise

// groupiterations_ee_test.go tests group iteration list operations via the
// gitlab_issue meta-tool against a live GitLab EE/Ultimate instance.
//
// NOTE: The groupiterations package exposes only iteration_list_group (read-only list).
// Create/Update/Delete actions are not registered in the action spec catalog.
//
// CAN parallelize: separate group per test.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupiterations"
)

// TestMeta_GroupIterations exercises group iteration list via gitlab_issue.
// Requires GitLab Premium (GITLAB_ENTERPRISE=true).
func TestMeta_GroupIterations(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		t.Skip("group iterations require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()

	t.Run("Meta/GroupIteration/List_Group0_Graceful", func(t *testing.T) {
		// Group ID 0 — expected to be empty or 404 on a fresh Docker EE.
		out, err := callToolOn[groupiterations.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "iteration_list_group",
			"params": map[string]any{
				"group_id": "0",
				"state":    "opened",
			},
		})
		// 404 is the expected outcome on a fresh EE instance; fail fast on
		// any other unexpected error and return early.
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Logf("iteration_list_group returned 404 (expected for group 0 on fresh EE): %v", err)
				return
			}
			t.Fatalf("iteration_list_group: unexpected error: %v", err)
		}
		t.Logf("iteration_list_group on group 0: %d iterations", len(out.Iterations))
	})

	t.Run("Meta/GroupIteration/List_WithTestGroup", func(t *testing.T) {
		// Create a group via the shared fixture so the resource is registered
		// in the per-test ledger and cleaned up automatically.
		e2e := NewE2EContext(t)
		grp := CreateGroupMeta(ctx, e2e, sess.meta, "e2e-iterations")
		groupIDStr := grp.gidStr()

		out, err := callToolOn[groupiterations.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "iteration_list_group",
			"params": map[string]any{
				"group_id": groupIDStr,
			},
		})
		// 404 or empty list are both acceptable outcomes on a fresh EE instance.
		// Return early on 404 to avoid the rest of the assertion path.
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Logf("iteration_list_group returned 404 for group %d (expected on fresh EE)", grp.ID)
				return
			}
			t.Fatalf("iteration_list_group on test group: unexpected error: %v", err)
		}
		t.Logf("Group %s iterations: %d (may be empty)", grp.Path, len(out.Iterations))
	})
}
