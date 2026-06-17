package jobs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionJobTrace  = "job.trace"
	actionJobRetry  = "job.retry"
	actionJobCancel = "job.cancel"
	actionJobGet    = "job.get"
)

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
		jobMutationSpec("play", toolutil.RouteAction(client, Play), "gitlab_job_play"),
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
	options.RelatedActions = []string{actionJobTrace, actionJobCancel, actionJobRetry}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or path that owns the job.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use project_id for project scope; pipeline_id is not a substitute for project_id."},
		},
		"job_id": {
			SemanticRole:     "job_identifier",
			ValueSource:      "Numeric CI job ID from pipeline/job list output or user-provided context.",
			ExampleBinding:   "params.job_id:12345",
			CommonConfusions: []string{"Use job_id from GitLab job records; do not pass pipeline ID as job ID."},
		},
	}
	options.IndividualTool.Description = "Get one CI job. Returns: status, stage, ref, pipeline ID, timing fields, runner/user details, and failure metadata. See also: gitlab_job_trace, gitlab_job_cancel, gitlab_job_retry."
	return toolutil.NewReadActionSpec("get", route, options)
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
	case "gitlab_job_list_project":
		options.Usage = "List jobs in one project. Use this when the prompt asks for recent, failed, manual, or retried jobs in a known project; combine filters and pagination as needed."
		options.Aliases = []string{"list project jobs", "show jobs in project", "find project jobs"}
		options.RelatedActions = []string{actionJobGet, actionJobTrace, "pipeline.get"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project identifier where jobs are listed.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"scope": {
				SemanticRole:     "job_status_filter",
				ValueSource:      "Status filter requested by task, for example failed, success, running, pending, or manual.",
				ExampleBinding:   `params.scope:["failed"]`,
				CommonConfusions: []string{"scope is an array of status strings; avoid using natural language values."},
			},
		}
		options.IndividualTool.Description = "List CI jobs in one project with filters and pagination. Returns: job summaries, status, stage, ref, and pipeline associations. See also: gitlab_job_get, gitlab_job_trace, gitlab_pipeline_get."
	case "gitlab_job_trace":
		options.Usage = "Get job log output (trace) for troubleshooting and diagnostics. Use with a known job_id after selecting a relevant job from list/get calls."
		options.Aliases = []string{"get job log", "job trace", "show job output"}
		options.RelatedActions = []string{actionJobGet, "job.list_project", actionJobRetry, actionJobCancel}
		options.IndividualTool.Description = "Get CI job trace output. Returns: text log with truncation metadata when logs exceed limits. See also: gitlab_job_get, gitlab_job_retry, gitlab_job_cancel."
	case "gitlab_job_download_single_artifact":
		options.Usage = "Download one artifact file path from a job by job_id and artifact_path. Use when the task requests one artifact file by explicit path; prefer job.artifacts for full archives."
	case "gitlab_job_retry":
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobCancel}
	case "gitlab_job_cancel":
		options.RelatedActions = []string{actionJobGet, actionJobTrace, actionJobRetry}
		options.IndividualTool.Description = "Cancel a CI job. Set force:true to cancel jobs already in a non-cancellable state (requires GitLab v17.2+). Returns: updated job state. See also: gitlab_job_get, gitlab_job_retry."
	}

	return options
}
