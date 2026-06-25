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
		Aliases:        []string{individualTool, "list group releases", "show group releases", "find group releases"},
		Usage:          "List releases across all projects in a group. Use this when the task asks for group-wide release history or the latest releases of every project in a group.",
		Tags:           []string{"group", "release", "tag"},
		RelatedActions: []string{"group.get", "release.list", "release.get"},
		OpenWorld:      true,
		OwnerPackage:   "groupreleases",
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group ID or full path whose projects' releases are listed.",
				ExampleBinding: `params.group_id:"my-group"`,
			},
		},
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
			Description: "List releases across all projects in a group with pagination, ordering, and keyset support. " +
				"Returns: tag names, release names, dates, author, commit, assets, milestones, evidences, and _links per release. " +
				"See also: gitlab_group_get, gitlab_release_list, gitlab_release_get.",
		},
	}
}
