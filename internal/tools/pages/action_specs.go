package pages

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project Pages actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pagesReadSpec("pages_get", toolutil.RouteAction(client, GetPages), "gitlab_pages_get"),
		pagesUpdateSpec("pages_update", toolutil.RouteAction(client, UpdatePages), "gitlab_pages_update"),
		pagesDeleteSpec("pages_unpublish", toolutil.DestructiveAction(client, unpublishOutput), "gitlab_pages_unpublish"),
		pagesReadSpec("pages_domain_list_all", toolutil.RouteAction(client, ListAllDomains), "gitlab_pages_domain_list_all"),
		pagesReadSpec("pages_domain_list", toolutil.RouteAction(client, ListDomains), "gitlab_pages_domain_list"),
		pagesReadSpec("pages_domain_get", toolutil.RouteAction(client, GetDomain), "gitlab_pages_domain_get"),
		pagesCreateSpec("pages_domain_create", toolutil.RouteAction(client, CreateDomain), "gitlab_pages_domain_create"),
		pagesUpdateSpec("pages_domain_update", toolutil.RouteAction(client, UpdateDomain), "gitlab_pages_domain_update"),
		pagesDeleteSpec("pages_domain_delete", toolutil.DestructiveAction(client, deleteDomainOutput), "gitlab_pages_domain_delete"),
	}
}

func unpublishOutput(ctx context.Context, client *gitlabclient.Client, input UnpublishPagesInput) (toolutil.DeleteOutput, error) {
	if err := UnpublishPages(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pages")
	return out, nil
}

func deleteDomainOutput(ctx context.Context, client *gitlabclient.Client, input DeleteDomainInput) (toolutil.DeleteOutput, error) {
	if err := DeleteDomain(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pages domain " + input.Domain)
	return out, nil
}

func pagesReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, pagesOptions(individualTool))
}

func pagesCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pagesOptions(individualTool))
}

func pagesUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pagesOptions(individualTool))
}

func pagesDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pagesOptions(individualTool))
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
