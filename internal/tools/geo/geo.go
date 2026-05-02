// Package geo implements MCP tools for GitLab Geo site management,
// providing CRUD operations and status retrieval for Geo replication sites.
//
// The package also registers Geo MCP tools and renders Markdown summaries for
// Geo site responses.
package geo

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput holds parameters for creating a new Geo site.
type CreateInput struct {
	Primary                          *bool     `json:"primary,omitempty"                            jsonschema:"Whether this is a primary site"`
	Enabled                          *bool     `json:"enabled,omitempty"                            jsonschema:"Whether the site is enabled"`
	Name                             *string   `json:"name,omitempty"                               jsonschema:"Unique name of the Geo site"`
	URL                              *string   `json:"url,omitempty"                                jsonschema:"External URL of the Geo site"`
	InternalURL                      *string   `json:"internal_url,omitempty"                       jsonschema:"Internal URL of the Geo site"`
	FilesMaxCapacity                 *int64    `json:"files_max_capacity,omitempty"                 jsonschema:"Max number of LFS/attachment backfill downloads"`
	ReposMaxCapacity                 *int64    `json:"repos_max_capacity,omitempty"                 jsonschema:"Max number of concurrent repository backfill syncs"`
	VerificationMaxCapacity          *int64    `json:"verification_max_capacity,omitempty"          jsonschema:"Max number of concurrent verification jobs"`
	ContainerRepositoriesMaxCapacity *int64    `json:"container_repositories_max_capacity,omitempty" jsonschema:"Max number of concurrent container repository syncs"`
	SyncObjectStorage                *bool     `json:"sync_object_storage,omitempty"                jsonschema:"Whether to sync object-stored data"`
	SelectiveSyncType                *string   `json:"selective_sync_type,omitempty"                jsonschema:"Selective sync type: namespaces or shards"`
	SelectiveSyncShards              *[]string `json:"selective_sync_shards,omitempty"              jsonschema:"Storage shards to sync (when selective_sync_type=shards)"`
	SelectiveSyncNamespaceIDs        *[]int64  `json:"selective_sync_namespace_ids,omitempty"       jsonschema:"Namespace IDs to sync (when selective_sync_type=namespaces)"`
	MinimumReverificationInterval    *int64    `json:"minimum_reverification_interval,omitempty"    jsonschema:"Minimum interval (days) before re-verification"`
}

// EditInput holds parameters for editing an existing Geo site.
type EditInput struct {
	ID                               int64     `json:"id"                                           jsonschema:"Numeric ID of the Geo site,required"`
	Enabled                          *bool     `json:"enabled,omitempty"                            jsonschema:"Whether the site is enabled"`
	Name                             *string   `json:"name,omitempty"                               jsonschema:"Unique name of the Geo site"`
	URL                              *string   `json:"url,omitempty"                                jsonschema:"External URL of the Geo site"`
	InternalURL                      *string   `json:"internal_url,omitempty"                       jsonschema:"Internal URL of the Geo site"`
	FilesMaxCapacity                 *int64    `json:"files_max_capacity,omitempty"                 jsonschema:"Max number of LFS/attachment backfill downloads"`
	ReposMaxCapacity                 *int64    `json:"repos_max_capacity,omitempty"                 jsonschema:"Max number of concurrent repository backfill syncs"`
	VerificationMaxCapacity          *int64    `json:"verification_max_capacity,omitempty"          jsonschema:"Max number of concurrent verification jobs"`
	ContainerRepositoriesMaxCapacity *int64    `json:"container_repositories_max_capacity,omitempty" jsonschema:"Max number of concurrent container repository syncs"`
	SelectiveSyncType                *string   `json:"selective_sync_type,omitempty"                jsonschema:"Selective sync type: namespaces or shards"`
	SelectiveSyncShards              *[]string `json:"selective_sync_shards,omitempty"              jsonschema:"Storage shards to sync"`
	SelectiveSyncNamespaceIDs        *[]int64  `json:"selective_sync_namespace_ids,omitempty"       jsonschema:"Namespace IDs to sync"`
	MinimumReverificationInterval    *int64    `json:"minimum_reverification_interval,omitempty"    jsonschema:"Minimum interval (days) before re-verification"`
}

// IDInput holds a Geo site ID for get/delete/repair operations.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the Geo site,required"`
}

// ListInput holds pagination parameters for listing Geo sites.
type ListInput struct {
	toolutil.PaginationInput
}

// ListStatusInput holds pagination parameters for listing all Geo site statuses.
type ListStatusInput struct {
	toolutil.PaginationInput
}

