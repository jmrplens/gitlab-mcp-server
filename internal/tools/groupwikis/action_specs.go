package groupwikis

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupWikiReadSpec("wiki_list", toolutil.RouteAction(client, List), "gitlab_group_wiki_list"),
		groupWikiReadSpec("wiki_get", toolutil.RouteAction(client, Get), "gitlab_group_wiki_get"),
		groupWikiCreateSpec("wiki_create", toolutil.RouteAction(client, Create), "gitlab_group_wiki_create"),
		groupWikiUpdateSpec("wiki_edit", toolutil.RouteAction(client, Edit), "gitlab_group_wiki_edit"),
		groupWikiDeleteSpec("wiki_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_wiki_delete"),
	}
}

func groupWikiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupWikiOptions(individualTool))
}

func groupWikiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupWikiOptions(individualTool))
}

func groupWikiUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupWikiOptions(individualTool))
}

func groupWikiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupWikiOptions(individualTool))
}

func groupWikiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupwikis domain action.", Tags: []string{"group", "wiki"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupwikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
