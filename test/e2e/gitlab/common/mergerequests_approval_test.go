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

// assertApprovalStateByEdition holds the approval state an approvals answer
// carried to the edition the run is on. Every Enterprise build answers the
// approvals GET, the approve and the unapprove with the merge request's whole
// approval state, licensed or not, because the override of present_approval
// checks no license; a Community Edition instance sends the four keys every
// edition sends and none of these, and the server publishes none of them
// rather than zeros.
func assertApprovalStateByEdition(e *harness.Env, action string, iid int64, state mrapprovals.EnterpriseApprovalState) {
	e.T.Helper()
	if !e.Runtime().Enterprise {
		if state.IID != nil || state.ApprovalsRequired != nil || state.ApprovalsLeft != nil || state.HasApprovalRules != nil {
			e.T.Errorf("%s on a Community Edition instance published Enterprise keys %+v, want none", action, state)
		}
		return
	}
	if state.IID == nil || *state.IID != iid {
		e.T.Errorf("%s on an Enterprise instance published iid %v, want %d", action, state.IID, iid)
	}
	if state.ApprovalsRequired == nil || state.ApprovalsLeft == nil || state.MergeRequestApproversAvailable == nil {
		e.T.Errorf("%s on an Enterprise instance published %+v, want approvals_required, approvals_left and merge_request_approvers_available", action, state)
	}
}

// TestMergeRequestApproval_Lifecycle_ApproveUnapproveMerge opens a request
// of its own on every surface, lists its pipelines, rebases it, approves it
// and reads that one approval stands, withdraws the approval and reads that
// none does, then merges it and reads the merged state off the answer.
//
// Replaces: TestIndividual_MRApproval, TestMeta_MRApproval
func TestMergeRequestApproval_Lifecycle_ApproveUnapproveMerge(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("mrapproval"))
		// The run holds one credential, so every request below is approved
		// by the user that opened it. A licensed instance refuses that with
		// 401 unless the project says otherwise, and an unlicensed one
		// permits it already.
		if !fixture.AllowAuthorApproval(e, project) {
			e.T.Fatalf("project %d still prevents approval by the author, so the approve action cannot be reached", project.ID)
		}
		return project
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
		assertApprovalStateByEdition(e, "approve", f.mr.IID, approved.EnterpriseApprovalState)

		// The withdrawal answers with the state it left, which client-go
		// discards and the server reads off the answer instead.
		unapproved := harness.Do[mergerequests.ApproveOutput](s, actionMergeRequestUnapprove, params)
		if unapproved.ApprovedBy != 0 || unapproved.UserHasApproved {
			e.T.Errorf("unapprove answered %+v, want no approval left and user_has_approved false", unapproved)
		}
		assertApprovalStateByEdition(e, "unapprove", f.mr.IID, unapproved.EnterpriseApprovalState)

		config := harness.Do[mrapprovals.ConfigOutput](s, actionMergeRequestApprovalConfig, params)
		if len(config.ApprovedBy) != 0 || config.UserHasApproved {
			e.T.Errorf("the request still lists %d approver(s) and user_has_approved=%t after the unapprove: %+v", len(config.ApprovedBy), config.UserHasApproved, config.ApprovedBy)
		}
		assertApprovalStateByEdition(e, "approval_config", f.mr.IID, config.EnterpriseApprovalState)

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
