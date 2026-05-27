package dependencyproxy

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for dependency proxy tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	options := dependencyProxyOptions("gitlab_purge_dependency_proxy")
	return []toolutil.ActionSpec{
		toolutil.NewDeleteActionSpec("dependency_proxy_delete", toolutil.DestructiveVoidAction(client, Purge), options),
	}
}

func dependencyProxyOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"purge dependency proxy", "clear dependency proxy cache", "dependency proxy cleanup"},
		Tags:           []string{"dependency-proxy"},
		Usage:          "Purge group dependency proxy cache. Use this for cache invalidation/cleanup when stale registry layers or package cache entries must be dropped.",
		RelatedActions: []string{"group.get", "project.package_registry_list"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group ID or full path that owns the dependency proxy cache.",
				ExampleBinding: `params.group_id:"my-group"`,
			},
		},
		OpenWorld:      true,
		OwnerPackage:   "dependencyproxy",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
