package pages

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
// Pages action, layering per-action Usage text and parameter
// guidance for the project_id and domain fields.
func pagesOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Manage project Pages settings and custom domains."
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

	if actionName == "pages_domain_list_all" {
		usage = "List Pages domains across accessible projects."
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"project", "pages"},
		Usage:             usage,
		RelatedActions:    []string{"project.get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "pages",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
