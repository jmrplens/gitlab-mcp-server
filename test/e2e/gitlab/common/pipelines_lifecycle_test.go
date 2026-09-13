//go:build e2e

// pipelines_lifecycle_test.go covers a pipeline through the server after
// its creation, which pipelines_test.go already drives on its own: the
// reads of one and of its project's list, its variables, its rename, the
// latest pipeline of the default branch, the cancel and the retry that move
// it, the jobs it ran and their traces, the test reports a pipeline without
// JUnit artifacts answers empty, and its delete. The old suite read the two
// test reports and never looked at the answer; both are asserted here.

package common

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The job the fixture's CI configuration runs, and what its script prints.
const (
	fixtureJobName   = "fast-pass"
	fixtureJobOutput = "e2e pipeline job"
)

// The cadence of the wait for a retried pipeline to succeed.
const (
	pipelineRetryPollInterval = 3 * time.Second
	pipelineRetryWait         = 5 * time.Minute
)

// pipelineIDs returns the identifiers of a pipeline listing.
func pipelineIDs(listed []pipelines.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, pipeline := range listed {
		ids = append(ids, pipeline.ID)
	}
	return ids
}

// jobNamed returns the job of a listing with the given name, failing the
// test when the listing has none. A pipeline that was retried lists the
// job that ran last under the name, since the listing leaves retried jobs
// out unless asked for them.
func jobNamed(e *harness.Env, listed jobs.ListOutput, name string) jobs.Output {
	e.T.Helper()
	for _, job := range listed.Jobs {
		if job.Name == name {
			return job
		}
	}
	e.T.Fatalf("no job named %q among the %d jobs listed", name, len(listed.Jobs))
	return jobs.Output{}
}

// createPipelineWithVariable creates a pipeline on the default branch of
// the project carrying one variable that names the surface, and returns
// what the create answered.
func createPipelineWithVariable(e *harness.Env, s *harness.Session, project fixture.Project, surface harness.Surface) pipelines.DetailOutput {
	e.T.Helper()

	created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, map[string]any{
		"project_id": project.IDParam(), "ref": project.DefaultBranch,
		"variables": []map[string]any{{"key": "E2E_SURFACE", "value": string(surface)}},
	})
	if created.ID == 0 || created.Ref != project.DefaultBranch || created.SHA == "" {
		e.T.Fatalf("pipeline create answered %+v, want a pipeline on %s with an ID and a commit", created, project.DefaultBranch)
	}
	return created
}

// inspectPipeline reads a freshly created pipeline back every way the
// server offers: by ID, in the project's listing, through its variables,
// through the rename of it, and as the latest pipeline of its branch.
func inspectPipeline(e *harness.Env, s *harness.Session, project fixture.Project, created pipelines.DetailOutput, surface harness.Surface) {
	e.T.Helper()
	params := map[string]any{"project_id": project.IDParam()}
	byID := withParams(params, map[string]any{"pipeline_id": created.ID})

	got := harness.Do[pipelines.DetailOutput](s, actionPipelineGet, byID)
	if got.ID != created.ID || got.Ref != project.DefaultBranch || got.SHA != created.SHA {
		e.T.Errorf("pipeline get answered %+v, want pipeline %d on %s at %s", got, created.ID, project.DefaultBranch, fixture.ShortSHA(created.SHA))
	}
	listed := harness.Do[pipelines.ListOutput](s, actionPipelineList, params)
	if !containsID(pipelineIDs(listed.Pipelines), created.ID) {
		e.T.Errorf("the pipeline listing does not hold %d: %v", created.ID, pipelineIDs(listed.Pipelines))
	}
	variables := harness.Do[pipelines.VariablesOutput](s, actionPipelineVariables, byID)
	if !slices.ContainsFunc(variables.Variables, func(v pipelines.VariableOutput) bool { return v.Key == "E2E_SURFACE" && v.Value == string(surface) }) {
		e.T.Errorf("the variables of pipeline %d do not hold E2E_SURFACE=%s: %+v", created.ID, surface, variables.Variables)
	}
	name := e.Name("renamed")
	renamed := harness.Do[pipelines.DetailOutput](s, actionPipelineUpdateMetadata, withParams(byID, map[string]any{"name": name}))
	if renamed.ID != created.ID || renamed.Name != name {
		e.T.Errorf("pipeline update_metadata answered %+v, want pipeline %d named %q", renamed, created.ID, name)
	}
	// The commit that put the CI configuration on the branch starts a
	// pipeline of its own, and GitLab creates that record in a background
	// job, which can land after the pipeline this test asked for. The
	// latest pipeline of the branch is therefore asserted to be that branch's
	// and no older than the one just created, rather than to be it.
	latest := harness.Do[pipelines.DetailOutput](s, actionPipelineLatest, params)
	if latest.ID < created.ID || latest.Ref != project.DefaultBranch {
		e.T.Errorf("the latest pipeline of %s is %d on %s, want one of that branch no older than the %d just created",
			project.DefaultBranch, latest.ID, latest.Ref, created.ID)
	}
}

