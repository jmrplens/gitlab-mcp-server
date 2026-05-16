package badges

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := badgeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, badgeOptions(individualTool))
}

func badgeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := badgeOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := badgeOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "badge"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "badges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func groupBadgeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBadgeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBadgeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupBadgeOptions(individualTool))
}

func groupBadgeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBadgeOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBadgeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBadgeOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBadgeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "badge"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "badges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
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
