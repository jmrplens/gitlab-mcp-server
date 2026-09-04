package bulkimports

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// StartMigration
// ---------------------------------------------------------------------------.

// ConfigurationInput mirrors gl.BulkImportStartMigrationConfiguration and
// carries the connection details for the external GitLab source instance.
type ConfigurationInput struct {
	URL         string `json:"url" jsonschema:"Source GitLab instance URL,required"`
	AccessToken string `json:"access_token" jsonschema:"Personal access token for source instance,required"`
}

// EntityInput mirrors gl.BulkImportStartMigrationEntity: a single entity
// (group or project) to migrate.
type EntityInput struct {
	SourceType           string `json:"source_type" jsonschema:"Source type: group_entity or project_entity"`
	SourceFullPath       string `json:"source_full_path" jsonschema:"Full path of the source entity"`
	DestinationSlug      string `json:"destination_slug" jsonschema:"Slug for destination"`
	DestinationNamespace string `json:"destination_namespace" jsonschema:"Destination namespace path"`
	MigrateProjects      *bool  `json:"migrate_projects,omitempty" jsonschema:"Whether to migrate projects"`
	MigrateMemberships   *bool  `json:"migrate_memberships,omitempty" jsonschema:"Whether to migrate memberships"`
}

// StartMigrationInput is the input for starting a bulk import migration. It
// mirrors gl.BulkImportStartMigrationOptions: a nested configuration object
// describing the source instance plus the list of entities to migrate.
type StartMigrationInput struct {
	Configuration ConfigurationInput `json:"configuration" jsonschema:"Source instance connection details (url, access_token),required"`
	Entities      []EntityInput      `json:"entities" jsonschema:"List of entities to migrate,required"`
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
			URL:         new(input.Configuration.URL),
			AccessToken: new(input.Configuration.AccessToken),
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
		CreatedAt:   resp.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   resp.UpdatedAt.Format(time.RFC3339),
		HasFailures: resp.HasFailures,
	}, nil
}

// ---------------------------------------------------------------------------
// List / Get / Cancel
// ---------------------------------------------------------------------------.

// MigrationSummary is a single bulk import migration entry returned by list/get.
type MigrationSummary struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	SourceType  string `json:"source_type"`
	SourceURL   string `json:"source_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	HasFailures bool   `json:"has_failures"`
}

// ListInput defines parameters for listing bulk import migrations. It mirrors
// gl.ListBulkImportsOptions, including keyset pagination (order_by, sort,
// pagination, page_token) via the embedded gl.ListOptions.
type ListInput struct {
	Status  string `json:"status,omitempty"   jsonschema:"Filter by status: created, started, finished, timeout, failed, canceled"`
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds the paginated list of bulk import migrations.
type ListOutput struct {
	toolutil.HintableOutput
	Migrations []MigrationSummary        `json:"migrations"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List returns all bulk import migrations visible to the caller.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListBulkImportsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Status != "" {
		opts.Status = &input.Status
	}
	imports, resp, err := client.GL().BulkImports.ListBulkImports(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("bulk_import_list", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, m := range imports {
		out.Migrations = append(out.Migrations, toSummary(m))
	}
	return out, nil
}

// GetInput defines parameters for retrieving a single bulk import migration.
type GetInput struct {
	ID int64 `json:"id" jsonschema:"Bulk import ID,required"`
}

// Get retrieves a single bulk import migration by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (MigrationSummary, error) {
	if input.ID <= 0 {
		return MigrationSummary{}, toolutil.ErrRequiredInt64("bulk_import_get", "id")
	}
	m, _, err := client.GL().BulkImports.GetBulkImport(input.ID, gl.WithContext(ctx))
	if err != nil {
		return MigrationSummary{}, toolutil.WrapErrWithStatusHint("bulk_import_get", err, http.StatusNotFound,
			"verify the migration id with gitlab_list_bulk_imports")
	}
	return toSummary(m), nil
}

// CancelInput defines parameters for canceling a bulk import migration.
type CancelInput struct {
	ID int64 `json:"id" jsonschema:"Bulk import ID,required"`
}

// Cancel cancels an in-progress bulk import migration.
func Cancel(ctx context.Context, client *gitlabclient.Client, input CancelInput) (MigrationSummary, error) {
	if input.ID <= 0 {
		return MigrationSummary{}, toolutil.ErrRequiredInt64("bulk_import_cancel", "id")
	}
	m, _, err := client.GL().BulkImports.CancelBulkImport(input.ID, gl.WithContext(ctx))
	if err != nil {
		return MigrationSummary{}, toolutil.WrapErrWithMessage("bulk_import_cancel", err)
	}
	return toSummary(m), nil
}

// toSummary converts a *gl.BulkImport to MigrationSummary.
func toSummary(m *gl.BulkImport) MigrationSummary {
	if m == nil {
		return MigrationSummary{}
	}
	return MigrationSummary{
		ID:          m.ID,
		Status:      m.Status,
		SourceType:  m.SourceType,
		SourceURL:   m.SourceURL,
		CreatedAt:   m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.Format(time.RFC3339),
		HasFailures: m.HasFailures,
	}
}

