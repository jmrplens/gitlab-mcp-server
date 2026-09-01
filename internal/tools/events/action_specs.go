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
	usage := "List the authenticated user's contribution events (pushes, comments, issue and merge request activity) across visible resources, with action/target filters and offset or keyset pagination."
	aliases := []string{individualTool, "list my activity", "my contribution events", "what have I done recently", "my recent activity"}
	related := []string{"user.get", "project.get"}
	description := "List the current user's contribution events. Returns: each event with action_name, target type and IID, push_data, embedded note, author object, created timestamp, and pagination metadata. See also: gitlab_project_event_list, gitlab_get_user."
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "event_list_project" {
		usage = "List the visible activity events for one specific project (pushes, comments, member changes, issue and merge request actions), with action/target filters and offset or keyset pagination."
		aliases = []string{individualTool, "list project activity", "project event feed", "recent activity in a project", "what happened in this project"}
		related = []string{"project.get", "user.get"}
		description = "List a project's visible activity events. Returns: each event with action_name, target type and IID, push_data, embedded note with author, data (ref, commits, repository), author object, created timestamp, and pagination metadata. See also: gitlab_user_contribution_event_list, gitlab_project_get."
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path whose activity events should be listed.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the project that owns the events. Do not pass a group path or user ID here."},
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"user", "event"},
		Usage:             usage,
		RelatedActions:    related,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "events",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
		InputSchemaOverrides: []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("action", map[string]any{
				"enum": []any{"created", "updated", "closed", "reopened", "pushed", "commented", "merged", "joined", "left", "destroyed", "expired", "approved"},
			}),
			toolutil.SchemaPropertyOverride("target_type", map[string]any{
				"enum": []any{"issue", "milestone", "merge_request", "note", "project", "snippet", "user", "epic"},
			}),
		},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
