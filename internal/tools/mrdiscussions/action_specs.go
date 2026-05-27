package mrdiscussions

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_mr_discussion_create"),
		mrDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_mr_discussion_list"),
		mrDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_mr_discussion_get"),
		mrDiscussionCreateSpec("discussion_reply", toolutil.RouteAction(client, Reply), "gitlab_mr_discussion_reply"),
		mrDiscussionUpdateSpec("discussion_resolve", toolutil.RouteAction(client, Resolve), "gitlab_mr_discussion_resolve"),
		mrDiscussionUpdateSpec("discussion_note_update", toolutil.RouteAction(client, UpdateNote), "gitlab_mr_discussion_note_update"),
		mrDiscussionDeleteSpec("discussion_note_delete", toolutil.DestructiveVoidAction(client, DeleteNote), "gitlab_mr_discussion_note_delete"),
	}
}

func mrDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mrdiscussions domain action.", Tags: []string{"merge_request", "review", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "mrdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
