//go:build e2e

// jobs_test.go covers a pipeline's jobs through the server, beyond the list,
// get and trace the pipeline lifecycle reads: the bridges a pipeline
// triggers, the four shapes an artifact is downloaded in, the keep of an
// archive and the delete of it, the play, cancel and retry of a manual job,
// the erase of a finished one, the project-wide job listing and artifact
// delete, and the CI job token scope with its two allowlists.

package common

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
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

// retriedJobSettledStatuses are the states in which a retried job can no
// longer hold the runner. canceling is not one of them: the runner is still
// finishing the job then, which is the state the forced cancel ends.
var retriedJobSettledStatuses = []string{"manual", "canceled", "skipped", "success", "failed", "created"}

// stopRetriedJob makes sure the job a retry created cannot hold the runner,
// by canceling it until it reads as one of retriedJobSettledStatuses.
//
// The cancel is repeated rather than sent once because the first one races
// the runner: a retried job is pending at once, and a cancel that reaches
// GitLab while the runner is picking it up answers 409, the optimistic lock
// on the build. The final CE run of the wave 1 stack met exactly that on the
// individual surface, and the forced cancel this used to fall back on
// answered 201 and changed nothing, since GitLab's force_cancel moves only a
// job already canceling; the job then ran its whole script past the wait. A
// plain cancel sent again after the lock is released stops it, and the
// forced one is kept for the one state it applies to.
func stopRetriedJob(e *harness.Env, s *harness.Session, byJob map[string]any) {
	e.T.Helper()

	var last jobs.Output
	err := harness.Poll(e.Ctx, jobStatusPollInterval, jobSettledWait, func() (bool, string, error) {
		job, getErr := harness.Try[jobs.Output](s, actionJobGet, byJob)
		if getErr != nil {
			return false, "", getErr
		}
		last = job
		if slices.Contains(retriedJobSettledStatuses, job.Status) {
			return true, job.Status, nil
		}
		cancel := byJob
		if job.Status == "canceling" {
			cancel = withParams(byJob, map[string]any{"force": true})
		}
		if _, cancelErr := harness.Try[jobs.Output](s, actionJobCancel, cancel); cancelErr != nil && !isJobConflict(cancelErr) {
			return false, "", cancelErr
		}
		return false, "the retried job is " + job.Status, nil
	})
	if err != nil {
		e.T.Fatalf("stopping the retried job %d, last seen %q: %v", last.ID, last.Status, err)
	}
	e.T.Logf("the retried job %d settled as %q", last.ID, last.Status)
}

// assertMeasuredOnlyWhenRun holds the listed jobs to what GitLab measured:
// the build job ran, so it carries a duration; the manual job has not been
// played, GitLab sends its durations as null, and the listing used to
// publish them as zero seconds.
func assertMeasuredOnlyWhenRun(e *harness.Env, ran, unplayed jobs.Output) {
	e.T.Helper()
	if ran.Duration == nil {
		e.T.Errorf("the job %d ran and was listed with no duration", ran.ID)
	}
	if unplayed.Duration != nil || unplayed.QueuedDuration != nil {
		e.T.Errorf("the manual job %d was listed with duration %v and queued duration %v before anyone played it, want neither", unplayed.ID, unplayed.Duration, unplayed.QueuedDuration)
	}
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
		// The child configuration goes in first and the main one last,
		// because only the main configuration's commit carries the skip
		// marker: the child's commit carries none, and pushed with the main
		// configuration already on the branch it would run the whole
		// pipeline, child included. In this order the child's push finds no
		// main configuration to run and the main configuration's push is
		// skipped, so each surface's pipeline below is the only one this
		// configuration runs, and its bridge is the one that triggers a
		// child.
		fixture.CommitFile(e, project, project.DefaultBranch, jobLifecycleChildCIPath, jobLifecycleChildCIYAML, "ci: add the child pipeline configuration")
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, jobLifecycleCIYAML, "ci: add the job lifecycle configuration "+fixture.SkipCIMarker)
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
		assertMeasuredOnlyWhenRun(e, build, manual)
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

