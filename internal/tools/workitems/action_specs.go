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
		// gitlab_get_work_item — fetch a single work item by namespace path and IID.
		workItemReadSpec("work_item_get", toolutil.RouteAction(client, Get), "gitlab_get_work_item"),
		// gitlab_list_work_items — list work items for a project or group with cursor pagination.
		workItemReadSpec("work_item_list", toolutil.RouteAction(client, List), "gitlab_list_work_items"),
		// gitlab_create_work_item — create a new work item under a namespace.
		workItemCreateSpec("work_item_create", toolutil.RouteAction(client, Create), "gitlab_create_work_item"),
		// gitlab_update_work_item — update an existing work item's fields, status, or labels.
		workItemUpdateSpec("work_item_update", toolutil.RouteAction(client, Update), "gitlab_update_work_item"),
		// gitlab_delete_work_item — permanently delete a work item by IID (destructive).
		workItemDeleteSpec("work_item_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_work_item"),
		// gitlab_list_work_item_types — list system-defined and custom work item types for a namespace.
		workItemReadSpec("work_item_type_list", toolutil.RouteAction(client, ListWorkItemTypes), "gitlab_list_work_item_types"),
	}
}

// deleteOutput adapts the void [Delete] handler into the catalog DeleteOutput contract
// so it composes with [toolutil.DestructiveAction] and surfaces a confirmation message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("work item #%d from %s", input.IID, input.FullPath))
	return out, nil
}

// workItemReadSpec builds the canonical read-only spec for a work item tool. The
// work-item-types list tool gets richer description, aliases, and related actions
// because it is the entry point for discovering type IDs required by create.
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

// workItemCreateSpec builds the canonical create spec for a work item tool.
func workItemCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, workItemOptions(individualTool))
}

// workItemUpdateSpec builds the canonical update spec for a work item tool.
func workItemUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, workItemOptions(individualTool))
}

// workItemDeleteSpec builds the canonical destructive delete spec for a work item tool.
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
