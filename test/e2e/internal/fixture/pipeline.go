//go:build e2e

// pipeline.go builds a pipeline and waits for it to finish, which needs the
// one thing a GitLab cannot provide by itself: a runner.
//
// Without a runner a pipeline stays pending forever, so the builder asks the
// harness whether one is registered and skips the test with that reason
// rather than spending the whole budget on nothing. With one, the wait is
// long and tolerant of a slow runner in an ephemeral container, and it fails
// the test when the budget runs out: a pipeline that never finished is a
// fact about the run, and the suite this replaces used to hand the caller a
// non-terminal status to make of what it could.

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

// CIYAML is a minimal .gitlab-ci.yml with one fast job and no runner tags, so
// it runs on whatever runner the instance has. Commit it with CIFile before
// creating a pipeline.
const CIYAML = `stages:
  - test

fast-pass:
  stage: test
  script:
    - echo "e2e pipeline job"
  tags: []
`

// CIFilePath is where GitLab looks for the pipeline configuration.
const CIFilePath = ".gitlab-ci.yml"

// ManualJobName is what ManualJobCIYAML calls the job a case plays.
const ManualJobName = "manual-deploy"

// ManualJobCIYAML is CIYAML with a manual job beside the fast one, for the
// world whose case plays a job rather than watching one run.
//
// The manual job is in the first stage on purpose: a manual job GitLab has
// not reached yet is still "created", and playing one in that state is the
// 400 "Unplayable Job" the plain configuration produces for every model. In
// the first stage GitLab marks it "manual" as soon as it creates the
// pipeline, and a manual job stays playable after the rest of the pipeline
// has finished.
const ManualJobCIYAML = `stages:
  - test

fast-pass:
  stage: test
  script:
    - echo "e2e pipeline job"
  tags: []

` + ManualJobName + `:
  stage: test
  when: manual
  script:
    - echo "e2e manual pipeline job"
  tags: []
`

// Pipeline is a pipeline a builder created and waited for.
type Pipeline struct {
	// ID is what every pipeline action takes.
	ID int64
	// Ref is the branch it ran on.
	Ref string
	// SHA is the commit it ran against.
	SHA string
	// Status is the terminal status it reached.
	Status string
}

// CIFile commits CIYAML to the project's default branch and returns the
// commit, so that a pipeline created next has a configuration to run.
func CIFile(e *harness.Env, project Project) Commit {
	e.T.Helper()
	return CommitFile(e, project, project.DefaultBranch, CIFilePath, CIYAML, "ci: add the e2e pipeline configuration")
}

// The pipeline wait's bounds: generous, because an ephemeral runner in a
// container pulls its image on the first job, and tolerant of a run of API
// errors, because a GitLab under load answers some polls with nothing.
const (
	pipelineWait                 = 15 * time.Minute
	pipelineMaxConsecutiveErrors = 10
)

// pipelinePollInterval is how often the wait asks; a variable so the
// package's own tests can ask faster than a real runner answers.
var pipelinePollInterval = 5 * time.Second

// NewPipeline creates a pipeline on ref and waits until it reaches a
// terminal status, failing the test if it does not within the budget. The
// test is skipped when no runner is registered.
//
// The pipeline goes with the project, so nothing is registered.
func NewPipeline(e *harness.Env, project Project, ref string) Pipeline {
	e.T.Helper()
	requireRunner(e)

	pipeline, err := retryTransient(e, "create pipeline", createRetries, func() (Pipeline, error) {
		created, _, err := e.Client().GL().Pipelines.CreatePipeline(project.ID, &gl.CreatePipelineOptions{Ref: new(ref)}, gl.WithContext(e.Ctx))
		if err != nil {
			return Pipeline{}, err
		}
		return Pipeline{ID: created.ID, Ref: created.Ref, SHA: created.SHA, Status: created.Status}, nil
	})
	if err != nil {
		e.T.Fatalf("creating a pipeline on %q in project %d: %v", ref, project.ID, err)
	}

	pipeline.Status = WaitForPipeline(e, project, pipeline.ID, pipelineWait)
	return pipeline
}

