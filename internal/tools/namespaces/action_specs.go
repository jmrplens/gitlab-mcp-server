package namespaces

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionNamespaceGet    = "namespace.get"
	actionGroupList       = "group.list"
	actionNamespaceSearch = "namespace.search"
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

// namespaceActionMetaEntry is the discovery metadata for one namespace action.
type namespaceActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// namespaceActionMeta maps each individual namespace tool to its non-generic
// discovery metadata (usage, natural-language aliases, related actions, and the
// "Returns: … See also: …" individual-tool description) for the 1:1 audit
// (R-META).
var namespaceActionMeta = map[string]namespaceActionMetaEntry{
	"gitlab_namespace_list": {
		usage:       "List namespaces (user and group namespaces) visible to the authenticated user. Use filters such as search, owned_only, top_level_only, order_by, sort, and pagination when scoping the result set.",
		aliases:     []string{"list namespaces", "show namespaces", "list groups and user namespaces"},
		related:     []string{actionNamespaceGet, actionNamespaceSearch, actionGroupList, "project.list"},
		description: "List namespaces visible to the authenticated user. Returns: matching namespaces with id, name, path, kind, plan, seat usage, and pagination metadata. See also: gitlab_namespace_get, gitlab_namespace_search, gitlab_group_list.",
	},
	"gitlab_namespace_get": {
		usage:       "Get one namespace by numeric ID or full path. Use after a list or search result, or when the prompt already names a concrete namespace.",
		aliases:     []string{"get namespace", "show namespace details", "fetch namespace"},
		related:     []string{"namespace.list", actionNamespaceSearch, "group.get"},
		description: "Get a single namespace by ID or path. Returns: the namespace with id, name, path, kind, full path, parent id, plan, trial state, and seat usage. See also: gitlab_namespace_list, gitlab_namespace_search.",
	},
	"gitlab_namespace_exists": {
		usage:       "Check whether a namespace path is available before creating a group or project. Optionally scope the check to a parent namespace with parent_id.",
		aliases:     []string{"check namespace availability", "namespace path exists", "is namespace taken"},
		related:     []string{actionNamespaceGet, actionNamespaceSearch, "group.create"},
		description: "Check whether a namespace path is taken. Returns: an exists flag and suggested alternative paths when the path is unavailable. See also: gitlab_namespace_get, gitlab_namespace_search.",
	},
	"gitlab_namespace_search": {
		usage:       "Search namespaces by query string across name and path. Use when the user provides a partial namespace name or path fragment.",
		aliases:     []string{"search namespaces", "find namespace", "lookup namespace by name"},
		related:     []string{"namespace.list", actionNamespaceGet, actionGroupList},
		description: "Search namespaces by name or path. Returns: matching namespaces with id, name, path, kind, and pagination metadata. See also: gitlab_namespace_list, gitlab_namespace_get.",
	},
}

func namespaceReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "namespace_get" || name == "namespace_exists" {
		guidance["id"] = toolutil.ParameterGuidance{
			SemanticRole:   "namespace_identifier",
			ValueSource:    "Namespace numeric ID or full path from namespace list output.",
			ExampleBinding: `params.id:"my-group/subgroup"`,
		}
	}
	if name == "namespace_search" {
		guidance["query"] = toolutil.ParameterGuidance{
			SemanticRole:   "search_query",
			ValueSource:    "User-provided namespace search text.",
			ExampleBinding: `params.query:"platform"`,
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"user", "namespace"},
		Usage:             "Use to execute namespaces domain action.",
		RelatedActions:    []string{actionGroupList, "project.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "namespaces",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := namespaceActionMeta[individualTool]; ok {
		options.Usage = meta.usage
		options.Aliases = append([]string{individualTool}, meta.aliases...)
		options.RelatedActions = append([]string(nil), meta.related...)
		options.IndividualTool.Description = meta.description
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
