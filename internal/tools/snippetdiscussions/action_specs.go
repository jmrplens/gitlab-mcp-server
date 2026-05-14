package snippetdiscussions

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for snippet discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_list_snippet_discussions"),
		snippetDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_snippet_discussion_note"),
		snippetDiscussionUpdateSpec("discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_snippet_discussion_note"),
		snippetDiscussionDeleteSpec("discussion_delete_note", toolutil.DestructiveVoidAction(client, DeleteNote), "gitlab_delete_snippet_discussion_note"),
	}
}

func snippetDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetDiscussionOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetDiscussionOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetDiscussionOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"snippet", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "snippetdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
