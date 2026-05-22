package events

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// UserActionSpecs returns canonical specs for event actions exposed through gitlab_user.
func UserActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userEventReadSpec("event_list_project", toolutil.RouteAction(client, ListProjectEvents), "gitlab_project_event_list"),
		userEventReadSpec("event_list_contributions", toolutil.RouteAction(client, ListCurrentUserContributionEvents), "gitlab_user_contribution_event_list"),
	}
}

func userEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"user", "event"},
		OpenWorld:      true,
		OwnerPackage:   "events",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
