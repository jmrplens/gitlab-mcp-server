package jobs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionJobTrace          = "job.trace"
	actionJobRetry          = "job.retry"
	actionJobCancel         = "job.cancel"
	actionJobGet            = "job.get"
	actionJobList           = "job.list"
	actionJobListProject    = "job.list_project"
	actionJobListBridges    = "job.list_bridges"
	actionJobArtifacts      = "job.artifacts"
	actionJobPlay           = "job.play"
	actionJobErase          = "job.erase"
	actionJobKeepArtifacts  = "job.keep_artifacts"
	actionJobWait           = "job.wait"
	actionJobDownloadArtfct = "job.download_artifacts"
	actionJobDownloadSingle = "job.download_single_artifact"
	actionPipelineGet       = "pipeline.get"
	actionPipelineList      = "pipeline.list"
	actionCommitGet         = "commit.get"
)

// guidanceProjectID is the shared parameter guidance for the project_id input
// surfaced on every job action.
func guidanceProjectID() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Project ID or full namespace path that owns the job.",
		ExampleBinding:   `params.project_id:"group/project"`,
		CommonConfusions: []string{"Use project_id for project scope. pipeline_id and job_id are not substitutes for project_id."},
	}
}

// guidanceJobID is the shared parameter guidance for the job_id input surfaced
// on per-job actions.
func guidanceJobID() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "job_identifier",
		ValueSource:      "Numeric CI job ID from pipeline/job list output or user-provided context.",
		ExampleBinding:   "params.job_id:12345",
		CommonConfusions: []string{"job_id is the global database ID, not the per-pipeline index. Do not pass a pipeline ID as job_id."},
	}
}

// guidancePipelineID is the shared parameter guidance for the pipeline_id input
// surfaced on pipeline-scoped list actions.
func guidancePipelineID() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "pipeline_identifier",
		ValueSource:      "Numeric pipeline ID from pipeline list output or user-provided context.",
		ExampleBinding:   "params.pipeline_id:67890",
		CommonConfusions: []string{"pipeline_id identifies the pipeline, not an individual job. Use job_id for a specific job."},
	}
}

// guidanceScope is the shared parameter guidance for the scope status filter on
// job list actions.
func guidanceScope() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "job_status_filter",
		ValueSource:      "Status filter requested by task, for example failed, success, running, pending, or manual.",
		ExampleBinding:   `params.scope:["failed"]`,
		CommonConfusions: []string{"scope is an array of status strings. Avoid natural-language values."},
	}
}

