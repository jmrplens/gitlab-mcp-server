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
	return toolutil.NewReadActionSpec(name, route, pagesOptions(name, individualTool))
}

func pagesCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pagesOptions(name, individualTool))
}

func pagesUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pagesOptions(name, individualTool))
}

func pagesDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pagesOptions(name, individualTool))
}

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
