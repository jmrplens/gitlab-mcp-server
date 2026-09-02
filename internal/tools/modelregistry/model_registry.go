package modelregistry

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// Download retrieves a single file from a machine learning model
// package via the GitLab Model Registry API
// (GET /projects/:id/ml/models/:model_version_id/:path/:filename).
// Returns the raw bytes base64-encoded in ContentBase64, with
// SizeBytes describing the decoded size.
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
		if errors.Is(err, gitlabclient.ErrResponseTooLarge) {
			return DownloadOutput{}, toolutil.WrapErrWithHint("download ml model package", err, errModelFileTooLarge)
		}
		return DownloadOutput{}, toolutil.WrapErrWithStatusHint("download ml model package", err, http.StatusNotFound, "verify project_id, model_version_id, path, and filename")
	}

	return downloadOutput(in, reader)
}

// maxModelFileBytes is the largest model package file this action returns in
// one response.
//
// The bound is on what the download becomes, not on what it costs to read: the
// file is buffered, base64-encoded a third larger again and copied into a
// JSON-RPC message, and the client-wide response ceiling caps only the read.
// A file above it is refused rather than truncated, for the reason a group
// export is: the next step for a model file is loading it, and a prefix of a
// .safetensors or a .gguf loads no more than a prefix of a .tar.gz imports.
//
// It is the group export's ceiling rather than the 1 MiB the jobs package
// applies to artifact content, because that one truncates and flags it, which
// is a sensible partial answer for a build log and not for a weights file, and
// because 1 MiB cuts through the middle of what this endpoint is usable for: a
// model card and a config.json are kilobytes, but a tokenizer.json for a large
// vocabulary reaches into the tens of MiB. A ceiling between that and the
// hundreds of MiB a weights file occupies separates the files a caller can use
// here from the ones they must fetch another way, and 32 MiB is in that gap.
const maxModelFileBytes = 32 << 20

// errModelFileTooLarge is the hint for a file this action will not carry,
// whichever ceiling stopped it: this one, or the client-wide response ceiling
// that stops the read before it reaches here.
const errModelFileTooLarge = "the file is too large to return in one MCP response; download it from GitLab directly (GET /projects/:id/packages/ml_models/:model_version_id/files/:path/:filename)"

// downloadOutput reads a model package file stream up to
// [maxModelFileBytes] and returns the bytes base64-encoded alongside the
// original input metadata.
func downloadOutput(in DownloadInput, reader io.Reader) (DownloadOutput, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxModelFileBytes+1))
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithMessage("read ml model package content", err)
	}
	if len(data) > maxModelFileBytes {
		return DownloadOutput{}, toolutil.WrapErrWithHint(
			"download ml model package",
			fmt.Errorf("%s exceeds the %d MiB this action returns", in.Filename, maxModelFileBytes>>20),
			errModelFileTooLarge,
		)
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
