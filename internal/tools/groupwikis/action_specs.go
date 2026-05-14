package groupwikis

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := groupWikiOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupWikiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupWikiOptions(individualTool))
}

func groupWikiUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupWikiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupWikiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "wiki"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupwikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
