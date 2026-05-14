package pages

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project Pages actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pagesReadSpec("pages_get", toolutil.RouteAction(client, GetPages), "gitlab_pages_get"),
		pagesUpdateSpec("pages_update", toolutil.RouteAction(client, UpdatePages), "gitlab_pages_update"),
		pagesDeleteSpec("pages_unpublish", toolutil.DestructiveVoidAction(client, UnpublishPages), "gitlab_pages_unpublish"),
		pagesReadSpec("pages_domain_list_all", toolutil.RouteAction(client, ListAllDomains), "gitlab_pages_domain_list_all"),
		pagesReadSpec("pages_domain_list", toolutil.RouteAction(client, ListDomains), "gitlab_pages_domain_list"),
		pagesReadSpec("pages_domain_get", toolutil.RouteAction(client, GetDomain), "gitlab_pages_domain_get"),
		pagesCreateSpec("pages_domain_create", toolutil.RouteAction(client, CreateDomain), "gitlab_pages_domain_create"),
		pagesUpdateSpec("pages_domain_update", toolutil.RouteAction(client, UpdateDomain), "gitlab_pages_domain_update"),
		pagesDeleteSpec("pages_domain_delete", toolutil.DestructiveVoidAction(client, DeleteDomain), "gitlab_pages_domain_delete"),
	}
}

func pagesReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pagesOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pagesCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, pagesOptions(individualTool))
}

func pagesUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pagesOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pagesDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pagesOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pagesOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "pages"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "pages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
