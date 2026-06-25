package runnercontrollerscopes

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical runner controller scope action names, reused for RelatedActions
// cross-links so the metadata stays in sync if a name ever changes.
const (
	actionScopeList           = "controller_scope_list"
	actionScopeAddInstance    = "controller_scope_add_instance"
	actionScopeRemoveInstance = "controller_scope_remove_instance"
	actionScopeAddRunner      = "controller_scope_add_runner"
	actionScopeRemoveRunner   = "controller_scope_remove_runner"
)

// ActionSpecs returns canonical specs for runner controller scope actions.
//
// These admin-only actions manage which runners a runner controller is
// permitted to operate: the instance-level scope grants the controller the
// whole shared instance runner fleet, while runner-level scopes pin the
// controller to specific instance runners. Each spec carries non-generic
// discovery metadata (action-specific Usage, runner-controller-scope-specific
// natural-language aliases, RelatedActions cross-links, and a
// "Returns: … See also: …" individual-tool description) per the 1:1 audit.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerScopeReadSpec(actionScopeList, toolutil.RouteAction(client, List), "gitlab_runner_controller_scope_list", listScopeMeta()),
		runnerControllerScopeCreateSpec(actionScopeAddInstance, toolutil.RouteAction(client, AddInstanceScope), "gitlab_runner_controller_scope_add_instance", addInstanceScopeMeta()),
		runnerControllerScopeDeleteSpec(actionScopeRemoveInstance, toolutil.DestructiveAction(client, removeInstanceScopeOutput), "gitlab_runner_controller_scope_remove_instance", removeInstanceScopeMeta()),
		runnerControllerScopeCreateSpec(actionScopeAddRunner, toolutil.RouteAction(client, AddRunnerScope), "gitlab_runner_controller_scope_add_runner", addRunnerScopeMeta()),
		runnerControllerScopeDeleteSpec(actionScopeRemoveRunner, toolutil.DestructiveAction(client, removeRunnerScopeOutput), "gitlab_runner_controller_scope_remove_runner", removeRunnerScopeMeta()),
	}
}

// scopeActionMeta carries the per-action discovery metadata that distinguishes
// one runner controller scope action from another in the find/meta surfaces.
type scopeActionMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// listScopeMeta describes the read action that lists every scope assigned to a
// runner controller (instance-level and runner-level).
func listScopeMeta() scopeActionMeta {
	return scopeActionMeta{
		usage:   "List the instance-level and runner-level scopes assigned to a runner controller to see which runners it may operate.",
		aliases: []string{"show runner controller scopes", "list runner controller scope assignments", "which runners is this runner controller scoped to"},
		related: []string{actionScopeAddInstance, actionScopeAddRunner, actionScopeRemoveInstance, actionScopeRemoveRunner},
		description: "List every scope assigned to a runner controller (admin only). " +
			"Returns: instance-level scopings with timestamps and runner-level scopings with runner IDs and timestamps. " +
			"See also: gitlab_runner_controller_scope_add_instance, gitlab_runner_controller_scope_add_runner, gitlab_runner_controller_scope_remove_runner.",
	}
}

// addInstanceScopeMeta describes the action that grants a controller the whole
// shared instance runner fleet.
func addInstanceScopeMeta() scopeActionMeta {
	return scopeActionMeta{
		usage:   "Grant a runner controller the instance-level scope so it may operate the entire shared instance runner fleet.",
		aliases: []string{"add instance scope to runner controller", "grant runner controller instance-wide scope", "scope runner controller to all instance runners"},
		related: []string{actionScopeRemoveInstance, actionScopeList, actionScopeAddRunner},
		description: "Grant a runner controller the instance-level scope (admin only). " +
			"Returns: the created instance-level scoping with created/updated timestamps. " +
			"See also: gitlab_runner_controller_scope_remove_instance, gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_add_runner.",
	}
}

// removeInstanceScopeMeta describes the destructive action that revokes a
// controller's instance-wide scope.
func removeInstanceScopeMeta() scopeActionMeta {
	return scopeActionMeta{
		usage:   "Revoke a runner controller's instance-level scope so it can no longer operate the shared instance runner fleet.",
		aliases: []string{"remove instance scope from runner controller", "revoke runner controller instance-wide scope", "unscope runner controller from all instance runners"},
		related: []string{actionScopeAddInstance, actionScopeList, actionScopeRemoveRunner},
		description: "Revoke a runner controller's instance-level scope (admin only, destructive). " +
			"Returns: a success confirmation naming the removed instance-level scope. " +
			"See also: gitlab_runner_controller_scope_add_instance, gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_remove_runner.",
	}
}

// addRunnerScopeMeta describes the action that pins a controller to one specific
// instance runner.
func addRunnerScopeMeta() scopeActionMeta {
	return scopeActionMeta{
		usage:   "Scope a runner controller to one specific instance runner by runner ID so it may operate only that runner.",
		aliases: []string{"add runner scope to runner controller", "scope runner controller to a specific runner", "grant runner controller access to one instance runner"},
		related: []string{actionScopeRemoveRunner, actionScopeList, actionScopeAddInstance},
		description: "Scope a runner controller to a specific instance runner by runner ID (admin only). " +
			"Returns: the created runner-level scoping with the runner ID and created/updated timestamps. " +
			"See also: gitlab_runner_controller_scope_remove_runner, gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_add_instance.",
	}
}

// removeRunnerScopeMeta describes the destructive action that unpins a
// controller from one specific instance runner.
func removeRunnerScopeMeta() scopeActionMeta {
	return scopeActionMeta{
		usage:   "Remove a specific runner from a runner controller's scope by runner ID so the controller can no longer operate that runner.",
		aliases: []string{"remove runner scope from runner controller", "unscope runner controller from a specific runner", "revoke runner controller access to one instance runner"},
		related: []string{actionScopeAddRunner, actionScopeList, actionScopeRemoveInstance},
		description: "Remove a specific runner from a runner controller's scope by runner ID (admin only, destructive). " +
			"Returns: a success confirmation naming the removed runner scope. " +
			"See also: gitlab_runner_controller_scope_add_runner, gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_remove_instance.",
	}
}

func removeInstanceScopeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInstanceScopeInput) (toolutil.DeleteOutput, error) {
	if err := RemoveInstanceScope(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("instance-level scope")
	return out, nil
}

func removeRunnerScopeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveRunnerScopeInput) (toolutil.DeleteOutput, error) {
	if err := RemoveRunnerScope(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner scope")
	return out, nil
}

func runnerControllerScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string, meta scopeActionMeta) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerScopeOptions(individualTool, meta))
}

func runnerControllerScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string, meta scopeActionMeta) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerScopeOptions(individualTool, meta))
}

func runnerControllerScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string, meta scopeActionMeta) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerScopeOptions(individualTool, meta))
}

func runnerControllerScopeOptions(individualTool string, meta scopeActionMeta) toolutil.ActionSpecOptions {
	aliases := append([]string{individualTool}, meta.aliases...)
	return toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Usage:          meta.usage,
		RelatedActions: append([]string(nil), meta.related...),
		Tags:           []string{"runner", "controller", "scope"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollerscopes",
		Edition:        "ultimate",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: meta.description},
	}
}
