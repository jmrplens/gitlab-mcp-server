package pipelines

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD pipeline actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineReadSpec("list", toolutil.RouteAction(client, List), "gitlab_pipeline_list"),
		pipelineReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_pipeline_get"),
		pipelineUpdateSpec("cancel", toolutil.RouteAction(client, Cancel), "gitlab_pipeline_cancel"),
		pipelineUpdateSpec("retry", toolutil.RouteAction(client, Retry), "gitlab_pipeline_retry"),
		pipelineDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_pipeline_delete"),
		pipelineReadSpec("variables", toolutil.RouteAction(client, GetVariables), "gitlab_pipeline_variables"),
		pipelineReadSpec("test_report", toolutil.RouteAction(client, GetTestReport), "gitlab_pipeline_test_report"),
		pipelineReadSpec("test_report_summary", toolutil.RouteAction(client, GetTestReportSummary), "gitlab_pipeline_test_report_summary"),
		pipelineReadSpec("latest", toolutil.RouteAction(client, GetLatest), "gitlab_pipeline_latest"),
		pipelineMutationSpec("create", toolutil.RouteAction(client, Create), "gitlab_pipeline_create"),
		pipelineUpdateSpec("update_metadata", toolutil.RouteAction(client, UpdateMetadata), "gitlab_pipeline_update_metadata"),
		pipelineReadSpec("wait", toolutil.RouteActionWithRequest(client, Wait), "gitlab_pipeline_wait"),
	}
}

func pipelineReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineMutationSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, pipelineOptions(individualTool))
}

func pipelineUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "pipeline"},
		OpenWorld:      true,
		OwnerPackage:   "pipelines",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
