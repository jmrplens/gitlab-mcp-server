//go:build e2e

// jobs_test.go covers a pipeline's jobs through the server, beyond the list,
// get and trace the pipeline lifecycle reads: the bridges a pipeline
// triggers, the four shapes an artifact is downloaded in, the keep of an
// archive and the delete of it, the play, cancel and retry of a manual job,
// the erase of a finished one, the project-wide job listing and artifact
// delete, and the CI job token scope with its two allowlists.

package common

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The jobs jobLifecycleCIYAML declares, the artifact the first of them
// writes, and the child configuration the bridge includes.
const (
	jobLifecycleBuildJob        = "build-artifacts"
	jobLifecycleManualJob       = "manual-sleep"
	jobLifecycleBridgeJob       = "trigger-child"
	jobLifecycleArtifactPath    = "out/artifact.txt"
	jobLifecycleArtifactContent = "e2e job artifact content"
	jobLifecycleChildCIPath     = "child-ci.yml"
)

// jobLifecycleChildCIYAML is the child pipeline the bridge triggers, so the
// bridge listing has a bridge to show.
const jobLifecycleChildCIYAML = `child-pass:
  script:
    - echo "e2e child pipeline job"
  tags: []
`

// jobLifecycleCIYAML runs one pipeline with a fast job that writes an
// artifact, a manual job that sleeps long enough to be canceled while it
// runs, and a bridge that triggers the child pipeline. A manual job allows
// failure by default, so the pipeline succeeds without it ever running.
const jobLifecycleCIYAML = `stages:
  - test
  - downstream

` + jobLifecycleBuildJob + `:
  stage: test
  script:
    - mkdir -p out
    - echo "` + jobLifecycleArtifactContent + `" > ` + jobLifecycleArtifactPath + `
  artifacts:
    paths:
      - out/
    expire_in: 1 day
  tags: []

` + jobLifecycleManualJob + `:
  stage: test
  script:
    - sleep 300
  when: manual
  tags: []

` + jobLifecycleBridgeJob + `:
  stage: downstream
  trigger:
    include:
      - local: ` + jobLifecycleChildCIPath + `
`

// The waits of the manual job's lifecycle: for the runner to start the
// played job, for the cancel to settle, and for the retried job to be
// stopped so it cannot hold the runner for the next surface.
const (
	jobStatusPollInterval = 3 * time.Second
	jobRunningWait        = 3 * time.Minute
	jobSettledWait        = 2 * time.Minute
	jobCancelRetryEvery   = 2 * time.Second
	jobCancelRetryFor     = 60 * time.Second
)

// jobStatusIn returns the predicate a job status wait uses.
func jobStatusIn(statuses ...string) func(jobs.Output) bool {
	return func(job jobs.Output) bool { return slices.Contains(statuses, job.Status) }
}

// isJobConflict reports whether a cancel was refused by the optimistic lock
// GitLab takes on a build the runner is updating at the same instant, which
// is worth trying again rather than a refusal of the cancel itself.
func isJobConflict(err error) bool {
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "409") || strings.Contains(lowered, "resource lock") || strings.Contains(lowered, "conflict")
}

// cancelRunningJob cancels a job the runner is executing, trying again
// through the rare lock conflict, and returns what the cancel answered.
func cancelRunningJob(e *harness.Env, s *harness.Session, byJob map[string]any) jobs.Output {
	e.T.Helper()

	var canceled jobs.Output
	err := harness.Poll(e.Ctx, jobCancelRetryEvery, jobCancelRetryFor, func() (bool, string, error) {
		answer, tryErr := harness.Try[jobs.Output](s, actionJobCancel, byJob)
		if tryErr != nil {
			if isJobConflict(tryErr) {
				return false, "cancel refused by a lock conflict: " + tryErr.Error(), nil
			}
			return false, "", tryErr
		}
		canceled = answer
		return true, "canceled", nil
	})
	if err != nil {
		e.T.Fatalf("canceling the running job: %v", err)
	}
	return canceled
}

