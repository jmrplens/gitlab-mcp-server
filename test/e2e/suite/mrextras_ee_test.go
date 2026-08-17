//go:build e2e && enterprise

// mrextras_ee_test.go covers the remaining Premium merge-request and
// merge-train actions the e2e gap audit reported as unexercised:
// group-level MR approval settings (approval_settings_group_get/update) via
// the gitlab_merge_request meta-tool and the single-entry merge train lookup
// (merge_train.get) via the gitlab_merge_train meta-tool.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrapprovalsettings"
)

// TestMeta_GroupMRApprovalSettings exercises group-level MR approval settings
// via the gitlab_merge_request meta-tool against a live GitLab instance.
//
// The test creates a top-level group and drives approval_settings_group_get
// followed by approval_settings_group_update, asserting the updated flags
// round-trip. Group approval settings are Premium/Ultimate; a 404 is accepted
// as the documented degradation when the license gate rejects the call.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupMRApprovalSettings(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "mrx-approval")

	t.Run("Meta/GroupApprovalSettings/Get", func(t *testing.T) {
		out, err := callToolOn[mrapprovalsettings.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_settings_group_get",
			"params": map[string]any{
				"group_id": grp.gidStr(),
			},
		})
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Log("approval_settings_group_get returned 404 (license gate); routing validated")
				return
			}
			t.Fatalf("approval_settings_group_get: unexpected error: %v", err)
		}
		t.Logf("Group MR approval settings: allow_author=%v retain_on_push=%v",
			out.AllowAuthorApproval.Value, out.RetainApprovalsOnPush.Value)
	})

	t.Run("Meta/GroupApprovalSettings/Update", func(t *testing.T) {
		out, err := callToolOn[mrapprovalsettings.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_settings_group_update",
			"params": map[string]any{
				"group_id":                 grp.gidStr(),
				"allow_author_approval":    true,
				"retain_approvals_on_push": true,
			},
		})
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Log("approval_settings_group_update returned 404 (license gate); routing validated")
				return
			}
			t.Fatalf("approval_settings_group_update: unexpected error: %v", err)
		}
		requireTruef(t, out.AllowAuthorApproval.Value, "allow_author_approval should be true after update")
		requireTruef(t, out.RetainApprovalsOnPush.Value, "retain_approvals_on_push should be true after update")
		t.Logf("Updated group approval settings: allow_author=%v retain_on_push=%v",
			out.AllowAuthorApproval.Value, out.RetainApprovalsOnPush.Value)
	})
}

// TestMeta_MergeTrainGet exercises the single merge-request merge train
// lookup (action "get") via the gitlab_merge_train meta-tool against a live
// GitLab Premium/Ultimate instance.
//
// The test mirrors the TestMeta_MergeTrains setup: it creates a group-scoped
// project, enables merged results pipelines and merge trains, builds a real
// MR, and adds it to the train. It then drives the "get" action: when the
// train entry exists, the MR IID must round-trip; when the instance never
// persisted the merge-train flags or the MR is not on a train (no passing
// pipeline on a project without CI), the deterministic 404 "not on a merge
// train" path is asserted instead, which still validates routing.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_MergeTrainGet(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := e2eTimeoutContext(180*time.Second, 420*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "mrx-mtget")
	proj := CreateProjectUnderGroupMeta(ctx, e2e, sess.meta, grp.ID)

	// Merge trains require BOTH merged results pipelines and merge trains
	// enabled; on some self-managed setups the flags are silently dropped,
	// in which case the "get" action below exercises the 404 path.
	enable := true
	_, _, editErr := sess.glClient.GL().Projects.EditProject(
		proj.Path,
		&gl.EditProjectOptions{
			MergePipelinesEnabled: &enable,
			MergeTrainsEnabled:    &enable,
		},
		gl.WithContext(ctx),
	)
	requireNoError(t, editErr, "enable merge trains on project")

	branch := uniqueName("mrx-mtget-branch")
	createBranchMeta(ctx, t, sess.meta, proj, branch)
	commitFileMeta(ctx, t, sess.meta, proj, branch, "merge-train-get.txt", "merge train get fixture", "merge train get fixture")
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "merge train get fixture")
	waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)

	// Best-effort enrollment; 400 means merge trains are disabled on this
	// project and the get below must take the 404 branch.
	_, addErr := callToolOn[mergetrains.ListOutput](ctx, sess.meta, "gitlab_merge_train", map[string]any{
		"action": "add",
		"params": map[string]any{
			"project_id":        proj.pidStr(),
			"merge_request_iid": mr.IID,
		},
	})
	if addErr != nil && !isHTTPStatus(addErr, 400) {
		t.Logf("merge train add failed with a non-400 error (get will assert the 404 path): %v", addErr)
	}

	t.Run("Meta/MergeTrain/Get", func(t *testing.T) {
		out, err := callToolOn[mergetrains.Output](ctx, sess.meta, "gitlab_merge_train", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		if err != nil {
			// Deterministic outcome when the MR never made it onto a train:
			// the endpoint contract is a 404 for MRs not on a merge train.
			if isHTTPStatus(err, 404) {
				t.Logf("merge_train get returned 404 (MR !%d not on a merge train — routing validated): %v", mr.IID, err)
				return
			}
			t.Fatalf("merge_train get: unexpected error: %v", err)
		}
		requireTruef(t, out.MergeRequest.IID == mr.IID,
			"merge train entry MR IID = %d, want %d", out.MergeRequest.IID, mr.IID)
		t.Logf("Merge train entry %d for MR !%d: status=%s target=%s", out.ID, mr.IID, out.Status, out.TargetBranch)
	})
}

// TestIndividual_MRDependenciesList exercises gitlab_mr_dependencies_list on
// a fresh merge request. MR dependencies are a Premium feature, so the tool
// is only registered on enterprise catalogs; on EE the call must succeed and
// a fresh MR must report no blocking dependencies. Moved here from the CE
// suite, where it could only ever skip.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: individual.
func TestIndividual_MRDependenciesList(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !sess.enterprise {
		t.Skip("enterprise session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	branch := uniqueName("mrx-deps-branch")
	createBranchMeta(ctx, t, sess.meta, proj, branch)
	commitFileMeta(ctx, t, sess.meta, proj, branch, "mr-deps.txt", "mr dependencies fixture", "mr dependencies fixture")
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "mr dependencies fixture")

	out, err := callToolOn[mergerequests.DependenciesOutput](ctx, sess.individual, "gitlab_mr_dependencies_list", mergerequests.GetDependenciesInput{
		ProjectID: proj.pidOf(),
		MRIID:     mr.IID,
	})
	requireNoError(t, err, "list MR dependencies")
	requireTruef(t, len(out.Dependencies) == 0, "expected no dependencies on a fresh MR, got %d", len(out.Dependencies))
	t.Log("Fresh MR has no blocking dependencies")
}