// The job of jobLargeArtifactsCIYAML, and the binary file it writes beside a
// blob larger than the one MiB a response carries of an archive.
const (
	jobLargeArtifactsJob  = "large-artifacts"
	jobBinaryArtifactPath = "out/logo.bin"
)

// jobBinaryArtifactBytes is what the job writes to jobBinaryArtifactPath: a
// PNG signature and four bytes no UTF-8 decoder keeps, which a JSON string
// used to turn into U+FFFD.
var jobBinaryArtifactBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0xff, 0xfe, 0x00, 0x80}

// jobLargeArtifactsCIYAML runs one job whose archive holds three million
// random bytes, which no compression brings under a MiB, and the binary file.
const jobLargeArtifactsCIYAML = `stages:
  - test

` + jobLargeArtifactsJob + `:
  stage: test
  script:
    - mkdir -p out
    - head -c 3000000 /dev/urandom > out/blob.bin
    - |
      printf '\211PNG\r\n\032\n\377\376\000\200' > ` + jobBinaryArtifactPath + `
  artifacts:
    paths:
      - out/
    expire_in: 1 day
  tags: []
`

// TestJob_Artifacts_LargeArchiveAndBinaryFile runs a pipeline whose archive
// is larger than a MiB and reads it on every surface: the archive comes back
// with its real size and no content, where its first MiB used to come back
// base64-encoded and unopenable, and the binary file in it comes back
// base64-encoded and byte for byte, where its invalid UTF-8 bytes used to
// come back as U+FFFD.
func TestJob_Artifacts_LargeArchiveAndBinaryFile(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("bigart"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, jobLargeArtifactsCIYAML, "ci: write a large archive and a binary file "+fixture.SkipCIMarker)
		pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
		if pipeline.Status != "success" {
			e.T.Fatalf("pipeline %d ended as %q, want success", pipeline.ID, pipeline.Status)
		}
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		byRef := withParams(params, map[string]any{"ref_name": project.DefaultBranch, "job": jobLargeArtifactsJob})

		archive := harness.Do[jobs.ArtifactsOutput](s, actionJobDownloadArtifacts, byRef)
		if !archive.Truncated || archive.Content != "" || archive.Size <= 1<<20 {
			e.T.Errorf("the large archive answered truncated %v, %d bytes, %d characters of content; want its real size over a MiB and no content",
				archive.Truncated, archive.Size, len(archive.Content))
		}

		single := harness.Do[jobs.SingleArtifactOutput](s, actionJobDownloadSingleArtifactByRef, withParams(byRef, map[string]any{"artifact_path": jobBinaryArtifactPath}))
		decoded, err := base64.StdEncoding.DecodeString(single.Content)
		if single.Encoding != "base64" || err != nil || !bytes.Equal(decoded, jobBinaryArtifactBytes) {
			e.T.Errorf("the binary artifact answered encoding %q and %x (%v), want base64 of %x", single.Encoding, decoded, err, jobBinaryArtifactBytes)
		}
	})
}

// The jobs of jobLongLogCIYAML, the one the runner timestamps and the one it
// does not, the line their logs end with, and the progress line each prints
// as three writes.
const (
	jobLongLogJob          = "long-log"
	jobLongLogUntimedJob   = "long-log-untimestamped"
	jobLongLogFatal        = "FATAL unit test TestCheckout failed expected 200 got 500"
	jobLongLogProgressLine = "progress 1\rprogress 2\rprogress done"
)

