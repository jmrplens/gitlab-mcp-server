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
	usage := "Operate bulk import migrations and entities."
	switch individualTool {
	case "gitlab_start_bulk_import":
		usage = "Start a new bulk import migration from an external GitLab source."
	case "gitlab_list_bulk_imports":
		usage = "List bulk import migrations visible to the current user or admin context."
	case "gitlab_get_bulk_import":
		usage = "Get status and metadata for one bulk import migration."
	case "gitlab_cancel_bulk_import":
		usage = "Cancel an in-progress bulk import migration."
	case "gitlab_list_bulk_import_entities":
		usage = "List entities associated with a bulk import migration or globally."
	case "gitlab_get_bulk_import_entity":
		usage = "Get details for one entity inside a bulk import migration."
	case "gitlab_list_bulk_import_entity_failures":
		usage = "List failure records for one bulk import entity."
	}

	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"admin", "import"},
		Usage:          usage,
		RelatedActions: []string{"project.import_status", "project.export_status"},
		OpenWorld:      true,
		OwnerPackage:   "bulkimports",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
