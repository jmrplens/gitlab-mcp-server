package runners

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Individual tool names that appear in multiple places (ActionSpecs
// registrations, runnerActionMeta map keys, and related-action slices).
// Centralizing them here lets the compiler catch typos at build time.
const (
	toolRunnerListProject      = "gitlab_runner_list_project"
	toolRunnerEnableProject    = "gitlab_runner_enable_project"
	toolRunnerDisableProject   = "gitlab_runner_disable_project"
	toolRunnerDeleteRegistered = "gitlab_runner_delete_registered"
	toolRunnerDeleteByToken    = "gitlab_runner_delete_by_token"
	toolRunnerResetInstanceReg = "gitlab_runner_reset_instance_reg_token"
	toolRunnerResetGroupReg    = "gitlab_runner_reset_group_reg_token"
	toolRunnerResetProjectReg  = "gitlab_runner_reset_project_reg_token"
	toolRunnerListManagers     = "gitlab_runner_list_managers"
)

// ActionSpecs returns canonical specs for runner and runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		runnerReadSpec("list", toolutil.RouteAction(client, List), "gitlab_runner_list"),
		runnerReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_runner_list_all"),
		runnerReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_runner_get"),
		runnerUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_runner_update"),
		runnerUpdateSpec("remove", toolutil.DestructiveAction(client, removeOutput), "gitlab_runner_remove"),
		runnerReadSpec("jobs", toolutil.RouteAction(client, ListJobs), "gitlab_runner_jobs"),
		runnerReadSpec("list_project", toolutil.RouteAction(client, ListProject), toolRunnerListProject),
		runnerCreateSpec("enable_project", toolutil.RouteAction(client, EnableProject), toolRunnerEnableProject),
		runnerUpdateSpec("disable_project", toolutil.DestructiveAction(client, disableProjectOutput), toolRunnerDisableProject),
		runnerReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_runner_list_group"),
		runnerCreateSpec("register", toolutil.RouteAction(client, Register), "gitlab_runner_register"),
		runnerUpdateSpec("delete_registered", toolutil.DestructiveAction(client, deleteByIDOutput), toolRunnerDeleteRegistered),
		runnerUpdateSpec("delete_by_token", toolutil.DestructiveAction(client, deleteByTokenOutput), toolRunnerDeleteByToken),
		runnerReadSpec("verify", toolutil.RouteAction(client, verifyOutput), "gitlab_runner_verify"),
		runnerUpdateSpec("reset_token", toolutil.RouteAction(client, ResetAuthToken), "gitlab_runner_reset_token"),
		runnerUpdateSpec("reset_instance_reg_token", toolutil.RouteAction(client, ResetInstanceRegToken), toolRunnerResetInstanceReg),
		runnerUpdateSpec("reset_group_reg_token", toolutil.RouteAction(client, ResetGroupRegToken), toolRunnerResetGroupReg),
		runnerUpdateSpec("reset_project_reg_token", toolutil.RouteAction(client, ResetProjectRegToken), toolRunnerResetProjectReg),
		runnerReadSpec("list_managers", toolutil.RouteAction(client, ListManagers), toolRunnerListManagers),
	}
	specs = append(specs, runnercontrollers.ActionSpecs(client)...)
	specs = append(specs, runnercontrollerscopes.ActionSpecs(client)...)
	specs = append(specs, runnercontrollertokens.ActionSpecs(client)...)
	return specs
}

func removeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (toolutil.DeleteOutput, error) {
	if err := Remove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner")
	return out, nil
}

func disableProjectOutput(ctx context.Context, client *gitlabclient.Client, input DisableProjectInput) (toolutil.DeleteOutput, error) {
	if err := DisableProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project runner assignment")
	return out, nil
}