// ActionSpecs returns canonical specs for CI/CD job actions exposed as
// MCP tools. The list, get, trace, mutation, artifact, and wait routes
// are projected into the dynamic, meta, individual, and audit surfaces
// by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_job_list — list jobs in one pipeline.
		jobReadSpec("list", toolutil.RouteAction(client, List), "gitlab_job_list"),
		// gitlab_job_list_project — list jobs across a project.
		jobReadSpec("list_project", toolutil.RouteAction(client, ListProject), "gitlab_job_list_project"),
		// gitlab_job_get — fetch a single job by ID (returns a structured not-found result on 404).
		jobGetSpec(toolutil.RouteAction(client, Get)),
		// gitlab_job_trace — read a job's log output.
		jobReadSpec("trace", toolutil.RouteAction(client, Trace), "gitlab_job_trace"),
		// gitlab_job_cancel — cancel a running job.
		jobMutationSpec("cancel", toolutil.RouteAction(client, Cancel), "gitlab_job_cancel"),
		// gitlab_job_retry — retry a failed or canceled job.
		jobMutationSpec("retry", toolutil.RouteAction(client, Retry), "gitlab_job_retry"),
		// gitlab_job_list_bridges — list pipeline bridge (trigger) jobs.
		jobReadSpec("list_bridges", toolutil.RouteAction(client, ListBridges), "gitlab_job_list_bridges"),
		// gitlab_job_artifacts — download the full artifact archive for a job.
		jobReadSpec("artifacts", toolutil.RouteAction(client, GetArtifacts), "gitlab_job_artifacts", "artifact"),
		// gitlab_job_download_artifacts — download the latest successful artifacts for a ref.
		jobReadSpec("download_artifacts", toolutil.RouteAction(client, DownloadArtifacts), "gitlab_job_download_artifacts", "artifact"),
		// gitlab_job_download_single_artifact — download one artifact path by job ID.
		jobReadSpec("download_single_artifact", toolutil.RouteAction(client, DownloadSingleArtifact), "gitlab_job_download_single_artifact", "artifact"),
		// gitlab_job_download_single_artifact_by_ref — download one artifact path by ref.
		jobReadSpec("download_single_artifact_by_ref", toolutil.RouteAction(client, DownloadSingleArtifactByRef), "gitlab_job_download_single_artifact_by_ref", "artifact"),
		// gitlab_job_erase — erase a job's trace and artifacts (destructive).
		jobDeleteSpec("erase", toolutil.DestructiveAction(client, Erase), "gitlab_job_erase"),
		// gitlab_job_keep_artifacts — prevent artifact expiration.
		jobMutationSpec("keep_artifacts", toolutil.RouteAction(client, KeepArtifacts), "gitlab_job_keep_artifacts", "artifact"),
		// gitlab_job_play — trigger a manual job.
		jobMutationSpec("play", playRoute(client), "gitlab_job_play"),
		// gitlab_job_delete_artifacts — delete a job's artifacts (destructive).
		jobDeleteSpec("delete_artifacts", toolutil.DestructiveVoidAction(client, DeleteArtifacts), "gitlab_job_delete_artifacts", "artifact"),
		// gitlab_job_delete_project_artifacts — delete every artifact in a project (destructive).
		jobDeleteSpec("delete_project_artifacts", toolutil.DestructiveVoidAction(client, DeleteProjectArtifacts), "gitlab_job_delete_project_artifacts", "artifact"),
		// gitlab_job_wait — poll a job until it reaches a terminal state.
		jobReadSpec("wait", toolutil.RouteActionWithRequest(client, Wait), "gitlab_job_wait"),
	}
}

// jobGetSpec builds the read-only [toolutil.ActionSpec] for the
// gitlab_job_get individual tool with bespoke usage, aliases, and
// parameter guidance.
func jobGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := jobOptionsForAction("get", "gitlab_job_get")
	options.Usage = "Get one CI job by project_id and job_id. Use this when the task already references a specific job and needs state, stage, runner, failure reason, or timing details."
	options.Aliases = []string{"get job", "show job details", "lookup job"}
	options.RelatedActions = []string{actionJobTrace, actionJobCancel, actionJobRetry, actionJobWait, actionCommitGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": guidanceProjectID(),
		"job_id":     guidanceJobID(),
	}
	options.IndividualTool.Description = "Get one CI job. Returns: status, stage, ref, embedded commit/pipeline/project/runner/user objects, timing fields, and failure metadata. See also: gitlab_job_trace, gitlab_job_cancel, gitlab_job_retry."
	return toolutil.NewReadActionSpec("get", route, options)
}

// playRoute returns the play action route with its input schema constrained:
// job_inputs values are limited to the shapes toolutil.BuildPipelineInputs
// accepts instead of the open map the struct field alone would advertise.
func playRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Play)
	route.InputSchema = toolutil.PipelineInputsSchema[PlayInput]("job_inputs")
	return route
}

// jobReadSpec builds a read-only [toolutil.ActionSpec] for a jobs
// action using the package's default [jobOptionsForAction].
func jobReadSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, jobOptionsForAction(name, individualTool, extraTags...))
}

// jobMutationSpec builds an update-style [toolutil.ActionSpec] for a
// jobs action (cancel, retry, play, keep) using the package's default
// [jobOptionsForAction].
func jobMutationSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, jobOptionsForAction(name, individualTool, extraTags...))
}

