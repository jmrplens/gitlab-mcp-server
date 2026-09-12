//go:build e2e

// mcp_wait_test.go covers the two polling actions the server offers over its
// own long-running waits: gitlab_pipeline_wait and gitlab_job_wait. They are
// the one place a tool call does not return until GitLab reaches a terminal
// state, so they need an instance CI runner and the lock the pipeline test
// takes, and they are exercised on the individual and meta surfaces where a
// model reaches them by their own tool and by the domain dispatcher.
//
// The dynamic surface is left out on purpose: the wait tools are not
// standalone, and the find-and-execute workflow that surface adds is proven by
// the dynamic tests; running the whole pipeline-and-wait cycle a third time
// would treble the one thing the single runner serializes.

package common

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// waitCallBudget bounds one wait call from the client side, above the
// timeout_seconds the call itself carries so the handler's own timeout is what
// ends it rather than the harness giving up first.
const waitCallBudget = 12 * time.Minute

// TestWaitTools_PipelineAndJob drives a pipeline to a terminal state through
// gitlab_pipeline_wait and its first job through gitlab_job_wait, on the
// individual and meta surfaces.
//
// Replaces: TestWaitTools
func TestWaitTools_PipelineAndJob(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.OnSurfaces(e, "the wait tools are not standalone and the single runner serializes the pipeline cycle; the dynamic find/execute workflow is covered by TestDynamic_*",
		[]harness.Surface{harness.SurfaceIndividual, harness.SurfaceMeta},
		func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)

			project := fixture.NewProject(e, fixture.WithNamePrefix("wait"))
			fixture.CIFile(e, project)

			created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate,
				map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch})
			if created.ID == 0 {
				e.T.Fatalf("pipeline.create answered %+v, want a pipeline with an id", created)
			}

			waited := harness.Do[pipelines.WaitOutput](s, actionPipelineWait, map[string]any{
				"project_id": project.IDParam(), "pipeline_id": created.ID,
				"interval_seconds": 5, "timeout_seconds": 600, "fail_on_error": false,
			}, harness.Within(waitCallBudget))
			assertReachedTerminal(e, "pipeline", waited.FinalStatus, waited.TimedOut, waited.PollCount, waited.WaitedFor)

			jobList := harness.Do[jobs.ListOutput](s, actionJobList,
				map[string]any{"project_id": project.IDParam(), "pipeline_id": created.ID})
			if len(jobList.Jobs) == 0 {
				e.T.Fatalf("job.list for pipeline %d answered no jobs", created.ID)
			}

			job := harness.Do[jobs.WaitOutput](s, actionJobWait, map[string]any{
				"project_id": project.IDParam(), "job_id": jobList.Jobs[0].ID,
				"interval_seconds": 5, "timeout_seconds": 600, "fail_on_error": false,
			}, harness.Within(waitCallBudget))
			assertReachedTerminal(e, "job", job.FinalStatus, job.TimedOut, job.PollCount, job.WaitedFor)
		})
}

// assertReachedTerminal holds a wait answer to what waiting is for: a terminal
// status reached without timing out, after at least one poll and some elapsed
// time.
func assertReachedTerminal(e *harness.Env, what, finalStatus string, timedOut bool, pollCount int, waitedFor string) {
	e.T.Helper()

	if timedOut {
		e.T.Fatalf("%s wait timed out with last status %q", what, finalStatus)
	}
	if finalStatus == "" {
		e.T.Errorf("%s wait answered an empty final_status", what)
	}
	if pollCount <= 0 {
		e.T.Errorf("%s wait reported poll_count %d, want at least one poll", what, pollCount)
	}
	if waitedFor == "" {
		e.T.Errorf("%s wait reported an empty waited_for", what)
	}
	e.T.Logf("%s wait: status=%s waited=%s polls=%d", what, finalStatus, waitedFor, pollCount)
}