// ---------------------------------------------------------------------------
// Entities
// ---------------------------------------------------------------------------.

// EntityStatItem mirrors gl.BulkImportEntityStatItem: source/fetched/imported
// counts for a single relation type within a migration entity.
type EntityStatItem struct {
	Source   int `json:"source"`
	Fetched  int `json:"fetched"`
	Imported int `json:"imported"`
}

// EntityStats mirrors gl.BulkImportEntityStats: per-relation import counts for a
// migration entity.
type EntityStats struct {
	Labels     EntityStatItem `json:"labels"`
	Milestones EntityStatItem `json:"milestones"`
}

// EntitySummary describes a single bulk import migration entity (group or project).
type EntitySummary struct {
	ID                   int64           `json:"id"`
	BulkImportID         int64           `json:"bulk_import_id"`
	Status               string          `json:"status"`
	EntityType           string          `json:"entity_type"`
	SourceFullPath       string          `json:"source_full_path"`
	DestinationFullPath  string          `json:"destination_full_path"`
	DestinationName      string          `json:"destination_name"`
	DestinationSlug      string          `json:"destination_slug"`
	DestinationNamespace string          `json:"destination_namespace"`
	ParentID             *int64          `json:"parent_id,omitempty"`
	NamespaceID          *int64          `json:"namespace_id,omitempty"`
	ProjectID            *int64          `json:"project_id,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	MigrateProjects      bool            `json:"migrate_projects"`
	MigrateMemberships   bool            `json:"migrate_memberships"`
	HasFailures          bool            `json:"has_failures"`
	Failures             []EntityFailure `json:"failures,omitempty"`
	Stats                EntityStats     `json:"stats"`
}

// ListEntitiesInput defines parameters for listing bulk import entities.
// When BulkImportID > 0, entities are scoped to that import; otherwise all
// entities visible to the caller are returned.
// It mirrors gl.ListBulkImportsEntitiesOptions, including keyset pagination
// (order_by, sort, pagination, page_token) via the embedded gl.ListOptions.
type ListEntitiesInput struct {
	BulkImportID int64  `json:"bulk_import_id,omitempty" jsonschema:"Bulk import ID. If omitted, returns entities across all imports."`
	Status       string `json:"status,omitempty"          jsonschema:"Filter by entity status: created, started, finished, timeout, failed, canceled"`
	OrderBy      string `json:"order_by,omitempty"        jsonschema:"Column to order results by (e.g. created_at)"`
	Sort         string `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListEntitiesOutput holds the paginated list of bulk import entities.
type ListEntitiesOutput struct {
	toolutil.HintableOutput
	Entities   []EntitySummary           `json:"entities"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListEntities returns bulk import entities, optionally scoped to a single import.
func ListEntities(ctx context.Context, client *gitlabclient.Client, input ListEntitiesInput) (ListEntitiesOutput, error) {
	if input.BulkImportID < 0 {
		return ListEntitiesOutput{}, errors.New("bulk_import_entity_list: bulk_import_id must be >= 0 (omit or set to 0 to list across all imports)")
	}
	opts := &gl.ListBulkImportsEntitiesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Status != "" {
		opts.Status = &input.Status
	}
	var (
		entities []*gl.BulkImportEntity
		resp     *gl.Response
		err      error
	)
	if input.BulkImportID > 0 {
		entities, resp, err = client.GL().BulkImports.ListBulkImportsEntitiesByID(input.BulkImportID, opts, gl.WithContext(ctx))
	} else {
		entities, resp, err = client.GL().BulkImports.ListBulkImportsEntities(opts, gl.WithContext(ctx))
	}
	if err != nil {
		return ListEntitiesOutput{}, toolutil.WrapErr("bulk_import_entity_list", err)
	}
	out := ListEntitiesOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, e := range entities {
		out.Entities = append(out.Entities, toEntitySummary(e))
	}
	return out, nil
}

// GetEntityInput defines parameters for retrieving a single bulk import entity.
type GetEntityInput struct {
	BulkImportID int64 `json:"bulk_import_id" jsonschema:"Bulk import ID,required"`
	EntityID     int64 `json:"entity_id"      jsonschema:"Entity ID within the bulk import,required"`
}

// GetEntity retrieves a single migration entity by ID.
func GetEntity(ctx context.Context, client *gitlabclient.Client, input GetEntityInput) (EntitySummary, error) {
	if input.BulkImportID <= 0 {
		return EntitySummary{}, toolutil.ErrRequiredInt64("bulk_import_entity_get", "bulk_import_id")
	}
	if input.EntityID <= 0 {
		return EntitySummary{}, toolutil.ErrRequiredInt64("bulk_import_entity_get", "entity_id")
	}
	e, _, err := client.GL().BulkImports.GetBulkImportEntity(input.BulkImportID, input.EntityID, gl.WithContext(ctx))
	if err != nil {
		return EntitySummary{}, toolutil.WrapErrWithStatusHint("bulk_import_entity_get", err, http.StatusNotFound,
			"verify bulk_import_id and entity_id with gitlab_list_bulk_import_entities")
	}
	return toEntitySummary(e), nil
}

// EntityFailure describes a single failed import record for a migration entity.
type EntityFailure struct {
	Relation           string `json:"relation"`
	ExceptionMessage   string `json:"exception_message"`
	ExceptionClass     string `json:"exception_class"`
	CorrelationIDValue string `json:"correlation_id_value"`
	SourceURL          string `json:"source_url"`
	SourceTitle        string `json:"source_title"`
	Step               string `json:"step"`
	CreatedAt          string `json:"created_at"`
	PipelineClass      string `json:"pipeline_class"`
	PipelineStep       string `json:"pipeline_step"`
}

// ListEntityFailuresInput defines parameters for listing failures of a migration entity.
type ListEntityFailuresInput struct {
	BulkImportID int64 `json:"bulk_import_id" jsonschema:"Bulk import ID,required"`
	EntityID     int64 `json:"entity_id"      jsonschema:"Entity ID within the bulk import,required"`
}

// ListEntityFailuresOutput holds the failure records for a migration entity.
type ListEntityFailuresOutput struct {
	toolutil.HintableOutput
	BulkImportID int64           `json:"bulk_import_id"`
	EntityID     int64           `json:"entity_id"`
	Failures     []EntityFailure `json:"failures"`
}

// ListEntityFailures returns failed import records for a single migration entity.
func ListEntityFailures(ctx context.Context, client *gitlabclient.Client, input ListEntityFailuresInput) (ListEntityFailuresOutput, error) {
	if input.BulkImportID <= 0 {
		return ListEntityFailuresOutput{}, toolutil.ErrRequiredInt64("bulk_import_entity_failures", "bulk_import_id")
	}
	if input.EntityID <= 0 {
		return ListEntityFailuresOutput{}, toolutil.ErrRequiredInt64("bulk_import_entity_failures", "entity_id")
	}
	failures, _, err := client.GL().BulkImports.GetBulkImportEntityFailures(input.BulkImportID, input.EntityID, gl.WithContext(ctx))
	if err != nil {
		return ListEntityFailuresOutput{}, toolutil.WrapErrWithStatusHint("bulk_import_entity_failures", err, http.StatusNotFound,
			"verify bulk_import_id and entity_id with gitlab_list_bulk_import_entities")
	}
	out := ListEntityFailuresOutput{BulkImportID: input.BulkImportID, EntityID: input.EntityID}
	for _, f := range failures {
		if f == nil {
			continue
		}
		out.Failures = append(out.Failures, toEntityFailure(f))
	}
	return out, nil
}

// toEntityFailure converts a *gl.BulkImportEntityFailure to EntityFailure.
func toEntityFailure(f *gl.BulkImportEntityFailure) EntityFailure {
	return EntityFailure{
		Relation:           f.Relation,
		ExceptionMessage:   f.ExceptionMessage,
		ExceptionClass:     f.ExceptionClass,
		CorrelationIDValue: f.CorrelationIDValue,
		SourceURL:          f.SourceURL,
		SourceTitle:        f.SourceTitle,
		Step:               f.Step,
		CreatedAt:          f.CreatedAt.Format(time.RFC3339),
		PipelineClass:      f.PipelineClass,
		PipelineStep:       f.PipelineStep,
	}
}

// toEntitySummary converts a *gl.BulkImportEntity to EntitySummary.
func toEntitySummary(e *gl.BulkImportEntity) EntitySummary {
	if e == nil {
		return EntitySummary{}
	}
	var failures []EntityFailure
	for _, f := range e.Failures {
		if f == nil {
			continue
		}
		failures = append(failures, toEntityFailure(f))
	}
	return EntitySummary{
		ID:                   e.ID,
		BulkImportID:         e.BulkImportID,
		Status:               e.Status,
		EntityType:           e.EntityType,
		SourceFullPath:       e.SourceFullPath,
		DestinationFullPath:  e.DestinationFullPath,
		DestinationName:      e.DestinationName,
		DestinationSlug:      e.DestinationSlug,
		DestinationNamespace: e.DestinationNamespace,
		ParentID:             e.ParentID,
		NamespaceID:          e.NamespaceID,
		ProjectID:            e.ProjectID,
		CreatedAt:            e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            e.UpdatedAt.Format(time.RFC3339),
		MigrateProjects:      e.MigrateProjects,
		MigrateMemberships:   e.MigrateMemberships,
		HasFailures:          e.HasFailures,
		Failures:             failures,
		Stats: EntityStats{
			Labels: EntityStatItem{
				Source:   e.Stats.Labels.Source,
				Fetched:  e.Stats.Labels.Fetched,
				Imported: e.Stats.Labels.Imported,
			},
			Milestones: EntityStatItem{
				Source:   e.Stats.Milestones.Source,
				Fetched:  e.Stats.Milestones.Fetched,
				Imported: e.Stats.Milestones.Imported,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
