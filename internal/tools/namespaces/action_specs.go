package namespaces

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for namespace actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		namespaceReadSpec("namespace_list", toolutil.RouteAction(client, List), "gitlab_namespace_list"),
		namespaceReadSpec("namespace_get", toolutil.RouteAction(client, Get), "gitlab_namespace_get"),
		namespaceReadSpec("namespace_exists", toolutil.RouteAction(client, Exists), "gitlab_namespace_exists"),
		namespaceReadSpec("namespace_search", toolutil.RouteAction(client, Search), "gitlab_namespace_search"),
	}
}

func namespaceReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"user", "namespace"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "namespaces",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewActionSpec(name, route, options)
}
