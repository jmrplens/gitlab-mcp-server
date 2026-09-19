package runnercontrollers

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Individual tool names, the one spelling the spec list, the schema overrides
// and the discovery metadata all read. Centralizing them lets the compiler
// catch a typo that would otherwise ship as a cross-link to nothing.
const (
	toolControllerList   = "gitlab_runner_controller_list"
	toolControllerGet    = "gitlab_runner_controller_get"
	toolControllerCreate = "gitlab_runner_controller_create"
	toolControllerUpdate = "gitlab_runner_controller_update"
	toolControllerDelete = "gitlab_runner_controller_delete"
)

// Canonical catalog IDs, the one form every surface resolves: a controller
// action is projected under the runner domain rather than the package name.
//
// They are what a RelatedActions list carries and what the Markdown hints
// name, and they are separate constants from the individual tool names above
// because the two are different names for the same action: the tool name is
// what the individual surface registers, and the ID is what
// gitlab_find_action publishes and gitlab_execute_action takes. The related
// lists used to be spelled with the tool names, which resolve through the
// alias each spec declares, so every cross-link worked when a model followed
// one and none of them could be looked up in a listing.
//
// The last two belong to the sibling packages whose actions this group also
// carries, and are named here because the hints in markdown.go cross-link to
// them.
const (
	actionControllerList      = "runner.controller_list"
	actionControllerGet       = "runner.controller_get"
	actionControllerCreate    = "runner.controller_create"
	actionControllerUpdate    = "runner.controller_update"
	actionControllerDelete    = "runner.controller_delete"
	actionControllerTokenList = "runner.controller_token_list"
	actionControllerScopeList = "runner.controller_scope_list"
)

// ActionSpecs returns canonical specs for runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerReadSpec("controller_list", toolutil.RouteAction(client, List), toolControllerList, listControllerMeta),
		runnerControllerReadSpec("controller_get", toolutil.RouteAction(client, Get), toolControllerGet, getControllerMeta),
		runnerControllerCreateSpec("controller_create", toolutil.RouteAction(client, Create), toolControllerCreate, createControllerMeta),
		runnerControllerUpdateSpec("controller_update", toolutil.RouteAction(client, Update), toolControllerUpdate, updateControllerMeta),
		runnerControllerDeleteSpec("controller_delete", toolutil.DestructiveAction(client, deleteOutput), toolControllerDelete, deleteControllerMeta),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner controller")
	return out, nil
}

func runnerControllerReadSpec(name string, route toolutil.ActionRoute, individualTool string, meta runnerControllerActionMetaEntry) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerOptions(individualTool, meta))
}

func runnerControllerCreateSpec(name string, route toolutil.ActionRoute, individualTool string, meta runnerControllerActionMetaEntry) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerOptions(individualTool, meta))
}

func runnerControllerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string, meta runnerControllerActionMetaEntry) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerControllerOptions(individualTool, meta))
}

func runnerControllerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string, meta runnerControllerActionMetaEntry) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerOptions(individualTool, meta))
}

// runnerControllerOptions builds one spec's options from its metadata. The
// metadata arrives as an argument rather than through a lookup keyed by tool
// name: a miss used to fall back to a generic usage line and no cross-links,
// which is a degradation nothing can see, and the compiler now refuses a spec
// that has none.
func runnerControllerOptions(individualTool string, meta runnerControllerActionMetaEntry) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        append([]string{individualTool}, meta.aliases...),
		Usage:          meta.usage,
		Tags:           []string{"runner", "controller"},
		RelatedActions: append([]string(nil), meta.related...),
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollers",
		Edition:        "ultimate",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	}
	if individualTool == toolControllerCreate || individualTool == toolControllerUpdate {
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("state", "disabled", "enabled", "dry_run"),
		}
	}
	return options
}

