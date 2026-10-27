// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupimportexport

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		return ScheduleExportOutput{}, toolutil.WrapErrWithStatusHint("schedule_group_export", err, http.StatusNotFound, "verify group_id with group.get")
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

// errExportTooLarge is the hint an operator or model gets for an archive this
// action will not carry, whichever ceiling stopped it: the shared download
// ceiling ([toolutil.MaxDownloadBytes]), or the client-wide response ceiling
// that stops the read before it reaches here. An archive above either is
// refused rather than truncated: the documented next step for this action is
// importing the archive into another group, and a prefix of a .tar.gz cannot
// be imported.
const errExportTooLarge = "the archive is too large to return in one MCP response; download it from GitLab directly (GET /groups/:id/export/download)"

// ExportDownload downloads the finished export archive of a group as base64.
//
// client-go buffers the body before it hands it back, so the shared ceiling
// is also set as a response limit on this one request, which stops the read
// of an archive over it where it happens.
func ExportDownload(ctx context.Context, client *gitlabclient.Client, input ExportDownloadInput) (ExportDownloadOutput, error) {
	limited := gitlabclient.WithResponseLimit(ctx, toolutil.MaxDownloadBytes)
	reader, _, err := client.GL().GroupImportExport.ExportDownload(string(input.GroupID), gl.WithContext(limited))
	if err != nil {
		if toolutil.IsDownloadTooLarge(err) {
			return ExportDownloadOutput{}, toolutil.WrapErrWithHint("download_group_export", err, errExportTooLarge)
		}
		return ExportDownloadOutput{}, toolutil.WrapErrWithStatusHint("download_group_export", err, http.StatusNotFound, "export must be scheduled first with group.group_export_schedule")
	}

	return exportDownloadOutput(reader)
}

func exportDownloadOutput(reader io.Reader) (ExportDownloadOutput, error) {
	data, err := toolutil.ReadDownload("download_group_export", reader, errExportTooLarge)
	if err != nil {
		return ExportDownloadOutput{}, err
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
	File     string `json:"file" jsonschema:"Canonical path to a local export archive (.tar.gz) under the current working directory, OS temp directory, or GITLAB_MCP_ALLOWED_IMPORT_DIRS. Symlinks are resolved and escapes are rejected,required"`
	ParentID *int64 `json:"parent_id,omitempty" jsonschema:"ID of the parent group to import into"`
}

// ImportFileOutput is the output for importing a group.
type ImportFileOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// ImportFile imports a group from an export archive.
func ImportFile(ctx context.Context, client *gitlabclient.Client, input ImportFileInput) (ImportFileOutput, error) {
	archivePath, err := toolutil.CanonicalImportArchivePath(input.File)
	if err != nil {
		return ImportFileOutput{}, toolutil.WrapErrWithMessage("import_group_file", err)
	}
	opts := &gl.GroupImportFileOptions{
		Name: new(input.Name),
		Path: new(input.Path),
		File: new(archivePath),
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}

	_, err = client.GL().GroupImportExport.ImportFile(opts, gl.WithContext(ctx))
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