// WaitForPipeline drains Sidekiq and polls the pipeline until it reaches a
// terminal status, returning that status. It fails the test when the budget
// runs out, naming the last status it saw, and when ten polls in a row
// failed. A zero timeout takes the default budget.
func WaitForPipeline(e *harness.Env, project Project, pipelineID int64, timeout time.Duration) string {
	e.T.Helper()

	if timeout <= 0 {
		timeout = pipelineWait
	}
	DrainSidekiq(e.Ctx, e.Client())
	status, err := waitForPipelineStatus(e.Ctx, e.Client(), project.ID, pipelineID, timeout)
	if err != nil {
		e.T.Fatalf("pipeline %d in project %d did not reach a terminal status within %s (last status: %s): %v",
			pipelineID, project.ID, timeout, status, err)
	}
	e.T.Logf("pipeline %d in project %d reached terminal status %s", pipelineID, project.ID, status)
	return status
}

// waitForPipelineStatus polls until the pipeline's status is terminal,
// returning the last status seen beside any error.
func waitForPipelineStatus(ctx context.Context, client *gitlabclient.Client, projectID, pipelineID int64, timeout time.Duration) (string, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	lastStatus := "unknown"
	consecutiveErrors := 0
	err := harness.Poll(pollCtx, pipelinePollInterval, timeout, func() (bool, string, error) {
		pipeline, _, err := client.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(pollCtx))
		if err != nil {
			consecutiveErrors++
			state := fmt.Sprintf("pipeline %d in project %d: last_status=%s consecutive_errors=%d/%d error=%v",
				pipelineID, projectID, lastStatus, consecutiveErrors, pipelineMaxConsecutiveErrors, err)
			if consecutiveErrors >= pipelineMaxConsecutiveErrors {
				return false, state, fmt.Errorf("reading pipeline %d of project %d failed %d times in a row: %w",
					pipelineID, projectID, consecutiveErrors, err)
			}
			return false, state, nil
		}
		consecutiveErrors = 0
		lastStatus = pipeline.Status
		return IsTerminalPipelineStatus(lastStatus), fmt.Sprintf("pipeline %d in project %d: status=%s", pipelineID, projectID, lastStatus), nil
	})
	return lastStatus, err
}

// NewPipelineNoWait creates a pipeline on ref and hands it back in whatever
// status GitLab created it in, without waiting and without a runner.
//
// It exists for the cases whose subject is a pipeline that has not finished:
// canceling one, or deleting one. Those declare no runner, so [NewPipeline]
// would skip them, and on an instance that does have a runner it would wait
// for the pipeline the case is about to cancel to finish first.
//
// The pipeline goes with the project, so nothing is registered.
func NewPipelineNoWait(e *harness.Env, project Project, ref string) Pipeline {
	e.T.Helper()

	pipeline, err := retryTransient(e, "create pipeline", createRetries, func() (Pipeline, error) {
		created, _, createErr := e.Client().GL().Pipelines.CreatePipeline(project.ID,
			&gl.CreatePipelineOptions{Ref: new(ref)}, gl.WithContext(e.Ctx))
		if createErr != nil {
			return Pipeline{}, createErr
		}
		return Pipeline{ID: created.ID, Ref: created.Ref, SHA: created.SHA, Status: created.Status}, nil
	})
	if err != nil {
		e.T.Fatalf("creating a pipeline on %q in project %d: %v", ref, project.ID, err)
	}
	return pipeline
}

// The wait for GitLab to create a pipeline's jobs, which happens a moment
// after the pipeline itself and later still on a loaded instance.
const (
	pipelineJobsWait     = 2 * time.Minute
	pipelineJobsInterval = 2 * time.Second
)

// FirstPipelineJobID returns the ID of the first job of the pipeline, which is
// what a job case addresses, waiting for GitLab to create it.
//
// A pipeline that never grows a job fails the test rather than handing back a
// zero. The zero used to be the caller's to judge and no caller judged it: a
// world rendered "0" as its job identifier and every model was asked about a
// job that has never existed.
func FirstPipelineJobID(e *harness.Env, project Project, pipelineID int64) int64 {
	e.T.Helper()
	return pipelineJobID(e, project, pipelineID, "", func(*gl.Job) bool { return true })
}

// ManualPipelineJobID returns the ID of the pipeline's manual job, which is
// the one a case that plays a job addresses.
//
// It is named rather than positional because which job comes first in a
// listing is GitLab's to decide, and a case that plays whichever job that
// happens to be is answered "Unplayable Job" for every job but one.
func ManualPipelineJobID(e *harness.Env, project Project, pipelineID int64) int64 {
	e.T.Helper()
	return pipelineJobID(e, project, pipelineID, "named "+ManualJobName,
		func(job *gl.Job) bool { return job.Name == ManualJobName })
}

