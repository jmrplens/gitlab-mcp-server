package featureflags

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, featureFlagOptions(individualTool))
}

func featureFlagCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, featureFlagOptions(individualTool))
}

func featureFlagUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, featureFlagOptions(individualTool))
}

func featureFlagDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, featureFlagOptions(individualTool))
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
