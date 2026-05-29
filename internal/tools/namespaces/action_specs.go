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
	usage := "List namespaces visible to the authenticated user."
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "namespace_get" || name == "namespace_exists" {
		guidance["id"] = toolutil.ParameterGuidance{
			SemanticRole:   "namespace_identifier",
			ValueSource:    "Namespace numeric ID or full path from namespace list output.",
			ExampleBinding: `params.id:"my-group/subgroup"`,
		}
	}
	if name == "namespace_search" {
		usage = "Search namespaces by query string."
		guidance["query"] = toolutil.ParameterGuidance{
			SemanticRole:   "search_query",
			ValueSource:    "User-provided namespace search text.",
			ExampleBinding: `params.query:"platform"`,
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"user", "namespace"},
		Usage:             usage,
		RelatedActions:    []string{"group.list", "project.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "namespaces",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
