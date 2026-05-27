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
	usage := "List current-user contribution events across visible resources."
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "event_list_project" {
		usage = "List events for one specific project with optional filters and pagination."
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path whose activity events should be listed.",
			ExampleBinding: `params.project_id:"group/project"`,
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"user", "event"},
		Usage:             usage,
		RelatedActions:    []string{"project.get", "user.get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "events",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
