package epics

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionEpicList   = "epic.list"
	actionEpicGet    = "epic.get"
	actionEpicUpdate = "epic.update"
)

// ActionSpecs returns canonical specs for group epic actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_epic_list — list epics in a group.
		epicReadSpec("epic_list", toolutil.RouteAction(client, List), "gitlab_epic_list"),
		// gitlab_epic_get — fetch a single epic by IID.
		epicReadSpec("epic_get", toolutil.RouteAction(client, Get), "gitlab_epic_get"),
		// gitlab_epic_get_links — list child epics linked to a parent epic.
		epicReadSpec("epic_get_links", toolutil.RouteAction(client, GetLinks), "gitlab_epic_get_links"),
		// gitlab_epic_create — create a new epic in a group.
		epicCreateSpec("epic_create", toolutil.RouteAction(client, Create), "gitlab_epic_create"),
		// gitlab_epic_update — update an existing epic.
		epicUpdateSpec("epic_update", toolutil.RouteAction(client, Update), "gitlab_epic_update"),
		// gitlab_epic_delete — delete an epic and its associations.
		epicDeleteSpec("epic_delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_epic_delete"),
	}
}

// DeleteOutput deletes an epic and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted epic &%d from group %s.", input.IID, input.FullPath),
	}, nil
}

func epicReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := epicOptions(individualTool)
	if individualTool == "gitlab_epic_list" {
		opts.InputSchemaOverrides = epicListEnumOverrides()
	}
	return toolutil.NewReadActionSpec(name, route, opts)
}

func epicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := epicOptions(individualTool)
	opts.InputSchemaOverrides = epicCreateEnumOverrides()
	return toolutil.NewCreateActionSpec(name, route, opts)
}

func epicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := epicOptions(individualTool)
	opts.InputSchemaOverrides = epicUpdateEnumOverrides()
	return toolutil.NewUpdateActionSpec(name, route, opts)
}

func epicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicOptions(individualTool))
}

// epicListEnumOverrides constrains the fixed-vocabulary filter fields on the
// epic list action. state and order_by are accepted by both the REST epics
// endpoint and the Work Items API path (IssuableState / WorkItemSort).
func epicListEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("state", map[string]any{
			"enum": []any{"opened", "closed", "all"},
		}),
		toolutil.SchemaPropertyOverride("order_by", map[string]any{
			"enum": []any{"created_at", "updated_at", "title"},
		}),
	}
}

// epicCreateEnumOverrides constrains health_status on the epic create action.
// Values match the GraphQL HealthStatus enum: onTrack, needsAttention, atRisk
// (source: client-go workitems.go line 747 comment + test fixtures).
func epicCreateEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("health_status", map[string]any{
			"enum": []any{"onTrack", "needsAttention", "atRisk"},
		}),
	}
}

// epicUpdateEnumOverrides constrains state_event, health_status, and status on
// the epic update action. state_event values come from WorkItemStateEvent SDK
// constants (CLOSE/REOPEN). health_status from the GraphQL HealthStatus enum.
// status from the mapStatusToID lookup table (WorkItemStatusID GIDs).
func epicUpdateEnumOverrides() []toolutil.InputSchemaOverride {
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

func epicOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute epics domain action.", Tags: []string{"group", "epic"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateEpicMeta(&opts, individualTool)
	return opts
}

// decorateEpicMeta fills the per-tool aliases, related actions, usage, and the
// "Returns: … See also: …" individual-tool description for each epic tool
// (R-META, 1:1 audit). It mirrors the discovery-metadata form used by the
// issues domain.
func decorateEpicMeta(opts *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_epic_list":
		opts.Usage = "List epics in a group with filtering (state, search, author, labels, my_reaction_emoji, created_after/before, order_by, sort) and pagination. Use when the prompt asks for matching or recent epics in a known group."
		opts.Aliases = []string{individualTool, "list epics", "show group epics", "find epics"}
		opts.RelatedActions = []string{actionEpicGet, "epic.create", "group.get"}
		opts.IndividualTool.Description = "List epics in a group with filtering and pagination. Returns: matching epics with state, labels, author, assignees, dates, and pagination cursors. See also: gitlab_epic_get, gitlab_epic_create, gitlab_epic_get_links."
	case "gitlab_epic_get":
		opts.Usage = "Get one exact epic by full_path plus epic_iid. Use this after list results or when the prompt already names a concrete epic number."
		opts.Aliases = []string{individualTool, "get epic", "show epic details", "fetch epic"}
		opts.RelatedActions = []string{actionEpicList, actionEpicUpdate, "epic.delete", "epic.get_links"}
		opts.IndividualTool.Description = "Get a single epic from a group by epic IID. Returns: epic metadata, state, labels, author, assignees, start/due dates, health status, weight, parent, and linked items. See also: gitlab_epic_list, gitlab_epic_update, gitlab_epic_get_links, gitlab_epic_delete."
	case "gitlab_epic_get_links":
		opts.Usage = "List the child epics of a parent epic by full_path plus epic_iid. Use when the prompt asks for sub-epics or the epic hierarchy below a known epic."
		opts.Aliases = []string{individualTool, "list child epics", "show sub-epics", "epic children"}
		opts.RelatedActions = []string{actionEpicGet, actionEpicList}
		opts.IndividualTool.Description = "List child epics linked to a parent epic. Returns: child epics with state, labels, author, dates, and parent reference. See also: gitlab_epic_get, gitlab_epic_list, gitlab_epic_update."
	case "gitlab_epic_create":
		opts.Usage = "Create a new epic in a group. Supply title plus optional description, labels, assignees, dates, color, weight, and health status."
		opts.Aliases = []string{individualTool, "create epic", "add epic", "new epic"}
		opts.RelatedActions = []string{actionEpicGet, actionEpicList, actionEpicUpdate}
		opts.IndividualTool.Description = "Create a new epic in a group. Returns: the created epic with IID, state, labels, assignees, dates, and web URL. See also: gitlab_epic_get, gitlab_epic_list, gitlab_epic_update."
	case "gitlab_epic_update":
		opts.Usage = "Update an existing epic by full_path plus epic_iid. Supports close/reopen via state_event, reparenting via parent_id, label add/remove, dates, weight, health status, and status."
		opts.Aliases = []string{individualTool, "update epic", "edit epic", "close epic", "reopen epic"}
		opts.RelatedActions = []string{actionEpicGet, actionEpicList, "epic.delete"}
		opts.IndividualTool.Description = "Update an existing epic. Supports close/reopen via state_event. Returns: the updated epic with its current state, labels, assignees, dates, and web URL. See also: gitlab_epic_get, gitlab_epic_list, gitlab_epic_delete."
	case "gitlab_epic_delete":
		opts.Usage = "Permanently delete an epic by full_path plus epic_iid. Destructive. Requires confirmation and Owner role at the group level."
		opts.Aliases = []string{individualTool, "delete epic", "remove epic", "destroy epic", "drop epic"}
		opts.RelatedActions = []string{actionEpicGet, actionEpicList, actionEpicUpdate}
		opts.IndividualTool.Description = "Permanently delete an epic from a group. Returns: a success confirmation naming the epic and group. See also: gitlab_epic_get, gitlab_epic_list, gitlab_epic_update."
	}
}
