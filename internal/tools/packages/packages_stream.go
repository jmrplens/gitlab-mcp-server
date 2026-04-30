// packages_stream.go implements streaming download for GitLab Generic Package
// files. Instead of loading the entire file into memory (as DownloadPackageFile
// does), it streams the HTTP response body directly to disk via io.Copy.
//
// This uses the client-go Client.Do(req, io.Writer) path which streams the
// response body to any io.Writer, preserving the client's authentication,
// rate limiting, and retry logic.
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// streamDownloadPackageFile downloads a package file by streaming the HTTP
// response body directly to disk. It computes the SHA-256 checksum during
// transfer using io.MultiWriter so the file is never fully loaded into memory.
//
// The function leverages client-go's Client.Do(req, io.Writer) code path,
// which calls io.Copy(writer, resp.Body), preserving the client's
// authentication headers, rate limiting, and retryable HTTP logic.
func streamDownloadPackageFile(
	ctx context.Context,
	req *mcp.CallToolRequest,
	client *gitlabclient.Client,
	input DownloadInput,
) (_ int64, _ string, _ error) {
	if err := ctx.Err(); err != nil {
		return 0, "", fmt.Errorf(fmtCtxCancelled, err)
	}

	projectID := string(input.ProjectID)

	apiPath, err := client.GL().GenericPackages.FormatPackageURL(
		projectID, input.PackageName, input.PackageVersion, input.FileName,
	)
	if err != nil {
		return 0, "", fmt.Errorf("format package URL: %w", err)
	}

	httpReq, err := client.GL().NewRequest(http.MethodGet, apiPath, nil, nil)
	if err != nil {
		return 0, "", fmt.Errorf("create download request: %w", err)
	}

	dir := filepath.Dir(input.OutputPath)
	if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
		return 0, "", fmt.Errorf("create output directory %s: %w", dir, mkdirErr)
	}

	outFile, err := os.Create(input.OutputPath)
	if err != nil {
		return 0, "", fmt.Errorf("create output file %s: %w", input.OutputPath, err)
	}
	defer outFile.Close()

	hasher := sha256.New()

	var baseWriter = io.MultiWriter(outFile, hasher)

	tracker := progress.FromRequest(req)
	if tracker.IsActive() {
		baseWriter = toolutil.NewProgressWriter(ctx, baseWriter, 0, tracker)
	}

	_, err = client.GL().Do(httpReq, baseWriter)
	if err != nil {
		return 0, "", fmt.Errorf("stream download %s: %w", input.FileName, err)
	}

	if err = outFile.Sync(); err != nil {
		return 0, "", fmt.Errorf("sync output file: %w", err)
	}

	info, err := outFile.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("stat output file: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	return info.Size(), checksum, nil
}
