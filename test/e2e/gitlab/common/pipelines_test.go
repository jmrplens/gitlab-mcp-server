// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// pipelines_test.go covers the creation of a pipeline through the server,
// which the old suite made only to build the pipeline its vulnerability
// scenario stood on, and a pipeline that ran nothing, read without the
// durations GitLab did not measure. The created pipeline is waited for, so
// it never contends with the next test's for the one runner the Docker
// stack registers.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestPipeline_Create_RunsOnTheDefaultBranch creates a pipeline on every
// surface in a shared project carrying the e2e CI configuration, reads the
// ref and the commit off the answer, and waits for it to finish.
//
// Replaces: TestMeta_VulnerabilityLifecycle
func TestPipeline_Create_RunsOnTheDefaultBranch(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("pipecreate"))
		fixture.CIFile(e, project)
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch})
		if created.ID == 0 || created.Ref != project.DefaultBranch || created.SHA == "" {
			e.T.Fatalf("pipeline create answered %+v, want a pipeline on %q with an ID and a commit", created, project.DefaultBranch)
		}
		e.T.Logf("pipeline %d ended in %q", created.ID, fixture.WaitForPipeline(e, project, created.ID, 0))
	})
}

// neverRunCIYAML has one job and it is manual, so the pipeline it creates
// runs nothing: GitLab measures neither the pipeline nor the job, and sends
// their durations as null.
const neverRunCIYAML = `held:
  stage: test
  script:
    - echo "e2e job nobody plays"
  when: manual
  tags: []
`

// TestPipeline_NeverRan_PublishesNoDuration creates a pipeline whose only job
// is manual and reads it, its job list and the job on every surface. None of
// them may carry a duration or a queued duration: client-go decodes GitLab's
// null as 0, which read as a pipeline and a job that ran for zero seconds,
// and the server now reads both from the captured answer. Nothing runs, so
// it needs no runner.
func TestPipeline_NeverRan_PublishesNoDuration(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("pipenever"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, neverRunCIYAML, "ci: add a manual job nobody plays "+fixture.SkipCIMarker)
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		pipeline := fixture.NewPipelineNoWait(e, project, project.DefaultBranch)
		byPipeline := withParams(params, map[string]any{"pipeline_id": pipeline.ID})

		got := harness.Do[pipelines.DetailOutput](s, actionPipelineGet, byPipeline)
		if got.Duration != nil || got.QueuedDuration != nil {
			e.T.Errorf("pipeline %d that ran nothing answered duration %v and queued duration %v, want neither", pipeline.ID, got.Duration, got.QueuedDuration)
		}

		listed := harness.Do[jobs.ListOutput](s, actionJobList, byPipeline)
		held := jobNamed(e, listed, "held")
		if held.Duration != nil || held.QueuedDuration != nil || held.Coverage != nil {
			e.T.Errorf("the manual job %d listed with duration %v, queued duration %v and coverage %v, want none", held.ID, held.Duration, held.QueuedDuration, held.Coverage)
		}
		one := harness.Do[jobs.Output](s, actionJobGet, withParams(params, map[string]any{"job_id": held.ID}))
		if one.Duration != nil || one.QueuedDuration != nil || one.Coverage != nil {
			e.T.Errorf("the manual job %d read with duration %v, queued duration %v and coverage %v, want none", one.ID, one.Duration, one.QueuedDuration, one.Coverage)
		}
	})
}
