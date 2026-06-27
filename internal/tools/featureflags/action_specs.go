package featureflags

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionFeatureFlagList   = "feature_flag.list"
	actionFeatureFlagGet    = "feature_flag.get"
	actionFeatureFlagCreate = "feature_flag.create"
	actionFeatureFlagUpdate = "feature_flag.update"
	actionFeatureFlagDelete = "feature_flag.delete"
)

// ActionSpecs returns canonical specs for feature flag actions exposed as MCP
// tools. The list, get, create, update, and delete routes are projected into
// the dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004). Each spec carries non-generic discovery metadata (Usage,
// natural-language Aliases, RelatedActions, and a "Returns: … See also: …"
// individual-tool description) per the 1:1 audit R-META requirement.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		featureFlagReadSpec("feature_flag_list", toolutil.RouteAction(client, ListFeatureFlags), "gitlab_feature_flag_list"),
		featureFlagReadSpec("feature_flag_get", toolutil.RouteAction(client, GetFeatureFlag), "gitlab_feature_flag_get"),
		featureFlagCreateSpec("feature_flag_create", toolutil.RouteAction(client, CreateFeatureFlag), "gitlab_feature_flag_create"),
		featureFlagUpdateSpec("feature_flag_update", toolutil.RouteAction(client, UpdateFeatureFlag), "gitlab_feature_flag_update"),
		featureFlagDeleteSpec("feature_flag_delete", toolutil.DestructiveVoidAction(client, DeleteFeatureFlag), "gitlab_feature_flag_delete"),
	}
}

func featureFlagReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	decorateFeatureFlagMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func featureFlagCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	decorateFeatureFlagMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func featureFlagUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	decorateFeatureFlagMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func featureFlagDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	decorateFeatureFlagMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// decorateFeatureFlagMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for a feature-flag action, replacing the generic placeholders from
// featureFlagOptions. It is a no-op for tools with no metadata entry.
func decorateFeatureFlagMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := featureFlagActionMeta[individualTool]
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

// featureFlagActionMetaEntry is the discovery metadata for one feature-flag action.
type featureFlagActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// featureFlagActionMeta maps each individual feature-flag tool to its discovery metadata.
var featureFlagActionMeta = map[string]featureFlagActionMetaEntry{
	"gitlab_feature_flag_list": {
		usage:       "List a project's feature flags. Use scope to filter enabled/disabled flags and order_by/sort plus pagination (offset or keyset) to page through large result sets. Requires GitLab Premium/Ultimate.",
		aliases:     []string{"list project feature flags", "show project feature flags", "find project feature flags"},
		related:     []string{actionFeatureFlagGet, actionFeatureFlagCreate, "environment.list"},
		description: "List feature flags in a project with optional scope filter, ordering, and pagination. Returns: matching feature flags with active state, version, scopes, strategies, and pagination metadata. See also: gitlab_feature_flag_get, gitlab_feature_flag_create.",
	},
	"gitlab_feature_flag_get": {
		usage:       "Get one feature flag by its name (case-sensitive). Use after a list result or when the prompt names a concrete flag to inspect its active state, scopes, and strategies.",
		aliases:     []string{"get feature flag", "show feature flag details", "fetch feature flag"},
		related:     []string{actionFeatureFlagList, actionFeatureFlagUpdate, actionFeatureFlagDelete},
		description: "Get a single feature flag by name. Returns: the flag with active state, version, environment scopes, and activation strategies. See also: gitlab_feature_flag_list, gitlab_feature_flag_update, gitlab_feature_flag_delete.",
	},
	"gitlab_feature_flag_create": {
		usage:       "Create a feature flag in a project. Provide a name and optionally description, version, active, and activation strategies (each with a name, parameters, and environment scopes). Requires Developer+ on a Premium/Ultimate project.",
		aliases:     []string{"create feature flag", "add feature flag", "new feature flag"},
		related:     []string{actionFeatureFlagGet, actionFeatureFlagUpdate, actionFeatureFlagList},
		description: "Create a feature flag with optional activation strategies and scopes. Returns: the created flag with active state, version, scopes, and strategies. See also: gitlab_feature_flag_get, gitlab_feature_flag_update.",
	},
	"gitlab_feature_flag_update": {
		usage:       "Update a feature flag by name. Use to rename it, change description or active state, or replace activation strategies. Requires Developer+ on a Premium/Ultimate project.",
		aliases:     []string{"update feature flag", "toggle feature flag", "rename feature flag", "edit feature flag"},
		related:     []string{actionFeatureFlagGet, actionFeatureFlagDelete, actionFeatureFlagList},
		description: "Update a feature flag's name, description, active state, or strategies. Returns: the updated flag with active state, version, scopes, and strategies. See also: gitlab_feature_flag_get, gitlab_feature_flag_delete.",
	},
	"gitlab_feature_flag_delete": {
		usage:       "Permanently delete a feature flag by name. Destructive and irreversible; confirm project_id and name before calling. Requires Maintainer+ on a Premium/Ultimate project.",
		aliases:     []string{"delete project feature flag", "remove project feature flag", "destroy project feature flag", "drop project feature flag"},
		related:     []string{actionFeatureFlagGet, actionFeatureFlagList},
		description: "Delete a feature flag permanently. Returns: a success confirmation naming the flag. See also: gitlab_feature_flag_get, gitlab_feature_flag_list.",
	},
}

func featureFlagOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute featureflags domain action.", Tags: []string{"feature_flags", "rollout"},
		RelatedActions: []string{"environment.list", "ci_variable.list"},
		OpenWorld:      true,
		OwnerPackage:   "featureflags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