// pipelineJobID polls the pipeline's jobs until one the predicate accepts is
// there, failing the test when the budget runs out. The description names what
// was wanted, for that failure.
func pipelineJobID(e *harness.Env, project Project, pipelineID int64, wanted string, accept func(*gl.Job) bool) int64 {
	e.T.Helper()

	var found int64
	err := harness.Poll(e.Ctx, pipelineJobsInterval, pipelineJobsWait, func() (bool, string, error) {
		id, state, listErr := findPipelineJob(e.Ctx, e.Client(), project.ID, pipelineID, accept)
		if listErr != nil {
			return false, state, listErr
		}
		found = id
		return id != 0, state, nil
	})
	if err != nil {
		e.T.Fatalf("pipeline %d in project %d grew no job %s within %s: %v",
			pipelineID, project.ID, jobDescription(wanted), pipelineJobsWait, err)
	}
	return found
}

// jobDescription spells what a job wait was looking for, for its failure.
func jobDescription(wanted string) string {
	if wanted == "" {
		return "at all"
	}
	return wanted
}

// findPipelineJob returns the ID of the first job of the pipeline the
// predicate accepts, or zero, beside what the listing held.
func findPipelineJob(ctx context.Context, client *gitlabclient.Client, projectID, pipelineID int64,
	accept func(*gl.Job) bool,
) (id int64, state string, err error) {
	jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, &gl.ListJobsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return 0, "", fmt.Errorf("listing the jobs of pipeline %d in project %d: %w", pipelineID, projectID, err)
	}
	names := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if job == nil {
			continue
		}
		names = append(names, job.Name+"="+job.Status)
		if accept(job) {
			return job.ID, "", nil
		}
	}
	return 0, fmt.Sprintf("pipeline %d holds %d job(s): %s", pipelineID, len(names), strings.Join(names, " ")), nil
}

// IsTerminalPipelineStatus reports whether status is one a pipeline never
// leaves.
func IsTerminalPipelineStatus(status string) bool {
	switch status {
	case "success", "failed", "canceled", "skipped":
		return true
	default:
		return false
	}
}

// requireRunner skips a test whose fixture needs a runner the run does not
// have.
func requireRunner(e *harness.Env) {
	e.T.Helper()
	if !e.HasRunner() {
		e.Skipf("a pipeline fixture needs a CI runner, and none is registered; declare harness.Needs(harness.NeedRunner)")
	}
}

// DockerRunnerDescription is the description register-runner.sh gives the
// runner the Docker stack registers, which is how a test finds it.
const DockerRunnerDescription = "e2e-docker-runner"

// runnerPageSize is how many runners one listing page carries.
const runnerPageSize = 100

// DockerRunnerID returns the ID of the instance runner the Docker stack
// registered, failing the test when it is not there. The runner actions need
// an ID and nothing else on the instance is a safe target for them.
func DockerRunnerID(e *harness.Env) int64 {
	e.T.Helper()

	id, count, err := findRunner(e.Ctx, e.Client(), DockerRunnerDescription)
	if err != nil {
		e.T.Fatalf("listing the instance runners: %v", err)
	}
	if id == 0 {
		e.T.Fatalf("no instance runner is described %q; %d runner(s) listed", DockerRunnerDescription, count)
	}
	return id
}

// findRunner lists every instance runner and returns the ID of the one with
// the given description, or zero, beside how many it looked at.
func findRunner(ctx context.Context, client *gitlabclient.Client, description string) (id int64, seen int, err error) {
	var page int64 = 1
	for page != 0 {
		opts := &gl.ListRunnersOptions{Type: new("instance_type")}
		opts.Page = page
		opts.PerPage = runnerPageSize
		runners, resp, listErr := client.GL().Runners.ListAllRunners(opts, gl.WithContext(ctx))
		if listErr != nil {
			return 0, seen, listErr
		}
		for _, runner := range runners {
			seen++
			if runner.Description == description {
				return runner.ID, seen, nil
			}
		}
		page = resp.NextPage
	}
	return 0, seen, nil
}
