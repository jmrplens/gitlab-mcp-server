package dbmigrations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs this package names as siblings after a migration has
// been marked. Neither "admin.version" nor "admin.health" was ever an action:
// the instance version and revision are read through admin.metadata_get, and
// the instance's own reachability through the server group's health check.
const (
	actionAdminMetadataGet = "admin.metadata_get"
	actionServerHealth     = "server.health_check"
)

// ActionSpecs returns canonical specs for database migration tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	individualDestructive := false
	options := databaseMigrationOptions("gitlab_mark_migration")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return []toolutil.ActionSpec{
		toolutil.NewDeleteActionSpec("db_migration_mark", toolutil.DestructiveAction(client, Mark), options),
	}
}

func databaseMigrationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"mark migration", "database migration", "schema migration"},
		Tags:           []string{"admin", "database"},
		Usage:          "Mark a database migration as applied/up/down for administrative migration state management.",
		RelatedActions: []string{actionAdminMetadataGet, actionServerHealth},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"version": {
				SemanticRole:   "migration_version",
				ValueSource:    "Migration timestamp/version identifier (for example 20240115100000).",
				ExampleBinding: "params.version:20240115100000",
			},
		},
		OpenWorld:      true,
		OwnerPackage:   "dbmigrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