// jobLongLogCIYAML runs two jobs that print three thousand lines, some 160
// KB with the runner's own output, then a progress line in three writes a
// second apart, the first two ending in a carriage return, and then fail with
// jobLongLogFatal. The first runs with the runner's timestamps (FF_TIMESTAMPS)
// and the second without, both set on the job so neither depends on the
// runner's default. A write whose bytes after its last line break hold a
// carriage return is one the runner flushes as a line of its own when it
// timestamps the log, and the next piece continues it, so the timestamped log
// holds the progress line in three pieces.
const jobLongLogCIYAML = `stages:
  - test

.long-log:
  stage: test
  script:
    - 'for i in $(seq 1 3000); do echo "installing dependency number $i of the build ......"; done'
    - 'printf "progress 1\r"; sleep 1; printf "progress 2\r"; sleep 1; echo "progress done"'
    - 'echo "` + jobLongLogFatal + `"'
    - exit 1
  tags: []

` + jobLongLogJob + `:
  extends: .long-log
  variables:
    FF_TIMESTAMPS: "true"

` + jobLongLogUntimedJob + `:
  extends: .long-log
  variables:
    FF_TIMESTAMPS: "false"
`

// runnerTimestampHeader is GitLab's pattern for the header the runner writes
// in front of every line of a log it timestamps
// (Gitlab::Ci::Trace::Stream::TIMESTAMP_HEADER_REGEX), anchored at the start
// of a line.
var runnerTimestampHeader = regexp.MustCompile(`^[0-9]{4}-[01][0-9]-[0-3][0-9]T[0-2][0-9]:[0-5][0-9]:[0-5][0-9]\.[0-9]{6}Z [0-9a-f]{2}[EO][+ ]`)

// runnerTimestampFlag is where, in a line of a log the runner timestamps, the
// byte that flags the line as continuing the one before it stands: the last
// of the 32 bytes of its header.
const runnerTimestampFlag = 31

// requireRunnerLogShapes holds the run to what the scenario needs of the
// runner, read from the two logs as GitLab stores them before any trace is
// read through the server. The job run with FF_TIMESTAMPS "true" wrote a
// header in front of every line, and flushed the second write of the progress
// line as a piece flagged + after the first. The job run with "false" wrote no
// header at all, and the progress line as one line. The assertions on the
// traces read only what the server returns, and a log the runner did not
// timestamp passes them all, so a runner that stopped honoring the flag would
// otherwise leave the scenario green with nothing stripped or joined.
func requireRunnerLogShapes(e *harness.Env, project fixture.Project, pipeline fixture.Pipeline) {
	e.T.Helper()
	listed, _, err := e.Client().GL().Jobs.ListPipelineJobs(project.ID, pipeline.ID, &gl.ListJobsOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("listing the jobs of pipeline %d: %v", pipeline.ID, err)
	}
	stored := func(name string) []string {
		for _, job := range listed {
			if job == nil || job.Name != name {
				continue
			}
			raw, _, readErr := e.Client().GL().Jobs.GetTraceFile(project.ID, job.ID, gl.WithContext(e.Ctx))
			if readErr != nil {
				e.T.Fatalf("reading the log of job %q (%d) from GitLab: %v", name, job.ID, readErr)
			}
			var log strings.Builder
			_, _ = raw.WriteTo(&log)
			return traceLines(log.String())
		}
		e.T.Fatalf("pipeline %d holds no job named %q", pipeline.ID, name)
		return nil
	}

	stamped := stored(jobLongLogJob)
	if i := slices.IndexFunc(stamped, func(line string) bool { return !runnerTimestampHeader.MatchString(line) }); i >= 0 {
		e.T.Fatalf("line %d of job %q, run with FF_TIMESTAMPS \"true\", opens with no timestamp header: %q; the scenario needs the runner to timestamp it", i, jobLongLogJob, stamped[i])
	}
	if !slices.ContainsFunc(stamped, func(line string) bool { return strings.HasPrefix(line[runnerTimestampFlag:], "+progress 2\r") }) {
		e.T.Fatalf("job %q, run with FF_TIMESTAMPS \"true\", holds no piece \"progress 2\" flagged as continuing the line before it; the scenario needs the runner to flush a write that ends in a carriage return as a piece", jobLongLogJob)
	}
	untimed := stored(jobLongLogUntimedJob)
	if i := slices.IndexFunc(untimed, runnerTimestampHeader.MatchString); i >= 0 {
		e.T.Fatalf("line %d of job %q, run with FF_TIMESTAMPS \"false\", opens with a timestamp header: %q; the scenario needs the runner to leave it untimestamped", i, jobLongLogUntimedJob, untimed[i])
	}
	if !slices.Contains(untimed, jobLongLogProgressLine) {
		e.T.Fatalf("job %q, run with FF_TIMESTAMPS \"false\", does not hold the progress line %q as one line", jobLongLogUntimedJob, jobLongLogProgressLine)
	}
}

