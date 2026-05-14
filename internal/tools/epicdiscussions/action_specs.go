package epicdiscussions

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicDiscussionReadSpec("epic_discussion_list", toolutil.RouteAction(client, List), "gitlab_list_epic_discussions"),
		epicDiscussionReadSpec("epic_discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_epic_discussion_note"),
		epicDiscussionUpdateSpec("epic_discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_epic_discussion_note"),
		epicDiscussionDeleteSpec("epic_discussion_delete_note", toolutil.DestructiveVoidAction(client, DeleteNote), "gitlab_delete_epic_discussion_note"),
	}
}

func epicDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicDiscussionOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicDiscussionOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicDiscussionOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "epic", "discussion"},
		RelatedActions: []string{"group.epic_get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
