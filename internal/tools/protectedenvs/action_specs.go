package protectedenvs

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs. Every protected-environment action is a route
// on the gitlab_environment group, so each reads environment.protected_*, and
// the three below name sibling actions of that same group the discovery
// metadata cross-links to. They live in one block because markdown.go builds
// its hints from the same names: nothing in the repository checks an ID
// against the catalog, so a spelling that drifts is answered "unknown action"
// the moment a model follows the hint.
const (
	actionEnvProtectedList      = "environment.protected_list"
	actionEnvProtectedGet       = "environment.protected_get"
	actionEnvProtectedProtect   = "environment.protected_protect"
	actionEnvProtectedUpdate    = "environment.protected_update"
	actionEnvProtectedUnprotect = "environment.protected_unprotect"

	actionEnvList = "environment.list"
	actionEnvGet  = "environment.get"
	// The deployment actions are merged into the gitlab_environment group too,
	// so their IDs carry the environment domain rather than a deployment one.
	actionEnvDeploymentList = "environment.deployment_list"
)

// ActionSpecs returns canonical specs for protected environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		protectedEnvironmentReadSpec("protected_list", toolutil.RouteAction(client, List), "gitlab_protected_environment_list"),
		protectedEnvironmentReadSpec("protected_get", toolutil.RouteAction(client, Get), "gitlab_protected_environment_get"),
		protectedEnvironmentCreateSpec("protected_protect", toolutil.RouteAction(client, Protect), "gitlab_protected_environment_protect"),
		protectedEnvironmentUpdateSpec("protected_update", toolutil.RouteAction(client, Update), "gitlab_protected_environment_update"),
		protectedEnvironmentDeleteSpec("protected_unprotect", toolutil.DestructiveAction(client, unprotectOutput), "gitlab_protected_environment_unprotect"),
	}
}

func unprotectOutput(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) (toolutil.DeleteOutput, error) {
	if err := Unprotect(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("protected environment")
	return out, nil
}

func protectedEnvironmentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"environment", "protected_environment"},
		Usage:          "Use project protected environment actions for project deployment gates. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. To require approvals, use approval_rules with required_approvals, not top-level required_approval_count.",
		RelatedActions: []string{actionEnvList, actionEnvGet, actionEnvDeploymentList},
		OpenWorld:      true,
		OwnerPackage:   "protectedenvs",
		Edition:        "premium",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: protectedEnvironmentDescription(individualTool),
		},
	}
	decorateProtectedEnvironmentMeta(&options, individualTool)
	return options
}

// decorateProtectedEnvironmentMeta replaces the generic shared Usage, the
// tool-name-only Aliases, and the default RelatedActions with action-specific
// discovery metadata so no protected-environment action is flagged as
// generic_usage, aliases_only_toolname, or empty_related (R-META). Aliases are
// project-scoped natural-language phrases kept distinct from the group-level
// protected-environment tools (groupprotectedenvs). It is a no-op for unknown
// tool names so the shared defaults remain in place.
//
// Every entry of the map fills all three fields, which
// TestProtectedEnvironmentActionMeta_EveryEntry_FillsAllThreeFields holds, so
// each assignment is unconditional: the per-field emptiness guards this used
// to carry could never be false, and a guard nothing can observe hides the
// half-filled entry it was meant to catch instead of reporting it.
func decorateProtectedEnvironmentMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := protectedEnvironmentActionMeta[individualTool]
	if !ok {
		return
	}
	options.Usage = meta.usage
	options.Aliases = append([]string(nil), meta.aliases...)
	options.RelatedActions = append([]string(nil), meta.related...)
}

// protectedEnvironmentActionMetaEntry is the discovery metadata for one
// protected-environment action.
type protectedEnvironmentActionMetaEntry struct {
	usage   string
	aliases []string
	related []string
}

