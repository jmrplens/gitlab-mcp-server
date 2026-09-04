//go:build e2e && enterprise

// mr_approval_ee_test.go exercises the project-level MR approval settings
// actions (approval_settings_project_get, approval_settings_project_update)
// via the gitlab_merge_request meta-tool against a live GitLab instance.
// MR approval settings are GitLab Premium/Ultimate-only; on CE GitLab the
// underlying API returns 404, so each subtest accepts a 404 outcome as a
// valid graceful-degradation signal rather than a failure.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrapprovalsettings"
)

// TestMeta_MRApprovalSettings exercises project-level MR approval settings
// via gitlab_merge_request meta-tool. Tests get and update paths.
// NOTE: MR approval settings require GitLab Premium/Ultimate; on CE GitLab the
// API returns 404. This test accepts 404 as a valid outcome (CE behavior confirmed).
func TestMeta_MRApprovalSettings(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	ctx := context.Background()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/ApprovalSettings/Get_Graceful404", func(t *testing.T) {
		if !sess.enterprise {
			t.Skip("MR approval settings are a Premium feature; the action is gated off CE")
		}
		out, err := callToolOn[mrapprovalsettings.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_settings_project_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		// 404 is expected on CE — approval settings are a Premium feature.
		// Fail fast on any other unexpected error and return early.
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Log("approval_settings_project_get returned 404 (expected on CE)")
				return
			}
			t.Fatalf("approval_settings_project_get: unexpected error: %v", err)
		}
		t.Logf("MR approval settings for project %s: allow_author=%v allow_committer=%v",
			proj.Path, out.AllowAuthorApproval.Value, out.AllowCommitterApproval.Value)
	})

	t.Run("Meta/ApprovalSettings/Update_Graceful404", func(t *testing.T) {
		if !sess.enterprise {
			t.Skip("MR approval settings are a Premium feature; the action is gated off CE")
		}
		out, err := callToolOn[mrapprovalsettings.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_settings_project_update",
			"params": map[string]any{
				"project_id":               proj.pidStr(),
				"allow_author_approval":    true,
				"retain_approvals_on_push": true,
			},
		})
		// 404 is expected on CE — approval settings are a Premium feature.
		// Fail fast on any other unexpected error and return early.
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Log("approval_settings_project_update returned 404 (expected on CE)")
				return
			}
			t.Fatalf("approval_settings_project_update: unexpected error: %v", err)
		}
		requireTruef(t, out.AllowAuthorApproval.Value, "allow_author_approval should be true after update")
		requireTruef(t, out.RetainApprovalsOnPush.Value, "retain_approvals_on_push should be true after update")
		t.Logf("Updated approval settings: allow_author=%v retain=%v",
			out.AllowAuthorApproval.Value, out.RetainApprovalsOnPush.Value)
	})
}
