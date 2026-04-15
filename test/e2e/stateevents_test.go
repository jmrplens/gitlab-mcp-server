//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
)

// TestMeta_StateEvents exercises issue/MR state event listing via meta-tools.
// It creates an issue, closes it, lists state events, then verifies MR state events too.
func TestMeta_StateEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	// Create an issue to generate state events.
	var issueIID int64

	t.Run("Meta/StateEvent/CreateIssue", func(t *testing.T) {
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "e2e-state-event-issue",
			},
		})
		requireNoError(t, err, "create issue for state events")
		requireTrue(t, out.IID > 0, "expected positive issue IID")
		issueIID = out.IID
		t.Logf("Created issue IID=%d for state event tests", issueIID)
	})

	t.Run("Meta/StateEvent/CloseIssue", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"state_event": "close",
			},
		})
		requireNoError(t, err, "close issue for state events")
		requireTrue(t, out.State == "closed", "expected issue state 'closed', got %q", out.State)
		t.Logf("Closed issue IID=%d", issueIID)
	})

	t.Run("Meta/StateEvent/ListIssueStateEvents", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[resourceevents.ListStateEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_state_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireNoError(t, err, "list issue state events")
		requireTrue(t, len(out.Events) >= 1, "expected at least 1 state event")
		t.Logf("Issue IID=%d has %d state event(s)", issueIID, len(out.Events))
	})

	// Create a branch + MR to test MR state events.
	t.Run("Meta/StateEvent/ListMRStateEvents", func(t *testing.T) {
		commitFileMeta(ctx, t, sess.meta, proj, "main", "state-event.txt", "content", "file for MR state events")
		br := createBranchMeta(ctx, t, sess.meta, proj, "e2e-state-event-branch")
		commitFileMeta(ctx, t, sess.meta, proj, br.Name, "state-event2.txt", "change", "commit on state event branch")
		mr := createMRMeta(ctx, t, sess.meta, proj, br.Name, "main", "e2e-state-event-mr")

		out, err := callToolOn[resourceevents.ListStateEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_state_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "list MR state events")
		// New MRs may not have state events until state changes, so just verify no error.
		t.Logf("MR IID=%d has %d state event(s)", mr.IID, len(out.Events))
	})

	t.Run("Meta/StateEvent/DeleteIssue", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireNoError(t, err, "delete issue for state events")
		t.Logf("Deleted issue IID=%d", issueIID)
	})
}
