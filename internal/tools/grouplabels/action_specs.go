package grouplabels

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupLabelGet  = "group_label.get"
	actionGroupLabelList = "group_label.list"
)

// ActionSpecs returns canonical specs for group label actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupLabelReadSpec("group_label_list", toolutil.RouteAction(client, List), "gitlab_group_label_list"),
		groupLabelReadSpec("group_label_get", toolutil.RouteAction(client, Get), "gitlab_group_label_get").
			WithEmbeddedResource("gitlab://group/{group_id}/label/{label_id}"),
		groupLabelCreateSpec("group_label_create", toolutil.RouteAction(client, Create), "gitlab_group_label_create"),
		groupLabelUpdateSpec("group_label_update", toolutil.RouteAction(client, Update), "gitlab_group_label_update"),
		groupLabelDeleteSpec("group_label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_label_delete"),
		groupLabelUpdateSpec("group_label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_group_label_subscribe"),
		groupLabelUpdateSpec("group_label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_group_label_unsubscribe"),
	}
}

func groupLabelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupLabelOptions(individualTool)
	decorateGroupLabelMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupLabelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupLabelOptions(individualTool)
	decorateGroupLabelMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func groupLabelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupLabelOptions(individualTool)
	decorateGroupLabelMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func groupLabelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupLabelOptions(individualTool)
	decorateGroupLabelMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func groupLabelOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute grouplabels domain action.", Tags: []string{"group", "label"},
		RelatedActions: []string{"group.get", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "grouplabels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupLabelMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for each group-label action so no tool inherits the generic placeholder
// metadata from groupLabelOptions (1:1 audit R-META).
func decorateGroupLabelMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupLabelActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// groupLabelActionMetaEntry is the discovery metadata for one group-label action.
type groupLabelActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupLabelActionMeta maps each individual group-label tool to its discovery metadata.
var groupLabelActionMeta = map[string]groupLabelActionMetaEntry{
	"gitlab_group_label_list": {
		usage:       "List labels defined in a group. Use filters such as search, with_counts, include_ancestor_groups, include_descendant_groups, only_group_labels, archived, order_by, sort, and pagination when the prompt asks for a group's labels.",
		aliases:     []string{"list group labels", "show group labels", "find labels in group"},
		related:     []string{actionGroupLabelGet, "group_label.create", "group.get"},
		description: "List labels in a group with filtering and pagination. Returns: matching labels with color, text color, description, issue and merge request counts, priority, subscription state, and pagination metadata. See also: gitlab_group_label_get, gitlab_group_label_create, gitlab_group_get.",
	},
	"gitlab_group_label_get": {
		usage:       "Get one group label by its numeric ID or name. Use after a list result or when the prompt names a concrete group label.",
		aliases:     []string{"get group label", "show group label details", "fetch group label"},
		related:     []string{actionGroupLabelList, "group_label.update", "group_label.delete"},
		description: "Get a single group label by ID or name. Returns: the label with color, text color, description, issue and merge request counts, priority, and subscription state. See also: gitlab_group_label_list, gitlab_group_label_update, gitlab_group_label_delete.",
	},
	"gitlab_group_label_create": {
		usage:       "Create a new label in a group. Provide group_id, a unique name, and a hex color. Add description, priority, or archived only when requested.",
		aliases:     []string{"create group label", "add group label", "new group label"},
		related:     []string{actionGroupLabelList, actionGroupLabelGet, "group_label.update"},
		description: "Create a new group label. Returns: the created label with id, name, color, text color, description, counts, priority, and subscription state. See also: gitlab_group_label_list, gitlab_group_label_get, gitlab_group_label_update.",
	},
	"gitlab_group_label_update": {
		usage:       "Update an existing group label. Provide at least one of new_name, color, description, priority, or archived to change the label.",
		aliases:     []string{"update group label", "edit group label", "rename group label"},
		related:     []string{actionGroupLabelGet, actionGroupLabelList, "group_label.delete"},
		description: "Update a group label. Returns: the updated label with id, name, color, text color, description, counts, priority, and subscription state. See also: gitlab_group_label_get, gitlab_group_label_list, gitlab_group_label_delete.",
	},
	"gitlab_group_label_delete": {
		usage:       "Permanently delete a group label by ID or name. Destructive. Confirm group_id and label_id before calling.",
		aliases:     []string{"delete group label", "remove group label", "drop group label", "destroy group label"},
		related:     []string{actionGroupLabelGet, actionGroupLabelList},
		description: "Delete a group label permanently. Returns: a success confirmation. See also: gitlab_group_label_get, gitlab_group_label_list.",
	},
	"gitlab_group_label_subscribe": {
		usage:       "Subscribe the authenticated user to a group label to receive notifications for issues and merge requests carrying it.",
		aliases:     []string{"subscribe to group label", "follow group label", "watch group label", "get notifications for group label"},
		related:     []string{actionGroupLabelGet, "group_label.unsubscribe"},
		description: "Subscribe to a group label's notifications. Returns: the label with the updated subscription state. See also: gitlab_group_label_get, gitlab_group_label_unsubscribe.",
	},
	"gitlab_group_label_unsubscribe": {
		usage:       "Unsubscribe the authenticated user from a group label so notifications for it stop.",
		aliases:     []string{"unsubscribe from group label", "unfollow group label", "unwatch group label", "stop group label notifications"},
		related:     []string{actionGroupLabelGet, "group_label.subscribe"},
		description: "Unsubscribe from a group label's notifications. Returns: a success confirmation. See also: gitlab_group_label_get, gitlab_group_label_subscribe.",
	},
}
