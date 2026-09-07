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
// epic list action. state is accepted by both the REST epics endpoint and the
// Work Items API path. order_by is published in the REST endpoint's spelling
// on both, because the Work Items query takes the order_by and sort pair as a
// single WorkItemSort value that [workItemsSort] assembles; publishing that
// enum instead would leave the REST path with a vocabulary it refuses.
//
// The wildcard and searchable-field vocabularies are published because a
// GraphQL enum coercion failure arrives as HTTP 200 carrying a GraphQL error,
// so the status-keyed hint on the handler never fires for one and a model that
// guessed "me" or "title" is told nothing it can act on. health_status_filter
// is not the HealthStatus list the create and update actions publish: it adds
// the upper-case wildcards ANY and NONE beside the lower-camel values.
//
// The four new date filters take their format here rather than from
// toolutil's canonical map, which carries the created, updated, last_used,
// last_activity, started, finished, deployed and expires pairs and none of
// these four. All eight parse through
// toolutil.ParseOptionalTime, which accepts RFC 3339 and silently ignores
// anything else, so the schema has to say date-time for all eight or the four
// new ones read as free text beside four siblings that do not.
func epicListEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("state", map[string]any{
			"enum": []any{"opened", "closed", "all"},
		}),
		toolutil.SchemaPropertyOverride("order_by", map[string]any{
			"enum": []any{"created_at", "updated_at", "title"},
		}),
		toolutil.SchemaEnumOverride("assignee_wildcard_id", "ANY", "ME", "NONE"),
		toolutil.SchemaEnumOverride("milestone_wildcard_id", "ANY", "NONE", "STARTED", "UPCOMING"),
		toolutil.SchemaEnumOverride("weight_wildcard_id", "ANY", "NONE"),
		toolutil.SchemaEnumOverride("subscribed", "EXPLICITLY_SUBSCRIBED", "EXPLICITLY_UNSUBSCRIBED"),
		toolutil.SchemaEnumOverride("health_status_filter", "ANY", "NONE", "atRisk", "needsAttention", "onTrack"),
		// in is [IssuableSearchableField!], so the vocabulary belongs on the
		// array's items; a top-level enum here is a schema no array satisfies.
		toolutil.SchemaPropertyOverride("in", map[string]any{
			"items": map[string]any{"type": "string", "enum": []any{"TITLE", "DESCRIPTION"}},
		}),
		toolutil.SchemaFormatOverride("closed_after", "date-time"),
		toolutil.SchemaFormatOverride("closed_before", "date-time"),
		toolutil.SchemaFormatOverride("due_after", "date-time"),
		toolutil.SchemaFormatOverride("due_before", "date-time"),
	}
}

// epicCreateEnumOverrides constrains health_status and the nested
// linked_items.link_type on the epic create action.
// Values match the GraphQL HealthStatus enum: onTrack, needsAttention, atRisk
// (source: client-go work_items.go line 747 comment + test fixtures).
// link_type values are the WorkItemRelatedLinkType enum.
//
// created_at declares its format for the reason the list filters do: the
// handler parses it with toolutil.ParseOptionalTime, which takes RFC 3339 and
// leaves anything else unset, so a caller sending a bare date would get an
// epic stamped with now and no complaint.
func epicCreateEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("health_status", map[string]any{
			"enum": []any{"onTrack", "needsAttention", "atRisk"},
		}),
		toolutil.SchemaPropertyOverride("linked_items.link_type", map[string]any{
			"enum": []any{"BLOCKED_BY", "BLOCKS", "RELATED"},
		}),
		toolutil.SchemaFormatOverride("created_at", "date-time"),
	}
}

// epicUpdateEnumOverrides constrains state_event and health_status on the epic
// update action. state_event values come from WorkItemStateEvent SDK constants
// (CLOSE/REOPEN). health_status from the GraphQL HealthStatus enum.
//
// There is no status override because there is no status input: an Epic work
// item carries no STATUS widget and the mutation refuses the field. See
// [UpdateInput] for the widget list that says so.
func epicUpdateEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("state_event", map[string]any{
			"enum": []any{"CLOSE", "REOPEN"},
		}),
		toolutil.SchemaPropertyOverride("health_status", map[string]any{
			"enum": []any{"onTrack", "needsAttention", "atRisk"},
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
		opts.Usage = "List epics in a group with filtering (state, search, in, author, assignees, labels, milestone, weight, health status, subscription, iids, ids, parent_ids, my_reaction_emoji, created/updated/closed/due ranges, order_by, sort) and pagination in both directions. Use when the prompt asks for matching or recent epics in a known group."
		opts.Aliases = []string{individualTool, "list epics", "show group epics", "find epics"}
		opts.RelatedActions = []string{actionEpicGet, "epic.create", "group.get"}
		opts.IndividualTool.Description = "List epics in a group with filtering and pagination. Returns: matching epics with state, labels, author, assignees, dates, children, and the pagination block of whichever API answered. See also: gitlab_epic_get, gitlab_epic_create, gitlab_epic_get_links."
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
		opts.Usage = "Create a new epic in a group. Supply title plus optional description, labels, assignees, milestone, parent epic, linked epics, dates, color, weight, and health status. parent_id creates a sub-epic in the same call rather than create-then-update."
		opts.Aliases = []string{individualTool, "create epic", "add epic", "new epic", "create sub-epic"}
		opts.RelatedActions = []string{actionEpicGet, actionEpicList, actionEpicUpdate}
		opts.IndividualTool.Description = "Create a new epic in a group, optionally under a parent epic and already linked to others. Returns: the created epic with IID, state, labels, assignees, dates, and web URL. See also: gitlab_epic_get, gitlab_epic_list, gitlab_epic_update."
	case "gitlab_epic_update":
		opts.Usage = "Update an existing epic by full_path plus epic_iid. Supports close/reopen via state_event, reparenting via parent_id, milestone assignment, label add/remove, dates, weight and health status."
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
