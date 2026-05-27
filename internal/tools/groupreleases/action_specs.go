package groupreleases

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group release actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupReleaseReadSpec("release_list", toolutil.RouteAction(client, List), "gitlab_group_release_list"),
	}
}

func groupReleaseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupReleaseOptions(individualTool))
}

func groupReleaseOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupreleases domain action.", Tags: []string{"group", "release"},
		RelatedActions: []string{"group.get", "project.release_list"},
		OpenWorld:      true,
		OwnerPackage:   "groupreleases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
