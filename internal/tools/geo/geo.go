package geo

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// ListInput holds pagination and ordering parameters for listing Geo sites.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListStatusInput holds pagination and ordering parameters for listing all
// Geo site statuses.
type ListStatusInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	WebGeoReplicationDetailsURL      string   `json:"web_geo_replication_details_url,omitempty"`
	Links                            Links    `json:"_links"`
}

// Links mirrors gl.GeoSiteLinks: navigation links for a Geo site.
type Links struct {
	Self   string `json:"self,omitempty"`
	Status string `json:"status,omitempty"`
	Repair string `json:"repair,omitempty"`
}

// StatusLinks mirrors gl.GeoSiteStatusLink: navigation links for a Geo site status.
type StatusLinks struct {
	Self string `json:"self,omitempty"`
	Site string `json:"site,omitempty"`
}

// ListOutput represents a paginated list of Geo sites.
type ListOutput struct {
	toolutil.HintableOutput
	Sites      []Output                  `json:"sites"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// StatusOutput represents the full replication status of a Geo site,
// mirroring gl.GeoSiteStatus field-for-field.
type StatusOutput struct {
	toolutil.HintableOutput
	GeoNodeID                                       int64       `json:"geo_node_id"`
	ProjectsCount                                   int64       `json:"projects_count"`
	ContainerRepositoriesReplicationEnabled         bool        `json:"container_repositories_replication_enabled"`
	LFSObjectsCount                                 int64       `json:"lfs_objects_count"`
	LFSObjectsChecksumTotalCount                    int64       `json:"lfs_objects_checksum_total_count"`
	LFSObjectsChecksummedCount                      int64       `json:"lfs_objects_checksummed_count"`
	LFSObjectsChecksumFailedCount                   int64       `json:"lfs_objects_checksum_failed_count"`
	LFSObjectsSyncedCount                           int64       `json:"lfs_objects_synced_count"`
	LFSObjectsFailedCount                           int64       `json:"lfs_objects_failed_count"`
	LFSObjectsRegistryCount                         int64       `json:"lfs_objects_registry_count"`
	LFSObjectsVerificationTotalCount                int64       `json:"lfs_objects_verification_total_count"`
	LFSObjectsVerifiedCount                         int64       `json:"lfs_objects_verified_count"`
	LFSObjectsVerificationFailedCount               int64       `json:"lfs_objects_verification_failed_count"`
	MergeRequestDiffsCount                          int64       `json:"merge_request_diffs_count"`
	MergeRequestDiffsChecksumTotalCount             int64       `json:"merge_request_diffs_checksum_total_count"`
	MergeRequestDiffsChecksummedCount               int64       `json:"merge_request_diffs_checksummed_count"`
	MergeRequestDiffsChecksumFailedCount            int64       `json:"merge_request_diffs_checksum_failed_count"`
	MergeRequestDiffsSyncedCount                    int64       `json:"merge_request_diffs_synced_count"`
	MergeRequestDiffsFailedCount                    int64       `json:"merge_request_diffs_failed_count"`
	MergeRequestDiffsRegistryCount                  int64       `json:"merge_request_diffs_registry_count"`
	MergeRequestDiffsVerificationTotalCount         int64       `json:"merge_request_diffs_verification_total_count"`
	MergeRequestDiffsVerifiedCount                  int64       `json:"merge_request_diffs_verified_count"`
	MergeRequestDiffsVerificationFailedCount        int64       `json:"merge_request_diffs_verification_failed_count"`
	PackageFilesCount                               int64       `json:"package_files_count"`
	PackageFilesChecksumTotalCount                  int64       `json:"package_files_checksum_total_count"`
	PackageFilesChecksummedCount                    int64       `json:"package_files_checksummed_count"`
	PackageFilesChecksumFailedCount                 int64       `json:"package_files_checksum_failed_count"`
	PackageFilesSyncedCount                         int64       `json:"package_files_synced_count"`
	PackageFilesFailedCount                         int64       `json:"package_files_failed_count"`
	PackageFilesRegistryCount                       int64       `json:"package_files_registry_count"`
	PackageFilesVerificationTotalCount              int64       `json:"package_files_verification_total_count"`
	PackageFilesVerifiedCount                       int64       `json:"package_files_verified_count"`
	PackageFilesVerificationFailedCount             int64       `json:"package_files_verification_failed_count"`
	TerraformStateVersionsCount                     int64       `json:"terraform_state_versions_count"`
	TerraformStateVersionsChecksumTotalCount        int64       `json:"terraform_state_versions_checksum_total_count"`
	TerraformStateVersionsChecksummedCount          int64       `json:"terraform_state_versions_checksummed_count"`
	TerraformStateVersionsChecksumFailedCount       int64       `json:"terraform_state_versions_checksum_failed_count"`
	TerraformStateVersionsSyncedCount               int64       `json:"terraform_state_versions_synced_count"`
	TerraformStateVersionsFailedCount               int64       `json:"terraform_state_versions_failed_count"`
	TerraformStateVersionsRegistryCount             int64       `json:"terraform_state_versions_registry_count"`
	TerraformStateVersionsVerificationTotalCount    int64       `json:"terraform_state_versions_verification_total_count"`
	TerraformStateVersionsVerifiedCount             int64       `json:"terraform_state_versions_verified_count"`
	TerraformStateVersionsVerificationFailedCount   int64       `json:"terraform_state_versions_verification_failed_count"`
	SnippetRepositoriesCount                        int64       `json:"snippet_repositories_count"`
	SnippetRepositoriesChecksumTotalCount           int64       `json:"snippet_repositories_checksum_total_count"`
	SnippetRepositoriesChecksummedCount             int64       `json:"snippet_repositories_checksummed_count"`
	SnippetRepositoriesChecksumFailedCount          int64       `json:"snippet_repositories_checksum_failed_count"`
	SnippetRepositoriesSyncedCount                  int64       `json:"snippet_repositories_synced_count"`
	SnippetRepositoriesFailedCount                  int64       `json:"snippet_repositories_failed_count"`
	SnippetRepositoriesRegistryCount                int64       `json:"snippet_repositories_registry_count"`
	SnippetRepositoriesVerificationTotalCount       int64       `json:"snippet_repositories_verification_total_count"`
	SnippetRepositoriesVerifiedCount                int64       `json:"snippet_repositories_verified_count"`
	SnippetRepositoriesVerificationFailedCount      int64       `json:"snippet_repositories_verification_failed_count"`
	GroupWikiRepositoriesCount                      int64       `json:"group_wiki_repositories_count"`
	GroupWikiRepositoriesChecksumTotalCount         int64       `json:"group_wiki_repositories_checksum_total_count"`
	GroupWikiRepositoriesChecksummedCount           int64       `json:"group_wiki_repositories_checksummed_count"`
	GroupWikiRepositoriesChecksumFailedCount        int64       `json:"group_wiki_repositories_checksum_failed_count"`
	GroupWikiRepositoriesSyncedCount                int64       `json:"group_wiki_repositories_synced_count"`
	GroupWikiRepositoriesFailedCount                int64       `json:"group_wiki_repositories_failed_count"`
	GroupWikiRepositoriesRegistryCount              int64       `json:"group_wiki_repositories_registry_count"`
	GroupWikiRepositoriesVerificationTotalCount     int64       `json:"group_wiki_repositories_verification_total_count"`
	GroupWikiRepositoriesVerifiedCount              int64       `json:"group_wiki_repositories_verified_count"`
	GroupWikiRepositoriesVerificationFailedCount    int64       `json:"group_wiki_repositories_verification_failed_count"`
	PipelineArtifactsCount                          int64       `json:"pipeline_artifacts_count"`
	PipelineArtifactsChecksumTotalCount             int64       `json:"pipeline_artifacts_checksum_total_count"`
	PipelineArtifactsChecksummedCount               int64       `json:"pipeline_artifacts_checksummed_count"`
	PipelineArtifactsChecksumFailedCount            int64       `json:"pipeline_artifacts_checksum_failed_count"`
	PipelineArtifactsSyncedCount                    int64       `json:"pipeline_artifacts_synced_count"`
	PipelineArtifactsFailedCount                    int64       `json:"pipeline_artifacts_failed_count"`
	PipelineArtifactsRegistryCount                  int64       `json:"pipeline_artifacts_registry_count"`
	PipelineArtifactsVerificationTotalCount         int64       `json:"pipeline_artifacts_verification_total_count"`
	PipelineArtifactsVerifiedCount                  int64       `json:"pipeline_artifacts_verified_count"`
	PipelineArtifactsVerificationFailedCount        int64       `json:"pipeline_artifacts_verification_failed_count"`
	PagesDeploymentsCount                           int64       `json:"pages_deployments_count"`
	PagesDeploymentsChecksumTotalCount              int64       `json:"pages_deployments_checksum_total_count"`
	PagesDeploymentsChecksummedCount                int64       `json:"pages_deployments_checksummed_count"`
	PagesDeploymentsChecksumFailedCount             int64       `json:"pages_deployments_checksum_failed_count"`
	PagesDeploymentsSyncedCount                     int64       `json:"pages_deployments_synced_count"`
	PagesDeploymentsFailedCount                     int64       `json:"pages_deployments_failed_count"`
	PagesDeploymentsRegistryCount                   int64       `json:"pages_deployments_registry_count"`
	PagesDeploymentsVerificationTotalCount          int64       `json:"pages_deployments_verification_total_count"`
	PagesDeploymentsVerifiedCount                   int64       `json:"pages_deployments_verified_count"`
	PagesDeploymentsVerificationFailedCount         int64       `json:"pages_deployments_verification_failed_count"`
	UploadsCount                                    int64       `json:"uploads_count"`
	UploadsChecksumTotalCount                       int64       `json:"uploads_checksum_total_count"`
	UploadsChecksummedCount                         int64       `json:"uploads_checksummed_count"`
	UploadsChecksumFailedCount                      int64       `json:"uploads_checksum_failed_count"`
	UploadsSyncedCount                              int64       `json:"uploads_synced_count"`
	UploadsFailedCount                              int64       `json:"uploads_failed_count"`
	UploadsRegistryCount                            int64       `json:"uploads_registry_count"`
	UploadsVerificationTotalCount                   int64       `json:"uploads_verification_total_count"`
	UploadsVerifiedCount                            int64       `json:"uploads_verified_count"`
	UploadsVerificationFailedCount                  int64       `json:"uploads_verification_failed_count"`
	JobArtifactsCount                               int64       `json:"job_artifacts_count"`
	JobArtifactsChecksumTotalCount                  int64       `json:"job_artifacts_checksum_total_count"`
	JobArtifactsChecksummedCount                    int64       `json:"job_artifacts_checksummed_count"`
	JobArtifactsChecksumFailedCount                 int64       `json:"job_artifacts_checksum_failed_count"`
	JobArtifactsSyncedCount                         int64       `json:"job_artifacts_synced_count"`
	JobArtifactsFailedCount                         int64       `json:"job_artifacts_failed_count"`
	JobArtifactsRegistryCount                       int64       `json:"job_artifacts_registry_count"`
	JobArtifactsVerificationTotalCount              int64       `json:"job_artifacts_verification_total_count"`
	JobArtifactsVerifiedCount                       int64       `json:"job_artifacts_verified_count"`
	JobArtifactsVerificationFailedCount             int64       `json:"job_artifacts_verification_failed_count"`
	CISecureFilesCount                              int64       `json:"ci_secure_files_count"`
	CISecureFilesChecksumTotalCount                 int64       `json:"ci_secure_files_checksum_total_count"`
	CISecureFilesChecksummedCount                   int64       `json:"ci_secure_files_checksummed_count"`
	CISecureFilesChecksumFailedCount                int64       `json:"ci_secure_files_checksum_failed_count"`
	CISecureFilesSyncedCount                        int64       `json:"ci_secure_files_synced_count"`
	CISecureFilesFailedCount                        int64       `json:"ci_secure_files_failed_count"`
	CISecureFilesRegistryCount                      int64       `json:"ci_secure_files_registry_count"`
	CISecureFilesVerificationTotalCount             int64       `json:"ci_secure_files_verification_total_count"`
	CISecureFilesVerifiedCount                      int64       `json:"ci_secure_files_verified_count"`
	CISecureFilesVerificationFailedCount            int64       `json:"ci_secure_files_verification_failed_count"`
	ContainerRepositoriesCount                      int64       `json:"container_repositories_count"`
	ContainerRepositoriesChecksumTotalCount         int64       `json:"container_repositories_checksum_total_count"`
	ContainerRepositoriesChecksummedCount           int64       `json:"container_repositories_checksummed_count"`
	ContainerRepositoriesChecksumFailedCount        int64       `json:"container_repositories_checksum_failed_count"`
	ContainerRepositoriesSyncedCount                int64       `json:"container_repositories_synced_count"`
	ContainerRepositoriesFailedCount                int64       `json:"container_repositories_failed_count"`
	ContainerRepositoriesRegistryCount              int64       `json:"container_repositories_registry_count"`
	ContainerRepositoriesVerificationTotalCount     int64       `json:"container_repositories_verification_total_count"`
	ContainerRepositoriesVerifiedCount              int64       `json:"container_repositories_verified_count"`
	ContainerRepositoriesVerificationFailedCount    int64       `json:"container_repositories_verification_failed_count"`
	DependencyProxyBlobsCount                       int64       `json:"dependency_proxy_blobs_count"`
	DependencyProxyBlobsChecksumTotalCount          int64       `json:"dependency_proxy_blobs_checksum_total_count"`
	DependencyProxyBlobsChecksummedCount            int64       `json:"dependency_proxy_blobs_checksummed_count"`
	DependencyProxyBlobsChecksumFailedCount         int64       `json:"dependency_proxy_blobs_checksum_failed_count"`
	DependencyProxyBlobsSyncedCount                 int64       `json:"dependency_proxy_blobs_synced_count"`
	DependencyProxyBlobsFailedCount                 int64       `json:"dependency_proxy_blobs_failed_count"`
	DependencyProxyBlobsRegistryCount               int64       `json:"dependency_proxy_blobs_registry_count"`
	DependencyProxyBlobsVerificationTotalCount      int64       `json:"dependency_proxy_blobs_verification_total_count"`
	DependencyProxyBlobsVerifiedCount               int64       `json:"dependency_proxy_blobs_verified_count"`
	DependencyProxyBlobsVerificationFailedCount     int64       `json:"dependency_proxy_blobs_verification_failed_count"`
	DependencyProxyManifestsCount                   int64       `json:"dependency_proxy_manifests_count"`
	DependencyProxyManifestsChecksumTotalCount      int64       `json:"dependency_proxy_manifests_checksum_total_count"`
	DependencyProxyManifestsChecksummedCount        int64       `json:"dependency_proxy_manifests_checksummed_count"`
	DependencyProxyManifestsChecksumFailedCount     int64       `json:"dependency_proxy_manifests_checksum_failed_count"`
	DependencyProxyManifestsSyncedCount             int64       `json:"dependency_proxy_manifests_synced_count"`
	DependencyProxyManifestsFailedCount             int64       `json:"dependency_proxy_manifests_failed_count"`
	DependencyProxyManifestsRegistryCount           int64       `json:"dependency_proxy_manifests_registry_count"`
	DependencyProxyManifestsVerificationTotalCount  int64       `json:"dependency_proxy_manifests_verification_total_count"`
	DependencyProxyManifestsVerifiedCount           int64       `json:"dependency_proxy_manifests_verified_count"`
	DependencyProxyManifestsVerificationFailedCount int64       `json:"dependency_proxy_manifests_verification_failed_count"`
	ProjectWikiRepositoriesCount                    int64       `json:"project_wiki_repositories_count"`
	ProjectWikiRepositoriesChecksumTotalCount       int64       `json:"project_wiki_repositories_checksum_total_count"`
	ProjectWikiRepositoriesChecksummedCount         int64       `json:"project_wiki_repositories_checksummed_count"`
	ProjectWikiRepositoriesChecksumFailedCount      int64       `json:"project_wiki_repositories_checksum_failed_count"`
	ProjectWikiRepositoriesSyncedCount              int64       `json:"project_wiki_repositories_synced_count"`
	ProjectWikiRepositoriesFailedCount              int64       `json:"project_wiki_repositories_failed_count"`
	ProjectWikiRepositoriesRegistryCount            int64       `json:"project_wiki_repositories_registry_count"`
	ProjectWikiRepositoriesVerificationTotalCount   int64       `json:"project_wiki_repositories_verification_total_count"`
	ProjectWikiRepositoriesVerifiedCount            int64       `json:"project_wiki_repositories_verified_count"`
	ProjectWikiRepositoriesVerificationFailedCount  int64       `json:"project_wiki_repositories_verification_failed_count"`
	GitFetchEventCountWeekly                        int64       `json:"git_fetch_event_count_weekly"`
	GitPushEventCountWeekly                         int64       `json:"git_push_event_count_weekly"`
	ProxyRemoteRequestsEventCountWeekly             int64       `json:"proxy_remote_requests_event_count_weekly"`
	ProxyLocalRequestsEventCountWeekly              int64       `json:"proxy_local_requests_event_count_weekly"`
	RepositoriesCheckedInPercentage                 string      `json:"repositories_checked_in_percentage"`
	ReplicationSlotsUsedInPercentage                string      `json:"replication_slots_used_in_percentage"`
	LFSObjectsSyncedInPercentage                    string      `json:"lfs_objects_synced_in_percentage"`
	LFSObjectsVerifiedInPercentage                  string      `json:"lfs_objects_verified_in_percentage"`
	MergeRequestDiffsSyncedInPercentage             string      `json:"merge_request_diffs_synced_in_percentage"`
	MergeRequestDiffsVerifiedInPercentage           string      `json:"merge_request_diffs_verified_in_percentage"`
	PackageFilesSyncedInPercentage                  string      `json:"package_files_synced_in_percentage"`
	PackageFilesVerifiedInPercentage                string      `json:"package_files_verified_in_percentage"`
	TerraformStateVersionsSyncedInPercentage        string      `json:"terraform_state_versions_synced_in_percentage"`
	TerraformStateVersionsVerifiedInPercentage      string      `json:"terraform_state_versions_verified_in_percentage"`
	SnippetRepositoriesSyncedInPercentage           string      `json:"snippet_repositories_synced_in_percentage"`
	SnippetRepositoriesVerifiedInPercentage         string      `json:"snippet_repositories_verified_in_percentage"`
	GroupWikiRepositoriesSyncedInPercentage         string      `json:"group_wiki_repositories_synced_in_percentage"`
	GroupWikiRepositoriesVerifiedInPercentage       string      `json:"group_wiki_repositories_verified_in_percentage"`
	PipelineArtifactsSyncedInPercentage             string      `json:"pipeline_artifacts_synced_in_percentage"`
	PipelineArtifactsVerifiedInPercentage           string      `json:"pipeline_artifacts_verified_in_percentage"`
	PagesDeploymentsSyncedInPercentage              string      `json:"pages_deployments_synced_in_percentage"`
	PagesDeploymentsVerifiedInPercentage            string      `json:"pages_deployments_verified_in_percentage"`
	UploadsSyncedInPercentage                       string      `json:"uploads_synced_in_percentage"`
	UploadsVerifiedInPercentage                     string      `json:"uploads_verified_in_percentage"`
	JobArtifactsSyncedInPercentage                  string      `json:"job_artifacts_synced_in_percentage"`
	JobArtifactsVerifiedInPercentage                string      `json:"job_artifacts_verified_in_percentage"`
	CISecureFilesSyncedInPercentage                 string      `json:"ci_secure_files_synced_in_percentage"`
	CISecureFilesVerifiedInPercentage               string      `json:"ci_secure_files_verified_in_percentage"`
	ContainerRepositoriesSyncedInPercentage         string      `json:"container_repositories_synced_in_percentage"`
	ContainerRepositoriesVerifiedInPercentage       string      `json:"container_repositories_verified_in_percentage"`
	DependencyProxyBlobsSyncedInPercentage          string      `json:"dependency_proxy_blobs_synced_in_percentage"`
	DependencyProxyBlobsVerifiedInPercentage        string      `json:"dependency_proxy_blobs_verified_in_percentage"`
	DependencyProxyManifestsSyncedInPercentage      string      `json:"dependency_proxy_manifests_synced_in_percentage"`
	DependencyProxyManifestsVerifiedInPercentage    string      `json:"dependency_proxy_manifests_verified_in_percentage"`
	ProjectWikiRepositoriesSyncedInPercentage       string      `json:"project_wiki_repositories_synced_in_percentage"`
	ProjectWikiRepositoriesVerifiedInPercentage     string      `json:"project_wiki_repositories_verified_in_percentage"`
	ReplicationSlotsCount                           int64       `json:"replication_slots_count"`
	ReplicationSlotsUsedCount                       int64       `json:"replication_slots_used_count"`
	Healthy                                         bool        `json:"healthy"`
	Health                                          string      `json:"health"`
	HealthStatus                                    string      `json:"health_status"`
	MissingOAuthApplication                         bool        `json:"missing_oauth_application"`
	DBReplicationLagSeconds                         int64       `json:"db_replication_lag_seconds"`
	ReplicationSlotsMaxRetainedWalBytes             int64       `json:"replication_slots_max_retained_wal_bytes"`
	RepositoriesCheckedCount                        int64       `json:"repositories_checked_count"`
	RepositoriesCheckedFailedCount                  int64       `json:"repositories_checked_failed_count"`
	LastEventID                                     int64       `json:"last_event_id"`
	LastEventTimestamp                              int64       `json:"last_event_timestamp"`
	CursorLastEventID                               int64       `json:"cursor_last_event_id"`
	CursorLastEventTimestamp                        int64       `json:"cursor_last_event_timestamp"`
	LastSuccessfulStatusCheckTimestamp              int64       `json:"last_successful_status_check_timestamp"`
	Version                                         string      `json:"version"`
	Revision                                        string      `json:"revision"`
	SelectiveSyncType                               string      `json:"selective_sync_type"`
	Namespaces                                      []string    `json:"namespaces,omitempty"`
	UpdatedAt                                       time.Time   `json:"updated_at"`
	StorageShardsMatch                              bool        `json:"storage_shards_match"`
	Links                                           StatusLinks `json:"_links"`
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

	opts := &gl.ListGeoSitesOptions{}
	applyGeoListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput, in.OrderBy, in.Sort)
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
	if site == nil {
		return Output{
			HintableOutput: toolutil.HintableOutput{NextSteps: []string{
				"Geo repair was accepted but GitLab returned an empty response; call action 'get' to refresh the Geo site.",
			}},
			ID: in.ID,
		}, nil
	}
	return toOutput(site), nil
}

// ListStatus retrieves the replication status of all Geo sites.
func ListStatus(ctx context.Context, client *gitlabclient.Client, in ListStatusInput) (ListStatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListStatusOutput{}, err
	}

	opts := &gl.ListStatusOfAllGeoSitesOptions{}
	applyGeoListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput, in.OrderBy, in.Sort)
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

// applyGeoListOptions wires offset/keyset pagination plus order_by/sort onto a
// gl.ListOptions for the Geo list endpoints, which both order by keyset.
func applyGeoListOptions(opts *gl.ListOptions, page toolutil.PaginationInput, keyset toolutil.KeysetPaginationInput, orderBy, sort string) {
	toolutil.ApplyListOptions(opts, page, keyset)
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
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
		WebGeoReplicationDetailsURL:      s.WebGeoReplicationDetailsURL,
		Links: Links{
			Self:   s.Links.Self,
			Status: s.Links.Status,
			Repair: s.Links.Repair,
		},
	}
}

func toStatusOutput(s *gl.GeoSiteStatus) StatusOutput {
	return StatusOutput{
		GeoNodeID:                                     s.GeoNodeID,
		ProjectsCount:                                 s.ProjectsCount,
		ContainerRepositoriesReplicationEnabled:       s.ContainerRepositoriesReplicationEnabled,
		LFSObjectsCount:                               s.LFSObjectsCount,
		LFSObjectsChecksumTotalCount:                  s.LFSObjectsChecksumTotalCount,
		LFSObjectsChecksummedCount:                    s.LFSObjectsChecksummedCount,
		LFSObjectsChecksumFailedCount:                 s.LFSObjectsChecksumFailedCount,
		LFSObjectsSyncedCount:                         s.LFSObjectsSyncedCount,
		LFSObjectsFailedCount:                         s.LFSObjectsFailedCount,
		LFSObjectsRegistryCount:                       s.LFSObjectsRegistryCount,
		LFSObjectsVerificationTotalCount:              s.LFSObjectsVerificationTotalCount,
		LFSObjectsVerifiedCount:                       s.LFSObjectsVerifiedCount,
		LFSObjectsVerificationFailedCount:             s.LFSObjectsVerificationFailedCount,
		MergeRequestDiffsCount:                        s.MergeRequestDiffsCount,
		MergeRequestDiffsChecksumTotalCount:           s.MergeRequestDiffsChecksumTotalCount,
		MergeRequestDiffsChecksummedCount:             s.MergeRequestDiffsChecksummedCount,
		MergeRequestDiffsChecksumFailedCount:          s.MergeRequestDiffsChecksumFailedCount,
		MergeRequestDiffsSyncedCount:                  s.MergeRequestDiffsSyncedCount,
		MergeRequestDiffsFailedCount:                  s.MergeRequestDiffsFailedCount,
		MergeRequestDiffsRegistryCount:                s.MergeRequestDiffsRegistryCount,
		MergeRequestDiffsVerificationTotalCount:       s.MergeRequestDiffsVerificationTotalCount,
		MergeRequestDiffsVerifiedCount:                s.MergeRequestDiffsVerifiedCount,
		MergeRequestDiffsVerificationFailedCount:      s.MergeRequestDiffsVerificationFailedCount,
		PackageFilesCount:                             s.PackageFilesCount,
		PackageFilesChecksumTotalCount:                s.PackageFilesChecksumTotalCount,
		PackageFilesChecksummedCount:                  s.PackageFilesChecksummedCount,
		PackageFilesChecksumFailedCount:               s.PackageFilesChecksumFailedCount,
		PackageFilesSyncedCount:                       s.PackageFilesSyncedCount,
		PackageFilesFailedCount:                       s.PackageFilesFailedCount,
		PackageFilesRegistryCount:                     s.PackageFilesRegistryCount,
		PackageFilesVerificationTotalCount:            s.PackageFilesVerificationTotalCount,
		PackageFilesVerifiedCount:                     s.PackageFilesVerifiedCount,
		PackageFilesVerificationFailedCount:           s.PackageFilesVerificationFailedCount,
		TerraformStateVersionsCount:                   s.TerraformStateVersionsCount,
		TerraformStateVersionsChecksumTotalCount:      s.TerraformStateVersionsChecksumTotalCount,
		TerraformStateVersionsChecksummedCount:        s.TerraformStateVersionsChecksummedCount,
		TerraformStateVersionsChecksumFailedCount:     s.TerraformStateVersionsChecksumFailedCount,
		TerraformStateVersionsSyncedCount:             s.TerraformStateVersionsSyncedCount,
		TerraformStateVersionsFailedCount:             s.TerraformStateVersionsFailedCount,
		TerraformStateVersionsRegistryCount:           s.TerraformStateVersionsRegistryCount,
		TerraformStateVersionsVerificationTotalCount:  s.TerraformStateVersionsVerificationTotalCount,
		TerraformStateVersionsVerifiedCount:           s.TerraformStateVersionsVerifiedCount,
		TerraformStateVersionsVerificationFailedCount: s.TerraformStateVersionsVerificationFailedCount,
		SnippetRepositoriesCount:                      s.SnippetRepositoriesCount,
		SnippetRepositoriesChecksumTotalCount:         s.SnippetRepositoriesChecksumTotalCount,
		SnippetRepositoriesChecksummedCount:           s.SnippetRepositoriesChecksummedCount,
		SnippetRepositoriesChecksumFailedCount:        s.SnippetRepositoriesChecksumFailedCount,
		SnippetRepositoriesSyncedCount:                s.SnippetRepositoriesSyncedCount,
		SnippetRepositoriesFailedCount:                s.SnippetRepositoriesFailedCount,
		SnippetRepositoriesRegistryCount:              s.SnippetRepositoriesRegistryCount,
		SnippetRepositoriesVerificationTotalCount:     s.SnippetRepositoriesVerificationTotalCount,
		SnippetRepositoriesVerifiedCount:              s.SnippetRepositoriesVerifiedCount,
		SnippetRepositoriesVerificationFailedCount:    s.SnippetRepositoriesVerificationFailedCount,
		GroupWikiRepositoriesCount:                    s.GroupWikiRepositoriesCount,
		GroupWikiRepositoriesChecksumTotalCount:       s.GroupWikiRepositoriesChecksumTotalCount,
		GroupWikiRepositoriesChecksummedCount:         s.GroupWikiRepositoriesChecksummedCount,
		GroupWikiRepositoriesChecksumFailedCount:      s.GroupWikiRepositoriesChecksumFailedCount,
		GroupWikiRepositoriesSyncedCount:              s.GroupWikiRepositoriesSyncedCount,
		GroupWikiRepositoriesFailedCount:              s.GroupWikiRepositoriesFailedCount,
		GroupWikiRepositoriesRegistryCount:            s.GroupWikiRepositoriesRegistryCount,
		// SDK field name carries an upstream typo (GrupWiki...); JSON tag is correct.
		GroupWikiRepositoriesVerificationTotalCount:     s.GrupWikiRepositoriesVerificationTotalCount,
		GroupWikiRepositoriesVerifiedCount:              s.GroupWikiRepositoriesVerifiedCount,
		GroupWikiRepositoriesVerificationFailedCount:    s.GroupWikiRepositoriesVerificationFailedCount,
		PipelineArtifactsCount:                          s.PipelineArtifactsCount,
		PipelineArtifactsChecksumTotalCount:             s.PipelineArtifactsChecksumTotalCount,
		PipelineArtifactsChecksummedCount:               s.PipelineArtifactsChecksummedCount,
		PipelineArtifactsChecksumFailedCount:            s.PipelineArtifactsChecksumFailedCount,
		PipelineArtifactsSyncedCount:                    s.PipelineArtifactsSyncedCount,
		PipelineArtifactsFailedCount:                    s.PipelineArtifactsFailedCount,
		PipelineArtifactsRegistryCount:                  s.PipelineArtifactsRegistryCount,
		PipelineArtifactsVerificationTotalCount:         s.PipelineArtifactsVerificationTotalCount,
		PipelineArtifactsVerifiedCount:                  s.PipelineArtifactsVerifiedCount,
		PipelineArtifactsVerificationFailedCount:        s.PipelineArtifactsVerificationFailedCount,
		PagesDeploymentsCount:                           s.PagesDeploymentsCount,
		PagesDeploymentsChecksumTotalCount:              s.PagesDeploymentsChecksumTotalCount,
		PagesDeploymentsChecksummedCount:                s.PagesDeploymentsChecksummedCount,
		PagesDeploymentsChecksumFailedCount:             s.PagesDeploymentsChecksumFailedCount,
		PagesDeploymentsSyncedCount:                     s.PagesDeploymentsSyncedCount,
		PagesDeploymentsFailedCount:                     s.PagesDeploymentsFailedCount,
		PagesDeploymentsRegistryCount:                   s.PagesDeploymentsRegistryCount,
		PagesDeploymentsVerificationTotalCount:          s.PagesDeploymentsVerificationTotalCount,
		PagesDeploymentsVerifiedCount:                   s.PagesDeploymentsVerifiedCount,
		PagesDeploymentsVerificationFailedCount:         s.PagesDeploymentsVerificationFailedCount,
		UploadsCount:                                    s.UploadsCount,
		UploadsChecksumTotalCount:                       s.UploadsChecksumTotalCount,
		UploadsChecksummedCount:                         s.UploadsChecksummedCount,
		UploadsChecksumFailedCount:                      s.UploadsChecksumFailedCount,
		UploadsSyncedCount:                              s.UploadsSyncedCount,
		UploadsFailedCount:                              s.UploadsFailedCount,
		UploadsRegistryCount:                            s.UploadsRegistryCount,
		UploadsVerificationTotalCount:                   s.UploadsVerificationTotalCount,
		UploadsVerifiedCount:                            s.UploadsVerifiedCount,
		UploadsVerificationFailedCount:                  s.UploadsVerificationFailedCount,
		JobArtifactsCount:                               s.JobArtifactsCount,
		JobArtifactsChecksumTotalCount:                  s.JobArtifactsChecksumTotalCount,
		JobArtifactsChecksummedCount:                    s.JobArtifactsChecksummedCount,
		JobArtifactsChecksumFailedCount:                 s.JobArtifactsChecksumFailedCount,
		JobArtifactsSyncedCount:                         s.JobArtifactsSyncedCount,
		JobArtifactsFailedCount:                         s.JobArtifactsFailedCount,
		JobArtifactsRegistryCount:                       s.JobArtifactsRegistryCount,
		JobArtifactsVerificationTotalCount:              s.JobArtifactsVerificationTotalCount,
		JobArtifactsVerifiedCount:                       s.JobArtifactsVerifiedCount,
		JobArtifactsVerificationFailedCount:             s.JobArtifactsVerificationFailedCount,
		CISecureFilesCount:                              s.CISecureFilesCount,
		CISecureFilesChecksumTotalCount:                 s.CISecureFilesChecksumTotalCount,
		CISecureFilesChecksummedCount:                   s.CISecureFilesChecksummedCount,
		CISecureFilesChecksumFailedCount:                s.CISecureFilesChecksumFailedCount,
		CISecureFilesSyncedCount:                        s.CISecureFilesSyncedCount,
		CISecureFilesFailedCount:                        s.CISecureFilesFailedCount,
		CISecureFilesRegistryCount:                      s.CISecureFilesRegistryCount,
		CISecureFilesVerificationTotalCount:             s.CISecureFilesVerificationTotalCount,
		CISecureFilesVerifiedCount:                      s.CISecureFilesVerifiedCount,
		CISecureFilesVerificationFailedCount:            s.CISecureFilesVerificationFailedCount,
		ContainerRepositoriesCount:                      s.ContainerRepositoriesCount,
		ContainerRepositoriesChecksumTotalCount:         s.ContainerRepositoriesChecksumTotalCount,
		ContainerRepositoriesChecksummedCount:           s.ContainerRepositoriesChecksummedCount,
		ContainerRepositoriesChecksumFailedCount:        s.ContainerRepositoriesChecksumFailedCount,
		ContainerRepositoriesSyncedCount:                s.ContainerRepositoriesSyncedCount,
		ContainerRepositoriesFailedCount:                s.ContainerRepositoriesFailedCount,
		ContainerRepositoriesRegistryCount:              s.ContainerRepositoriesRegistryCount,
		ContainerRepositoriesVerificationTotalCount:     s.ContainerRepositoriesVerificationTotalCount,
		ContainerRepositoriesVerifiedCount:              s.ContainerRepositoriesVerifiedCount,
		ContainerRepositoriesVerificationFailedCount:    s.ContainerRepositoriesVerificationFailedCount,
		DependencyProxyBlobsCount:                       s.DependencyProxyBlobsCount,
		DependencyProxyBlobsChecksumTotalCount:          s.DependencyProxyBlobsChecksumTotalCount,
		DependencyProxyBlobsChecksummedCount:            s.DependencyProxyBlobsChecksummedCount,
		DependencyProxyBlobsChecksumFailedCount:         s.DependencyProxyBlobsChecksumFailedCount,
		DependencyProxyBlobsSyncedCount:                 s.DependencyProxyBlobsSyncedCount,
		DependencyProxyBlobsFailedCount:                 s.DependencyProxyBlobsFailedCount,
		DependencyProxyBlobsRegistryCount:               s.DependencyProxyBlobsRegistryCount,
		DependencyProxyBlobsVerificationTotalCount:      s.DependencyProxyBlobsVerificationTotalCount,
		DependencyProxyBlobsVerifiedCount:               s.DependencyProxyBlobsVerifiedCount,
		DependencyProxyBlobsVerificationFailedCount:     s.DependencyProxyBlobsVerificationFailedCount,
		DependencyProxyManifestsCount:                   s.DependencyProxyManifestsCount,
		DependencyProxyManifestsChecksumTotalCount:      s.DependencyProxyManifestsChecksumTotalCount,
		DependencyProxyManifestsChecksummedCount:        s.DependencyProxyManifestsChecksummedCount,
		DependencyProxyManifestsChecksumFailedCount:     s.DependencyProxyManifestsChecksumFailedCount,
		DependencyProxyManifestsSyncedCount:             s.DependencyProxyManifestsSyncedCount,
		DependencyProxyManifestsFailedCount:             s.DependencyProxyManifestsFailedCount,
		DependencyProxyManifestsRegistryCount:           s.DependencyProxyManifestsRegistryCount,
		DependencyProxyManifestsVerificationTotalCount:  s.DependencyProxyManifestsVerificationTotalCount,
		DependencyProxyManifestsVerifiedCount:           s.DependencyProxyManifestsVerifiedCount,
		DependencyProxyManifestsVerificationFailedCount: s.DependencyProxyManifestsVerificationFailedCount,
		ProjectWikiRepositoriesCount:                    s.ProjectWikiRepositoriesCount,
		ProjectWikiRepositoriesChecksumTotalCount:       s.ProjectWikiRepositoriesChecksumTotalCount,
		ProjectWikiRepositoriesChecksummedCount:         s.ProjectWikiRepositoriesChecksummedCount,
		ProjectWikiRepositoriesChecksumFailedCount:      s.ProjectWikiRepositoriesChecksumFailedCount,
		ProjectWikiRepositoriesSyncedCount:              s.ProjectWikiRepositoriesSyncedCount,
		ProjectWikiRepositoriesFailedCount:              s.ProjectWikiRepositoriesFailedCount,
		ProjectWikiRepositoriesRegistryCount:            s.ProjectWikiRepositoriesRegistryCount,
		ProjectWikiRepositoriesVerificationTotalCount:   s.ProjectWikiRepositoriesVerificationTotalCount,
		ProjectWikiRepositoriesVerifiedCount:            s.ProjectWikiRepositoriesVerifiedCount,
		ProjectWikiRepositoriesVerificationFailedCount:  s.ProjectWikiRepositoriesVerificationFailedCount,
		GitFetchEventCountWeekly:                        s.GitFetchEventCountWeekly,
		GitPushEventCountWeekly:                         s.GitPushEventCountWeekly,
		ProxyRemoteRequestsEventCountWeekly:             s.ProxyRemoteRequestsEventCountWeekly,
		ProxyLocalRequestsEventCountWeekly:              s.ProxyLocalRequestsEventCountWeekly,
		RepositoriesCheckedInPercentage:                 s.RepositoriesCheckedInPercentage,
		ReplicationSlotsUsedInPercentage:                s.ReplicationSlotsUsedInPercentage,
		LFSObjectsSyncedInPercentage:                    s.LFSObjectsSyncedInPercentage,
		LFSObjectsVerifiedInPercentage:                  s.LFSObjectsVerifiedInPercentage,
		MergeRequestDiffsSyncedInPercentage:             s.MergeRequestDiffsSyncedInPercentage,
		MergeRequestDiffsVerifiedInPercentage:           s.MergeRequestDiffsVerifiedInPercentage,
		PackageFilesSyncedInPercentage:                  s.PackageFilesSyncedInPercentage,
		PackageFilesVerifiedInPercentage:                s.PackageFilesVerifiedInPercentage,
		TerraformStateVersionsSyncedInPercentage:        s.TerraformStateVersionsSyncedInPercentage,
		TerraformStateVersionsVerifiedInPercentage:      s.TerraformStateVersionsVerifiedInPercentage,
		SnippetRepositoriesSyncedInPercentage:           s.SnippetRepositoriesSyncedInPercentage,
		SnippetRepositoriesVerifiedInPercentage:         s.SnippetRepositoriesVerifiedInPercentage,
		GroupWikiRepositoriesSyncedInPercentage:         s.GroupWikiRepositoriesSyncedInPercentage,
		GroupWikiRepositoriesVerifiedInPercentage:       s.GroupWikiRepositoriesVerifiedInPercentage,
		PipelineArtifactsSyncedInPercentage:             s.PipelineArtifactsSyncedInPercentage,
		PipelineArtifactsVerifiedInPercentage:           s.PipelineArtifactsVerifiedInPercentage,
		PagesDeploymentsSyncedInPercentage:              s.PagesDeploymentsSyncedInPercentage,
		PagesDeploymentsVerifiedInPercentage:            s.PagesDeploymentsVerifiedInPercentage,
		UploadsSyncedInPercentage:                       s.UploadsSyncedInPercentage,
		UploadsVerifiedInPercentage:                     s.UploadsVerifiedInPercentage,
		JobArtifactsSyncedInPercentage:                  s.JobArtifactsSyncedInPercentage,
		JobArtifactsVerifiedInPercentage:                s.JobArtifactsVerifiedInPercentage,
		CISecureFilesSyncedInPercentage:                 s.CISecureFilesSyncedInPercentage,
		CISecureFilesVerifiedInPercentage:               s.CISecureFilesVerifiedInPercentage,
		ContainerRepositoriesSyncedInPercentage:         s.ContainerRepositoriesSyncedInPercentage,
		ContainerRepositoriesVerifiedInPercentage:       s.ContainerRepositoriesVerifiedInPercentage,
		DependencyProxyBlobsSyncedInPercentage:          s.DependencyProxyBlobsSyncedInPercentage,
		DependencyProxyBlobsVerifiedInPercentage:        s.DependencyProxyBlobsVerifiedInPercentage,
		DependencyProxyManifestsSyncedInPercentage:      s.DependencyProxyManifestsSyncedInPercentage,
		DependencyProxyManifestsVerifiedInPercentage:    s.DependencyProxyManifestsVerifiedInPercentage,
		ProjectWikiRepositoriesSyncedInPercentage:       s.ProjectWikiRepositoriesSyncedInPercentage,
		ProjectWikiRepositoriesVerifiedInPercentage:     s.ProjectWikiRepositoriesVerifiedInPercentage,
		ReplicationSlotsCount:                           s.ReplicationSlotsCount,
		ReplicationSlotsUsedCount:                       s.ReplicationSlotsUsedCount,
		Healthy:                                         s.Healthy,
		Health:                                          s.Health,
		HealthStatus:                                    s.HealthStatus,
		MissingOAuthApplication:                         s.MissingOAuthApplication,
		DBReplicationLagSeconds:                         s.DBReplicationLagSeconds,
		ReplicationSlotsMaxRetainedWalBytes:             s.ReplicationSlotsMaxRetainedWalBytes,
		RepositoriesCheckedCount:                        s.RepositoriesCheckedCount,
		RepositoriesCheckedFailedCount:                  s.RepositoriesCheckedFailedCount,
		LastEventID:                                     s.LastEventID,
		LastEventTimestamp:                              s.LastEventTimestamp,
		CursorLastEventID:                               s.CursorLastEventID,
		CursorLastEventTimestamp:                        s.CursorLastEventTimestamp,
		LastSuccessfulStatusCheckTimestamp:              s.LastSuccessfulStatusCheckTimestamp,
		Version:                                         s.Version,
		Revision:                                        s.Revision,
		SelectiveSyncType:                               s.SelectiveSyncType,
		Namespaces:                                      s.Namespaces,
		UpdatedAt:                                       s.UpdatedAt,
		StorageShardsMatch:                              s.StorageShardsMatch,
		Links: StatusLinks{
			Self: s.Links.Self,
			Site: s.Links.Site,
		},
	}
}
