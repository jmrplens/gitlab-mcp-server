package badges

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ProjectActionSpecs returns canonical specs for project badge actions.
func ProjectActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		badgeReadSpec("badge_list", toolutil.RouteAction(client, ListProject), "gitlab_list_project_badges"),
		badgeReadSpec("badge_get", projectBadgeGetRoute(client), "gitlab_get_project_badge"),
		badgeCreateSpec("badge_add", toolutil.RouteAction(client, AddProject), "gitlab_add_project_badge"),
		badgeUpdateSpec("badge_edit", toolutil.RouteAction(client, EditProject), "gitlab_edit_project_badge"),
		badgeDeleteSpec("badge_delete", toolutil.DestructiveAction(client, DeleteProjectOutput), "gitlab_delete_project_badge"),
		badgeReadSpec("badge_preview", toolutil.RouteAction(client, PreviewProject), "gitlab_preview_project_badge"),
	}
}

// GroupActionSpecs returns canonical specs for group badge actions.
func GroupActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupBadgeReadSpec("badge_list", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_badges"),
		groupBadgeReadSpec("badge_get", groupBadgeGetRoute(client), "gitlab_get_group_badge"),
		groupBadgeCreateSpec("badge_add", toolutil.RouteAction(client, AddGroup), "gitlab_add_group_badge"),
		groupBadgeUpdateSpec("badge_edit", toolutil.RouteAction(client, EditGroup), "gitlab_edit_group_badge"),
		groupBadgeDeleteSpec("badge_delete", toolutil.DestructiveAction(client, DeleteGroupOutput), "gitlab_delete_group_badge"),
		groupBadgeReadSpec("badge_preview", toolutil.RouteAction(client, PreviewGroup), "gitlab_preview_group_badge"),
	}
}

func badgeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, badgeOptions(name, individualTool))
}

func badgeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, badgeOptions(name, individualTool))
}

func badgeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, badgeOptions(name, individualTool))
}

func badgeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, badgeOptions(name, individualTool))
}

func badgeOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute badges domain action.", Tags: []string{"project", "badge"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "badges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return badgeGuidance(actionName, options)
}

func groupBadgeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupBadgeOptions(name, individualTool))
}

func groupBadgeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupBadgeOptions(name, individualTool))
}

func groupBadgeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupBadgeOptions(name, individualTool))
}

func groupBadgeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupBadgeOptions(name, individualTool))
}

func groupBadgeOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute badges domain action.", Tags: []string{"group", "badge"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "badges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return badgeGuidance(actionName, options)
}

func badgeGuidance(actionName string, options toolutil.ActionSpecOptions) toolutil.ActionSpecOptions {
	scope := "project"
	idParam := "project_id"
	otherParam := "group_id"
	if len(options.Tags) > 0 && options.Tags[0] == "group" {
		scope = "group"
		idParam = "group_id"
		otherParam = "project_id"
	}
	verb := strings.TrimPrefix(actionName, "badge_")
	options.Usage = fmt.Sprintf("%s Use %s for %s badge operations; do not use %s. %s", badgeActionDescription(verb, scope), idParam, scope, otherParam, badgeScopeBoundary(scope))
	if scope == "group" {
		options.Aliases = []string{verb + " group badge", verb + " badge in group"}
	} else {
		options.Aliases = []string{verb + " project badge", verb + " badge in project"}
	}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		idParam: {
			SemanticRole: "scope_" + scope,
			ValueSource:  badgeTitle(scope) + " that owns the badge.",
			CommonConfusions: []string{
				fmt.Sprintf("Do not use %s for %s badge actions.", otherParam, scope),
			},
		},
	}
	return badgeEditGuidance(actionName, options)
}

func badgeScopeBoundary(scope string) string {
	if scope == "group" {
		return "Use only when the task says group badge; project badge CRUD belongs to gitlab_project."
	}
	return "Use when the task says project badge; do not use gitlab_group for project badge CRUD."
}

func badgeActionDescription(verb, scope string) string {
	switch verb {
	case "add":
		return fmt.Sprintf("Add a %s badge.", scope)
	case "get":
		return fmt.Sprintf("Get a %s badge.", scope)
	case "edit":
		return fmt.Sprintf("Edit a %s badge.", scope)
	case "delete":
		return fmt.Sprintf("Delete a %s badge.", scope)
	case "list":
		return fmt.Sprintf("List %s badges.", scope)
	case "preview":
		return fmt.Sprintf("Preview a %s badge.", scope)
	default:
		return fmt.Sprintf("Manage %s badges.", scope)
	}
}

func badgeTitle(scope string) string {
	return strings.ToUpper(scope[:1]) + scope[1:]
}

func badgeEditGuidance(actionName string, options toolutil.ActionSpecOptions) toolutil.ActionSpecOptions {
	if actionName != "badge_edit" {
		return options
	}
	options.Usage += " Use the parameter name name for a new badge name; new_name is not supported."
	options.Aliases = append(options.Aliases, fmt.Sprintf("rename %s badge", options.Tags[0]), fmt.Sprintf("update %s badge name", options.Tags[0]))
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		options.Tags[0] + "_id": options.ParameterGuidance[options.Tags[0]+"_id"],
		"name": {
			SemanticRole: "badge_display_name",
			ValueSource:  "Optional replacement badge name. The parameter is named name.",
			CommonConfusions: []string{
				"Do not send new_name; badge_edit accepts name, link_url, and image_url.",
			},
		},
	}
	return options
}

func projectBadgeGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, GetProject)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return badgeNotFoundOutput{
				Resource:   "Project Badge",
				Identifier: fmt.Sprintf("badge %v in project %v", input["badge_id"], input["project_id"]),
				Hints: []string{
					"Use gitlab_list_project_badges to list badges for this project",
					"Verify the badge_id is correct",
				},
			}, nil
		}
		return result, err
	}
	return route
}

func groupBadgeGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, GetGroup)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return badgeNotFoundOutput{
				Resource:   "Group Badge",
				Identifier: fmt.Sprintf("badge %v in group %v", input["badge_id"], input["group_id"]),
				Hints: []string{
					"Use gitlab_list_group_badges to list badges for this group",
					"Verify the badge_id and group_id are correct",
				},
			}, nil
		}
		return result, err
	}
	return route
}

// DeleteProjectOutput deletes a project badge and returns the legacy success message shape.
func DeleteProjectOutput(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) (toolutil.DeleteOutput, error) {
	if err := DeleteProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted project badge."}, nil
}

// DeleteGroupOutput deletes a group badge and returns the legacy success message shape.
func DeleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group badge."}, nil
}
