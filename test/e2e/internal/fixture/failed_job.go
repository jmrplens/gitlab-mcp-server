//go:build e2e

// failed_job.go builds a pipeline whose job fails and leaves an artifact
// behind, which is what an artifact download, a job retry and a job trace
// case each need.
//
// Two facts about the configuration are load-bearing. The artifacts are
// declared `when: always`, or GitLab keeps nothing from a job that exited
// non-zero and the download case has nothing to download. And the wait is for
// a failed job rather than for the pipeline: a pipeline carrying a manual job
// never reaches a terminal status by itself, so waiting on the pipeline would
// spend the whole budget on a job that had already failed.

package fixture

import (
	"context"
	"fmt"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// FailedJobArtifactPath is where the failing job writes its artifact, which is
// the path an artifact case asks for.
const FailedJobArtifactPath = "coverage/report.xml"

// FailingCIYAML is a pipeline configuration with one job that fails after
// writing an artifact, and one manual job a case may play.
//
// GIT_STRATEGY is none so the job needs no clone, which is what keeps it fast
// on a runner that would otherwise fetch the repository for a job that reads
// nothing from it.
const FailingCIYAML = `stages:
  - test

variables:
  GIT_STRATEGY: none

failing-fixture:
  stage: test
  script:
    - mkdir -p coverage
    - printf '<coverage />\n' > coverage/report.xml
    - echo 'e2e: this job fails on purpose'
    - exit 1
  artifacts:
    when: always
    paths:
      - coverage/report.xml

manual-fixture:
  stage: test
  when: manual
  script:
    - echo "e2e manual job"
`

// FailedJob is the pipeline and job a builder made fail.
type FailedJob struct {
	// PipelineID is the pipeline the job ran in.
	PipelineID int64
	// JobID is the job that failed.
	JobID int64
	// ArtifactPath is the one path the job left behind.
	ArtifactPath string
}

// failedJobWait is the wait's budget: as generous as the pipeline wait,
// because the first job of a run pulls the runner's image.
const failedJobWait = 10 * time.Minute

// failedJobPollInterval is how often the wait asks; a variable so the
// package's own tests can ask faster than a runner answers.
var failedJobPollInterval = 5 * time.Second

// NewFailedJob commits the failing configuration to the project's default
// branch, creates a pipeline on it and waits for the job to fail.
//
// The test is skipped when no runner is registered: without one the job never
// starts, and a fixture that waited would spend ten minutes proving it.
func NewFailedJob(e *harness.Env, project Project) FailedJob {
	e.T.Helper()
	requireRunner(e)

	CommitFile(e, project, project.DefaultBranch, CIFilePath, FailingCIYAML, "ci: add the failing e2e pipeline configuration")

	pipeline, err := retryTransient(e, "create failing pipeline", createRetries, func() (Pipeline, error) {
		created, _, createErr := e.Client().GL().Pipelines.CreatePipeline(project.ID,
			&gl.CreatePipelineOptions{Ref: new(project.DefaultBranch)}, gl.WithContext(e.Ctx))
		if createErr != nil {
			return Pipeline{}, createErr
		}
		return Pipeline{ID: created.ID, Ref: created.Ref, SHA: created.SHA, Status: created.Status}, nil
	})
	if err != nil {
		e.T.Fatalf("creating a pipeline for the failing job in project %d: %v", project.ID, err)
	}

	DrainSidekiq(e.Ctx, e.Client())
	jobID, err := waitForFailedJob(e.Ctx, e.Client(), project.ID, pipeline.ID, failedJobWait)
	if err != nil {
		e.T.Fatalf("waiting for a failed job in pipeline %d of project %d: %v", pipeline.ID, project.ID, err)
	}
	return FailedJob{PipelineID: pipeline.ID, JobID: jobID, ArtifactPath: FailedJobArtifactPath}
}

// failedJobStatus is the status the wait below is looking for.
const failedJobStatus = "failed"

// jobListPageSize is how many jobs one pipeline page carries. The fixture
// pipeline has two; the page size is what keeps the wait from paging.
const jobListPageSize = 100

// waitForFailedJob polls the pipeline's jobs until one has failed, returning
// its ID, and names every job's status when the budget runs out.
func waitForFailedJob(ctx context.Context, client *gitlabclient.Client, projectID, pipelineID int64, wait time.Duration) (int64, error) {
	pollCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	var found int64
	err := harness.Poll(pollCtx, failedJobPollInterval, wait, func() (bool, string, error) {
		jobs, _, listErr := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID,
			&gl.ListJobsOptions{PerPage: jobListPageSize}, gl.WithContext(pollCtx))
		if listErr != nil {
			// A pipeline GitLab has not finished creating answers about no
			// jobs at all on some releases, so an error is a state to wait
			// through rather than an ending.
			return false, fmt.Sprintf("listing the jobs of pipeline %d: %v", pipelineID, listErr), nil
		}
		seen := make([]string, 0, len(jobs))
		for _, job := range jobs {
			if job == nil {
				continue
			}
			seen = append(seen, job.Name+":"+job.Status)
			if job.Status == failedJobStatus {
				found = job.ID
			}
		}
		return found != 0, fmt.Sprintf("pipeline %d jobs: %s", pipelineID, strings.Join(seen, ", ")), nil
	})
	if err != nil {
		return 0, err
	}
	return found, nil
}
