package dependencyproxy

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog action IDs this package names to a model, and the two
// the served spec for the same tool already names.
//
// The purge reaches the catalog through internal/tools/adminspecs, which
// registers it in the gitlab_admin group as "admin.dependency_proxy_delete"
// and cross-links it to the group that owns the cache and to the instance
// settings that switch the dependency proxy on. This copy of the metadata had
// drifted from that one and named "project.package_registry_list", which
// resolves to nothing. The nearest resolving neighbor is not the answer
// either: a project package list and a group's cache of upstream container
// images are different resources, and no catalog action reads what the
// dependency proxy holds, so the cross-link is the settings the served spec
// names rather than somebody else's packages.
const (
	actionGroupGet    = "group.get"
	actionSettingsGet = "admin.settings_get"
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
		RelatedActions: []string{actionGroupGet, actionSettingsGet},
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
