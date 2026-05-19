package groups

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for core group and group hook actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupReadSpec("list", toolutil.RouteAction(client, List), "gitlab_group_list"),
		groupReadSpec("get", groupGetRoute(client), "gitlab_group_get"),
		groupCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_group_create"),
		groupUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_group_update"),
		groupDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_delete"),
		groupUpdateSpec("restore", toolutil.RouteAction(client, Restore), "gitlab_group_restore"),
		groupUpdateSpec("archive", toolutil.RouteAction(client, ArchiveOutput), "gitlab_group_archive"),
		groupUpdateSpec("unarchive", toolutil.RouteAction(client, UnarchiveOutput), "gitlab_group_unarchive"),
		groupReadSpec("search", toolutil.RouteAction(client, Search), "gitlab_group_search"),
		groupUpdateSpec("transfer_project", toolutil.RouteAction(client, TransferProject), "gitlab_group_transfer_project"),
		groupReadSpec("projects", toolutil.RouteAction(client, ListProjects), "gitlab_group_projects"),
		groupReadSpec("members", toolutil.RouteAction(client, MembersList), "gitlab_group_members_list"),
		groupReadSpec("subgroups", toolutil.RouteAction(client, SubgroupsList), "gitlab_subgroups_list"),
		groupReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_group_hook_list"),
		groupReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_group_hook_get"),
		groupCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_group_hook_add"),
		groupUpdateSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_group_hook_edit"),
		groupDeleteSpec("hook_delete", toolutil.DestructiveVoidAction(client, DeleteHook), "gitlab_group_hook_delete"),
	}
}

func groupGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return groupNotFoundOutput{Identifier: fmt.Sprint(input["group_id"])}, nil
		}
		return result, err
	}
	return route
}

// ArchiveOutput archives a GitLab group and returns the legacy success message shape.
func ArchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Archive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s archived successfully", input.GroupID)}, nil
}

// UnarchiveOutput unarchives a GitLab group and returns the legacy success message shape.
func UnarchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Unarchive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s unarchived successfully", input.GroupID)}, nil
}

func groupReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupOptions(individualTool))
}

func groupUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group"},
		OpenWorld:      true,
		OwnerPackage:   "groups",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
