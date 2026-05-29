package importservice

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for external import tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		importServiceCreateSpec("import_github", toolutil.RouteAction(client, ImportFromGitHub), "gitlab_import_from_github"),
		importServiceUpdateSpec("import_cancel_github", toolutil.RouteAction(client, CancelGitHubImport), "gitlab_cancel_github_import"),
		importServiceCreateSpec("import_gists", toolutil.RouteVoidAction(client, ImportGists), "gitlab_import_github_gists"),
		importServiceCreateSpec("import_bitbucket", toolutil.RouteAction(client, ImportFromBitbucketCloud), "gitlab_import_from_bitbucket_cloud"),
		importServiceCreateSpec("import_bitbucket_server", toolutil.RouteAction(client, ImportFromBitbucketServer), "gitlab_import_from_bitbucket_server"),
	}
}

func importServiceCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, importServiceOptions(individualTool))
}

func importServiceUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, importServiceOptions(individualTool))
}

func importServiceOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute importservice domain action.", Tags: []string{"import"},
		OpenWorld:      true,
		OwnerPackage:   "importservice",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
