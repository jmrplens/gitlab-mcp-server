package cicatalog

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for CI/CD Catalog routes, reused across the per-tool
// discovery metadata so RelatedActions stay in sync with the catalog.
const (
	actionCatalogList = "cicatalog.list"
	actionCatalogGet  = "cicatalog.get"
)

// ActionSpecs returns canonical specs for CI/CD Catalog actions exposed as
// MCP tools. Both routes are GraphQL-backed (CI/CD Catalog resources) and
// projected into the dynamic, meta, and individual surfaces by the action
// catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_catalog_resources — browse published CI/CD component projects in the catalog.
		catalogSpec("list", toolutil.RouteAction(client, List), "gitlab_list_catalog_resources"),
		// gitlab_get_catalog_resource — fetch one catalog resource with its components, inputs, and versions.
		catalogSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_catalog_resource"),
	}
}

// catalogSpec builds a read-only [toolutil.ActionSpec] for a CI/CD Catalog
// action and applies the per-tool, non-generic discovery metadata
// (action-specific Usage, distinctive natural-language Aliases, canonical
// RelatedActions, and a "Returns: … See also: …" individual-tool
// description) defined in [catalogActionMeta].
func catalogSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute cicatalog domain action.", Tags: []string{"ci_catalog", "component", "graphql"},
		RelatedActions: []string{"template.lint", "pipeline.create", "project.get"},
		OpenWorld:      true,
		OwnerPackage:   "cicatalog",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateCatalogMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateCatalogMeta overwrites the generic placeholder Usage, Aliases,
// RelatedActions, and individual-tool Description with the action-specific
// discovery metadata for the given individual tool. It is a no-op for tools
// not present in [catalogActionMeta].
func decorateCatalogMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := catalogActionMeta[individualTool]
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

// catalogActionMetaEntry is the discovery metadata for one CI/CD Catalog action.
type catalogActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// catalogActionMeta maps each individual CI/CD Catalog tool to its
// non-generic discovery metadata (1:1 audit R-META). Aliases use CI/CD
// component catalog phrasing distinct from pipeline, template, and project
// domains so dynamic find resolves catalog intents unambiguously.
var catalogActionMeta = map[string]catalogActionMetaEntry{
	"gitlab_list_catalog_resources": {
		usage:   "Browse the CI/CD Catalog of published component projects. Use search to match by name or description, scope to limit to your namespace (NAMESPACED) or the whole instance (ALL), and sort by name, latest release, or star count when the prompt asks for reusable pipeline components or templates to include.",
		aliases: []string{"browse ci/cd catalog", "list cicd components", "search component catalog", "find reusable pipeline components", "list catalog resources"},
		related: []string{actionCatalogGet, "template.lint", "pipeline.create"},
		description: "List published CI/CD Catalog resources (component projects) with optional search, scope, and sort. " +
			"Returns: catalog resources with name, full path, description, latest version, star and fork counts, open issue and MR counts, web URL, and keyset pagination metadata. " +
			"See also: gitlab_get_catalog_resource, gitlab_ci_lint, gitlab_create_pipeline.",
	},
	"gitlab_get_catalog_resource": {
		usage:   "Fetch one CI/CD Catalog resource by its GID or by the full_path of the hosting project. Use after browsing the catalog when the prompt names a specific component project and you need its components, input parameters, README, and released versions to wire an include into a pipeline.",
		aliases: []string{"get catalog resource", "show cicd component details", "inspect catalog component inputs", "fetch component project versions"},
		related: []string{actionCatalogList, "template.lint", "project.get"},
		description: "Get a single CI/CD Catalog resource by GID or full path. " +
			"Returns: the resource with description, README HTML, latest-version components and their typed inputs, version history, star and fork counts, and web URL. " +
			"See also: gitlab_list_catalog_resources, gitlab_ci_lint, gitlab_get_project.",
	},
}
