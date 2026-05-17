package jobs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD job actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		jobReadSpec("list", toolutil.RouteAction(client, List), "gitlab_job_list"),
		jobReadSpec("list_project", toolutil.RouteAction(client, ListProject), "gitlab_job_list_project"),
		jobGetSpec(toolutil.RouteAction(client, Get)),
		jobReadSpec("trace", toolutil.RouteAction(client, Trace), "gitlab_job_trace"),
		jobMutationSpec("cancel", toolutil.RouteAction(client, Cancel), "gitlab_job_cancel"),
		jobMutationSpec("retry", toolutil.RouteAction(client, Retry), "gitlab_job_retry"),
		jobReadSpec("list_bridges", toolutil.RouteAction(client, ListBridges), "gitlab_job_list_bridges"),
		jobReadSpec("artifacts", toolutil.RouteAction(client, GetArtifacts), "gitlab_job_artifacts", "artifact"),
		jobReadSpec("download_artifacts", toolutil.RouteAction(client, DownloadArtifacts), "gitlab_job_download_artifacts", "artifact"),
		jobReadSpec("download_single_artifact", toolutil.RouteAction(client, DownloadSingleArtifact), "gitlab_job_download_single_artifact", "artifact"),
		jobReadSpec("download_single_artifact_by_ref", toolutil.RouteAction(client, DownloadSingleArtifactByRef), "gitlab_job_download_single_artifact_by_ref", "artifact"),
		jobDeleteSpec("erase", toolutil.DestructiveAction(client, Erase), "gitlab_job_erase"),
		jobMutationSpec("keep_artifacts", toolutil.RouteAction(client, KeepArtifacts), "gitlab_job_keep_artifacts", "artifact"),
		jobMutationSpec("play", toolutil.RouteAction(client, Play), "gitlab_job_play"),
		jobDeleteSpec("delete_artifacts", toolutil.DestructiveVoidAction(client, DeleteArtifacts), "gitlab_job_delete_artifacts", "artifact"),
		jobDeleteSpec("delete_project_artifacts", toolutil.DestructiveVoidAction(client, DeleteProjectArtifacts), "gitlab_job_delete_project_artifacts", "artifact"),
		jobReadSpec("wait", toolutil.RouteActionWithRequest(client, Wait), "gitlab_job_wait"),
	}
}

func jobGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := jobOptions("gitlab_job_get")
	options.ReadOnly = true
	options.Idempotent = true
	options.RelatedActions = []string{"job.trace", "job.cancel", "job.retry"}
	return toolutil.NewActionSpec("get", route, options)
}

func jobReadSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := jobOptions(individualTool, extraTags...)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobMutationSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := jobOptions(individualTool, extraTags...)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobDeleteSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := jobOptions(individualTool, extraTags...)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobOptions(individualTool string, extraTags ...string) toolutil.ActionSpecOptions {
	tags := append([]string{"ci", "job"}, extraTags...)
	return toolutil.ActionSpecOptions{
		Tags:           tags,
		OpenWorld:      true,
		OwnerPackage:   "jobs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
