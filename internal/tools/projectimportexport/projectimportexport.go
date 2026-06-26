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

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	HTTPMethod string `json:"http_method,omitempty" jsonschema:"HTTP method to use for the upload (PUT or POST; default PUT)"`
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
	// flat fields for backward compatibility.
	uploadURL := input.UploadURL
	uploadHTTP := input.UploadHTTP
	if input.Upload != nil {
		uploadURL = input.Upload.URL
		uploadHTTP = input.Upload.HTTPMethod
	}
	if uploadURL != "" {
		opts.Upload = gl.ScheduleExportUploadOptions{
			URL: new(uploadURL),
		}
		if uploadHTTP != "" {
			opts.Upload.HTTPMethod = new(uploadHTTP)
		}
	}

	_, err := client.GL().ProjectImportExport.ScheduleExport(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleExportOutput{}, toolutil.WrapErrWithStatusHint("schedule_export", err, http.StatusForbidden,
			"requires Owner role on the project; only one export at a time per project \u2014 wait for the previous to finish or use gitlab_export_status to check")
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
	Message           string `json:"message,omitempty"`
	APIURL            string `json:"api_url,omitempty"`
	WebURL            string `json:"web_url,omitempty"`
}

// GetExportStatus returns the export status of a project.
func GetExportStatus(ctx context.Context, client *gitlabclient.Client, input ExportStatusInput) (ExportStatusOutput, error) {
	status, _, err := client.GL().ProjectImportExport.ExportStatus(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ExportStatusOutput{}, toolutil.WrapErrWithStatusHint("export_status", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; export must be scheduled first via gitlab_schedule_export; status values: none, started, finished, after_export_action_failed")
	}

	out := ExportStatusOutput{
		ID:                status.ID,
		Description:       status.Description,
		Name:              status.Name,
		NameWithNamespace: status.NameWithNamespace,
		Path:              status.Path,
		PathWithNamespace: status.PathWithNamespace,
		ExportStatus:      status.ExportStatus,
		Message:           status.Message,
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

// ExportDownload downloads the finished export archive of a project as base64.
func ExportDownload(ctx context.Context, client *gitlabclient.Client, input ExportDownloadInput) (ExportDownloadOutput, error) {
	data, _, err := client.GL().ProjectImportExport.ExportDownload(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ExportDownloadOutput{}, toolutil.WrapErrWithStatusHint("export_download", err, http.StatusNotFound,
			"export must be in 'finished' state \u2014 check gitlab_export_status first; archives are kept for 24 hours after generation")
	}
	return ExportDownloadOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Import From File
// ---------------------------------------------------------------------------.

// ImportOverrideParamsInput mirrors the project-create attributes that GitLab
// accepts under the import-from-file `override_params` field. It maps to
// [gl.CreateProjectOptions] (the type of [gl.ImportFileOptions.OverrideParams]).
//
// GitLab documents override_params as accepting the full set of create-project
// attributes (the entire [gl.CreateProjectOptions] set, ~79 fields). Exposing
// all of them as flat MCP input fields would bloat this tool's schema and
// duplicate the dedicated gitlab_project create/update tools, so this struct
// deliberately surfaces only the commonly-overridden subset a user realistically
// changes during an import (identity, visibility, default branch, merge method,
// access-request/LFS/shared-runner toggles, and the high-traffic feature access
// levels). Full project configuration remains available via the dedicated
// gitlab_project create/update tools. Only set fields are forwarded.
type ImportOverrideParamsInput struct {
	Name                         string `json:"name,omitempty" jsonschema:"Override the imported project's name"`
	Path                         string `json:"path,omitempty" jsonschema:"Override the imported project's URL path (slug)"`
	NamespaceID                  *int64 `json:"namespace_id,omitempty" jsonschema:"Override the namespace ID the imported project is created in"`
	Description                  string `json:"description,omitempty" jsonschema:"Override the imported project's description"`
	Visibility                   string `json:"visibility,omitempty" jsonschema:"Override visibility level (private, internal, public)"`
	DefaultBranch                string `json:"default_branch,omitempty" jsonschema:"Override the default branch name"`
	MergeMethod                  string `json:"merge_method,omitempty" jsonschema:"Override merge method (merge, rebase_merge, ff)"`
	RequestAccessEnabled         *bool  `json:"request_access_enabled,omitempty" jsonschema:"Override whether users can request access"`
	LFSEnabled                   *bool  `json:"lfs_enabled,omitempty" jsonschema:"Override Git LFS enablement"`
	SharedRunnersEnabled         *bool  `json:"shared_runners_enabled,omitempty" jsonschema:"Override whether shared (instance) CI runners are enabled"`
	IssuesAccessLevel            string `json:"issues_access_level,omitempty" jsonschema:"Override issues access level (disabled, private, enabled)"`
	MergeRequestsAccessLevel     string `json:"merge_requests_access_level,omitempty" jsonschema:"Override merge requests access level (disabled, private, enabled)"`
	WikiAccessLevel              string `json:"wiki_access_level,omitempty" jsonschema:"Override wiki access level (disabled, private, enabled)"`
	BuildsAccessLevel            string `json:"builds_access_level,omitempty" jsonschema:"Override CI/CD builds access level (disabled, private, enabled)"`
	SnippetsAccessLevel          string `json:"snippets_access_level,omitempty" jsonschema:"Override snippets access level (disabled, private, enabled)"`
	ContainerRegistryAccessLevel string `json:"container_registry_access_level,omitempty" jsonschema:"Override container registry access level (disabled, private, enabled)"`
}

// ImportFromFileInput is the input for importing a project from an archive
// file. Fields mirror [gl.ImportFileOptions] 1:1 (override_params is exposed as
// a curated nested struct over [gl.CreateProjectOptions]).
type ImportFromFileInput struct {
	FilePath       string                     `json:"file_path,omitempty" jsonschema:"Canonical path to a local export archive (.tar.gz) under the current working directory, OS temp directory, or GITLAB_MCP_ALLOWED_IMPORT_DIRS. Symlinks are resolved and escapes are rejected. Only one of file_path or content_base64 should be provided."`
	ContentBase64  string                     `json:"content_base64,omitempty" jsonschema:"Base64-encoded export archive content. Only one of file_path or content_base64 should be provided."`
	Namespace      string                     `json:"namespace,omitempty" jsonschema:"Namespace to import the project into (user or group path)"`
	Name           string                     `json:"name,omitempty" jsonschema:"Name for the imported project"`
	Path           string                     `json:"path,omitempty" jsonschema:"URL path for the imported project"`
	Overwrite      *bool                      `json:"overwrite,omitempty" jsonschema:"If true, overwrite an existing project with the same path"`
	OverrideParams *ImportOverrideParamsInput `json:"override_params,omitempty" jsonschema:"Optional project attributes to override on the imported project (mirrors the create-project attributes accepted by override_params[])"`
}

// ImportStatusOutput is the output for import operations.
type ImportStatusOutput struct {
	toolutil.HintableOutput
	ID                int64  `json:"id"`
	Description       string `json:"description"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	CreatedAt         string `json:"created_at,omitempty"`
	ImportStatus      string `json:"import_status"`
	ImportType        string `json:"import_type,omitempty"`
	CorrelationID     string `json:"correlation_id,omitempty"`
	ImportError       string `json:"import_error,omitempty"`
}

// importStatusAPI is a raw-decode superset of the documented import-status
// response (doc/api/project_import_export.md import/status responses). It exists
// because gl.ImportStatus tags its timestamp field as `create_at` (an SDK typo),
// so it never captures the documented `created_at` attribute. Decoding into this
// superset reads the documented `created_at` first and falls back to the legacy
// `create_at` spelling, so the value is surfaced regardless of which spelling the
// instance returns. All other fields mirror gl.ImportStatus 1:1.
type importStatusAPI struct {
	ID                int64      `json:"id"`
	Description       string     `json:"description"`
	Name              string     `json:"name"`
	NameWithNamespace string     `json:"name_with_namespace"`
	Path              string     `json:"path"`
	PathWithNamespace string     `json:"path_with_namespace"`
	CreatedAt         *time.Time `json:"created_at"`
	CreateAt          *time.Time `json:"create_at"`
	ImportStatus      string     `json:"import_status"`
	ImportType        string     `json:"import_type"`
	CorrelationID     string     `json:"correlation_id"`
	ImportError       string     `json:"import_error"`
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
		file, err := os.Open(archivePath) //#nosec G304 -- archivePath is canonicalized, extension-checked, regular-file checked, and constrained to allowed import directories.
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
			"archive must be a valid GitLab project export (.tar.gz from gitlab_export_download); namespace must exist and you need create-project permission there; path must be unique unless overwrite=true")
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
			"verify project_id with gitlab_project_get; status values: none, scheduled, started, finished, failed; check import_error field for failure reasons")
	}
	return rawImportStatusToOutput(status), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// buildOverrideParams maps the curated [ImportOverrideParamsInput] onto the
// SDK's [gl.CreateProjectOptions] used by [gl.ImportFileOptions.OverrideParams].
// It returns nil when no override field is set so the API receives no
// override_params payload.
func buildOverrideParams(in *ImportOverrideParamsInput) *gl.CreateProjectOptions {
	if in == nil {
		return nil
	}
	opts := &gl.CreateProjectOptions{}
	set := false
	if in.Name != "" {
		opts.Name = new(in.Name)
		set = true
	}
	if in.Path != "" {
		opts.Path = new(in.Path)
		set = true
	}
	if in.NamespaceID != nil {
		opts.NamespaceID = in.NamespaceID
		set = true
	}
	if in.Description != "" {
		opts.Description = new(in.Description)
		set = true
	}
	if in.Visibility != "" {
		v := gl.VisibilityValue(in.Visibility)
		opts.Visibility = &v
		set = true
	}
	if in.DefaultBranch != "" {
		opts.DefaultBranch = new(in.DefaultBranch)
		set = true
	}
	if in.MergeMethod != "" {
		m := gl.MergeMethodValue(in.MergeMethod)
		opts.MergeMethod = &m
		set = true
	}
	if in.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = in.RequestAccessEnabled
		set = true
	}
	if in.LFSEnabled != nil {
		opts.LFSEnabled = in.LFSEnabled
		set = true
	}
	if in.SharedRunnersEnabled != nil {
		opts.SharedRunnersEnabled = in.SharedRunnersEnabled
		set = true
	}
	if in.IssuesAccessLevel != "" {
		v := gl.AccessControlValue(in.IssuesAccessLevel)
		opts.IssuesAccessLevel = &v
		set = true
	}
	if in.MergeRequestsAccessLevel != "" {
		v := gl.AccessControlValue(in.MergeRequestsAccessLevel)
		opts.MergeRequestsAccessLevel = &v
		set = true
	}
	if in.WikiAccessLevel != "" {
		v := gl.AccessControlValue(in.WikiAccessLevel)
		opts.WikiAccessLevel = &v
		set = true
	}
	if in.BuildsAccessLevel != "" {
		v := gl.AccessControlValue(in.BuildsAccessLevel)
		opts.BuildsAccessLevel = &v
		set = true
	}
	if in.SnippetsAccessLevel != "" {
		v := gl.AccessControlValue(in.SnippetsAccessLevel)
		opts.SnippetsAccessLevel = &v
		set = true
	}
	if in.ContainerRegistryAccessLevel != "" {
		v := gl.AccessControlValue(in.ContainerRegistryAccessLevel)
		opts.ContainerRegistryAccessLevel = &v
		set = true
	}
	if !set {
		return nil
	}
	return opts
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