// runnerControllerActionMetaEntry is the discovery metadata for one runner
// controller action.
type runnerControllerActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// The discovery metadata of each runner controller action, named beside the
// spec it belongs to. Aliases use distinctive "runner controller" phrasing to
// avoid catalog collisions with the runner and runner-controller-scope/token
// packages, and every description follows the norm's "Returns: … See also: …"
// form so the model sees the result shape and adjacent tools. Runner controllers
// are an experimental, admin-only GitLab API for the agentic CI runner control
// plane.
var (
	listControllerMeta = runnerControllerActionMetaEntry{
		usage:       "List every registered runner controller on the instance (admin-only, experimental API). Use to discover controller IDs and their enabled/disabled/dry_run state before getting, updating, or deleting one. Supports offset and keyset pagination for large fleets.",
		aliases:     []string{"list runner controllers", "runner controller fleet", "registered runner controllers"},
		related:     []string{actionControllerGet, actionControllerCreate, actionControllerUpdate, actionControllerDelete},
		description: "List registered runner controllers (admin-only, experimental API) with offset or keyset pagination. Returns: controllers with id, description, state, created/updated timestamps, plus pagination metadata. See also: gitlab_runner_controller_get, gitlab_runner_controller_create, gitlab_runner_controller_update.",
	}
	getControllerMeta = runnerControllerActionMetaEntry{
		usage:       "Fetch one runner controller by numeric controller_id, including its live connection status (admin-only, experimental API). Use after gitlab_runner_controller_list to inspect a specific controller before updating or deleting it.",
		aliases:     []string{"get runner controller", "runner controller details", "runner controller connection status"},
		related:     []string{actionControllerList, actionControllerUpdate, actionControllerDelete},
		description: "Get one runner controller by controller_id (admin-only, experimental API). Returns: the controller with id, description, state, connected flag, and created/updated timestamps. See also: gitlab_runner_controller_list, gitlab_runner_controller_update, gitlab_runner_controller_delete.",
	}
	createControllerMeta = runnerControllerActionMetaEntry{
		usage:       "Register a new runner controller with an optional description and initial state (enabled, disabled, or dry_run). admin-only, experimental API. Use to onboard a controller into the agentic runner control plane before it connects.",
		aliases:     []string{"create runner controller", "register runner controller", "onboard runner controller"},
		related:     []string{actionControllerGet, actionControllerUpdate, actionControllerList},
		description: "Register a new runner controller (admin-only, experimental API) with optional description and state (enabled/disabled/dry_run). Returns: the created controller with id, description, state, and timestamps. See also: gitlab_runner_controller_get, gitlab_runner_controller_update, gitlab_runner_controller_list.",
	}
	updateControllerMeta = runnerControllerActionMetaEntry{
		usage:       "Update an existing runner controller's description or state (enabled, disabled, or dry_run) by controller_id. admin-only, experimental API. Use to pause (disabled), resume (enabled), or stage (dry_run) a controller in the runner control plane.",
		aliases:     []string{"update runner controller", "edit runner controller", "set runner controller state"},
		related:     []string{actionControllerGet, actionControllerList, actionControllerDelete},
		description: "Update a runner controller's description or state (enabled/disabled/dry_run) by controller_id (admin-only, experimental API). Returns: the updated controller with id, description, state, and timestamps. See also: gitlab_runner_controller_get, gitlab_runner_controller_list, gitlab_runner_controller_delete.",
	}
	deleteControllerMeta = runnerControllerActionMetaEntry{
		usage:       "Permanently remove a runner controller by controller_id (destructive, admin-only, experimental API). Use to decommission a controller from the runner control plane. Verify the controller_id with gitlab_runner_controller_list first.",
		aliases:     []string{"delete runner controller", "remove runner controller", "decommission runner controller"},
		related:     []string{actionControllerGet, actionControllerList, actionControllerUpdate},
		description: "Delete a runner controller by controller_id (destructive, admin-only, experimental API). Returns: a success confirmation. See also: gitlab_runner_controller_list, gitlab_runner_controller_get, gitlab_runner_controller_update.",
	}
)