// stopRetriedJob makes sure the job a retry created cannot hold the runner:
// a retried manual job goes back to the manual state and refuses a plain
// cancel, which the forced cancel absorbs; one the runner took is canceled
// outright. Either way the job is then waited for to leave the states in
// which it could still run.
func stopRetriedJob(e *harness.Env, s *harness.Session, byJob map[string]any) {
	e.T.Helper()

	if _, err := harness.Try[jobs.Output](s, actionJobCancel, byJob); err != nil {
		if _, forceErr := harness.Try[jobs.Output](s, actionJobCancel, withParams(byJob, map[string]any{"force": true})); forceErr != nil {
			e.T.Logf("the retried job refused both cancels (plain: %v; forced: %v)", firstLine(err.Error()), firstLine(forceErr.Error()))
		}
	}
	settled := harness.Eventually(s, actionJobGet, byJob, jobStatusPollInterval, jobSettledWait,
		jobStatusIn("manual", "canceled", "canceling", "skipped", "success", "failed", "created"))
	e.T.Logf("the retried job %d settled as %q", settled.ID, settled.Status)
}

// TestJob_Lifecycle_ArtifactsManualPlayCancelRetryAndErase runs one
// pipeline per surface from a configuration with an artifact job, a manual
// job and a bridge, then drives the job actions against it: the bridge
// listing, the artifact archive by job and by ref, one file out of it by
// job and by ref, the keep of the archive, the play of the manual job, its
// cancel once the runner has it and its retry once the cancel settled, and
// last the delete of the archive and the erase of the job that wrote it.
//
// Replaces: TestIndividual_JobExtras
func TestJob_Lifecycle_ArtifactsManualPlayCancelRetryAndErase(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("jobs"))
		// The child configuration goes in first, so the pipeline the main
		// configuration's commit triggers can already resolve the include.
		fixture.CommitFile(e, project, project.DefaultBranch, jobLifecycleChildCIPath, jobLifecycleChildCIYAML, "ci: add the child pipeline configuration")
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, jobLifecycleCIYAML, "ci: add the job lifecycle configuration")
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
		if pipeline.Status != "success" {
			e.T.Fatalf("pipeline %d ended as %q, want success", pipeline.ID, pipeline.Status)
		}
		byPipeline := withParams(params, map[string]any{"pipeline_id": pipeline.ID})

		listed := harness.Do[jobs.ListOutput](s, actionJobList, byPipeline)
		build := jobNamed(e, listed, jobLifecycleBuildJob)
		manual := jobNamed(e, listed, jobLifecycleManualJob)
		byBuild := withParams(params, map[string]any{"job_id": build.ID})
		byManual := withParams(params, map[string]any{"job_id": manual.ID})

		bridges := harness.Do[jobs.BridgeListOutput](s, actionJobListBridges, byPipeline)
		if len(bridges.Bridges) != 1 || bridges.Bridges[0].Name != jobLifecycleBridgeJob {
			e.T.Errorf("the bridge listing of pipeline %d answered %+v, want the one bridge %s", pipeline.ID, bridges.Bridges, jobLifecycleBridgeJob)
		}

		archive := harness.Do[jobs.ArtifactsOutput](s, actionJobArtifacts, byBuild)
		decoded, err := base64.StdEncoding.DecodeString(archive.Content)
		if err != nil || archive.Size == 0 || len(decoded) != archive.Size {
			e.T.Errorf("the artifact archive of job %d answered %d bytes and %d decoded (%v), want a non-empty archive whose content is its size", build.ID, archive.Size, len(decoded), err)
		}
		byRef := harness.Do[jobs.ArtifactsOutput](s, actionJobDownloadArtifacts, withParams(params, map[string]any{"ref_name": project.DefaultBranch, "job": jobLifecycleBuildJob}))
		if byRef.Size != archive.Size {
			e.T.Errorf("the archive downloaded by ref is %d bytes, and by job %d bytes", byRef.Size, archive.Size)
		}
		single := harness.Do[jobs.SingleArtifactOutput](s, actionJobDownloadSingleArtifact, withParams(byBuild, map[string]any{"artifact_path": jobLifecycleArtifactPath}))
		if strings.TrimSpace(single.Content) != jobLifecycleArtifactContent {
			e.T.Errorf("the single artifact of job %d reads %q, want %q", build.ID, single.Content, jobLifecycleArtifactContent)
		}
		singleByRef := harness.Do[jobs.SingleArtifactOutput](s, actionJobDownloadSingleArtifactByRef, withParams(params, map[string]any{
			"ref_name": project.DefaultBranch, "artifact_path": jobLifecycleArtifactPath, "job": jobLifecycleBuildJob,
		}))
		if strings.TrimSpace(singleByRef.Content) != jobLifecycleArtifactContent {
			e.T.Errorf("the single artifact by ref reads %q, want %q", singleByRef.Content, jobLifecycleArtifactContent)
		}
		kept := harness.Do[jobs.Output](s, actionJobKeepArtifacts, byBuild)
		if kept.ID != build.ID {
			e.T.Errorf("keep artifacts answered job %d, want %d", kept.ID, build.ID)
		}

		played := harness.Do[jobs.Output](s, actionJobPlay, byManual)
		if played.ID != manual.ID {
			e.T.Errorf("play answered job %d, want the manual job %d", played.ID, manual.ID)
		}
		// Playing only queues the job; canceling before the runner has it
		// stably running races the build's state machine, so the cancel
		// waits for the running state and the retry for the canceled one.
		running := harness.Eventually(s, actionJobGet, byManual, jobStatusPollInterval, jobRunningWait, jobStatusIn("running"))
		e.T.Logf("the manual job %d is %s", running.ID, running.Status)
		canceled := cancelRunningJob(e, s, byManual)
		if canceled.ID != manual.ID {
			e.T.Errorf("cancel answered job %d, want the manual job %d", canceled.ID, manual.ID)
		}
		settled := harness.Eventually(s, actionJobGet, byManual, jobStatusPollInterval, jobSettledWait, jobStatusIn("canceled"))
		e.T.Logf("the manual job %d is %s", settled.ID, settled.Status)

		retried := harness.Do[jobs.Output](s, actionJobRetry, byManual)
		if retried.ID == 0 || retried.ID == manual.ID {
			e.T.Fatalf("retry answered %+v, want a new job in place of %d", retried, manual.ID)
		}
		stopRetriedJob(e, s, withParams(params, map[string]any{"job_id": retried.ID}))

		// The destructive pair goes last, since it removes the archive and
		// the trace the reads above depend on.
		harness.DoVoid(s, actionJobDeleteArtifacts, byBuild)
		erased := harness.Do[jobs.Output](s, actionJobErase, byBuild)
		if erased.ID != build.ID {
			e.T.Errorf("erase answered job %d, want %d", erased.ID, build.ID)
		}
	})
}

