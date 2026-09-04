package runnercontrollers

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerReadSpec("controller_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_list"),
		runnerControllerReadSpec("controller_get", toolutil.RouteAction(client, Get), "gitlab_runner_controller_get"),
		runnerControllerCreateSpec("controller_create", toolutil.RouteAction(client, Create), "gitlab_runner_controller_create"),
		runnerControllerUpdateSpec("controller_update", toolutil.RouteAction(client, Update), "gitlab_runner_controller_update"),
		runnerControllerDeleteSpec("controller_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_runner_controller_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner controller")
	return out, nil
}

func runnerControllerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute runnercontrollers domain action.", Tags: []string{"runner", "controller"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollers",
		Edition:        "ultimate",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := runnerControllerActionMeta[individualTool]; ok {
		options.Usage = meta.usage
		options.Aliases = append([]string{individualTool}, meta.aliases...)
		options.RelatedActions = append([]string(nil), meta.related...)
		options.IndividualTool.Description = meta.description
	}
	switch individualTool {
	case "gitlab_runner_controller_create", "gitlab_runner_controller_update":
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

// runnerControllerActionMeta maps each individual runner controller tool to its
// discovery metadata. Aliases use distinctive "runner controller" phrasing to
// avoid catalog collisions with the runner and runner-controller-scope/token
// packages, and every description follows the norm's "Returns: … See also: …"
// form so the model sees the result shape and adjacent tools. Runner controllers
// are an experimental, admin-only GitLab API for the agentic CI runner control
// plane.
var runnerControllerActionMeta = map[string]runnerControllerActionMetaEntry{
	"gitlab_runner_controller_list": {
		usage:       "List every registered runner controller on the instance (admin-only, experimental API). Use to discover controller IDs and their enabled/disabled/dry_run state before getting, updating, or deleting one. Supports offset and keyset pagination for large fleets.",
		aliases:     []string{"list runner controllers", "runner controller fleet", "registered runner controllers"},
		related:     []string{"gitlab_runner_controller_get", "gitlab_runner_controller_create", "gitlab_runner_controller_update", "gitlab_runner_controller_delete"},
		description: "List registered runner controllers (admin-only, experimental API) with offset or keyset pagination. Returns: controllers with id, description, state, created/updated timestamps, plus pagination metadata. See also: gitlab_runner_controller_get, gitlab_runner_controller_create, gitlab_runner_controller_update.",
	},
	"gitlab_runner_controller_get": {
		usage:       "Fetch one runner controller by numeric controller_id, including its live connection status (admin-only, experimental API). Use after gitlab_runner_controller_list to inspect a specific controller before updating or deleting it.",
		aliases:     []string{"get runner controller", "runner controller details", "runner controller connection status"},
		related:     []string{"gitlab_runner_controller_list", "gitlab_runner_controller_update", "gitlab_runner_controller_delete"},
		description: "Get one runner controller by controller_id (admin-only, experimental API). Returns: the controller with id, description, state, connected flag, and created/updated timestamps. See also: gitlab_runner_controller_list, gitlab_runner_controller_update, gitlab_runner_controller_delete.",
	},
	"gitlab_runner_controller_create": {
		usage:       "Register a new runner controller with an optional description and initial state (enabled, disabled, or dry_run). admin-only, experimental API. Use to onboard a controller into the agentic runner control plane before it connects.",
		aliases:     []string{"create runner controller", "register runner controller", "onboard runner controller"},
		related:     []string{"gitlab_runner_controller_get", "gitlab_runner_controller_update", "gitlab_runner_controller_list"},
		description: "Register a new runner controller (admin-only, experimental API) with optional description and state (enabled/disabled/dry_run). Returns: the created controller with id, description, state, and timestamps. See also: gitlab_runner_controller_get, gitlab_runner_controller_update, gitlab_runner_controller_list.",
	},
	"gitlab_runner_controller_update": {
		usage:       "Update an existing runner controller's description or state (enabled, disabled, or dry_run) by controller_id. admin-only, experimental API. Use to pause (disabled), resume (enabled), or stage (dry_run) a controller in the runner control plane.",
		aliases:     []string{"update runner controller", "edit runner controller", "set runner controller state"},
		related:     []string{"gitlab_runner_controller_get", "gitlab_runner_controller_list", "gitlab_runner_controller_delete"},
		description: "Update a runner controller's description or state (enabled/disabled/dry_run) by controller_id (admin-only, experimental API). Returns: the updated controller with id, description, state, and timestamps. See also: gitlab_runner_controller_get, gitlab_runner_controller_list, gitlab_runner_controller_delete.",
	},
	"gitlab_runner_controller_delete": {
		usage:       "Permanently remove a runner controller by controller_id (destructive, admin-only, experimental API). Use to decommission a controller from the runner control plane. Verify the controller_id with gitlab_runner_controller_list first.",
		aliases:     []string{"delete runner controller", "remove runner controller", "decommission runner controller"},
		related:     []string{"gitlab_runner_controller_get", "gitlab_runner_controller_list", "gitlab_runner_controller_update"},
		description: "Delete a runner controller by controller_id (destructive, admin-only, experimental API). Returns: a success confirmation. See also: gitlab_runner_controller_list, gitlab_runner_controller_get, gitlab_runner_controller_update.",
	},
}
