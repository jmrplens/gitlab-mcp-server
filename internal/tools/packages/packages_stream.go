package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// newDownloadRequest builds the package file GET. The request is built here
// rather than through a service method, so the context goes in as the same
// option every service call takes: without it client-go builds the request
// from context.Background(), and neither the action deadline nor an abandoned
// call would end a transfer nothing else bounds.
//
// It is a package variable so a test can reach the construction failure,
// which no input reaches: FormatPackageURL escapes every segment it
// interpolates, so the path always unescapes, and the one option passed
// never fails.
var newDownloadRequest = func(ctx context.Context, client *gitlabclient.Client, path string) (*retryablehttp.Request, error) {
	return client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
}

// streamDownloadPackageFile downloads a package file by streaming the HTTP
// response body directly to disk. It computes the SHA-256 checksum during
// transfer using io.MultiWriter so the file is never fully loaded into memory.
//
// The function leverages client-go's Client.Do(req, io.Writer) code path,
// which calls io.Copy(writer, resp.Body), preserving the client's
// authentication headers, rate limiting, and retryable HTTP logic.
//
// The body goes through [toolutil.WriteDownloadOutputFile], so output_path is
// written only once the whole body has arrived: an error answer, a body cut
// short or a cancelled call leaves it exactly as it was.
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
		hint := ""
		if errors.Is(err, gl.ErrInvalidFileName) {
			hint = " (file_name segments between / separators must not be empty, \".\", or \"..\")"
		}
		return 0, "", fmt.Errorf("format package URL: %w%s", err, hint)
	}

	httpReq, err := newDownloadRequest(ctx, client, apiPath)
	if err != nil {
		return 0, "", fmt.Errorf("create download request: %w", err)
	}

	hasher := sha256.New()
	size, err := toolutil.WriteDownloadOutputFile(input.OutputPath, func(file io.Writer) error {
		body := io.MultiWriter(file, hasher)
		tracker := progress.FromRequest(req)
		if tracker.IsActive() {
			body = toolutil.NewProgressWriter(ctx, body, 0, tracker)
		}
		if _, doErr := client.GL().Do(httpReq, body); doErr != nil {
			return fmt.Errorf("stream download %s: %w", input.FileName, doErr)
		}
		return nil
	})
	if err != nil {
		return 0, "", err
	}

	return size, hex.EncodeToString(hasher.Sum(nil)), nil
}
