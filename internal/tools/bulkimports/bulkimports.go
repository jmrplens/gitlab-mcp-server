// Package bulkimports implements MCP tools for GitLab Bulk Imports API.
package bulkimports

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// StartMigration
// ---------------------------------------------------------------------------.

// EntityInput represents a single entity to migrate.
type EntityInput struct {
	SourceType           string `json:"source_type" jsonschema:"Source type: group_entity or project_entity"`
	SourceFullPath       string `json:"source_full_path" jsonschema:"Full path of the source entity"`
	DestinationSlug      string `json:"destination_slug" jsonschema:"Slug for destination"`
	DestinationNamespace string `json:"destination_namespace" jsonschema:"Destination namespace path"`
	MigrateProjects      *bool  `json:"migrate_projects,omitempty" jsonschema:"Whether to migrate projects"`
	MigrateMemberships   *bool  `json:"migrate_memberships,omitempty" jsonschema:"Whether to migrate memberships"`
}

// StartMigrationInput is the input for starting a bulk import migration.
type StartMigrationInput struct {
	URL         string        `json:"url" jsonschema:"Source GitLab instance URL,required"`
	AccessToken string        `json:"access_token" jsonschema:"Personal access token for source instance,required"`
	Entities    []EntityInput `json:"entities" jsonschema:"List of entities to migrate,required"`
}

// MigrationOutput is the output for a bulk import migration.
type MigrationOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	SourceType  string `json:"source_type"`
	SourceURL   string `json:"source_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	HasFailures bool   `json:"has_failures"`
}

// StartMigration starts a new bulk import migration.
func StartMigration(ctx context.Context, client *gitlabclient.Client, input StartMigrationInput) (MigrationOutput, error) {
	entities := make([]gl.BulkImportStartMigrationEntity, 0, len(input.Entities))
	for _, e := range input.Entities {
		entity := gl.BulkImportStartMigrationEntity{
			SourceType:           new(e.SourceType),
			SourceFullPath:       new(e.SourceFullPath),
			DestinationSlug:      new(e.DestinationSlug),
			DestinationNamespace: new(e.DestinationNamespace),
		}
		if e.MigrateProjects != nil {
			entity.MigrateProjects = e.MigrateProjects
		}
		if e.MigrateMemberships != nil {
			entity.MigrateMemberships = e.MigrateMemberships
		}
		entities = append(entities, entity)
	}

	opts := &gl.BulkImportStartMigrationOptions{
		Configuration: &gl.BulkImportStartMigrationConfiguration{
			URL:         new(input.URL),
			AccessToken: new(input.AccessToken),
		},
		Entities: entities,
	}

	resp, _, err := client.GL().BulkImports.StartMigration(opts, gl.WithContext(ctx))
	if err != nil {
		return MigrationOutput{}, toolutil.WrapErrWithStatusHint("start_bulk_import", err, http.StatusBadRequest, "verify source_type (group_entity or project_entity), source_full_path, and destination_namespace")
	}

	return MigrationOutput{
		ID:          resp.ID,
		Status:      resp.Status,
		SourceType:  resp.SourceType,
		SourceURL:   resp.SourceURL,
		CreatedAt:   resp.CreatedAt.String(),
		UpdatedAt:   resp.UpdatedAt.String(),
		HasFailures: resp.HasFailures,
	}, nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
