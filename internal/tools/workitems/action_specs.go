package workitems

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for work item actions exposed through gitlab_issue.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		workItemReadSpec("work_item_get", toolutil.RouteAction(client, Get), "gitlab_get_work_item"),
		workItemReadSpec("work_item_list", toolutil.RouteAction(client, List), "gitlab_list_work_items"),
		workItemCreateSpec("work_item_create", toolutil.RouteAction(client, Create), "gitlab_create_work_item"),
		workItemUpdateSpec("work_item_update", toolutil.RouteAction(client, Update), "gitlab_update_work_item"),
		workItemDeleteSpec("work_item_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_work_item"),
		workItemReadSpec("work_item_type_list", toolutil.RouteAction(client, ListWorkItemTypes), "gitlab_list_work_item_types"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("work item #%d from %s", input.IID, input.FullPath))
	return out, nil
}

func workItemReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := workItemOptions(individualTool)
	if individualTool == "gitlab_list_work_item_types" {
		opts.Usage = "List available work item types (system-defined and custom) for a project or group namespace. Supports filtering by name and availability, with cursor-based pagination. Returns: type definitions with id, name, and enabled status. Experimental: the Work Items API may introduce breaking changes between minor versions."
		opts.Aliases = []string{"list work item types", "show work item types", "find work item types", individualTool}
		opts.RelatedActions = []string{"work_item.list", "work_item.create"}
		opts.IndividualTool.Description = "List work item types for a namespace. Returns: id, name, and enabled flag for each type. Supports name filter, only_available flag, and cursor pagination. Experimental. See also: gitlab_list_work_items, gitlab_create_work_item."
	}
	return toolutil.NewReadActionSpec(name, route, opts)
}

func workItemCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, workItemOptions(individualTool))
}

func workItemUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, workItemOptions(individualTool))
}

func workItemDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, workItemOptions(individualTool))
}

func workItemOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute workitems domain action.", Tags: []string{"issue", "work_item"},
		OpenWorld:      true,
		OwnerPackage:   "workitems",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
