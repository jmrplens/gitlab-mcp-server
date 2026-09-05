package groupprotectedenvs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupProtectedEnvProtect   = "groupprotectedenvs.protected_env_protect"
	actionGroupProtectedEnvGet       = "groupprotectedenvs.protected_env_get"
	actionGroupProtectedEnvUnprotect = "groupprotectedenvs.protected_env_unprotect"
)

// ActionSpecs returns canonical specs for group protected environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupProtectedEnvReadSpec("protected_env_list", toolutil.RouteAction(client, List), "gitlab_group_protected_environment_list"),
		groupProtectedEnvReadSpec("protected_env_get", toolutil.RouteAction(client, Get), "gitlab_group_protected_environment_get"),
		groupProtectedEnvCreateSpec("protected_env_protect", toolutil.RouteAction(client, Protect), "gitlab_group_protected_environment_protect"),
		groupProtectedEnvUpdateSpec("protected_env_update", toolutil.RouteAction(client, Update), "gitlab_group_protected_environment_update"),
		groupProtectedEnvDeleteSpec("protected_env_unprotect", toolutil.DestructiveVoidAction(client, Unprotect), "gitlab_group_protected_environment_unprotect"),
	}
}

func groupProtectedEnvReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"group", "protected-environment"},
		Usage:          "Use group protected environment actions for group-level deployment gates. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. To require approvals, use approval_rules with required_approvals, not top-level required_approval_count.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedenvs",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: groupProtectedEnvDescription(individualTool),
		},
	}
	decorateGroupProtectedEnvMeta(&options, individualTool)
	options.InputSchemaOverrides = groupProtectedEnvTierOverrides(individualTool)
	return options
}

// environmentTiers are the deployment tiers a group-level protected environment
// is addressed by (https://docs.gitlab.com/api/group_protected_environments/):
// every endpoint names the environment by tier, never by a free-form name.
var environmentTiers = []string{"production", "staging", "testing", "development", "other"}

// groupProtectedEnvTierOverrides constrains each tier-valued parameter of a
// group protected environment action to the documented deployment tiers: the
// environment being addressed and, on protect and update, the name being set.
func groupProtectedEnvTierOverrides(individualTool string) []toolutil.InputSchemaOverride {
	switch individualTool {
	case "gitlab_group_protected_environment_get", "gitlab_group_protected_environment_unprotect":
		return []toolutil.InputSchemaOverride{toolutil.SchemaEnumOverride("environment", environmentTiers...)}
	case "gitlab_group_protected_environment_protect":
		return []toolutil.InputSchemaOverride{toolutil.SchemaEnumOverride("name", environmentTiers...)}
	case "gitlab_group_protected_environment_update":
		return []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("environment", environmentTiers...),
			toolutil.SchemaEnumOverride("name", environmentTiers...),
		}
	default:
		return nil
	}
}

// decorateGroupProtectedEnvMeta replaces the generic shared Usage, the
// tool-name-only Aliases, and the default RelatedActions with action-specific
// discovery metadata so no group-protected-environment action is flagged as
// generic_usage, aliases_only_toolname, or empty_related (R-META). Aliases are
// group-scoped natural-language phrases kept distinct from the project-level
// protected-environment tools (protectedenvs). It is a no-op for unknown tool
// names so the shared defaults remain in place.
func decorateGroupProtectedEnvMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupProtectedEnvActionMeta[individualTool]
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
}

// groupProtectedEnvActionMetaEntry is the discovery metadata for one
// group-protected-environment action.
type groupProtectedEnvActionMetaEntry struct {
	usage   string
	aliases []string
	related []string
}

// groupProtectedEnvActionMeta maps each individual group-protected-environment
// tool to its action-specific Usage, natural-language Aliases, and canonical
// RelatedActions. Aliases avoid the bare tool name and use group-scoped wording
// ("group", "across subgroups", "subgroup projects") to stay distinct from the
// project-level protected-environment surface (protectedenvs).
var groupProtectedEnvActionMeta = map[string]groupProtectedEnvActionMetaEntry{
	"gitlab_group_protected_environment_list": {
		usage:   "List the protected environment tiers configured on a group, including their deploy access levels and approval rules. Use this when the prompt asks which group-level environment tiers are gated or who can deploy across the group's subgroup projects.",
		aliases: []string{"list group protected environments", "show group deployment gates", "which group environment tiers are protected"},
		related: []string{actionGroupProtectedEnvGet, actionGroupProtectedEnvProtect, "group.get"},
	},
	"gitlab_group_protected_environment_get": {
		usage:   "Fetch a single group-level protected environment tier by name. Use after a group list result or when the prompt names a concrete group environment tier and you need its deploy access levels and approval rules.",
		aliases: []string{"get group protected environment", "show group deployment gate for a tier", "view group environment protection settings"},
		related: []string{"groupprotectedenvs.protected_env_list", "groupprotectedenvs.protected_env_update", actionGroupProtectedEnvUnprotect},
	},
	"gitlab_group_protected_environment_protect": {
		usage:   "Protect a group-level environment tier by setting its deploy access levels and approval rules. The gate cascades to every subgroup project. Use when the prompt asks to gate deployments across a group, restrict who can deploy, or require approvals at the group level. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. Require approvals via approval_rules with required_approvals.",
		aliases: []string{"protect a group environment tier", "gate deployments across a group", "restrict group-wide deployment access", "require group deployment approvals"},
		related: []string{actionGroupProtectedEnvGet, "groupprotectedenvs.protected_env_update", actionGroupProtectedEnvUnprotect},
	},
	"gitlab_group_protected_environment_update": {
		usage:   "Change the deploy access levels or approval rules on an already-protected group environment tier. Pass _destroy on an existing entry to remove it. Use when adjusting who can deploy across subgroup projects or how many approvals a group-level gate needs.",
		aliases: []string{"update group protected environment rules", "change group deployment access levels", "adjust group environment approval rules", "edit group deployment gate"},
		related: []string{actionGroupProtectedEnvGet, actionGroupProtectedEnvProtect, actionGroupProtectedEnvUnprotect},
	},
	"gitlab_group_protected_environment_unprotect": {
		usage:   "Remove protection from a group-level environment tier, deleting its deployment gates from the group and its subgroup projects. Destructive. Confirm the group id and the environment tier name before calling.",
		aliases: []string{"unprotect a group environment tier", "remove group deployment gate", "stop gating a group environment across subgroups"},
		related: []string{"groupprotectedenvs.protected_env_list", actionGroupProtectedEnvProtect},
	},
}

// groupProtectedEnvDescription returns the "Returns: … See also: …" tool
// description for each group-protected-environment action (R-META).
func groupProtectedEnvDescription(individualTool string) string {
	switch individualTool {
	case "gitlab_group_protected_environment_list":
		return "List group-level protected environments with order_by/sort and offset or keyset pagination. Returns: group protected environments with their deploy access levels, required approval count, approval rules, and pagination metadata. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect, gitlab_group_get."
	case "gitlab_group_protected_environment_get":
		return "Get a single group-level protected environment by tier name. Returns: the environment with its deploy access levels (id, access level, user/group, group inheritance) and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_update, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_protect":
		return "Protect a group-level environment tier with deploy access levels and approval rules. Protection cascades to all subgroup projects. Returns: the newly protected environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_update, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_update":
		return "Update a group-level protected environment's deploy access levels and approval rules (use _destroy to remove an entry). Returns: the updated environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_unprotect":
		return "Unprotect a group-level environment tier, removing its deployment gates from the group and its subgroup projects. Returns: a success confirmation. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_protect."
	default:
		return ""
	}
}
