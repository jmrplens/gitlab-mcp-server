package dbmigrations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		Tags:           []string{"admin", "database"},
		OpenWorld:      true,
		OwnerPackage:   "dbmigrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