// TestJob_Trace_LongLogKeepsItsFailure runs a job whose log is longer than
// the 100 KB one trace read returns and ends in a failure, and reads it on
// every surface: the log keeps its end, the failure included, says how much
// of its beginning it left out, and carries none of the runner's color codes,
// section markers or timestamp headers, with the progress line the runner
// wrote in pieces whole again; tail_lines returns exactly the last lines of
// that end, and reaches the failure once it asks for as many lines as the
// failure is from the end. The same job run without timestamps reads as the
// same lines, empty lines aside. Before any of that, the two logs as GitLab
// stores them are held
// to what the runner writes with the flag on and off, so the scenario cannot
// pass on a runner that wrote neither header nor pieces. The read used to keep
// the first 100 KB, so the failure was
// exactly what it dropped, and then kept the header at the start of every
// line, with each piece of a line the runner flushed early as a line of its
// own.
func TestJob_Trace_LongLogKeepsItsFailure(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	type run struct {
		project  fixture.Project
		pipeline fixture.Pipeline
	}
	harness.SurfacesWith(e, func(e *harness.Env) run {
		project := fixture.NewProject(e, fixture.WithNamePrefix("longlog"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, jobLongLogCIYAML, "ci: print a long log and fail "+fixture.SkipCIMarker)
		pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
		if pipeline.Status != "failed" {
			e.T.Fatalf("pipeline %d ended as %q, want failed", pipeline.ID, pipeline.Status)
		}
		requireRunnerLogShapes(e, project, pipeline)
		return run{project: project, pipeline: pipeline}
	}, func(e *harness.Env, surface harness.Surface, r run) {
		s := e.On(surface)
		listed := harness.Do[jobs.ListOutput](s, actionJobList, map[string]any{"project_id": r.project.IDParam(), "pipeline_id": r.pipeline.ID})
		job := jobNamed(e, listed, jobLongLogJob)
		byJob := map[string]any{"project_id": r.project.IDParam(), "job_id": job.ID}

		trace := harness.Do[jobs.TraceOutput](s, actionJobTrace, byJob)
		if !trace.Truncated || trace.OmittedBytes <= 0 || len(trace.Trace) > 100<<10 {
			e.T.Errorf("the long log answered truncated %v, %d bytes, %d omitted; want its last 100 KB at most and the rest counted", trace.Truncated, len(trace.Trace), trace.OmittedBytes)
		}
		if !strings.Contains(trace.Trace, jobLongLogFatal) {
			e.T.Errorf("the long log does not carry its failing line %q", jobLongLogFatal)
		}
		if strings.Contains(trace.Trace, "\x1b[") || strings.Contains(trace.Trace, "section_start:") {
			e.T.Errorf("the log still carries terminal markup: %q", firstLine(trace.Trace))
		}
		lines := traceLines(trace.Trace)
		if i := slices.IndexFunc(lines, runnerTimestampHeader.MatchString); i >= 0 {
			e.T.Errorf("line %d of the log still carries the runner's timestamp header: %q", i, lines[i])
		}
		if !slices.Contains(lines, jobLongLogProgressLine) {
			e.T.Errorf("the log does not carry the progress line %q whole, as one line", jobLongLogProgressLine)
		}
		untimed := jobNamed(e, listed, jobLongLogUntimedJob)
		plain := harness.Do[jobs.TraceOutput](s, actionJobTrace, map[string]any{"project_id": r.project.IDParam(), "job_id": untimed.ID})
		assertSameLogText(e, trace.Trace, plain.Trace)

		// The runner writes lines of its own after the script's last output,
		// the cleanup section and "ERROR: Job failed" among them, and how many
		// is the runner's to decide, so the tails are held to the end of the
		// log read whole: five lines are its last five, and as many lines as
		// the failure is from the end start at the failure.
		failure := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, jobLongLogFatal) })
		if failure < 0 || len(lines) < 5 {
			e.T.Fatalf("the long log read whole has %d lines and its failure at %d, want five or more and the failure", len(lines), failure)
		}
		last := harness.Do[jobs.TraceOutput](s, actionJobTrace, withParams(byJob, map[string]any{"tail_lines": 5}))
		if got := traceLines(last.Trace); !slices.Equal(got, lines[len(lines)-5:]) {
			e.T.Errorf("tail_lines 5 answered %q, want the last five lines of the log %q", got, lines[len(lines)-5:])
		}
		fromFailure := len(lines) - failure
		reaching := harness.Do[jobs.TraceOutput](s, actionJobTrace, withParams(byJob, map[string]any{"tail_lines": fromFailure}))
		if got := traceLines(reaching.Trace); len(got) != fromFailure || !strings.Contains(got[0], jobLongLogFatal) {
			e.T.Errorf("tail_lines %d answered %q, want %d lines starting at the failure", fromFailure, got, fromFailure)
		}
	})
}