// cancelAndRetryPipeline cancels the pipeline, waits for it to settle,
// retries it and waits for its success.
//
// The cancel is asserted on the pipeline it answers and not on a canceled
// status: a runner that picked the one echo job up in the second between
// the create and the cancel has already finished it, and GitLab answers the
// cancel of a finished pipeline with the pipeline as it is.
func cancelAndRetryPipeline(e *harness.Env, s *harness.Session, project fixture.Project, pipelineID int64) {
	e.T.Helper()
	byID := map[string]any{"project_id": project.IDParam(), "pipeline_id": pipelineID}

	canceled := harness.Do[pipelines.DetailOutput](s, actionPipelineCancel, byID)
	if canceled.ID != pipelineID {
		e.T.Errorf("pipeline cancel answered %+v, want pipeline %d", canceled, pipelineID)
	}
	afterCancel := fixture.WaitForPipeline(e, project, pipelineID, 0)
	if afterCancel != "canceled" && afterCancel != "success" {
		e.T.Errorf("pipeline %d settled as %q after its cancel (answered %q), want canceled, or success for a job the runner outran the cancel with", pipelineID, afterCancel, canceled.Status)
	}

	retried := harness.Do[pipelines.DetailOutput](s, actionPipelineRetry, byID)
	if retried.ID != pipelineID {
		e.T.Errorf("pipeline retry answered %+v, want pipeline %d", retried, pipelineID)
	}
	// The wait is for success and not for a terminal status: a pipeline
	// just retried still reads as canceled for a moment, which is terminal,
	// and a wait that stopped there would read the status the retry was
	// meant to change.
	final := harness.Eventually(s, actionPipelineGet, byID, pipelineRetryPollInterval, pipelineRetryWait,
		func(out pipelines.DetailOutput) bool { return out.Status == "success" })
	e.T.Logf("pipeline %d is %s after its retry", final.ID, final.Status)
}

// inspectPipelineJobs lists the jobs of a finished pipeline, reads the
// fixture's job and its trace, and reads the two test reports a pipeline
// without JUnit artifacts answers empty.
func inspectPipelineJobs(e *harness.Env, s *harness.Session, project fixture.Project, pipelineID int64) {
	e.T.Helper()
	params := map[string]any{"project_id": project.IDParam()}
	byID := withParams(params, map[string]any{"pipeline_id": pipelineID})

	job := jobNamed(e, harness.Do[jobs.ListOutput](s, actionJobList, byID), fixtureJobName)
	byJob := withParams(params, map[string]any{"job_id": job.ID})
	jobGot := harness.Do[jobs.Output](s, actionJobGet, byJob)
	if jobGot.ID != job.ID || jobGot.Name != fixtureJobName || jobGot.Status != "success" {
		e.T.Errorf("job get answered %+v, want the successful job %d named %s", jobGot, job.ID, fixtureJobName)
	}
	trace := harness.Do[jobs.TraceOutput](s, actionJobTrace, byJob)
	if trace.JobID != job.ID || !strings.Contains(trace.Trace, fixtureJobOutput) {
		e.T.Errorf("job trace answered job %d with %d bytes not carrying %q", trace.JobID, len(trace.Trace), fixtureJobOutput)
	}

	report := harness.Do[pipelines.TestReportOutput](s, actionPipelineTestReport, byID)
	if report.TotalCount != 0 || len(report.TestSuites) != 0 {
		e.T.Errorf("the test report of pipeline %d answered %+v, want an empty report", pipelineID, report)
	}
	summary := harness.Do[pipelines.TestReportSummaryOutput](s, actionPipelineTestReportSummary, byID)
	if summary.TotalCount != 0 || len(summary.TestSuites) != 0 {
		e.T.Errorf("the test report summary of pipeline %d answered %+v, want an empty summary", pipelineID, summary)
	}
}

// TestPipeline_Lifecycle_InspectCancelRetryDelete creates a pipeline per
// surface on a shared project carrying the fixture's CI configuration,
// reads it back every way the server offers, cancels it, waits, retries it
// and waits for its success, lists and reads its job and the job's trace,
// reads the empty test reports, deletes it and asserts the read afterwards
// is refused as not found.
//
// Replaces: TestPipelines, TestMeta_PipelinesExtended
func TestPipeline_Lifecycle_InspectCancelRetryDelete(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("pipelife"))
		fixture.CIFile(e, project)
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		created := createPipelineWithVariable(e, s, project, surface)
		inspectPipeline(e, s, project, created, surface)
		cancelAndRetryPipeline(e, s, project, created.ID)
		inspectPipelineJobs(e, s, project, created.ID)

		byID := map[string]any{"project_id": project.IDParam(), "pipeline_id": created.ID}
		harness.DoVoid(s, actionPipelineDelete, byID)
		refused := harness.Refused(s, actionPipelineGet, byID, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}

// TestPipeline_Latest_RefusesAProjectWithoutPipelines asks a shared project
// that never ran a pipeline for its latest one on every surface, which the
// action refuses by saying so, and lists its pipelines, which are none.
//
// Replaces: TestMeta_PipelinesExtended
func TestPipeline_Latest_RefusesAProjectWithoutPipelines(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("nopipe"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		refused := harness.ExpectToolError(s, actionPipelineLatest, params, "no pipelines found")
		e.T.Logf("the latest pipeline of a project without one is refused: %s", firstLine(refused))

		listed := harness.Do[pipelines.ListOutput](s, actionPipelineList, params)
		if len(listed.Pipelines) != 0 {
			e.T.Errorf("the pipeline listing of a project without one answered %d pipelines", len(listed.Pipelines))
		}
	})
}