// protectedEnvironmentActionMeta maps each individual protected-environment
// tool to its action-specific Usage, natural-language Aliases, and canonical
// RelatedActions. Aliases avoid the bare tool name and the "group" wording
// used by the group-level protected-environment surface.
var protectedEnvironmentActionMeta = map[string]protectedEnvironmentActionMetaEntry{
	"gitlab_protected_environment_list": {
		usage:   "List the protected environments configured on a project, including their deploy access levels and approval rules. Use this when the prompt asks which project environments are gated or who can deploy to them.",
		aliases: []string{"list project protected environments", "show project deployment gates", "which project environments are protected"},
		related: []string{actionEnvProtectedGet, actionEnvList, actionEnvDeploymentList},
	},
	"gitlab_protected_environment_get": {
		usage:   "Fetch a single project protected environment by name (including wildcard tiers such as production). Use after a list result or when the prompt names a concrete environment and you need its deploy access levels and approval rules.",
		aliases: []string{"get project protected environment", "show deployment gate for an environment", "view environment protection settings"},
		related: []string{actionEnvProtectedList, actionEnvProtectedUpdate, actionEnvProtectedUnprotect},
	},
	"gitlab_protected_environment_protect": {
		usage:   "Protect a project environment (or wildcard tier) by setting its deploy access levels and approval rules. Use when the prompt asks to gate deployments, restrict who can deploy, or require approvals on a project environment. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. Require approvals via approval_rules with required_approvals.",
		aliases: []string{"protect a project environment", "gate project deployments", "restrict who can deploy to an environment", "require deployment approvals"},
		related: []string{actionEnvProtectedGet, actionEnvProtectedUpdate, actionEnvProtectedUnprotect},
	},
	"gitlab_protected_environment_update": {
		usage:   "Change the deploy access levels or approval rules on an already-protected project environment. Pass _destroy on an existing entry to remove it. Use when adjusting who can deploy or how many approvals a gated environment needs.",
		aliases: []string{"update protected environment rules", "change deployment access levels", "adjust environment approval rules", "edit project deployment gate"},
		related: []string{actionEnvProtectedGet, actionEnvProtectedProtect, actionEnvProtectedUnprotect},
	},
	"gitlab_protected_environment_unprotect": {
		usage:   "Remove protection from a project environment (or wildcard tier), deleting its deployment gates and approval rules. Destructive. Confirm project_id and the environment name before calling.",
		aliases: []string{"unprotect a project environment", "remove project deployment gate", "stop gating an environment"},
		related: []string{actionEnvProtectedList, actionEnvProtectedProtect},
	},
}

// protectedEnvironmentDescription returns the "Returns: … See also: …" tool
// description for each protected-environment action (R-META).
func protectedEnvironmentDescription(individualTool string) string {
	switch individualTool {
	case "gitlab_protected_environment_list":
		return "List protected environments in a project with order_by/sort and offset or keyset pagination. Returns: protected environments with their deploy access levels, required approval count, approval rules, and pagination metadata. See also: gitlab_protected_environment_get, gitlab_protected_environment_protect, gitlab_environment_list."
	case "gitlab_protected_environment_get":
		return "Get a single protected (or wildcard) environment by name. Returns: the environment with its deploy access levels (id, access level, user/group, group inheritance) and approval rules. See also: gitlab_protected_environment_list, gitlab_protected_environment_update, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_protect":
		return "Protect a single environment or wildcard with deploy access levels and approval rules. Returns: the newly protected environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_protected_environment_get, gitlab_protected_environment_update, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_update":
		return "Update a protected environment's deploy access levels and approval rules (use _destroy to remove an entry). Returns: the updated environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_protected_environment_get, gitlab_protected_environment_protect, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_unprotect":
		return "Unprotect a single environment or wildcard, removing its deployment gates. Returns: a success confirmation naming the unprotected environment. See also: gitlab_protected_environment_list, gitlab_protected_environment_protect."
	default:
		return ""
	}
}
