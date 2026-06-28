package workitems

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionWorkItemList = "work_item.list"
	actionWorkItemGet  = "work_item.get"
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
	switch individualTool {
	case "gitlab_get_work_item":
		opts.Usage = "Get one exact work item by namespace full_path plus work_item_iid. Use this after list/search results or when the prompt already names a concrete work item number; prefer work_item.get over work_item.list when the target is already known."
		opts.Aliases = []string{"get work item", "show work item details", "fetch work item by iid", individualTool}
		opts.RelatedActions = []string{actionWorkItemList, "work_item.update", "work_item.delete"}
		opts.IndividualTool.Description = "Get a single work item by namespace path and IID. Returns: id, iid, type, state, status, title, description, author, assignees, labels, linked items, confidentiality, timestamps, and web URL. Experimental. See also: gitlab_list_work_items, gitlab_update_work_item, gitlab_delete_work_item."
	case "gitlab_list_work_items":
		opts.Usage = "List work items in one project or group namespace. Use filters such as state, search, types, author_username, label_name, confidential, sort, include_ancestors/descendants, and cursor pagination when the prompt asks for matching work items in a known namespace."
		opts.Aliases = []string{"list work items", "find work items in namespace", "show open work items", individualTool}
		opts.RelatedActions = []string{actionWorkItemGet, "work_item.create", "work_item.type_list"}
		opts.IndividualTool.Description = "List work items in a project or group namespace with filtering and cursor pagination. Returns: matching work items with type, state, title, author, labels, and timestamps. Experimental. See also: gitlab_get_work_item, gitlab_create_work_item, gitlab_list_work_item_types."
		opts.InputSchemaOverrides = workItemListEnumOverrides()
	case "gitlab_list_work_item_types":
		opts.Usage = "List available work item types (system-defined and custom) for a project or group namespace. Supports filtering by name and availability, with cursor-based pagination. Returns: type definitions with id, name, and enabled status. Experimental: the Work Items API may introduce breaking changes between minor versions."
		opts.Aliases = []string{"list work item types", "show work item types", "find work item types", individualTool}
		opts.RelatedActions = []string{actionWorkItemList, "work_item.create"}
		opts.IndividualTool.Description = "List work item types for a namespace. Returns: id, name, and enabled flag for each type. Supports name filter, only_available flag, and cursor pagination. Experimental. See also: gitlab_list_work_items, gitlab_create_work_item."
	}
	return toolutil.NewReadActionSpec(name, route, opts)
}

// workItemCreateSpec builds the canonical create spec for a work item tool.
func workItemCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := workItemOptions(individualTool)
	opts.Usage = "Create a new work item under a project or group namespace. Provide full_path, a work_item_type_id (discover with work_item.type_list), and a title; add description, confidential, assignee_ids, milestone_id, label_ids, weight, health_status, color, dates, or linked_items only when requested."
	opts.Aliases = []string{"create work item", "open new work item", "add epic or task", individualTool}
	opts.RelatedActions = []string{actionWorkItemGet, actionWorkItemList, "work_item.type_list"}
	opts.IndividualTool.Description = "Create a new work item under a namespace. Returns: the created work item with id, iid, type, state, title, description, assignees, labels, linked items, and web URL. Experimental. See also: gitlab_get_work_item, gitlab_list_work_items, gitlab_list_work_item_types."
	opts.InputSchemaOverrides = workItemCreateEnumOverrides()
	return toolutil.NewCreateActionSpec(name, route, opts)
}

// workItemUpdateSpec builds the canonical update spec for a work item tool.
func workItemUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := workItemOptions(individualTool)
	opts.Usage = "Update an existing work item's fields, status, labels, or hierarchy. Provide full_path plus work_item_iid; set state_event to CLOSE or REOPEN to change state, and supply only the widget-supported fields you want to change."
	opts.Aliases = []string{"update work item", "edit work item fields", "close or reopen work item", individualTool}
	opts.RelatedActions = []string{actionWorkItemGet, "work_item.delete", actionWorkItemList}
	opts.IndividualTool.Description = "Update an existing work item by namespace path and IID. Returns: the updated work item with id, iid, type, state, status, title, description, assignees, labels, linked items, and web URL. Experimental. See also: gitlab_get_work_item, gitlab_delete_work_item, gitlab_list_work_items."
	opts.InputSchemaOverrides = workItemUpdateEnumOverrides()
	return toolutil.NewUpdateActionSpec(name, route, opts)
}

// workItemDeleteSpec builds the canonical destructive delete spec for a work item tool.
func workItemDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := workItemOptions(individualTool)
	opts.Usage = "Permanently delete a work item by namespace full_path plus work_item_iid. Deletion is irreversible; only the author or a Maintainer/Owner can delete, and some work item types are protected."
	opts.Aliases = []string{"delete work item", "remove work item permanently", "destroy work item", individualTool}
	opts.RelatedActions = []string{actionWorkItemGet, "work_item.update"}
	opts.IndividualTool.Description = "Delete a work item permanently by namespace path and IID. Returns: a success confirmation naming the work item and namespace. Experimental. See also: gitlab_get_work_item, gitlab_update_work_item."
	return toolutil.NewDeleteActionSpec(name, route, opts)
}

func workItemOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute workitems domain action.", Tags: []string{"issue", "work_item"},
		OpenWorld:      true,
		OwnerPackage:   "workitems",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// workItemListEnumOverrides constrains the state filter on the work item list
// action. The field maps to IssuableState in the GraphQL query; the three
// values below are the safe, stable subset documented for work items.
func workItemListEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("state", map[string]any{
			"enum": []any{"opened", "closed", "all"},
		}),
	}
}

// workItemCreateEnumOverrides constrains health_status and the nested
// linked_items.link_type on the work item create action.
// health_status values: GraphQL HealthStatus enum (client-go workitems.go:747).
// link_type values: SDK comment on CreateWorkItemOptionsLinkedItems.LinkType.
func workItemCreateEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("health_status", map[string]any{
			"enum": []any{"onTrack", "needsAttention", "atRisk"},
		}),
		toolutil.SchemaPropertyOverride("linked_items.link_type", map[string]any{
			"enum": []any{"BLOCKED_BY", "BLOCKS", "RELATED"},
		}),
	}
}

// workItemUpdateEnumOverrides constrains state_event, health_status, and
// status on the work item update action. state_event values come from
// WorkItemStateEvent SDK constants. health_status from the GraphQL HealthStatus
// enum. status from the mapStatusToID lookup table (WorkItemStatusID GIDs).
func workItemUpdateEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("state_event", map[string]any{
			"enum": []any{"CLOSE", "REOPEN"},
		}),
		toolutil.SchemaPropertyOverride("health_status", map[string]any{
			"enum": []any{"onTrack", "needsAttention", "atRisk"},
		}),
		toolutil.SchemaPropertyOverride("status", map[string]any{
			"enum": []any{"TODO", "IN_PROGRESS", "DONE", "WONT_DO", "DUPLICATE"},
		}),
	}
}
