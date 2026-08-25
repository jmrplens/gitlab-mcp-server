//go:build e2e && enterprise

// mrdeps_ee_test.go covers the merge request blocking-dependency actions the
// e2e gap audit reported as unexercised: dependency_create and
// dependency_delete via the gitlab_merge_request meta-tool. Blocking MR
// dependencies are a Premium feature in practice, so the coverage lives in
// the enterprise suite even though the catalog currently classifies the
// actions as free.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
)

// mrDepsCreateMR builds one branch with a commit and opens a merge request
// from it, waiting until GitLab reports the MR ready so dependency calls do
// not race MR creation.
func mrDepsCreateMR(ctx context.Context, t *testing.T, proj ProjectFixture, branchPrefix, title string) MRFixture {
	t.Helper()
	branch := uniqueName(branchPrefix)
	createBranchMeta(ctx, t, sess.meta, proj, branch)
	commitFileMeta(ctx, t, sess.meta, proj, branch, branch+".txt", "blocking dependency fixture", title)
	// Deliberately no waitForMRReady here. A dependency links two MR
	// records; none of the endpoints under test read DetailedMergeStatus,
	// so waiting for GitLab to compute it buys nothing — and two
	// sequential readiness waits, each capped at the 300-second
	// enterprise budget, cannot fit inside this test's 420 seconds on a
	// loaded parallel run. That arithmetic is exactly how this test once
	// burned its whole deadline inside a best-effort helper and then
	// failed on the next call with a bare "context deadline exceeded".
	return createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, title)
}

// mrDepsGlobalID resolves the instance-global merge request ID for an IID,
// because dependency_create expects the blocking MR's database ID, not its
// project-scoped IID.
func mrDepsGlobalID(ctx context.Context, t *testing.T, proj ProjectFixture, mrIID int64) int64 {
	t.Helper()
	mr, _, err := sess.glClient.GL().MergeRequests.GetMergeRequest(proj.ID, mrIID, nil, gl.WithContext(ctx))
	requireNoError(t, err, "resolve merge request global ID")
	requireTruef(t, mr.ID > 0, "expected global MR ID > 0")
	return mr.ID
}

// TestMeta_MRBlockingDependencies exercises dependency_create and
// dependency_delete through the gitlab_merge_request meta-tool.
//
// The test creates two merge requests in one project, marks the first as a
// blocker of the second via dependency_create, verifies the link through
// the dependencies_list action, and removes it via dependency_delete
// asserting the list is empty afterwards. A 403 on create is tolerated as
// the documented Premium license gate (the endpoint rejects Free-tier
// runtimes even though the catalog lists the action as free); every later
// step is asserted strictly once the create succeeded.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_MRBlockingDependencies(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := e2eTimeoutContext(180*time.Second, 420*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)
	blocking := mrDepsCreateMR(ctx, t, proj, "mrdeps-blocking", "mrdeps blocking fixture")
	dependent := mrDepsCreateMR(ctx, t, proj, "mrdeps-dependent", "mrdeps dependent fixture")
	blockingGlobalID := mrDepsGlobalID(ctx, t, proj, blocking.IID)

	// The delete endpoint addresses the block record created below, not the
	// blocking MR itself, so the created dependency ID is shared across
	// subtests.
	var dependencyID int64

	t.Run("DependencyCreate", func(t *testing.T) {
		out, err := callToolOn[mergerequests.DependencyOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "dependency_create",
			"params": map[string]any{
				"project_id":                proj.pidStr(),
				"merge_request_iid":         dependent.IID,
				"blocking_merge_request_id": blockingGlobalID,
			},
		})
		if err != nil && isHTTPStatus(err, 403) {
			t.Skipf("blocking MR dependencies rejected with 403 (Premium license gate); routing validated: %v", err)
		}
		requireNoError(t, err, "dependency_create")
		requireTruef(t, out.ID > 0, "expected dependency ID > 0")
		requireTruef(t, out.BlockingMergeRequest != nil && out.BlockingMergeRequest.IID == blocking.IID,
			"expected blocking MR IID %d in dependency output", blocking.IID)
		dependencyID = out.ID
		t.Logf("Created dependency %d: MR !%d blocked by MR !%d", out.ID, dependent.IID, blocking.IID)
	})

	t.Run("DependenciesList", func(t *testing.T) {
		out, err := callToolOn[mergerequests.DependenciesOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "dependencies_list",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": dependent.IID,
			},
		})
		if err != nil && isHTTPStatus(err, 403) {
			t.Skipf("blocking MR dependencies rejected with 403 (Premium license gate): %v", err)
		}
		requireNoError(t, err, "dependencies_list")
		requireTruef(t, len(out.Dependencies) == 1, "expected 1 dependency, got %d", len(out.Dependencies))
	})

	t.Run("DependencyDelete", func(t *testing.T) {
		if dependencyID == 0 {
			t.Skip("dependency was never created (Premium license gate); nothing to delete")
		}
		// GitLab's delete endpoint takes the block record ID in the path; the
		// action input reuses the blocking_merge_request_id key for it.
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "dependency_delete",
			"params": map[string]any{
				"project_id":                proj.pidStr(),
				"merge_request_iid":         dependent.IID,
				"blocking_merge_request_id": dependencyID,
			},
		})
		if err != nil && isHTTPStatus(err, 403) {
			t.Skipf("blocking MR dependencies rejected with 403 (Premium license gate): %v", err)
		}
		requireNoError(t, err, "dependency_delete")

		out, listErr := callToolOn[mergerequests.DependenciesOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "dependencies_list",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": dependent.IID,
			},
		})
		requireNoError(t, listErr, "dependencies_list after delete")
		requireTruef(t, len(out.Dependencies) == 0, "expected 0 dependencies after delete, got %d", len(out.Dependencies))
	})
}
