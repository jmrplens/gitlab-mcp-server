// Package modelregistry implements MCP tools for GitLab ML model registry,
// providing download access to machine learning model package files.
//
// The package also registers model registry MCP tools and renders Markdown
// summaries for downloaded model package files.
package modelregistry

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// DownloadInput holds parameters for downloading a ML model package file.
type DownloadInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	ModelVersionID toolutil.StringOrInt `json:"model_version_id" jsonschema:"Model version ID (numeric or string like candidate:5),required"`
	Path           string               `json:"path"             jsonschema:"Path within the model package,required"`
	Filename       string               `json:"filename"         jsonschema:"Name of the file to download,required"`
}

// DownloadOutput represents the downloaded ML model package file.
type DownloadOutput struct {
	toolutil.HintableOutput
	ProjectID      string `json:"project_id"`
	ModelVersionID string `json:"model_version_id"`
	Path           string `json:"path"`
	Filename       string `json:"filename"`
	ContentBase64  string `json:"content_base64"`
	SizeBytes      int    `json:"size_bytes"`
}

// Download retrieves a machine learning model package file.
func Download(ctx context.Context, client *gitlabclient.Client, in DownloadInput) (DownloadOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.ModelVersionID.String() == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("model_version_id")
	}
	if in.Path == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("path")
	}
	if in.Filename == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("filename")
	}

	reader, _, err := client.GL().ModelRegistry.DownloadMachineLearningModelPackage(
		in.ProjectID.String(), in.ModelVersionID.String(), in.Path, in.Filename,
		gl.WithContext(ctx),
	)
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithStatusHint("download ml model package", err, http.StatusNotFound, "verify project_id, model_version_id, path, and filename")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithMessage("read ml model package content", err)
	}

	return DownloadOutput{
		ProjectID:      in.ProjectID.String(),
		ModelVersionID: in.ModelVersionID.String(),
		Path:           in.Path,
		Filename:       in.Filename,
		ContentBase64:  base64.StdEncoding.EncodeToString(data),
		SizeBytes:      len(data),
	}, nil
}
