package pages

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionPagesDomainList = "pages.domain_list"
	actionPagesDomainGet  = "pages.domain_get"
)

// ActionSpecs returns canonical specs for project Pages actions
// exposed as MCP tools. The settings, custom domain, and admin
// routes are projected into the dynamic, meta, individual, and
// audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_pages_get — read Pages settings for a project.
		pagesReadSpec("pages_get", toolutil.RouteAction(client, GetPages), "gitlab_pages_get"),
		// gitlab_pages_update — update Pages settings for a project.
		pagesUpdateSpec("pages_update", toolutil.RouteAction(client, UpdatePages), "gitlab_pages_update"),
		// gitlab_pages_unpublish — unpublish a project's Pages site (destructive).
		pagesDeleteSpec("pages_unpublish", toolutil.DestructiveAction(client, unpublishOutput), "gitlab_pages_unpublish"),
		// gitlab_pages_domain_list_all — list every Pages domain (admin only).
		pagesReadSpec("pages_domain_list_all", toolutil.RouteAction(client, ListAllDomains), "gitlab_pages_domain_list_all"),
		// gitlab_pages_domain_list — list Pages domains for a project.
		pagesReadSpec("pages_domain_list", toolutil.RouteAction(client, ListDomains), "gitlab_pages_domain_list"),
		// gitlab_pages_domain_get — fetch a single Pages domain.
		pagesReadSpec("pages_domain_get", toolutil.RouteAction(client, GetDomain), "gitlab_pages_domain_get"),
		// gitlab_pages_domain_create — add a new custom Pages domain.
		pagesCreateSpec("pages_domain_create", toolutil.RouteAction(client, CreateDomain), "gitlab_pages_domain_create"),
		// gitlab_pages_domain_update — update an existing Pages domain.
		pagesUpdateSpec("pages_domain_update", toolutil.RouteAction(client, UpdateDomain), "gitlab_pages_domain_update"),
		// gitlab_pages_domain_delete — remove a Pages domain (destructive).
		pagesDeleteSpec("pages_domain_delete", toolutil.DestructiveAction(client, deleteDomainOutput), "gitlab_pages_domain_delete"),
	}
}

// unpublishOutput adapts the package's [UnpublishPages] handler to
// the [toolutil.DestructiveAction] contract, returning a structured
// success result.
func unpublishOutput(ctx context.Context, client *gitlabclient.Client, input UnpublishPagesInput) (toolutil.DeleteOutput, error) {
	if err := UnpublishPages(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pages")
	return out, nil
}

// deleteDomainOutput adapts the package's [DeleteDomain] handler to
// the [toolutil.DestructiveAction] contract, returning a structured
// success result that names the deleted domain in the message.
func deleteDomainOutput(ctx context.Context, client *gitlabclient.Client, input DeleteDomainInput) (toolutil.DeleteOutput, error) {
	if err := DeleteDomain(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pages domain " + input.Domain)
	return out, nil
}

// pagesReadSpec builds a read-only [toolutil.ActionSpec] for a Pages
// action using the package's default [pagesOptions].
func pagesReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, pagesOptions(name, individualTool))
}

// pagesCreateSpec builds a create-style [toolutil.ActionSpec] for a
// Pages action using the package's default [pagesOptions].
func pagesCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pagesOptions(name, individualTool))
}

// pagesUpdateSpec builds an update-style [toolutil.ActionSpec] for a
// Pages action using the package's default [pagesOptions].
func pagesUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pagesOptions(name, individualTool))
}

// pagesDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// Pages action using the package's default [pagesOptions].
func pagesDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pagesOptions(name, individualTool))
}

// pagesOptions returns the base [toolutil.ActionSpecOptions] for a
// Pages action, layering per-action Usage text, natural-language
// Aliases, canonical RelatedActions cross-links, and parameter
// guidance for the project_id and domain fields (1:1 audit R-META).
func pagesOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	meta := pagesActionMeta[actionName]
	guidance := map[string]toolutil.ParameterGuidance{}

	if actionName != "pages_domain_list_all" {
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path owning the Pages configuration.",
			ExampleBinding: `params.project_id:"group/project"`,
		}
	}

	if actionName == "pages_domain_get" || actionName == "pages_domain_create" || actionName == "pages_domain_update" || actionName == "pages_domain_delete" {
		guidance["domain"] = toolutil.ParameterGuidance{
			SemanticRole:   "pages_domain",
			ValueSource:    "Fully qualified domain name of the Pages domain.",
			ExampleBinding: `params.domain:"example.com"`,
		}
	}

	aliases := append([]string{individualTool}, meta.aliases...)

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"project", "pages"},
		Usage:             meta.usage,
		RelatedActions:    append([]string(nil), meta.related...),
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "pages",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: pagesActionDescriptions[actionName],
		},
	}
}

// pagesActionMetadata carries the per-action discovery metadata
// (non-generic Usage, distinctive natural-language Aliases, and
// canonical RelatedActions cross-links) layered onto each Pages
// ActionSpec by [pagesOptions] (1:1 audit R-META).
type pagesActionMetadata struct {
	usage   string
	aliases []string
	related []string
}