func deleteByIDOutput(ctx context.Context, client *gitlabclient.Client, input DeleteByIDInput) (toolutil.DeleteOutput, error) {
	if err := DeleteByID(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("registered runner")
	return out, nil
}

func deleteByTokenOutput(ctx context.Context, client *gitlabclient.Client, input DeleteByTokenInput) (toolutil.DeleteOutput, error) {
	if err := DeleteByToken(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("registered runner")
	return out, nil
}

func verifyOutput(ctx context.Context, client *gitlabclient.Client, input VerifyInput) (toolutil.DeleteOutput, error) {
	if err := Verify(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Runner token is valid."}, nil
}

func runnerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Use to execute runners domain action."
	options := toolutil.ActionSpecOptions{
		Aliases:              []string{individualTool},
		Usage:                usage,
		Tags:                 []string{"runner"},
		ParameterGuidance:    runnerParameterGuidance(actionName),
		InputSchemaOverrides: runnerListInputSchemaOverrides(actionName),
		OpenWorld:            true,
		OwnerPackage:         "runners",
		IndividualTool:       toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateRunnerMeta(&options, individualTool)
	return options
}

// runnerListInputSchemaOverrides returns input-schema enum overrides for the
// runner list actions (list, list_all, list_project, list_group). Returns nil
// for all other actions.
func runnerListInputSchemaOverrides(actionName string) []toolutil.InputSchemaOverride {
	switch actionName {
	case "list", "list_all", "list_project", "list_group":
	default:
		return nil
	}
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("type", map[string]any{
			"enum":        []any{"instance_type", "group_type", "project_type"},
			"description": "Runner type filter: instance_type (instance-level shared runners), group_type (group runners), or project_type (project-specific runners).",
		}),
		toolutil.SchemaPropertyOverride("status", map[string]any{
			"enum":        []any{"online", "offline", "stale", "never_contacted"},
			"description": "Runner status filter: online, offline, stale, or never_contacted.",
		}),
	}
}

// decorateRunnerMeta fills the discovery metadata gap (R-META) for runner
// actions: natural-language Aliases, RelatedActions cross-links, and the
// "Returns: … See also: …" individual-tool description the model sees. It is a
// no-op for any tool not present in runnerActionMeta.
func decorateRunnerMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := runnerActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append(append([]string(nil), individualTool), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// runnerActionMetaEntry is the discovery metadata for one runner action.
type runnerActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// resetScopedRegTokenEntry builds the runnerActionMetaEntry for a scoped runner
// registration-token reset action. The group and project variants share the same
// sentence structure; only the scope name, the scope-ID parameter name, and the
// two sibling related-action IDs differ.
func resetScopedRegTokenEntry(scope, scopeParam, related1, related2 string) runnerActionMetaEntry {
	return runnerActionMetaEntry{
		usage: fmt.Sprintf(
			"Reset a %s's runner registration token by %s (deprecated registration flow). "+
				"Use only for legacy %s-scoped runner registration; modern flows use runner.register with a created token.",
			scope, scopeParam, scope),
		aliases: []string{
			"reset " + scope + " registration token",
			"rotate " + scope + " runner registration token",
			"renew " + scope + " registration token",
			"regenerate " + scope + " runner token",
		},
		related: []string{related1, related2},
		description: fmt.Sprintf(
			"Reset a %s's runner registration token by %s (deprecated). "+
				"Returns: the new registration token and its expiry. See also: %s, %s.",
			scope, scopeParam, related1, related2),
	}
}

// runnerActionMeta maps each individual runner tool to its discovery metadata.
// Every entry's description follows the norm's "Returns: … See also: …" form so
// the model sees the result shape and adjacent tools.
var runnerActionMeta = map[string]runnerActionMetaEntry{
	// Owned-runner inspection actions.
	"gitlab_runner_list": {
		usage:       "List runners owned by the authenticated user. Use type, status, tag_list, and scope filters to narrow results when the prompt asks for the caller's own runners; prefer runner.list_all for an instance-wide admin view.",
		aliases:     []string{"list owned runners", "my runners", "show my runners", "browse my runners"},
		related:     []string{"gitlab_runner_list_all", "gitlab_runner_get", toolRunnerListProject, "gitlab_runner_list_group"},
		description: "List runners owned by the authenticated user, with type/status/tag/scope filters and pagination. Returns: runners with id, name, type, status, paused/shared/online flags, plus pagination metadata. See also: gitlab_runner_get, gitlab_runner_list_all, gitlab_runner_list_project.",
	},
	"gitlab_runner_list_all": {
		usage:       "List every runner registered on the instance (requires an admin token). Use when the prompt asks for a fleet-wide or instance-level inventory rather than the caller's own runners.",
		aliases:     []string{"list all runners", "instance runners", "show all runners", "browse instance runners"},
		related:     []string{"gitlab_runner_list", "gitlab_runner_get"},
		description: "List every runner on the instance (admin token required), with type/status/tag/scope filters and pagination. Returns: runners with id, name, type, status, flags, plus pagination metadata. See also: gitlab_runner_list, gitlab_runner_get.",
	},
	"gitlab_runner_get": {
		usage:       "Get the full configuration of one runner by its numeric runner_id. Use after a runner.list result or when the prompt names a concrete runner to inspect tags, lock state, access level, and timeout.",
		aliases:     []string{"get runner", "runner details", "show runner", "fetch runner"},
		related:     []string{"gitlab_runner_list", "gitlab_runner_update", "gitlab_runner_jobs", toolRunnerListManagers},
		description: "Get full configuration for one runner by numeric runner_id. Returns: runner details including tags, locked, access level, maximum timeout, contact time, and associated groups/projects. See also: gitlab_runner_list, gitlab_runner_update, gitlab_runner_jobs.",
	},
	// Runner lifecycle actions.
	"gitlab_runner_update": {
		usage:       "Update a runner's configuration by runner_id. Set description, paused, tag_list, locked, access_level, or maximum_timeout; pass only the fields the prompt asks to change (e.g. paused to pause or resume a runner).",
		aliases:     []string{"update runner", "edit runner", "pause runner", "resume runner", "modify runner"},
		related:     []string{"gitlab_runner_get", "gitlab_runner_remove"},
		description: "Update a runner's configuration (description, pause state, tags, locked, access level, timeout) by runner_id. Returns: the updated runner details. See also: gitlab_runner_get, gitlab_runner_remove.",
	},
	"gitlab_runner_remove": {
		usage:       "Remove (unregister) a runner by its numeric runner_id. Use runner.list or runner.list_project to obtain the runner_id first. Destructive: the runner is permanently deleted.",
		aliases:     []string{"remove runner", "unregister runner", "delete runner", "drop runner"},
		related:     []string{"gitlab_runner_get", toolRunnerDisableProject, toolRunnerDeleteRegistered},
		description: "Remove (unregister) a runner by numeric runner_id. Returns: a success confirmation. See also: gitlab_runner_disable_project, gitlab_runner_delete_registered, gitlab_runner_get.",
	},
	"gitlab_runner_jobs": {
		usage:       "List the CI jobs processed by a runner, identified by runner_id. Use status, order_by, and sort filters when the prompt asks which jobs a specific runner has executed.",
		aliases:     []string{"runner jobs", "jobs processed by runner", "list jobs for runner", "show runner jobs"},
		related:     []string{"gitlab_runner_get", "gitlab_runner_list"},
		description: "List jobs processed by a runner, with status/order/sort filters and pagination. Returns: jobs with id, name, status, stage, ref, and duration, plus pagination metadata. See also: gitlab_runner_get, gitlab_job_get.",
	},
	// Project and group assignment actions.
	toolRunnerListProject: {
		usage:       "List the runners assigned to a project, identified by project_id. Use when the prompt scopes runners to one project; use runner.enable_project or runner.disable_project to change those assignments.",
		aliases:     []string{"list project runners", "runners for project", "show project runners", "browse project runners"},
		related:     []string{toolRunnerEnableProject, toolRunnerDisableProject, "gitlab_runner_list"},
		description: "List runners assigned to a project, with type/status/tag/scope filters and pagination. Returns: runners with id, name, type, status, flags, plus pagination metadata. See also: gitlab_runner_enable_project, gitlab_runner_disable_project, gitlab_runner_list.",
	},
	toolRunnerEnableProject: {
		usage:       "Assign an existing runner to a project. Provide project_id for the target project and runner_id for the runner to enable; use runner.list to find an available runner_id first.",
		aliases:     []string{"enable project runner", "assign runner to project", "attach runner to project"},
		related:     []string{toolRunnerDisableProject, toolRunnerListProject},
		description: "Assign an existing runner to a project by project_id and runner_id. Returns: the enabled runner. See also: gitlab_runner_disable_project, gitlab_runner_list_project.",
	},
	toolRunnerDisableProject: {
		usage:       "Remove a runner assignment from a project. Provide project_id for the project and runner_id for the runner to unassign; this detaches the runner without unregistering it (use runner.remove to delete the runner itself).",
		aliases:     []string{"disable project runner", "unassign runner from project", "detach runner from project"},
		related:     []string{toolRunnerEnableProject, toolRunnerListProject},
		description: "Remove a runner assignment from a project by project_id and runner_id. Returns: a success confirmation. See also: gitlab_runner_enable_project, gitlab_runner_list_project.",
	},
	"gitlab_runner_list_group": {
		usage:       "List the runners available to a group, identified by group_id, including group-specific and shared runners. Use type and status filters when the prompt scopes runners to a group or subgroup.",
		aliases:     []string{"list group runners", "runners for group", "show group runners", "browse group runners"},
		related:     []string{"gitlab_runner_list", toolRunnerListProject},
		description: "List runners available in a group (specific and shared), with type/status/tag filters and pagination. Returns: runners with id, name, type, status, flags, plus pagination metadata. See also: gitlab_runner_list, gitlab_runner_list_project.",
	},
	// Runner registration actions.
	"gitlab_runner_register": {
		usage:       "Register a new runner using a registration token. Provide the token plus optional description, tag_list, locked, run_untagged, and access_level; the response includes the runner's authentication token needed to configure the runner agent.",
		aliases:     []string{"register runner", "create runner", "new runner", "provision runner"},
		related:     []string{"gitlab_runner_verify", "gitlab_runner_list"},
		description: "Register a new runner with a registration token, optional info hashmap, tags, and configuration. Returns: the created runner including its authentication token. See also: gitlab_runner_verify, gitlab_runner_list.",
	},
	toolRunnerDeleteRegistered: {
		usage:       "Delete a registered runner by its numeric runner_id. Use this runner-registration-API variant when you have the runner_id; prefer runner.delete_by_token when only the authentication token is known.",
		aliases:     []string{"delete registered runner", "delete runner by id", "remove registered runner", "unregister runner by id"},
		related:     []string{toolRunnerDeleteByToken, "gitlab_runner_remove"},
		description: "Delete a registered runner by its numeric runner_id. Returns: a success confirmation. See also: gitlab_runner_delete_by_token, gitlab_runner_remove.",
	},
	toolRunnerDeleteByToken: {
		usage:       "Delete a registered runner using its authentication token instead of a runner_id. Use when the runner agent's token is available but the numeric runner_id is not; prefer runner.delete_registered when you have the runner_id.",
		aliases:     []string{"delete runner by token", "unregister runner by token", "remove runner by token", "drop runner by token"},
		related:     []string{toolRunnerDeleteRegistered, "gitlab_runner_remove"},
		description: "Delete a registered runner by its authentication token. Returns: a success confirmation. See also: gitlab_runner_delete_registered, gitlab_runner_remove.",
	},
	"gitlab_runner_verify": {
		usage:       "Verify that a runner authentication token is valid without registering or modifying anything. Use to check a token before configuring a runner agent or after runner.reset_token.",
		aliases:     []string{"verify runner token", "validate runner token", "check runner token", "test runner token"},
		related:     []string{"gitlab_runner_register", "gitlab_runner_reset_token"},
		description: "Verify that a runner authentication token is valid. Returns: a success confirmation when the token authenticates. See also: gitlab_runner_register, gitlab_runner_reset_token.",
	},
	"gitlab_runner_reset_token": {
		usage:       "Reset a runner's authentication token by runner_id, invalidating the previous token. Use when a runner token is compromised or must be rotated; the response returns the new token.",
		aliases:     []string{"reset runner token", "rotate runner authentication token", "renew runner token", "regenerate runner token"},
		related:     []string{"gitlab_runner_verify", "gitlab_runner_get"},
		description: "Reset a runner's authentication token by runner_id. Returns: the new token and its expiry. See also: gitlab_runner_verify, gitlab_runner_get.",
	},
	// Deprecated registration-token reset actions.
	toolRunnerResetInstanceReg: {
		usage:       "Reset the instance-wide runner registration token (admin only, deprecated registration flow). Use only for legacy instance-level runner registration; modern flows use runner.register with a created token.",
		aliases:     []string{"reset instance registration token", "rotate instance runner registration token", "renew instance registration token", "regenerate instance runner token"},
		related:     []string{toolRunnerResetGroupReg, toolRunnerResetProjectReg},
		description: "Reset the instance-level runner registration token (deprecated, admin only). Returns: the new registration token and its expiry. See also: gitlab_runner_reset_group_reg_token, gitlab_runner_reset_project_reg_token.",
	},
	toolRunnerResetGroupReg:   resetScopedRegTokenEntry("group", "group_id", toolRunnerResetInstanceReg, toolRunnerResetProjectReg),
	toolRunnerResetProjectReg: resetScopedRegTokenEntry("project", "project_id", toolRunnerResetInstanceReg, toolRunnerResetGroupReg),
	// Manager inspection.
	toolRunnerListManagers: {
		usage:       "List the managers (individual runner processes/hosts) of a runner by runner_id. Use to see each manager's system id, version, platform, architecture, status, and contact IP for a single runner.",
		aliases:     []string{"list runner managers", "runner managers", "show runner managers", "browse runner managers"},
		related:     []string{"gitlab_runner_get", "gitlab_runner_list"},
		description: "List the managers of a runner by runner_id. Returns: runner managers with system id, version, revision, platform, architecture, status, and IP. See also: gitlab_runner_get, gitlab_runner_list.",
	},
}

func runnerParameterGuidance(actionName string) map[string]toolutil.ParameterGuidance {
	guidance := make(map[string]toolutil.ParameterGuidance)
	if runnerActionUsesRunnerID(actionName) {
		guidance["runner_id"] = toolutil.ParameterGuidance{
			SemanticRole: "runner_identifier",
			ValueSource:  "Use gitlab_runner_list, gitlab_runner_list_project, gitlab_runner_list_group, or gitlab_runner_get; runner_id is the global runner ID.",
			CommonConfusions: []string{
				"Do not pass project_id as runner_id.",
				"For project assignment actions, project_id identifies the project scope while runner_id identifies the runner.",
			},
		}
	}
	if runnerActionUsesProjectID(actionName) {
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole: "scope_owner_project",
			ValueSource:  "Use gitlab_project get/list outputs; accepts numeric ID or namespace/project path.",
			CommonConfusions: []string{
				"project_id identifies the project scope, not the runner.",
				"Use runner_id for the runner to enable, disable, reset, or inspect.",
			},
		}
	}
	if runnerActionUsesGroupID(actionName) {
		guidance["group_id"] = toolutil.ParameterGuidance{
			SemanticRole: "scope_owner_group",
			ValueSource:  "Use gitlab_group get/list outputs; accepts numeric ID or full group path.",
		}
	}
	if len(guidance) == 0 {
		return nil
	}
	return guidance
}

func runnerActionUsesRunnerID(actionName string) bool {
	switch actionName {
	case "get", "update", "remove", "jobs", "enable_project", "disable_project", "delete_registered", "reset_token":
		return true
	default:
		return false
	}
}

func runnerActionUsesProjectID(actionName string) bool {
	switch actionName {
	case "list_project", "enable_project", "disable_project", "reset_project_reg_token":
		return true
	default:
		return false
	}
}

func runnerActionUsesGroupID(actionName string) bool {
	switch actionName {
	case "list_group", "reset_group_reg_token":
		return true
	default:
		return false
	}
}
