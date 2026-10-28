// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package projectimportexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Schedule Export
// ---------------------------------------------------------------------------.

// ScheduleExportUploadInput mirrors [gl.ScheduleExportUploadOptions]: the
// nested upload destination for a scheduled export. When set, GitLab uploads
// the generated archive to the given URL instead of (or in addition to)
// keeping it for download.
type ScheduleExportUploadInput struct {
	URL        string `json:"url,omitempty" jsonschema:"URL to upload the exported project archive to after export completes"`
	HTTPMethod string `json:"http_method,omitempty" jsonschema:"HTTP method to use for the upload (PUT or POST. Default PUT)"`
}

// ScheduleExportInput is the input for scheduling a project export. Fields
// mirror [gl.ScheduleExportOptions] 1:1.
type ScheduleExportInput struct {
	ProjectID   toolutil.StringOrInt       `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Description string                     `json:"description,omitempty" jsonschema:"Override the project description in the export"`
	Upload      *ScheduleExportUploadInput `json:"upload,omitempty" jsonschema:"Optional upload destination for the generated export archive (mirrors the GitLab upload[] options)"`

	// Deprecated: use Upload.URL. Retained as a flat alias for backward
	// compatibility; ignored when Upload is set.
	UploadURL string `json:"upload_url,omitempty" jsonschema:"Deprecated: use upload.url. URL to upload the exported project to after export completes"`
	// Deprecated: use Upload.HTTPMethod. Retained as a flat alias for backward
	// compatibility; ignored when Upload is set.
	UploadHTTP string `json:"upload_http_method,omitempty" jsonschema:"Deprecated: use upload.http_method. HTTP method to use for the upload (PUT or POST)"`
}

// ScheduleExportOutput is the output for scheduling a project export.
type ScheduleExportOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// ScheduleExport schedules an asynchronous project export.
func ScheduleExport(ctx context.Context, client *gitlabclient.Client, input ScheduleExportInput) (ScheduleExportOutput, error) {
	opts := &gl.ScheduleExportOptions{}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}

	// Prefer the structured nested upload options; fall back to the deprecated
	// flat fields for backward compatibility, read as the nested input they
	// stand for, so the upload destination is built from one shape.
	upload := input.Upload
	if upload == nil {
		upload = &ScheduleExportUploadInput{URL: input.UploadURL, HTTPMethod: input.UploadHTTP}
	}
	if upload.URL != "" {
		opts.Upload = gl.ScheduleExportUploadOptions{
			URL: new(upload.URL),
		}
		if upload.HTTPMethod != "" {
			opts.Upload.HTTPMethod = new(upload.HTTPMethod)
		}
	}

	_, err := client.GL().ProjectImportExport.ScheduleExport(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleExportOutput{}, toolutil.WrapErrWithStatusHint("schedule_export", err, http.StatusForbidden,
			"requires Owner role on the project; only one export at a time per project. Wait for the previous to finish or use project.export_status to check")
	}
	return ScheduleExportOutput{Message: "Export scheduled successfully"}, nil
}

// ---------------------------------------------------------------------------
// Export Status
// ---------------------------------------------------------------------------.

// ExportStatusInput is the input for getting export status.
type ExportStatusInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ExportStatusOutput is the output for getting export status.
type ExportStatusOutput struct {
	toolutil.HintableOutput
	ID                int64  `json:"id"`
	Description       string `json:"description"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	CreatedAt         string `json:"created_at,omitempty"`
	ExportStatus      string `json:"export_status"`
	APIURL            string `json:"api_url,omitempty"`
	WebURL            string `json:"web_url,omitempty"`
}

