// Package groupimportexport implements MCP tool handlers for the GitLab
// Group Import/Export API. It wraps the GroupImportExportService from
// client-go v2 to schedule group exports, download export archives,
// and import groups from file archives.
package groupimportexport

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Schedule Export
// ---------------------------------------------------------------------------.

// ScheduleExportInput is the input for scheduling a group export.
type ScheduleExportInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ScheduleExportOutput is the output for scheduling a group export.
type ScheduleExportOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// ScheduleExport schedules an asynchronous group export.
func ScheduleExport(ctx context.Context, client *gitlabclient.Client, input ScheduleExportInput) (ScheduleExportOutput, error) {
	_, err := client.GL().GroupImportExport.ScheduleExport(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return ScheduleExportOutput{}, toolutil.WrapErrWithStatusHint("schedule_group_export", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
	}
	return ScheduleExportOutput{Message: "Group export scheduled successfully"}, nil
}

// ---------------------------------------------------------------------------
// Export Download
// ---------------------------------------------------------------------------.

// ExportDownloadInput is the input for downloading a group export.
type ExportDownloadInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ExportDownloadOutput is the output for downloading a group export.
type ExportDownloadOutput struct {
	toolutil.HintableOutput
	ContentBase64 string `json:"content_base64"`
	SizeBytes     int    `json:"size_bytes"`
}

// ExportDownload downloads the finished export archive of a group as base64.
func ExportDownload(ctx context.Context, client *gitlabclient.Client, input ExportDownloadInput) (ExportDownloadOutput, error) {
	reader, _, err := client.GL().GroupImportExport.ExportDownload(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return ExportDownloadOutput{}, toolutil.WrapErrWithStatusHint("download_group_export", err, http.StatusNotFound, "export must be scheduled first with gitlab_schedule_group_export")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return ExportDownloadOutput{}, toolutil.WrapErrWithMessage("download_group_export", fmt.Errorf("reading export data: %w", err))
	}

	return ExportDownloadOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Import File
// ---------------------------------------------------------------------------.

// ImportFileInput is the input for importing a group from an archive file.
type ImportFileInput struct {
	Name     string `json:"name" jsonschema:"Name for the imported group,required"`
	Path     string `json:"path" jsonschema:"URL path for the imported group,required"`
	File     string `json:"file" jsonschema:"Absolute path to a local export archive (.tar.gz) on the MCP server filesystem,required"`
	ParentID *int64 `json:"parent_id,omitempty" jsonschema:"ID of the parent group to import into"`
}

// ImportFileOutput is the output for importing a group.
type ImportFileOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// ImportFile imports a group from an export archive.
func ImportFile(ctx context.Context, client *gitlabclient.Client, input ImportFileInput) (ImportFileOutput, error) {
	opts := &gl.GroupImportFileOptions{
		Name: new(input.Name),
		Path: new(input.Path),
		File: new(input.File),
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}

	_, err := client.GL().GroupImportExport.ImportFile(opts, gl.WithContext(ctx))
	if err != nil {
		return ImportFileOutput{}, toolutil.WrapErrWithStatusHint("import_group_file", err, http.StatusBadRequest, "verify the file path points to a valid .tar.gz group export archive")
	}
	return ImportFileOutput{Message: "Group import started successfully"}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// ValidActions returns description of available meta-tool actions.
func ValidActions() string {
	actions := []string{"schedule_export", "export_download", "import_file"}
	return strings.Join(actions, ", ")
}