// traceLines splits a job log into its lines, the newline that ends the last
// one dropped.
func traceLines(trace string) []string {
	return strings.Split(strings.TrimSuffix(trace, "\n"), "\n")
}

// assertSameLogText holds the end of a log the runner timestamped, as the
// server reads it, to the end of the same job's log written without
// timestamps: the lines both reach, counted back from their ends, are the
// same lines, which they are only once the headers are gone and the lines the
// runner wrote in pieces are whole again. Both ends start at a line, and each
// is cut to the same number of bytes of text, so one may reach a line further
// back than the other. The empty lines of both are left out. When it
// timestamps a log, the runner (19.5) writes the marker that closes the
// job's last section as a piece of its own and the verdict after it ("ERROR:
// Job failed") on a line it does not flag as continuing that piece, so
// GitLab's viewer does not join them and neither does the server: the
// timestamped text holds an empty line where the marker was, and the
// untimestamped log carries the verdict on the marker's line. The runner also
// ends a log it timestamps with a line break of its own when its last write
// has none.
func assertSameLogText(e *harness.Env, timestamped, plain string) {
	e.T.Helper()
	trim := func(trace string) []string {
		return slices.DeleteFunc(strings.Split(trace, "\n"), func(line string) bool { return line == "" })
	}
	stamped, untimed := trim(timestamped), trim(plain)
	shared := min(len(stamped), len(untimed))
	if shared < 1000 {
		e.T.Errorf("the timestamped log ends in %d lines and the untimestamped one in %d, want a thousand or more of each", len(stamped), len(untimed))
	}
	stamped, untimed = stamped[len(stamped)-shared:], untimed[len(untimed)-shared:]
	for i := range shared {
		if stamped[i] != untimed[i] {
			e.T.Errorf("of the last %d lines both logs reach, line %d reads %q in the timestamped log and %q in the untimestamped one", shared, i, stamped[i], untimed[i])
			return
		}
	}
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

// inboundProjectIDs returns the project IDs of an inbound allowlist, whose
// rows are the basic project GitLab renders there.
func inboundProjectIDs(listed []projects.BasicOutput) []int64 {
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
