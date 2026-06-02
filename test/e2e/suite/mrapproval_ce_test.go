//go:build e2e && !enterprise

// mrapproval_ce_test.go tests the MR approval and merge lifecycle MCP tools against
// a live GitLab instance. Covers pipelines listing, rebase, approve, unapprove,
// and merge for both individual and meta-tool modes.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
)

// TestIndividual_MRApproval exercises the MR approval/merge lifecycle via individual tools.
func TestIndividual_MRApproval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProject(ctx, t, sess.individual)
	if sess.enterprise {
		allowAuthorApproval := true
		disableCommitterApproval := false
		_, err := callToolOn[projects.ApprovalConfigOutput](ctx, sess.individual, "gitlab_project_approval_config_change", projects.ChangeApprovalConfigInput{
			ProjectID:                              proj.pidOf(),
			MergeRequestsAuthorApproval:            &allowAuthorApproval,
			MergeRequestsDisableCommittersApproval: &disableCommitterApproval,
		})
		requireNoError(t, err, "allow MR author approval")
	}

	commitFile(ctx, t, sess.individual, proj, "main", "approval.txt", "base", "base commit for approval")
	createBranch(ctx, t, sess.individual, proj, "feature-approval")
	commitFile(ctx, t, sess.individual, proj, "feature-approval", "feature.txt", "new feature", "feature commit")
	mr := createMR(ctx, t, sess.individual, proj, "feature-approval", "main", "MR for approval test")

	t.Run("Individual/MR/Pipelines", func(t *testing.T) {
		out, err := callToolOn[mergerequests.PipelinesOutput](ctx, sess.individual, "gitlab_mr_pipelines", mergerequests.PipelinesInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "list MR pipelines")
		t.Logf("MR !%d has %d pipelines", mr.IID, len(out.Pipelines))
	})

	t.Run("Individual/MR/Rebase", func(t *testing.T) {
		out, err := callToolOn[mergerequests.RebaseOutput](ctx, sess.individual, "gitlab_mr_rebase", mergerequests.RebaseInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			SkipCI:    true,
		})
		requireNoError(t, err, "rebase MR")
		t.Logf("Rebase MR !%d: in_progress=%v", mr.IID, out.RebaseInProgress)
	})

	t.Run("Individual/MR/Approve", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ApproveOutput](ctx, sess.individual, "gitlab_mr_approve", mergerequests.ApproveInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "approve MR")
		t.Logf("Approved MR !%d (approved=%v)", mr.IID, out.Approved)
	})

	t.Run("Individual/MR/Unapprove", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_unapprove", mergerequests.ApproveInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "unapprove MR")
		t.Logf("Unapproved MR !%d", mr.IID)
	})

	t.Run("Individual/MR/Merge", func(t *testing.T) {
		_, err := callToolOn[mergerequests.ApproveOutput](ctx, sess.individual, "gitlab_mr_approve", mergerequests.ApproveInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "re-approve MR before merge")
		drainSidekiq(ctx, t, sess.glClient)
		waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)
		var out mergerequests.Output
		for i := range 5 {
			out, err = callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_merge", mergerequests.MergeInput{
				ProjectID:                proj.pidOf(),
				MRIID:                    mr.IID,
				ShouldRemoveSourceBranch: new(true),
			})
			if err == nil {
				break
			}
			t.Logf("merge attempt %d: %v", i+1, err)
			waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)
		}
		requireNoError(t, err, "merge MR")
		requireTruef(t, out.State == "merged", "expected state 'merged', got %q", out.State)
		t.Logf("Merged MR !%d", mr.IID)
	})
}

// TestMeta_MRApproval exercises the MR approval/merge lifecycle via the gitlab_merge_request meta-tool.
func TestMeta_MRApproval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)
	if sess.enterprise {
		allowAuthorApproval := true
		disableCommitterApproval := false
		_, err := callToolOn[projects.ApprovalConfigOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_config_change",
			"params": map[string]any{
				"project_id":                                 proj.pidStr(),
				"merge_requests_author_approval":             allowAuthorApproval,
				"merge_requests_disable_committers_approval": disableCommitterApproval,
			},
		})
		requireNoError(t, err, "allow meta MR author approval")
	}

	commitFileMeta(ctx, t, sess.meta, proj, "main", "approval.txt", "base", "base commit for approval")
	createBranchMeta(ctx, t, sess.meta, proj, "feature-approval")
	commitFileMeta(ctx, t, sess.meta, proj, "feature-approval", "feature.txt", "new feature", "feature commit")
	mr := createMRMeta(ctx, t, sess.meta, proj, "feature-approval", "main", "MR for approval test")

	t.Run("Meta/MR/Pipelines", func(t *testing.T) {
		_, err := callToolOn[mergerequests.PipelinesOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "pipelines",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "meta MR pipelines")
		t.Logf("MR pipelines listed via meta-tool")
	})

	t.Run("Meta/MR/Rebase", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "rebase",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"skip_ci":           true,
			},
		})
		requireNoError(t, err, "meta MR rebase")
		t.Logf("Rebased MR !%d via meta-tool", mr.IID)
	})

	t.Run("Meta/MR/Approve", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ApproveOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approve",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "meta approve MR")
		t.Logf("Approved MR !%d (approved=%v)", mr.IID, out.Approved)
	})

	t.Run("Meta/MR/Unapprove", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "unapprove",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "meta unapprove MR")
		t.Logf("Unapproved MR !%d via meta-tool", mr.IID)
	})

	t.Run("Meta/MR/Merge", func(t *testing.T) {
		drainSidekiq(ctx, t, sess.glClient)
		waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)
		var out mergerequests.Output
		var err error
		for i := range 5 {
			out, err = callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
				"action": "merge",
				"params": map[string]any{
					"project_id":                  proj.pidStr(),
					"merge_request_iid":           mr.IID,
					"should_remove_source_branch": true,
				},
			})
			if err == nil {
				break
			}
			t.Logf("meta merge attempt %d: %v", i+1, err)
			waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)
		}
		requireNoError(t, err, "meta merge MR")
		requireTruef(t, out.State == "merged", "expected state merged, got %q", out.State)
		t.Logf("Merged MR !%d via meta-tool", mr.IID)
	})
}

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
