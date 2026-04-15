//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
)

// TestIndividual_MRApproval exercises the MR approval/merge lifecycle via individual tools.
func TestIndividual_MRApproval(t *testing.T) {
	ctx := context.Background()
	proj := createProject(ctx, t, sess.individual)

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
		var out mergerequests.Output
		var err error
		for i := range 5 {
			out, err = callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_merge", mergerequests.MergeInput{
				ProjectID:                proj.pidOf(),
				MRIID:                    mr.IID,
				ShouldRemoveSourceBranch: new(true),
			})
			if err == nil {
				break
			}
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
		requireNoError(t, err, "merge MR")
		requireTrue(t, out.State == "merged", "expected state 'merged', got %q", out.State)
		t.Logf("Merged MR !%d", mr.IID)
	})
}

// TestMeta_MRApproval exercises the MR approval/merge lifecycle via the gitlab_merge_request meta-tool.
func TestMeta_MRApproval(t *testing.T) {
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	commitFileMeta(ctx, t, sess.meta, proj, "main", "approval.txt", "base", "base commit for approval")
	createBranchMeta(ctx, t, sess.meta, proj, "feature-approval")
	commitFileMeta(ctx, t, sess.meta, proj, "feature-approval", "feature.txt", "new feature", "feature commit")
	mr := createMRMeta(ctx, t, sess.meta, proj, "feature-approval", "main", "MR for approval test")

	t.Run("Meta/MR/Pipelines", func(t *testing.T) {
		_, err := callToolOn[mergerequests.PipelinesOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "pipelines",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "meta MR pipelines")
		t.Logf("MR pipelines listed via meta-tool")
	})

	t.Run("Meta/MR/Rebase", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "rebase",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
				"skip_ci":    true,
			},
		})
		requireNoError(t, err, "meta MR rebase")
		t.Logf("Rebased MR !%d via meta-tool", mr.IID)
	})

	t.Run("Meta/MR/Approve", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ApproveOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approve",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "meta approve MR")
		t.Logf("Approved MR !%d (approved=%v)", mr.IID, out.Approved)
	})

	t.Run("Meta/MR/Unapprove", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "unapprove",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "meta unapprove MR")
		t.Logf("Unapproved MR !%d via meta-tool", mr.IID)
	})

	t.Run("Meta/MR/Merge", func(t *testing.T) {
		var out mergerequests.Output
		var err error
		for i := range 5 {
			out, err = callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
				"action": "merge",
				"params": map[string]any{
					"project_id":                  proj.pidStr(),
					"mr_iid":                      mr.IID,
					"should_remove_source_branch": true,
				},
			})
			if err == nil {
				break
			}
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
		requireNoError(t, err, "meta merge MR")
		requireTrue(t, out.State == "merged", "expected state merged, got %q", out.State)
		t.Logf("Merged MR !%d via meta-tool", mr.IID)
	})
}
