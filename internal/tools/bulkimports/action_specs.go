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
	var description string
	switch individualTool {
	case "gitlab_start_bulk_import":
		usage = "Start a new bulk import migration from an external GitLab source."
		description = "Start a new direct-transfer migration from an external GitLab source. Returns: the created migration with id, status, source type, source URL, timestamps, and failure flag. See also: gitlab_get_bulk_import, gitlab_list_bulk_imports, gitlab_cancel_bulk_import."
	case "gitlab_list_bulk_imports":
		usage = "List bulk import migrations visible to the current user or admin context."
		description = "List bulk import migrations visible to the caller, with status filtering and offset/keyset pagination. Returns: matching migrations with status, source, timestamps, and pagination metadata. See also: gitlab_get_bulk_import, gitlab_list_bulk_import_entities, gitlab_start_bulk_import."
	case "gitlab_get_bulk_import":
		usage = "Get status and metadata for one bulk import migration."
		description = "Get status and metadata for one bulk import migration by id. Returns: the migration with status, source type, source URL, timestamps, and failure flag. See also: gitlab_list_bulk_imports, gitlab_list_bulk_import_entities, gitlab_cancel_bulk_import."
	case "gitlab_cancel_bulk_import":
		usage = "Cancel an in-progress bulk import migration."
		description = "Cancel an in-progress bulk import migration by id. Returns: the migration with its updated status. See also: gitlab_get_bulk_import, gitlab_list_bulk_imports."
	case "gitlab_list_bulk_import_entities":
		usage = "List entities associated with a bulk import migration or globally."
		description = "List migration entities for one bulk import or across all imports, with status filtering and offset/keyset pagination. Returns: entities with type, status, source/destination paths, per-relation stats, failures, and pagination metadata. See also: gitlab_get_bulk_import_entity, gitlab_list_bulk_import_entity_failures, gitlab_get_bulk_import."
	case "gitlab_get_bulk_import_entity":
		usage = "Get details for one entity inside a bulk import migration."
		description = "Get one migration entity inside a bulk import by entity id. Returns: the entity with type, status, source/destination paths, migration flags, per-relation stats, and failure records. See also: gitlab_list_bulk_import_entities, gitlab_list_bulk_import_entity_failures."
	case "gitlab_list_bulk_import_entity_failures":
		usage = "List failure records for one bulk import entity."
		description = "List failed import records for one bulk import entity. Returns: failure records with relation, exception class and message, pipeline step, source URL, and timestamps. See also: gitlab_get_bulk_import_entity, gitlab_list_bulk_import_entities."
	}

	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"admin", "import"},
		Usage:          usage,
		RelatedActions: []string{"project.import_status", "project.export_status"},
		OpenWorld:      true,
		OwnerPackage:   "bulkimports",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}
