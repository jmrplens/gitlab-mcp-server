//go:build e2e

// mergerequests_approval_test.go covers the Free half of a request's
// approval and merge lifecycle through the server: the pipelines listing,
// the rebase, the approve and unapprove GitLab serves on every edition, the
// configuration read that shows who approved, and the merge. The approval
// rules and settings a license adds live in the ee package. Each surface
// opens a request of its own in one shared project, since the merge at the
// end consumes it.

package common

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The merge waits: a rebase runs in a background job, and until it is done
// GitLab answers the merge with 405, which is the answer the old suite met
// and retried five times.
const (
	mergeInterval = 3 * time.Second
	mergeWait     = 120 * time.Second
)

// TestMergeRequestApproval_Lifecycle_ApproveUnapproveMerge opens a request
// of its own on every surface, lists its pipelines, rebases it, approves it
// and reads that one approval stands, withdraws the approval and reads that
// none does, then merges it and reads the merged state off the answer.
//
// Replaces: TestIndividual_MRApproval, TestMeta_MRApproval
func TestMergeRequestApproval_Lifecycle_ApproveUnapproveMerge(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrapproval"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "approval")
		params := f.params()

		// The count is not asserted, because an instance with Auto DevOps on
		// runs a pipeline for a branch that carries no configuration and one
		// with it off runs none; what is asserted is that every row the
		// listing does carry is a pipeline of this request's own branch.
		listed := harness.Do[mergerequests.PipelinesOutput](s, actionMergeRequestPipelines, params)
		for _, pipeline := range listed.Pipelines {
			if pipeline.ID == 0 || pipeline.Ref != f.branch.Name {
				e.T.Errorf("the request's pipelines hold %+v, want a pipeline with an ID on the branch %s", pipeline, f.branch.Name)
			}
		}
		e.T.Logf("the request's branch carries %d pipeline(s) before the merge", len(listed.Pipelines))

		// The answer is whether the queued job is still pending, so a rebase
		// Sidekiq finished first legitimately answers false; what the call
		// proves is that GitLab accepted it.
		rebase := harness.Do[mergerequests.RebaseOutput](s, actionMergeRequestRebase, withParams(params, map[string]any{"skip_ci": true}))
		e.T.Logf("rebase accepted: in_progress=%t", rebase.RebaseInProgress)

		// What the fixture guarantees is that one approval now stands;
		// whether the request counts as approved is GitLab's reading of a
		// request that requires none, which is logged rather than asserted.
		approved := harness.Do[mergerequests.ApproveOutput](s, actionMergeRequestApprove, params)
		if approved.ApprovedBy == 0 {
			e.T.Errorf("approve answered %+v, want the request approved by one user", approved)
		}
		e.T.Logf("approved by %d user(s): approved=%t", approved.ApprovedBy, approved.Approved)

		harness.DoVoid(s, actionMergeRequestUnapprove, params)
		config := harness.Do[mrapprovals.ConfigOutput](s, actionMergeRequestApprovalConfig, params)
		if len(config.ApprovedBy) != 0 || config.UserHasApproved {
			e.T.Errorf("the request still lists %d approver(s) and user_has_approved=%t after the unapprove: %+v", len(config.ApprovedBy), config.UserHasApproved, config.ApprovedBy)
		}

		// The rebase above rewrites the source branch, and GitLab recomputes
		// the merge status afterwards; the merge is asked for once that has
		// settled, and retried through the 405 a still-running rebase answers.
		status := fixture.WaitForMergeRequest(e, project, f.mr.IID)
		e.T.Logf("merge status before the merge: %s", status)
		merged := harness.Eventually(s, actionMergeRequestMerge, withParams(params, map[string]any{"should_remove_source_branch": true}),
			mergeInterval, mergeWait, func(out mergerequests.Output) bool { return out.State == "merged" })
		if merged.IID != f.mr.IID {
			e.T.Errorf("merge answered request !%d, want !%d", merged.IID, f.mr.IID)
		}
	})
}
