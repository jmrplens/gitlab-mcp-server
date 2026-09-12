//go:build e2e

// ci_runner_test.go ports the full pipeline and job lifecycle the old suite
// drove against a project with a CI runner: create a pipeline, read it and the
// project's pipelines, wait for it, list and read its jobs and their trace,
// retry the pipeline and delete it.
//
// It runs on the individual surface alone, because the lifecycle needs a
// runner and minutes per run, and the pipeline creation and the job reads are
// covered on the dynamic and meta surfaces by the pipelines and jobs families.
// It declares the runner lock so its pipelines never contend with another
// test's for the one runner the Docker stack registers.

package common

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// ciRunnerPipelineWait bounds one pipeline the test waits for. It is generous
// because an ephemeral runner pulls its image on the first job.
const ciRunnerPipelineWait = 15 * time.Minute

// TestCIRunner_PipelineAndJobLifecycle drives a project carrying the e2e CI
// configuration through pipeline create, get, list, wait, job list, get and
// trace, retry and delete.
//
// Replaces: TestIndividual_CIRunner
func TestCIRunner_PipelineAndJobLifecycle(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.OnSurfaces(e, "a full CI pipeline lifecycle needs a runner and minutes per run; the pipelines and jobs "+
		"families cover the creation and the job reads on the dynamic and meta surfaces",
		[]harness.Surface{harness.SurfaceIndividual}, func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)
			project := fixture.NewProject(e, fixture.WithNamePrefix("cirunner"))
			fixture.CIFile(e, project)

			created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch})
			if created.ID == 0 {
				e.T.Fatalf("pipeline create answered %+v, want a pipeline with an ID", created)
			}
			pipelineID := created.ID

			got := harness.Do[pipelines.DetailOutput](s, actionPipelineGet, map[string]any{"project_id": project.IDParam(), "pipeline_id": pipelineID})
			if got.ID != pipelineID {
				e.T.Errorf("pipeline get answered %d, want the created %d", got.ID, pipelineID)
			}

			list := harness.Do[pipelines.ListOutput](s, actionPipelineList, map[string]any{"project_id": project.IDParam()})
			if len(list.Pipelines) == 0 {
				e.T.Errorf("pipeline list answered no pipelines, want at least the one just created")
			}

			status := fixture.WaitForPipeline(e, project, pipelineID, ciRunnerPipelineWait)
			e.T.Logf("pipeline %d finished with status %q", pipelineID, status)

			jobList := harness.Do[jobs.ListOutput](s, actionJobList, map[string]any{"project_id": project.IDParam(), "pipeline_id": pipelineID})
			if len(jobList.Jobs) == 0 {
				e.T.Fatalf("job list answered no jobs, want at least the fast-pass job")
			}
			jobID := jobList.Jobs[0].ID

			job := harness.Do[jobs.Output](s, actionJobGet, map[string]any{"project_id": project.IDParam(), "job_id": jobID})
			if job.ID != jobID {
				e.T.Errorf("job get answered %d, want %d", job.ID, jobID)
			}

			trace := harness.Do[jobs.TraceOutput](s, actionJobTrace, map[string]any{"project_id": project.IDParam(), "job_id": jobID})
			if len(trace.Trace) == 0 {
				e.T.Errorf("job trace answered an empty trace for job %d", jobID)
			}

			retried := harness.Do[pipelines.DetailOutput](s, actionPipelineRetry, map[string]any{"project_id": project.IDParam(), "pipeline_id": pipelineID})
			if retried.ID != pipelineID {
				e.T.Errorf("pipeline retry answered %d, want the same pipeline %d", retried.ID, pipelineID)
			}
			fixture.WaitForPipeline(e, project, pipelineID, ciRunnerPipelineWait)

			harness.DoVoid(s, actionPipelineDelete, map[string]any{"project_id": project.IDParam(), "pipeline_id": pipelineID})
		})
}
