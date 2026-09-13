//go:build e2e

// b7_mr_automerge_test.go covers the cancel that disarms auto-merge, which is
// the one merge request action that cannot be reached without first putting
// the request into a state nothing else in this package needs.
//
// Auto-merge is "merge when the pipeline succeeds", so arming it needs a head
// pipeline that has not finished: with no pipeline at all GitLab merges the
// request there and then, and there is nothing left to cancel. The branch
// therefore carries a configuration whose only job is pinned to a runner tag
// nothing on any instance has. The pipeline is created on the push and its job
// is never picked up, so the pipeline stays pending — which GitLab counts as
// active — for as long as the scenario needs, on an instance with a runner and
// on one without. That is also why this declares no harness.NeedRunner: a
// runner would make the scenario flakier rather than possible, since a job it
// could pick up might finish before the merge is asked for.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// mrAutoMergeCIYAML declares one job pinned to a runner tag nothing carries,
// so the pipeline the branch's push creates is registered and stays pending.
const mrAutoMergeCIYAML = `blocked:
  script:
    - echo this job waits for a runner that does not exist
  tags:
    - e2e-no-runner-carries-this-tag
`

// activePipelineStatuses are the statuses GitLab counts as a pipeline that has
// not finished, which is the condition under which auto-merge is armed rather
// than the merge happening at once.
var activePipelineStatuses = []string{"created", "waiting_for_resource", "preparing", "pending", "running", "scheduled"}

// TestMergeRequest_CancelAutoMerge_DisarmsTheScheduledMerge opens a merge
// request of its own on every surface whose branch carries a pipeline that
// cannot finish, waits until that pipeline is the request's head pipeline,
// merges it with auto_merge so the merge is scheduled rather than done, and
// then cancels the schedule: the answer must be the same request, still open,
// and no longer waiting to merge.
func TestMergeRequest_CancelAutoMerge_DisarmsTheScheduledMerge(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("automerge"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestWithCI(e, project, "automerge", mrAutoMergeCIYAML)
		params := f.params()

		// GitLab attaches the branch's pipeline to the request in a background
		// job, and arming auto-merge before that has happened would merge the
		// request instead of scheduling it.
		ready := harness.Eventually(s, actionMergeRequestGet, params, mergeRequestReadInterval, mergeRequestReadWait,
			func(out mergerequests.Output) bool {
				return out.HeadPipeline != nil && slices.Contains(activePipelineStatuses, out.HeadPipeline.Status)
			})
		e.T.Logf("the request's head pipeline %d is %s", ready.HeadPipeline.ID, ready.HeadPipeline.Status)

		armed := harness.Do[mergerequests.Output](s, actionMergeRequestMerge,
			withParams(params, map[string]any{"auto_merge": true}))
		if armed.State != "opened" || !armed.MergeWhenPipelineSucceeds {
			e.T.Fatalf("merge with auto_merge answered %+v, want request !%d still open with the merge scheduled", armed, f.mr.IID)
		}

		cancelled := harness.Do[mergerequests.Output](s, actionMergeRequestCancelAutoMerge, params)
		if cancelled.IID != f.mr.IID || cancelled.State != "opened" || cancelled.MergeWhenPipelineSucceeds {
			e.T.Errorf("cancel_auto_merge answered %+v, want request !%d open with no merge scheduled", cancelled, f.mr.IID)
		}
	})
}
