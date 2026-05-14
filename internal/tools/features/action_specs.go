package features

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for instance feature flag tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		featureReadSpec("feature_list", toolutil.RouteAction(client, List), "gitlab_list_features"),
		featureReadSpec("feature_list_definitions", toolutil.RouteAction(client, ListDefinitions), "gitlab_list_feature_definitions"),
		featureSetSpec(client),
		featureDeleteSpec("feature_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_feature_flag"),
	}
}

func featureReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func featureSetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualIdempotent := false
	options := featureOptions("gitlab_set_feature_flag")
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewActionSpec("feature_set", SetRoute(client), options)
}

func featureDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := featureOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func featureOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "feature"},
		OpenWorld:      true,
		OwnerPackage:   "features",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