// pagesActionMeta maps each canonical Pages action to its discovery
// metadata. RelatedActions use canonical "domain.action" IDs.
var pagesActionMeta = map[string]pagesActionMetadata{
	"pages_get": {
		usage:   "Read the GitLab Pages settings for a project, including the published URL and HTTPS configuration.",
		aliases: []string{"get pages settings", "show pages configuration", "view pages site url"},
		related: []string{"pages.update", "pages.unpublish", actionPagesDomainList},
	},
	"pages_update": {
		usage:   "Update a project's GitLab Pages settings such as force-HTTPS, unique-domain, and the primary domain.",
		aliases: []string{"update pages settings", "configure pages https", "set pages primary domain"},
		related: []string{"pages.get", "pages.unpublish", "pages.domain_create"},
	},
	"pages_unpublish": {
		usage:   "Unpublish and tear down a project's GitLab Pages site, removing the deployed content.",
		aliases: []string{"unpublish pages site", "remove pages deployment", "take down pages"},
		related: []string{"pages.get", "pages.update"},
	},
	"pages_domain_list_all": {
		usage:   "List every GitLab Pages custom domain across the whole instance (admin only).",
		aliases: []string{"list all pages domains", "audit pages custom domains", "show instance pages domains"},
		related: []string{actionPagesDomainList, actionPagesDomainGet},
	},
	"pages_domain_list": {
		usage:   "List the custom Pages domains attached to a project, with verification and SSL status.",
		aliases: []string{"list project pages domains", "show custom domains", "find pages domains for project"},
		related: []string{actionPagesDomainGet, "pages.domain_create", "pages.domain_list_all"},
	},
	"pages_domain_get": {
		usage:   "Fetch one custom Pages domain by name, including its verification code and certificate details.",
		aliases: []string{"get pages domain", "show custom domain details", "check pages domain verification"},
		related: []string{actionPagesDomainList, "pages.domain_update", "pages.domain_delete"},
	},
	"pages_domain_create": {
		usage:   "Attach a new custom Pages domain to a project and obtain its DNS verification code.",
		aliases: []string{"add pages domain", "create custom domain", "attach domain to pages"},
		related: []string{actionPagesDomainGet, "pages.domain_update", actionPagesDomainList},
	},
	"pages_domain_update": {
		usage:   "Update a custom Pages domain's auto-SSL setting or TLS certificate and key.",
		aliases: []string{"update pages domain", "set pages domain certificate", "toggle pages domain auto ssl"},
		related: []string{actionPagesDomainGet, "pages.domain_delete", actionPagesDomainList},
	},
	"pages_domain_delete": {
		usage:   "Detach and delete a custom Pages domain from a project.",
		aliases: []string{"delete pages domain", "remove custom domain", "detach pages domain"},
		related: []string{actionPagesDomainGet, actionPagesDomainList},
	},
}

// pagesActionDescriptions maps each Pages action to a non-generic
// individual-tool description in the "Returns: … See also: …" form
// (1:1 audit R-META). Keyed by canonical action name.
var pagesActionDescriptions = map[string]string{
	"pages_get":             "Get the Pages settings for a project. Returns: the Pages URL, force-HTTPS and unique-domain flags, primary domain, and recent deployments. See also: gitlab_pages_update, gitlab_pages_unpublish, gitlab_pages_domain_list.",
	"pages_update":          "Update the Pages settings for a project. Returns: the updated Pages settings with URL, force-HTTPS and unique-domain flags, primary domain, and deployments. See also: gitlab_pages_get, gitlab_pages_domain_create, gitlab_pages_unpublish.",
	"pages_unpublish":       "Unpublish a project's Pages site (destructive). Returns: a success confirmation. See also: gitlab_pages_get, gitlab_pages_update.",
	"pages_domain_list_all": "List every Pages custom domain across the instance (admin only). Returns: all domains with verification status, auto-SSL flag, project ID, and certificate subject. See also: gitlab_pages_domain_list, gitlab_pages_domain_get.",
	"pages_domain_list":     "List the custom Pages domains for a project with pagination. Returns: matching domains with verification status, auto-SSL flag, project ID, certificate details, and pagination metadata. See also: gitlab_pages_domain_get, gitlab_pages_domain_create, gitlab_pages_domain_list_all.",
	"pages_domain_get":      "Get a single Pages custom domain by name. Returns: the domain with verification status and code, auto-SSL flag, enabled-until date, and certificate subject/expiration. See also: gitlab_pages_domain_list, gitlab_pages_domain_update, gitlab_pages_domain_delete.",
	"pages_domain_create":   "Add a new custom Pages domain to a project. Returns: the created domain with verification code, auto-SSL flag, and certificate details. See also: gitlab_pages_domain_get, gitlab_pages_domain_update, gitlab_pages_domain_list.",
	"pages_domain_update":   "Update an existing Pages custom domain's auto-SSL flag or certificate. Returns: the updated domain with verification status and certificate details. See also: gitlab_pages_domain_get, gitlab_pages_domain_delete, gitlab_pages_domain_list.",
	"pages_domain_delete":   "Remove a custom Pages domain from a project (destructive). Returns: a success confirmation naming the deleted domain. See also: gitlab_pages_domain_get, gitlab_pages_domain_list.",
}