// TestJob_ProjectWide_ListsAndDeletesArtifacts lists the jobs of a shared
// project on every surface and deletes the project's artifacts, which a
// project that never ran a pipeline has none of and accepts anyway.
//
// Replaces: TestMeta_JobsExtended
func TestJob_ProjectWide_ListsAndDeletesArtifacts(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("projjobs"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		listed := harness.Do[jobs.ListOutput](s, actionJobListProject, params)
		if len(listed.Jobs) != 0 {
			e.T.Errorf("the job listing of a project without a pipeline answered %d jobs", len(listed.Jobs))
		}
		harness.DoVoid(s, actionJobDeleteProjectArtifacts, params)
	})
}

// tokenScopeFixture is what the job token scope scenario stands on: the
// project whose scope is changed, another project and a group to allow.
type tokenScopeFixture struct {
	project fixture.Project
	other   fixture.Project
	group   fixture.Group
}

// inboundProjectIDs returns the project IDs of an inbound allowlist.
func inboundProjectIDs(listed []jobtokenscope.AllowlistProjectItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, project := range listed {
		ids = append(ids, project.ID)
	}
	return ids
}

// allowlistGroupIDs returns the group IDs of a group allowlist.
func allowlistGroupIDs(listed []jobtokenscope.AllowlistGroupItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, group := range listed {
		ids = append(ids, group.ID)
	}
	return ids
}