// jobDeleteSpec builds a destructive [toolutil.ActionSpec] for a jobs
// action (erase, delete_artifacts) using the package's default
// [jobOptionsForAction].
func jobDeleteSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, jobOptionsForAction(name, individualTool, extraTags...))
}

// jobOptionsForAction returns the base [toolutil.ActionSpecOptions] for
// a jobs action, layering the ci/job tags and any per-action extras,
// and customizing the Usage/Aliases/Description for the most common
// individual tools.
func jobOptionsForAction(actionName, individualTool string, extraTags ...string) toolutil.ActionSpecOptions {
	_ = actionName

	tags := append([]string{"ci", "job"}, extraTags...)
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute jobs domain action.", Tags: tags,
		OpenWorld:      true,
		OwnerPackage:   "jobs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_job_list":
		options.Usage = "List jobs for one pipeline by project_id and pipeline_id. Use this when the prompt references a specific pipeline and asks which jobs ran, failed, or are pending. Filter by scope and paginate as needed."
		options.Aliases = []string{"list pipeline jobs", "show jobs in pipeline", "find pipeline jobs"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionPipelineGet, actionPipelineList, actionJobListBridges}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "pipeline_id": guidancePipelineID(), "scope": guidanceScope(),
		}
		options.IndividualTool.Description = "List CI jobs for one pipeline with status filters, keyset pagination, and ordering. Returns: job summaries with status, stage, ref, and pipeline association. See also: gitlab_job_get, gitlab_job_trace, gitlab_pipeline_get."
	case "gitlab_job_list_project":
		options.Usage = "List jobs in one project. Use this when the prompt asks for recent, failed, manual, or retried jobs in a known project. Combine filters and pagination as needed."
		options.Aliases = []string{"list project jobs", "show jobs in project", "find project jobs"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionPipelineGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "scope": guidanceScope(),
		}
		options.IndividualTool.Description = "List CI jobs in one project with filters, keyset pagination, and ordering. Returns: job summaries, status, stage, ref, and pipeline associations. See also: gitlab_job_get, gitlab_job_trace, gitlab_pipeline_get."
	case "gitlab_job_list_bridges":
		options.Usage = "List bridge (trigger) jobs for one pipeline by project_id and pipeline_id. Use this to inspect downstream or multi-project pipeline triggers. Bridges exist only on pipelines that trigger child pipelines."
		options.Aliases = []string{"list bridge jobs", "show trigger jobs", "list downstream triggers"}
		options.RelatedActions = []string{actionJobList, actionPipelineGet, actionJobGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "pipeline_id": guidancePipelineID(), "scope": guidanceScope(),
		}
		options.IndividualTool.Description = "List CI bridge (trigger) jobs for one pipeline with keyset pagination. Returns: bridge summaries with status and downstream pipeline references. See also: gitlab_job_list, gitlab_pipeline_get, gitlab_job_get."
	case "gitlab_job_trace":
		options.Usage = "Get job log output (trace) for troubleshooting and diagnostics. Use with a known job_id after selecting a relevant job from list/get calls."
		options.Aliases = []string{"get job log", "job trace", "show job output"}
		options.RelatedActions = []string{actionJobGet, actionJobListProject, actionJobRetry, actionJobCancel}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Get CI job trace output. Returns: text log with truncation metadata when logs exceed limits. See also: gitlab_job_get, gitlab_job_retry, gitlab_job_cancel."
	case "gitlab_job_retry":
		options.Usage = "Retry a failed or canceled CI job by project_id and job_id. Use this to re-run a job that ended in failure. Running or successful jobs cannot be retried."
		options.Aliases = []string{"retry job", "rerun job", "re-run failed job"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobCancel}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Retry a failed or canceled CI job. Returns: the newly created job state. See also: gitlab_job_get, gitlab_job_trace, gitlab_job_cancel."
	case "gitlab_job_cancel":
		options.Usage = "Cancel a running or pending CI job by project_id and job_id. Use force:true to cancel jobs already in a non-cancellable state (requires GitLab v17.2+)."
		options.Aliases = []string{"cancel job", "stop job", "abort job"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobRetry}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Cancel a CI job. Set force:true to cancel jobs already in a non-cancellable state (requires GitLab v17.2+). Returns: updated job state. See also: gitlab_job_get, gitlab_job_retry."
	case "gitlab_job_play":
		options.Usage = "Run a manual CI job by project_id and job_id, optionally injecting job_variables_attributes. Use this for jobs defined as when:manual that have not yet run. Use retry for finished jobs."
		options.Aliases = []string{"play job", "run manual job", "trigger manual job"}
		options.RelatedActions = []string{actionJobGet, actionJobRetry, actionJobTrace}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
			"job_variables_attributes": {
				SemanticRole:     "job_variable_overrides",
				ValueSource:      "List of key/value (and optional variable_type) variable overrides to inject into the manual run.",
				ExampleBinding:   `params.job_variables_attributes:[{"key":"ENV","value":"production"}]`,
				CommonConfusions: []string{"Each entry needs an explicit key and value. variable_type defaults to env_var."},
			},
		}
		options.IndividualTool.Description = "Run a manual CI job with optional variable overrides. Returns: the started job state. See also: gitlab_job_get, gitlab_job_retry, gitlab_job_trace."
	case "gitlab_job_keep_artifacts":
		options.Usage = "Prevent a CI job's artifacts from expiring by project_id and job_id. Use this to retain build outputs indefinitely by clearing the artifact expire_at. Requires Maintainer+."
		options.Aliases = []string{"keep job artifacts", "retain artifacts", "prevent artifact expiry"}
		options.RelatedActions = []string{actionJobArtifacts, actionJobGet, actionJobDownloadArtfct}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Keep a CI job's artifacts by clearing their expiration. Returns: updated job state. See also: gitlab_job_artifacts, gitlab_job_get, gitlab_job_download_artifacts."
	case "gitlab_job_erase":
		options.Usage = "Erase a finished CI job's trace log and artifacts by project_id and job_id. Use this destructive action to wipe sensitive output. The job must be in a finished state and requires Maintainer+."
		options.Aliases = []string{"erase job", "wipe job", "clear job trace and artifacts"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobArtifacts}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Erase a finished CI job's trace log and artifacts (destructive). Returns: updated job state. See also: gitlab_job_get, gitlab_job_trace, gitlab_job_artifacts."
	case "gitlab_job_wait":
		options.Usage = "Poll a CI job by project_id and job_id until it reaches a terminal state or times out. Use this to block on a job before reading its result, with progress notifications on each poll."
		options.Aliases = []string{"wait for job", "poll job", "watch job until done"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobRetry}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Wait for a CI job to finish, polling until terminal state or timeout. Returns: final job snapshot, wait duration, poll count, and timed-out flag. See also: gitlab_job_get, gitlab_job_trace, gitlab_job_retry."
	case "gitlab_job_artifacts":
		options.Usage = "Download the full artifact archive for a CI job by project_id and job_id. Use this to retrieve all build outputs as a base64-encoded archive. Prefer download_single_artifact for one file."
		options.Aliases = []string{"download job artifacts", "get artifact archive", "fetch job artifacts"}
		options.RelatedActions = []string{actionJobDownloadSingle, actionJobKeepArtifacts, actionJobGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Download the full artifact archive for a CI job (base64-encoded, truncated at 1MB). Returns: archive size, content, and truncation flag. See also: gitlab_job_download_single_artifact, gitlab_job_keep_artifacts, gitlab_job_get."
	case "gitlab_job_download_artifacts":
		options.Usage = "Download the latest successful artifacts for a ref by project_id, ref_name, and job name. Use this to fetch the most recent build output for a branch or tag without knowing the job ID."
		options.Aliases = []string{"download latest artifacts", "get artifacts for ref", "fetch ref artifacts"}
		options.RelatedActions = []string{actionJobArtifacts, actionJobDownloadSingle, actionJobGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(),
			"ref_name": {
				SemanticRole:   "git_ref",
				ValueSource:    "Branch or tag name whose latest successful pipeline produced the artifacts.",
				ExampleBinding: `params.ref_name:"main"`,
			},
		}
		options.IndividualTool.Description = "Download the latest successful artifacts archive for a ref and job name. Returns: archive size, content, and truncation flag. See also: gitlab_job_artifacts, gitlab_job_download_single_artifact, gitlab_job_get."
	case "gitlab_job_download_single_artifact":
		options.Usage = "Download one artifact file path from a job by job_id and artifact_path. Use when the task requests one artifact file by explicit path. Prefer job.artifacts for full archives."
		options.Aliases = []string{"download single artifact", "get one artifact file", "fetch artifact by path"}
		options.RelatedActions = []string{actionJobArtifacts, actionJobDownloadArtfct, actionJobGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
			"artifact_path": {
				SemanticRole:   "artifact_path",
				ValueSource:    "Path of the file inside the artifact archive.",
				ExampleBinding: `params.artifact_path:"coverage/index.html"`,
			},
		}
		options.IndividualTool.Description = "Download one artifact file from a job by job_id and artifact_path. Returns: file size, raw content, and truncation flag. See also: gitlab_job_artifacts, gitlab_job_download_artifacts, gitlab_job_get."
	case "gitlab_job_download_single_artifact_by_ref":
		options.Usage = "Download one artifact file by project_id, ref_name, job name, and artifact_path. Use this to fetch a single file from the latest successful job on a ref without knowing the job ID."
		options.Aliases = []string{"download single artifact by ref", "get artifact file for ref", "fetch ref artifact path"}
		options.RelatedActions = []string{actionJobDownloadArtfct, actionJobDownloadSingle, actionJobGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(),
			"ref_name": {
				SemanticRole:   "git_ref",
				ValueSource:    "Branch or tag name whose latest successful job produced the artifact.",
				ExampleBinding: `params.ref_name:"main"`,
			},
			"artifact_path": {
				SemanticRole:   "artifact_path",
				ValueSource:    "Path of the file inside the artifact archive.",
				ExampleBinding: `params.artifact_path:"coverage/index.html"`,
			},
		}
		options.IndividualTool.Description = "Download one artifact file from the latest successful job on a ref by name and path. Returns: file size, raw content, and truncation flag. See also: gitlab_job_download_artifacts, gitlab_job_download_single_artifact, gitlab_job_get."
	case "gitlab_job_delete_artifacts":
		options.Usage = "Delete the artifacts for one CI job by project_id and job_id. Use this destructive action to free storage for a finished job. Requires Maintainer+ role."
		options.Aliases = []string{"delete job artifacts", "remove job artifacts", "purge job artifacts"}
		options.RelatedActions = []string{actionJobArtifacts, actionJobGet, "job.delete_project_artifacts"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(), "job_id": guidanceJobID(),
		}
		options.IndividualTool.Description = "Delete the artifacts for one CI job (destructive). Returns: a success confirmation. See also: gitlab_job_artifacts, gitlab_job_get, gitlab_job_delete_project_artifacts."
	case "gitlab_job_delete_project_artifacts":
		options.Usage = "Delete every eligible artifact across a project by project_id. Use this irreversible bulk action to reclaim storage across all jobs. Requires Maintainer+ role."
		options.Aliases = []string{"delete all project artifacts", "purge project artifacts", "bulk delete artifacts"}
		options.RelatedActions = []string{"job.delete_artifacts", actionJobListProject, actionJobArtifacts}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": guidanceProjectID(),
		}
		options.IndividualTool.Description = "Delete every eligible artifact across a project (destructive, irreversible). Returns: a success confirmation. See also: gitlab_job_delete_artifacts, gitlab_job_list_project, gitlab_job_artifacts."
	}

	return options
}