// GetExportStatus returns the export status of a project.
func GetExportStatus(ctx context.Context, client *gitlabclient.Client, input ExportStatusInput) (ExportStatusOutput, error) {
	status, _, err := client.GL().ProjectImportExport.ExportStatus(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ExportStatusOutput{}, toolutil.WrapErrWithStatusHint("export_status", err, http.StatusNotFound,
			"verify project_id with project.get; export must be scheduled first via project.export_schedule; status values: none, started, finished, after_export_action_failed")
	}

	out := ExportStatusOutput{
		ID:                status.ID,
		Description:       status.Description,
		Name:              status.Name,
		NameWithNamespace: status.NameWithNamespace,
		Path:              status.Path,
		PathWithNamespace: status.PathWithNamespace,
		ExportStatus:      status.ExportStatus,
	}
	if status.CreatedAt != nil {
		out.CreatedAt = status.CreatedAt.Format(time.RFC3339)
	}
	if status.Links.APIURL != "" {
		out.APIURL = status.Links.APIURL
	}
	if status.Links.WebURL != "" {
		out.WebURL = status.Links.WebURL
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Export Download
// ---------------------------------------------------------------------------.

// ExportDownloadInput is the input for downloading a project export.
type ExportDownloadInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ExportDownloadOutput is the output for downloading a project export.
type ExportDownloadOutput struct {
	toolutil.HintableOutput
	ContentBase64 string `json:"content_base64"`
	SizeBytes     int    `json:"size_bytes"`
}

// errExportTooLarge is the way out for an archive this action will not carry,
// whichever ceiling stopped it: the shared download ceiling, or the
// client-wide response ceiling.
const errExportTooLarge = "the archive is too large to return in one MCP response; download it from GitLab directly (GET /projects/:id/export/download)"

// ExportDownload downloads the finished export archive of a project as base64.
//
// client-go reads the whole body before it returns it, so the shared download
// ceiling is applied where the body is read, as a response limit on this one
// request: an archive over it stops the read there instead of being buffered
// up to the client's own ceiling and then refused, and is answered with the
// way out rather than returned or truncated.
func ExportDownload(ctx context.Context, client *gitlabclient.Client, input ExportDownloadInput) (ExportDownloadOutput, error) {
	limited := gitlabclient.WithResponseLimit(ctx, toolutil.MaxDownloadBytes)
	data, _, err := client.GL().ProjectImportExport.ExportDownload(string(input.ProjectID), gl.WithContext(limited))
	if err != nil {
		if toolutil.IsDownloadTooLarge(err) {
			return ExportDownloadOutput{}, toolutil.WrapErrWithHint("export_download", err, errExportTooLarge)
		}
		return ExportDownloadOutput{}, toolutil.WrapErrWithStatusHint("export_download", err, http.StatusNotFound,
			"export must be in 'finished' state. Check project.export_status first; archives are kept for 24 hours after generation")
	}
	return ExportDownloadOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Import From File
// ---------------------------------------------------------------------------.

// ImportOverrideParamsInput mirrors the project attributes an import from
// file applies from its `override_params` field: every one that
// [gl.CreateProjectOptions] (the type of [gl.ImportFileOptions.OverrideParams])
// models and the import keeps. Only set fields are forwarded, so a boolean or
// a number is a pointer: overriding a setting the export carries as true with
// false is the point of the field.
//
// The project's name, path and namespace are not among them: GitLab declares
// them as the import's own params and not under override_params, whose
// declared params (optional_project_params) Grape keeps alone, so an
// override_params name, path, namespace_id or default_branch was dropped.
// The name, path and namespace of [ImportFromFileInput] place the project.
//
// Nor is any key GitLab declares under override_params and the import never
// applies. The tree restorer merges the overrides into the archive's project
// attributes and keeps only those import_export.yml permits for a project
// (Gitlab::ImportExport::AttributesPermitter, through
// Group::RelationTreeRestorer#filter_attributes), which leaves out the CI/CD
// configuration path, the cleanup policy, the email, mirroring, runner and
// template settings among others, and excludes lfs_enabled, mirror,
// repository_storage and tag_list by name. Two keys it permits are left out
// too, because the restorer cannot assign them: topics is the project's
// association with Projects::Topic, which a list of names does not fill, and
// the project has no jobs_enabled (lib/api/projects.rb renames it to
// builds_enabled for the create and the edit alone), so either one fails the
// restore and the import with it. builds_access_level sets what jobs_enabled
// would.
type ImportOverrideParamsInput struct {
	Description                  string `json:"description,omitempty" jsonschema:"Override the imported project's description"`
	Visibility                   string `json:"visibility,omitempty" jsonschema:"Override visibility level (private, internal, public)"`
	MergeMethod                  string `json:"merge_method,omitempty" jsonschema:"Override merge method (merge, rebase_merge, ff)"`
	RequestAccessEnabled         *bool  `json:"request_access_enabled,omitempty" jsonschema:"Override whether users can request access"`
	SharedRunnersEnabled         *bool  `json:"shared_runners_enabled,omitempty" jsonschema:"Override whether shared (instance) CI runners are enabled"`
	IssuesAccessLevel            string `json:"issues_access_level,omitempty" jsonschema:"Override issues access level (disabled, private, enabled)"`
	MergeRequestsAccessLevel     string `json:"merge_requests_access_level,omitempty" jsonschema:"Override merge requests access level (disabled, private, enabled)"`
	WikiAccessLevel              string `json:"wiki_access_level,omitempty" jsonschema:"Override wiki access level (disabled, private, enabled)"`
	BuildsAccessLevel            string `json:"builds_access_level,omitempty" jsonschema:"Override CI/CD builds access level (disabled, private, enabled)"`
	SnippetsAccessLevel          string `json:"snippets_access_level,omitempty" jsonschema:"Override snippets access level (disabled, private, enabled)"`
	ContainerRegistryAccessLevel string `json:"container_registry_access_level,omitempty" jsonschema:"Override container registry access level (disabled, private, enabled)"`

	RepositoryAccessLevel            string `json:"repository_access_level,omitempty" jsonschema:"Override repository access level (disabled, private, enabled)"`
	ForkingAccessLevel               string `json:"forking_access_level,omitempty" jsonschema:"Override forking access level (disabled, private, enabled)"`
	AnalyticsAccessLevel             string `json:"analytics_access_level,omitempty" jsonschema:"Override analytics access level (disabled, private, enabled)"`
	PagesAccessLevel                 string `json:"pages_access_level,omitempty" jsonschema:"Override Pages access level (disabled, private, enabled, public)"`
	PackageRegistryAccessLevel       string `json:"package_registry_access_level,omitempty" jsonschema:"Override package registry access level (disabled, private, enabled)"`
	ReleasesAccessLevel              string `json:"releases_access_level,omitempty" jsonschema:"Override releases access level (disabled, private, enabled)"`
	EnvironmentsAccessLevel          string `json:"environments_access_level,omitempty" jsonschema:"Override environments access level (disabled, private, enabled)"`
	FeatureFlagsAccessLevel          string `json:"feature_flags_access_level,omitempty" jsonschema:"Override feature flags access level (disabled, private, enabled)"`
	InfrastructureAccessLevel        string `json:"infrastructure_access_level,omitempty" jsonschema:"Override infrastructure access level (disabled, private, enabled)"`
	MonitorAccessLevel               string `json:"monitor_access_level,omitempty" jsonschema:"Override monitor access level (disabled, private, enabled)"`
	RequirementsAccessLevel          string `json:"requirements_access_level,omitempty" jsonschema:"Override requirements access level (disabled, private, enabled)"`
	SecurityAndComplianceAccessLevel string `json:"security_and_compliance_access_level,omitempty" jsonschema:"Override security and compliance access level (disabled, private, enabled)"`
	ModelExperimentsAccessLevel      string `json:"model_experiments_access_level,omitempty" jsonschema:"Override model experiments access level (disabled, private, enabled)"`
	ModelRegistryAccessLevel         string `json:"model_registry_access_level,omitempty" jsonschema:"Override model registry access level (disabled, private, enabled)"`

	SquashOption                              string `json:"squash_option,omitempty" jsonschema:"Override the squash option (never, always, default_on, default_off)"`
	OnlyAllowMergeIfPipelineSucceeds          *bool  `json:"only_allow_merge_if_pipeline_succeeds,omitempty" jsonschema:"Override whether a merge waits for a successful pipeline"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool  `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Override whether a merge waits for every discussion to be resolved"`
	AllowMergeOnSkippedPipeline               *bool  `json:"allow_merge_on_skipped_pipeline,omitempty" jsonschema:"Override whether a merge is allowed when the pipeline is skipped"`
	RemoveSourceBranchAfterMerge              *bool  `json:"remove_source_branch_after_merge,omitempty" jsonschema:"Override whether the source branch is removed after a merge by default"`
	AutocloseReferencedIssues                 *bool  `json:"autoclose_referenced_issues,omitempty" jsonschema:"Override whether a merge closes the issues it references"`
	ResolveOutdatedDiffDiscussions            *bool  `json:"resolve_outdated_diff_discussions,omitempty" jsonschema:"Override whether outdated diff discussions are resolved on push"`
	PrintingMergeRequestLinkEnabled           *bool  `json:"printing_merge_request_link_enabled,omitempty" jsonschema:"Override whether a push prints the create-merge-request link"`
	MergeCommitTemplate                       string `json:"merge_commit_template,omitempty" jsonschema:"Override the merge commit message template"`
	SquashCommitTemplate                      string `json:"squash_commit_template,omitempty" jsonschema:"Override the squash commit message template"`
	SuggestionCommitMessage                   string `json:"suggestion_commit_message,omitempty" jsonschema:"Override the default commit message for applied suggestions"`
	ApprovalsBeforeMerge                      *int64 `json:"approvals_before_merge,omitempty" tier:"premium" jsonschema:"Override how many approvals a merge request needs (deprecated: use the merge request approvals API)"`

	ServiceDeskEnabled *bool `json:"service_desk_enabled,omitempty" jsonschema:"Override whether Service Desk is enabled"`
	AutoDevopsEnabled  *bool `json:"auto_devops_enabled,omitempty" jsonschema:"Override whether Auto DevOps is enabled"`

	AutoDevopsDeployStrategy                 string `json:"auto_devops_deploy_strategy,omitempty" jsonschema:"Override the Auto DevOps deploy strategy (continuous, manual, timed_incremental)"`
	AutoCancelPendingPipelines               string `json:"auto_cancel_pending_pipelines,omitempty" jsonschema:"Override whether pending pipelines are auto-canceled (enabled, disabled)"`
	BuildGitStrategy                         string `json:"build_git_strategy,omitempty" jsonschema:"Override the Git strategy of jobs (fetch, clone)"`
	BuildTimeout                             *int64 `json:"build_timeout,omitempty" jsonschema:"Override the job timeout in seconds"`
	ExternalAuthorizationClassificationLabel string `json:"external_authorization_classification_label,omitempty" jsonschema:"Override the external authorization classification label"`

	// The deprecated toggles GitLab still declares under override_params
	// and the import applies, each sent as the caller gives it.
	IssuesEnabled            *bool `json:"issues_enabled,omitempty" jsonschema:"Override whether issues are enabled (deprecated: use issues_access_level)"`
	MergeRequestsEnabled     *bool `json:"merge_requests_enabled,omitempty" jsonschema:"Override whether merge requests are enabled (deprecated: use merge_requests_access_level)"`
	WikiEnabled              *bool `json:"wiki_enabled,omitempty" jsonschema:"Override whether the wiki is enabled (deprecated: use wiki_access_level)"`
	SnippetsEnabled          *bool `json:"snippets_enabled,omitempty" jsonschema:"Override whether snippets are enabled (deprecated: use snippets_access_level)"`
	ContainerRegistryEnabled *bool `json:"container_registry_enabled,omitempty" jsonschema:"Override whether the container registry is enabled (deprecated: use container_registry_access_level)"`
	PublicBuilds             *bool `json:"public_builds,omitempty" jsonschema:"Override whether pipelines and job logs are public (the import applies this name, not public_jobs)"`
}

// ImportFromFileInput is the input for importing a project from an archive
// file. Fields mirror [gl.ImportFileOptions] 1:1, override_params as the
// nested struct over [gl.CreateProjectOptions] that ImportOverrideParamsInput
// mirrors.
type ImportFromFileInput struct {
	FilePath       string                     `json:"file_path,omitempty" jsonschema:"Canonical path to a local export archive (.tar.gz) under the current working directory, OS temp directory, or GITLAB_MCP_ALLOWED_IMPORT_DIRS. Symlinks are resolved and escapes are rejected. Only one of file_path or content_base64 should be provided."`
	ContentBase64  string                     `json:"content_base64,omitempty" jsonschema:"Base64-encoded export archive content. Only one of file_path or content_base64 should be provided."`
	Namespace      string                     `json:"namespace,omitempty" jsonschema:"Namespace to import the project into (user or group path)"`
	Name           string                     `json:"name,omitempty" jsonschema:"Name for the imported project"`
	Path           string                     `json:"path,omitempty" jsonschema:"URL path for the imported project,required"`
	Overwrite      *bool                      `json:"overwrite,omitempty" jsonschema:"If true, overwrite an existing project with the same path"`
	OverrideParams *ImportOverrideParamsInput `json:"override_params,omitempty" jsonschema:"Optional project attributes to override on the imported project (mirrors the create-project attributes accepted by override_params[])"`
}

// ImportStatusOutput is the output for import operations: every key
// lib/api/entities/project_import_status.rb sends. FailedRelations are the
// relations the import could not bring over, at most a hundred, and Stats is
// what a GitHub import has fetched and imported so far, per object type,
// which GitLab sends as null for any other import. client-go's ImportStatus
// models neither, and both are read by the raw decode this package already
// makes.
type ImportStatusOutput struct {
	toolutil.HintableOutput
	ID                int64                       `json:"id"`
	Description       string                      `json:"description"`
	Name              string                      `json:"name"`
	NameWithNamespace string                      `json:"name_with_namespace"`
	Path              string                      `json:"path"`
	PathWithNamespace string                      `json:"path_with_namespace"`
	CreatedAt         string                      `json:"created_at,omitempty"`
	ImportStatus      string                      `json:"import_status"`
	ImportType        string                      `json:"import_type,omitempty"`
	CorrelationID     string                      `json:"correlation_id,omitempty"`
	ImportError       string                      `json:"import_error,omitempty"`
	FailedRelations   []FailedRelationOutput      `json:"failed_relations"`
	Stats             map[string]map[string]int64 `json:"stats,omitempty"`
}

// FailedRelationOutput is one relation an import could not bring over, as
// lib/api/entities/project_import_failed_relation.rb renders it: when and
// where it failed, the exception's class, and the relation and line it was
// reading. The entity also exposes exception_message through a block that
// returns nil whatever the relation, so it is never filled and is not
// published.
type FailedRelationOutput struct {
	ID             int64  `json:"id"`
	CreatedAt      string `json:"created_at,omitempty"`
	ExceptionClass string `json:"exception_class"`
	Source         string `json:"source"`
	RelationName   string `json:"relation_name"`
	LineNumber     int64  `json:"line_number"`
}

// failedRelationAPI is one failed relation as the raw decode reads it.
type failedRelationAPI struct {
	ID             int64      `json:"id"`
	CreatedAt      *time.Time `json:"created_at"`
	ExceptionClass string     `json:"exception_class"`
	Source         string     `json:"source"`
	RelationName   string     `json:"relation_name"`
	LineNumber     int64      `json:"line_number"`
}

// importStatusAPI is a raw-decode superset of the documented import-status
// response (doc/api/project_import_export.md import/status responses). It exists
// because gl.ImportStatus tags its timestamp field as `create_at` (an SDK typo),
// so it never captures the documented `created_at` attribute. Decoding into this
// superset reads the documented `created_at` first and falls back to the legacy
// `create_at` spelling, so the value is surfaced regardless of which spelling the
// instance returns. The other fields mirror gl.ImportStatus 1:1, and the two it
// does not model, failed_relations and stats, are read here too.
type importStatusAPI struct {
	ID                int64                       `json:"id"`
	Description       string                      `json:"description"`
	Name              string                      `json:"name"`
	NameWithNamespace string                      `json:"name_with_namespace"`
	Path              string                      `json:"path"`
	PathWithNamespace string                      `json:"path_with_namespace"`
	CreatedAt         *time.Time                  `json:"created_at"`
	CreateAt          *time.Time                  `json:"create_at"`
	ImportStatus      string                      `json:"import_status"`
	ImportType        string                      `json:"import_type"`
	CorrelationID     string                      `json:"correlation_id"`
	ImportError       string                      `json:"import_error"`
	FailedRelations   []failedRelationAPI         `json:"failed_relations"`
	Stats             map[string]map[string]int64 `json:"stats"`
}

// rawImportStatusToOutput maps the raw-decode superset onto ImportStatusOutput,
// preferring the documented `created_at` attribute over the legacy `create_at`.
func rawImportStatusToOutput(s *importStatusAPI) ImportStatusOutput {
	out := ImportStatusOutput{
		ID:                s.ID,
		Description:       s.Description,
		Name:              s.Name,
		NameWithNamespace: s.NameWithNamespace,
		Path:              s.Path,
		PathWithNamespace: s.PathWithNamespace,
		ImportStatus:      s.ImportStatus,
		ImportType:        s.ImportType,
		CorrelationID:     s.CorrelationID,
		ImportError:       s.ImportError,
		FailedRelations:   make([]FailedRelationOutput, len(s.FailedRelations)),
		Stats:             s.Stats,
	}
	for i, r := range s.FailedRelations {
		out.FailedRelations[i] = FailedRelationOutput{
			ID:             r.ID,
			CreatedAt:      toolutil.RFC3339Ptr(r.CreatedAt),
			ExceptionClass: r.ExceptionClass,
			Source:         r.Source,
			RelationName:   r.RelationName,
			LineNumber:     r.LineNumber,
		}
	}
	if created := s.CreatedAt; created != nil {
		out.CreatedAt = created.Format(time.RFC3339)
	} else if s.CreateAt != nil {
		out.CreatedAt = s.CreateAt.Format(time.RFC3339)
	}
	return out
}

// rawGetImportStatus issues a raw REST GET against the import-status path,
// decoding the documented response (including the documented `created_at`
// attribute the SDK mistags) into an [importStatusAPI].
func rawGetImportStatus(ctx context.Context, client *gitlabclient.Client, path string) (*importStatusAPI, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var status importStatusAPI
	_, err = client.GL().Do(req, &status)
	return &status, err
}

// rawImportFromFile issues a raw multipart REST POST against the import path,
// decoding the documented response (including the documented `created_at`
// attribute the SDK mistags) into an [importStatusAPI]. It mirrors the SDK's
// ProjectImportExport.ImportFromFile upload but reads the documented timestamp.
func rawImportFromFile(ctx context.Context, client *gitlabclient.Client, archive io.Reader, opts *gl.ImportFileOptions) (*importStatusAPI, error) {
	req, err := client.GL().UploadRequest(
		http.MethodPost,
		"projects/import",
		archive,
		"archive.tar.gz",
		gl.UploadFile,
		opts,
		[]gl.RequestOptionFunc{gl.WithContext(ctx)},
	)
	if err != nil {
		return nil, err
	}
	var status importStatusAPI
	_, err = client.GL().Do(req, &status)
	return &status, err
}

// openArchive opens an import archive that already passed every check. It is
// a variable so a test can take the branch where the open fails anyway, which
// a real file reaches only through a permission root does not lack.
var openArchive = os.Open

// ImportFromFile imports a project from an export archive.
func ImportFromFile(ctx context.Context, client *gitlabclient.Client, input ImportFromFileInput) (ImportStatusOutput, error) {
	hasFilePath := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", errors.New("provide only one of file_path or content_base64, not both"))
	}
	if !hasFilePath && !hasBase64 {
		return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", errors.New("one of file_path or content_base64 is required"))
	}

	var archiveReader io.Reader
	if hasFilePath {
		archivePath, err := toolutil.CanonicalImportArchivePath(input.FilePath)
		if err != nil {
			return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", err)
		}
		file, err := openArchive(archivePath) // archivePath is canonicalized, extension-checked, regular-file checked, and constrained to the allowed import directories by CanonicalImportArchivePath above.
		if err != nil {
			return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", fmt.Errorf("open archive: %w", err))
		}
		defer file.Close()
		archiveReader = file
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", fmt.Errorf("invalid base64: %w", err))
		}
		archiveReader = bytes.NewReader(decoded)
	}

	opts := &gl.ImportFileOptions{}
	if input.Namespace != "" {
		opts.Namespace = new(input.Namespace)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Overwrite != nil {
		opts.Overwrite = input.Overwrite
	}
	if op := buildOverrideParams(input.OverrideParams); op != nil {
		opts.OverrideParams = op
	}

	status, err := rawImportFromFile(ctx, client, archiveReader, opts)
	if err != nil {
		return ImportStatusOutput{}, toolutil.WrapErrWithStatusHint("import_from_file", err, http.StatusBadRequest,
			"archive must be a valid GitLab project export (.tar.gz from project.export_download); namespace must exist and you need create-project permission there; path must be unique unless overwrite=true")
	}
	return rawImportStatusToOutput(status), nil
}

// ---------------------------------------------------------------------------
// Import Status
// ---------------------------------------------------------------------------.

// GetImportStatusInput is the input for getting import status.
type GetImportStatusInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// GetImportStatus returns the import status of a project.
func GetImportStatus(ctx context.Context, client *gitlabclient.Client, input GetImportStatusInput) (ImportStatusOutput, error) {
	path := fmt.Sprintf("projects/%s/import", gl.PathEscape(string(input.ProjectID)))
	status, err := rawGetImportStatus(ctx, client, path)
	if err != nil {
		return ImportStatusOutput{}, toolutil.WrapErrWithStatusHint("import_status", err, http.StatusNotFound,
			"verify project_id with project.get; status values: none, scheduled, started, finished, failed; check import_error field for failure reasons")
	}
	return rawImportStatusToOutput(status), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// buildOverrideParams maps [ImportOverrideParamsInput] onto the SDK's
// [gl.CreateProjectOptions] used by [gl.ImportFileOptions.OverrideParams].
// It returns nil when no override field is set so the API receives no
// override_params payload.
func buildOverrideParams(in *ImportOverrideParamsInput) *gl.CreateProjectOptions {
	if in == nil {
		return nil
	}
	opts := &gl.CreateProjectOptions{
		Description:          optionalString(in.Description),
		RequestAccessEnabled: in.RequestAccessEnabled,
		SharedRunnersEnabled: in.SharedRunnersEnabled,
	}
	if in.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(in.Visibility))
	}
	applyOverrideAccessLevels(opts, in)
	applyOverrideMergeSettings(opts, in)
	applyOverrideFeatureSettings(opts, in)
	applyOverrideDeprecatedToggles(opts, in)
	if *opts == (gl.CreateProjectOptions{}) {
		return nil
	}
	return opts
}

// applyOverrideAccessLevels sets the feature access levels an override names.
func applyOverrideAccessLevels(opts *gl.CreateProjectOptions, in *ImportOverrideParamsInput) {
	opts.IssuesAccessLevel = accessLevel(in.IssuesAccessLevel)
	opts.MergeRequestsAccessLevel = accessLevel(in.MergeRequestsAccessLevel)
	opts.WikiAccessLevel = accessLevel(in.WikiAccessLevel)
	opts.BuildsAccessLevel = accessLevel(in.BuildsAccessLevel)
	opts.SnippetsAccessLevel = accessLevel(in.SnippetsAccessLevel)
	opts.ContainerRegistryAccessLevel = accessLevel(in.ContainerRegistryAccessLevel)
	opts.RepositoryAccessLevel = accessLevel(in.RepositoryAccessLevel)
	opts.ForkingAccessLevel = accessLevel(in.ForkingAccessLevel)
	opts.AnalyticsAccessLevel = accessLevel(in.AnalyticsAccessLevel)
	opts.PagesAccessLevel = accessLevel(in.PagesAccessLevel)
	opts.PackageRegistryAccessLevel = accessLevel(in.PackageRegistryAccessLevel)
	opts.ReleasesAccessLevel = accessLevel(in.ReleasesAccessLevel)
	opts.EnvironmentsAccessLevel = accessLevel(in.EnvironmentsAccessLevel)
	opts.FeatureFlagsAccessLevel = accessLevel(in.FeatureFlagsAccessLevel)
	opts.InfrastructureAccessLevel = accessLevel(in.InfrastructureAccessLevel)
	opts.MonitorAccessLevel = accessLevel(in.MonitorAccessLevel)
	opts.RequirementsAccessLevel = accessLevel(in.RequirementsAccessLevel)
	opts.SecurityAndComplianceAccessLevel = accessLevel(in.SecurityAndComplianceAccessLevel)
	opts.ModelExperimentsAccessLevel = accessLevel(in.ModelExperimentsAccessLevel)
	opts.ModelRegistryAccessLevel = accessLevel(in.ModelRegistryAccessLevel)
}

// applyOverrideMergeSettings sets the merge request settings an override
// names.
func applyOverrideMergeSettings(opts *gl.CreateProjectOptions, in *ImportOverrideParamsInput) {
	if in.MergeMethod != "" {
		opts.MergeMethod = new(gl.MergeMethodValue(in.MergeMethod))
	}
	if in.SquashOption != "" {
		opts.SquashOption = new(gl.SquashOptionValue(in.SquashOption))
	}
	opts.OnlyAllowMergeIfPipelineSucceeds = in.OnlyAllowMergeIfPipelineSucceeds
	opts.OnlyAllowMergeIfAllDiscussionsAreResolved = in.OnlyAllowMergeIfAllDiscussionsAreResolved
	opts.AllowMergeOnSkippedPipeline = in.AllowMergeOnSkippedPipeline
	opts.RemoveSourceBranchAfterMerge = in.RemoveSourceBranchAfterMerge
	opts.AutocloseReferencedIssues = in.AutocloseReferencedIssues
	opts.ResolveOutdatedDiffDiscussions = in.ResolveOutdatedDiffDiscussions
	opts.PrintingMergeRequestLinkEnabled = in.PrintingMergeRequestLinkEnabled
	opts.MergeCommitTemplate = optionalString(in.MergeCommitTemplate)
	opts.SquashCommitTemplate = optionalString(in.SquashCommitTemplate)
	opts.SuggestionCommitMessage = optionalString(in.SuggestionCommitMessage)
	opts.ApprovalsBeforeMerge = in.ApprovalsBeforeMerge //nolint:staticcheck // 1:1 with the param GitLab declares under override_params; the approvals API is its replacement.
}

// applyOverrideFeatureSettings sets the feature and CI/CD settings an
// override names.
func applyOverrideFeatureSettings(opts *gl.CreateProjectOptions, in *ImportOverrideParamsInput) {
	opts.ServiceDeskEnabled = in.ServiceDeskEnabled
	opts.AutoDevopsEnabled = in.AutoDevopsEnabled
	opts.AutoDevopsDeployStrategy = optionalString(in.AutoDevopsDeployStrategy)
	opts.AutoCancelPendingPipelines = optionalString(in.AutoCancelPendingPipelines)
	opts.BuildGitStrategy = optionalString(in.BuildGitStrategy)
	opts.BuildTimeout = in.BuildTimeout
	opts.ExternalAuthorizationClassificationLabel = optionalString(in.ExternalAuthorizationClassificationLabel)
}

// applyOverrideDeprecatedToggles sets the deprecated toggles GitLab still
// declares under override_params and the import applies, each as the caller
// gave it.
func applyOverrideDeprecatedToggles(opts *gl.CreateProjectOptions, in *ImportOverrideParamsInput) {
	opts.IssuesEnabled = in.IssuesEnabled                       //nolint:staticcheck // 1:1 with the declared param; prefer issues_access_level.
	opts.MergeRequestsEnabled = in.MergeRequestsEnabled         //nolint:staticcheck // 1:1 with the declared param; prefer merge_requests_access_level.
	opts.WikiEnabled = in.WikiEnabled                           //nolint:staticcheck // 1:1 with the declared param; prefer wiki_access_level.
	opts.SnippetsEnabled = in.SnippetsEnabled                   //nolint:staticcheck // 1:1 with the declared param; prefer snippets_access_level.
	opts.ContainerRegistryEnabled = in.ContainerRegistryEnabled //nolint:staticcheck // 1:1 with the declared param; prefer container_registry_access_level.
	opts.PublicBuilds = in.PublicBuilds                         //nolint:staticcheck // 1:1 with the declared param, the name the import applies.
}

// optionalString returns a pointer to s, or nil for the empty string so an
// override left out is not sent.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return new(s)
}

// accessLevel returns s as an access level, or nil for the empty string.
func accessLevel(s string) *gl.AccessControlValue {
	if s == "" {
		return nil
	}
	return new(gl.AccessControlValue(s))
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