// TestJob_TokenScope_EnablesAndAllowlistsProjectsAndGroups turns the CI job
// token scope of a shared project on, reads the setting back, and adds and
// removes another project and a group on the two allowlists, reading each
// listing between the writes.
//
// Replaces: TestMeta_JobTokenScope
func TestJob_TokenScope_EnablesAndAllowlistsProjectsAndGroups(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) tokenScopeFixture {
		return tokenScopeFixture{
			project: fixture.NewProject(e, fixture.WithNamePrefix("scope")),
			other:   fixture.NewProject(e, fixture.WithNamePrefix("allowed")),
			group:   fixture.NewGroup(e, fixture.WithGroupNamePrefix("allowed")),
		}
	}, func(e *harness.Env, surface harness.Surface, f tokenScopeFixture) {
		s := e.On(surface)
		params := map[string]any{"project_id": f.project.IDParam()}

		// The patch answers a status rather than the settings it wrote, so
		// the effect is read back rather than taken from the reply.
		patched := harness.Do[toolutil.DeleteOutput](s, actionJobTokenScopePatch, withParams(params, map[string]any{"enabled": true}))
		if patched.Status != "updated" {
			e.T.Errorf("token scope patch answered %+v, want the status updated", patched)
		}
		settings := harness.Do[jobtokenscope.AccessSettingsOutput](s, actionJobTokenScopeGet, params)
		if !settings.InboundEnabled {
			e.T.Errorf("the token scope reads back as %+v after asking for it to be enabled", settings)
		}

		added := harness.Do[jobtokenscope.InboundAllowItemOutput](s, actionJobTokenScopeAddProject, withParams(params, map[string]any{"target_project_id": f.other.ID}))
		if added.SourceProjectID != f.project.ID || added.TargetProjectID != f.other.ID {
			e.T.Errorf("token scope add project answered %+v, want %d allowed into %d", added, f.other.ID, f.project.ID)
		}
		inbound := harness.Do[jobtokenscope.ListInboundAllowlistOutput](s, actionJobTokenScopeListInbound, params)
		if !containsID(inboundProjectIDs(inbound.Projects), f.other.ID) {
			e.T.Errorf("the inbound allowlist does not hold project %d: %v", f.other.ID, inboundProjectIDs(inbound.Projects))
		}
		harness.DoVoid(s, actionJobTokenScopeRemoveProject, withParams(params, map[string]any{"target_project_id": f.other.ID}))
		inboundAfter := harness.Do[jobtokenscope.ListInboundAllowlistOutput](s, actionJobTokenScopeListInbound, params)
		if containsID(inboundProjectIDs(inboundAfter.Projects), f.other.ID) {
			e.T.Errorf("the inbound allowlist still holds project %d after its removal: %v", f.other.ID, inboundProjectIDs(inboundAfter.Projects))
		}

		groupAdded := harness.Do[jobtokenscope.GroupAllowlistItemOutput](s, actionJobTokenScopeAddGroup, withParams(params, map[string]any{"target_group_id": f.group.ID}))
		if groupAdded.SourceProjectID != f.project.ID || groupAdded.TargetGroupID != f.group.ID {
			e.T.Errorf("token scope add group answered %+v, want group %d allowed into %d", groupAdded, f.group.ID, f.project.ID)
		}
		groups := harness.Do[jobtokenscope.ListGroupAllowlistOutput](s, actionJobTokenScopeListGroups, params)
		if !containsID(allowlistGroupIDs(groups.Groups), f.group.ID) {
			e.T.Errorf("the group allowlist does not hold group %d: %v", f.group.ID, allowlistGroupIDs(groups.Groups))
		}
		harness.DoVoid(s, actionJobTokenScopeRemoveGroup, withParams(params, map[string]any{"target_group_id": f.group.ID}))
		groupsAfter := harness.Do[jobtokenscope.ListGroupAllowlistOutput](s, actionJobTokenScopeListGroups, params)
		if containsID(allowlistGroupIDs(groupsAfter.Groups), f.group.ID) {
			e.T.Errorf("the group allowlist still holds group %d after its removal: %v", f.group.ID, allowlistGroupIDs(groupsAfter.Groups))
		}
	})
}
