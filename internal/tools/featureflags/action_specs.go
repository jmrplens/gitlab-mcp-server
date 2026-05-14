package featureflags

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for feature flag actions.
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
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func featureFlagCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, featureFlagOptions(individualTool))
}

func featureFlagUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func featureFlagDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureFlagOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func featureFlagOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"feature_flags", "rollout"},
		RelatedActions: []string{"environment.list", "ci_variable.list"},
		OpenWorld:      true,
		OwnerPackage:   "featureflags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