// Output represents a single Geo site.
type Output struct {
	toolutil.HintableOutput
	ID                               int64    `json:"id"`
	Name                             string   `json:"name"`
	URL                              string   `json:"url"`
	InternalURL                      string   `json:"internal_url,omitempty"`
	Primary                          bool     `json:"primary"`
	Enabled                          bool     `json:"enabled"`
	Current                          bool     `json:"current"`
	FilesMaxCapacity                 int64    `json:"files_max_capacity"`
	ReposMaxCapacity                 int64    `json:"repos_max_capacity"`
	VerificationMaxCapacity          int64    `json:"verification_max_capacity"`
	ContainerRepositoriesMaxCapacity int64    `json:"container_repositories_max_capacity"`
	SelectiveSyncType                string   `json:"selective_sync_type,omitempty"`
	SelectiveSyncShards              []string `json:"selective_sync_shards,omitempty"`
	SelectiveSyncNamespaceIDs        []int64  `json:"selective_sync_namespace_ids,omitempty"`
	MinimumReverificationInterval    int64    `json:"minimum_reverification_interval"`
	SyncObjectStorage                bool     `json:"sync_object_storage"`
	WebEditURL                       string   `json:"web_edit_url,omitempty"`
}

// ListOutput represents a paginated list of Geo sites.
type ListOutput struct {
	toolutil.HintableOutput
	Sites      []Output                  `json:"sites"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// StatusOutput represents the status of a Geo site (summary fields).
type StatusOutput struct {
	toolutil.HintableOutput
	GeoNodeID                      int64     `json:"geo_node_id"`
	Healthy                        bool      `json:"healthy"`
	Health                         string    `json:"health"`
	HealthStatus                   string    `json:"health_status"`
	MissingOAuthApplication        bool      `json:"missing_oauth_application"`
	DBReplicationLagSeconds        int64     `json:"db_replication_lag_seconds"`
	ProjectsCount                  int64     `json:"projects_count"`
	LFSObjectsSyncedInPercentage   string    `json:"lfs_objects_synced_in_percentage"`
	JobArtifactsSyncedInPercentage string    `json:"job_artifacts_synced_in_percentage"`
	UploadsSyncedInPercentage      string    `json:"uploads_synced_in_percentage"`
	Version                        string    `json:"version"`
	Revision                       string    `json:"revision"`
	StorageShardsMatch             bool      `json:"storage_shards_match"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

// ListStatusOutput represents a paginated list of Geo site statuses.
type ListStatusOutput struct {
	toolutil.HintableOutput
	Statuses   []StatusOutput            `json:"statuses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Create creates a new Geo site.
func Create(ctx context.Context, client *gitlabclient.Client, in CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	opts := &gl.CreateGeoSitesOptions{
		Primary:                          in.Primary,
		Enabled:                          in.Enabled,
		Name:                             in.Name,
		URL:                              in.URL,
		InternalURL:                      in.InternalURL,
		FilesMaxCapacity:                 in.FilesMaxCapacity,
		ReposMaxCapacity:                 in.ReposMaxCapacity,
		VerificationMaxCapacity:          in.VerificationMaxCapacity,
		ContainerRepositoriesMaxCapacity: in.ContainerRepositoriesMaxCapacity,
		SyncObjectStorage:                in.SyncObjectStorage,
		SelectiveSyncType:                in.SelectiveSyncType,
		SelectiveSyncShards:              in.SelectiveSyncShards,
		SelectiveSyncNamespaceIDs:        in.SelectiveSyncNamespaceIDs,
		MinimumReverificationInterval:    in.MinimumReverificationInterval,
	}
	site, _, err := client.GL().GeoSites.CreateGeoSite(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("create geo site", err, http.StatusBadRequest,
			"name must be unique; url must be reachable; only one site may have primary=true; selective_sync_type must be 'namespaces' or 'shards' \u2014 requires admin access and GitLab Premium/Ultimate license")
	}
	return toOutput(site), nil
}

// List retrieves all Geo sites.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListGeoSitesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	sites, resp, err := client.GL().GeoSites.ListGeoSites(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list geo sites", err, http.StatusForbidden,
			"requires admin access and GitLab Premium/Ultimate license; ensure the instance is configured for Geo")
	}

	out := ListOutput{Sites: make([]Output, 0, len(sites))}
	for _, s := range sites {
		out.Sites = append(out.Sites, toOutput(s))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a specific Geo site by ID.
func Get(ctx context.Context, client *gitlabclient.Client, in IDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	site, _, err := client.GL().GeoSites.GetGeoSite(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get geo site", err, http.StatusNotFound,
			"verify id with gitlab_list_geo_sites; requires admin access")
	}
	return toOutput(site), nil
}

// Edit updates an existing Geo site.
func Edit(ctx context.Context, client *gitlabclient.Client, in EditInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	opts := &gl.EditGeoSiteOptions{
		Enabled:                          in.Enabled,
		Name:                             in.Name,
		URL:                              in.URL,
		InternalURL:                      in.InternalURL,
		FilesMaxCapacity:                 in.FilesMaxCapacity,
		ReposMaxCapacity:                 in.ReposMaxCapacity,
		VerificationMaxCapacity:          in.VerificationMaxCapacity,
		ContainerRepositoriesMaxCapacity: in.ContainerRepositoriesMaxCapacity,
		SelectiveSyncType:                in.SelectiveSyncType,
		SelectiveSyncShards:              in.SelectiveSyncShards,
		SelectiveSyncNamespaceIDs:        in.SelectiveSyncNamespaceIDs,
		MinimumReverificationInterval:    in.MinimumReverificationInterval,
	}
	site, _, err := client.GL().GeoSites.EditGeoSite(in.ID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("edit geo site", err, http.StatusBadRequest,
			"verify id with gitlab_list_geo_sites; cannot toggle primary status (recreate site instead); selective_sync_type must be 'namespaces' or 'shards'")
	}
	return toOutput(site), nil
}

// Delete removes a Geo site by ID.
func Delete(ctx context.Context, client *gitlabclient.Client, in IDInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.ID == 0 {
		return toolutil.ErrFieldRequired("id")
	}

	_, err := client.GL().GeoSites.DeleteGeoSite(in.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete geo site", err, http.StatusForbidden,
			"requires admin access; cannot delete the primary site while secondaries exist; deletion is irreversible \u2014 the site must be re-registered to rejoin")
	}
	return nil
}

// Repair triggers OAuth repair for a Geo site.
func Repair(ctx context.Context, client *gitlabclient.Client, in IDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	site, _, err := client.GL().GeoSites.RepairGeoSite(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("repair geo site", err, http.StatusNotFound,
			"verify id with gitlab_list_geo_sites; repair re-creates the OAuth application for the secondary site \u2014 must be run from the primary")
	}
	return toOutput(site), nil
}

// ListStatus retrieves the replication status of all Geo sites.
func ListStatus(ctx context.Context, client *gitlabclient.Client, in ListStatusInput) (ListStatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListStatusOutput{}, err
	}

	opts := &gl.ListStatusOfAllGeoSitesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	statuses, resp, err := client.GL().GeoSites.ListStatusOfAllGeoSites(opts, gl.WithContext(ctx))
	if err != nil {
		return ListStatusOutput{}, toolutil.WrapErrWithStatusHint("list geo site statuses", err, http.StatusForbidden,
			"requires admin access; status data is collected by the primary site \u2014 secondary sites may show stale data if replication is lagging")
	}

	out := ListStatusOutput{Statuses: make([]StatusOutput, 0, len(statuses))}
	for _, s := range statuses {
		out.Statuses = append(out.Statuses, toStatusOutput(s))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// GetStatus retrieves the replication status of a specific Geo site.
func GetStatus(ctx context.Context, client *gitlabclient.Client, in IDInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}
	if in.ID == 0 {
		return StatusOutput{}, toolutil.ErrFieldRequired("id")
	}

	status, _, err := client.GL().GeoSites.GetStatusOfGeoSite(in.ID, gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, toolutil.WrapErrWithStatusHint("get geo site status", err, http.StatusNotFound,
			"verify id with gitlab_list_geo_sites; the site must have reported status at least once for data to be available")
	}
	return toStatusOutput(status), nil
}

func toOutput(s *gl.GeoSite) Output {
	return Output{
		ID:                               s.ID,
		Name:                             s.Name,
		URL:                              s.URL,
		InternalURL:                      s.InternalURL,
		Primary:                          s.Primary,
		Enabled:                          s.Enabled,
		Current:                          s.Current,
		FilesMaxCapacity:                 s.FilesMaxCapacity,
		ReposMaxCapacity:                 s.ReposMaxCapacity,
		VerificationMaxCapacity:          s.VerificationMaxCapacity,
		ContainerRepositoriesMaxCapacity: s.ContainerRepositoriesMaxCapacity,
		SelectiveSyncType:                s.SelectiveSyncType,
		SelectiveSyncShards:              s.SelectiveSyncShards,
		SelectiveSyncNamespaceIDs:        s.SelectiveSyncNamespaceIDs,
		MinimumReverificationInterval:    s.MinimumReverificationInterval,
		SyncObjectStorage:                s.SyncObjectStorage,
		WebEditURL:                       s.WebEditURL,
	}
}

func toStatusOutput(s *gl.GeoSiteStatus) StatusOutput {
	return StatusOutput{
		GeoNodeID:                      s.GeoNodeID,
		Healthy:                        s.Healthy,
		Health:                         s.Health,
		HealthStatus:                   s.HealthStatus,
		MissingOAuthApplication:        s.MissingOAuthApplication,
		DBReplicationLagSeconds:        s.DBReplicationLagSeconds,
		ProjectsCount:                  s.ProjectsCount,
		LFSObjectsSyncedInPercentage:   s.LFSObjectsSyncedInPercentage,
		JobArtifactsSyncedInPercentage: s.JobArtifactsSyncedInPercentage,
		UploadsSyncedInPercentage:      s.UploadsSyncedInPercentage,
		Version:                        s.Version,
		Revision:                       s.Revision,
		StorageShardsMatch:             s.StorageShardsMatch,
		UpdatedAt:                      s.UpdatedAt,
	}
}
