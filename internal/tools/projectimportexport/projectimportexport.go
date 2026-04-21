// Package projectimportexport implements MCP tool handlers for the GitLab
// Project Import/Export API. It wraps the ProjectImportExportService from
// client-go v2 to schedule exports, check export/import status, download
// export archives, and import projects from file archives.
package projectimportexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Schedule Export
// ---------------------------------------------------------------------------.

// ScheduleExportInput is the input for scheduling a project export.
type ScheduleExportInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Description string               `json:"description,omitempty" jsonschema:"Override the project description in the export"`
	UploadURL   string               `json:"upload_url,omitempty" jsonschema:"URL to upload the exported project to after export completes"`
	UploadHTTP  string               `json:"upload_http_method,omitempty" jsonschema:"HTTP method to use for the upload (PUT or POST)"`
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
	if input.UploadURL != "" {
		opts.Upload = gl.ScheduleExportUploadOptions{
			URL: new(input.UploadURL),
		}
		if input.UploadHTTP != "" {
			opts.Upload.HTTPMethod = new(input.UploadHTTP)
		}
	}

	_, err := client.GL().ProjectImportExport.ScheduleExport(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleExportOutput{}, toolutil.WrapErrWithMessage("schedule_export", err)
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
		return ExportStatusOutput{}, toolutil.WrapErrWithMessage("export_status", err)
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
		return ExportDownloadOutput{}, toolutil.WrapErrWithMessage("export_download", err)
	}
	return ExportDownloadOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Import From File
// ---------------------------------------------------------------------------.

// ImportFromFileInput is the input for importing a project from an archive file.
type ImportFromFileInput struct {
	FilePath      string `json:"file_path,omitempty" jsonschema:"Absolute path to a local export archive (.tar.gz) on the MCP server filesystem. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"Base64-encoded export archive content. Only one of file_path or content_base64 should be provided."`
	Namespace     string `json:"namespace,omitempty" jsonschema:"Namespace to import the project into (user or group path)"`
	Name          string `json:"name,omitempty" jsonschema:"Name for the imported project"`
	Path          string `json:"path,omitempty" jsonschema:"URL path for the imported project"`
	Overwrite     *bool  `json:"overwrite,omitempty" jsonschema:"If true, overwrite an existing project with the same path"`
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

	var archiveReader *bytes.Reader
	if hasFilePath {
		data, err := os.ReadFile(input.FilePath)
		if err != nil {
			return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", fmt.Errorf("reading file: %w", err))
		}
		archiveReader = bytes.NewReader(data)
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

	status, _, err := client.GL().ProjectImportExport.ImportFromFile(archiveReader, opts, gl.WithContext(ctx))
	if err != nil {
		return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_from_file", err)
	}
	return importStatusToOutput(status), nil
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
	status, _, err := client.GL().ProjectImportExport.ImportStatus(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ImportStatusOutput{}, toolutil.WrapErrWithMessage("import_status", err)
	}
	return importStatusToOutput(status), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// importStatusToOutput converts the GitLab API response to the tool output format.
func importStatusToOutput(s *gl.ImportStatus) ImportStatusOutput {
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
	if s.CreateAt != nil {
		out.CreatedAt = s.CreateAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
