package bulkimports

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for bulk import tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		bulkImportCreateSpec("bulk_import_start", toolutil.RouteAction(client, StartMigration), "gitlab_start_bulk_import"),
		bulkImportReadSpec("bulk_import_list", toolutil.RouteAction(client, List), "gitlab_list_bulk_imports"),
		bulkImportReadSpec("bulk_import_get", toolutil.RouteAction(client, Get), "gitlab_get_bulk_import"),
		bulkImportUpdateSpec("bulk_import_cancel", toolutil.RouteAction(client, Cancel), "gitlab_cancel_bulk_import"),
		bulkImportReadSpec("bulk_import_entity_list", toolutil.RouteAction(client, ListEntities), "gitlab_list_bulk_import_entities"),
		bulkImportReadSpec("bulk_import_entity_get", toolutil.RouteAction(client, GetEntity), "gitlab_get_bulk_import_entity"),
		bulkImportReadSpec("bulk_import_entity_failures", toolutil.RouteAction(client, ListEntityFailures), "gitlab_list_bulk_import_entity_failures"),
	}
}

func bulkImportReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, bulkImportOptions(individualTool))
}

func bulkImportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, bulkImportOptions(individualTool))
}

func bulkImportUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, bulkImportOptions(individualTool))
}

func bulkImportOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "import"},
		OpenWorld:      true,
		OwnerPackage:   "bulkimports",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
