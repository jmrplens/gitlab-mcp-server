package groups

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// UploadAvatarInput defines parameters for uploading a group avatar.
//
// Exactly one of FilePath or ContentBase64 must be provided to supply the image
// bytes. This mirrors the project avatar upload tool: file_path reads a local
// file from the MCP server filesystem (suitable for large images), while
// content_base64 carries inline base64-encoded image content.
type UploadAvatarInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Filename      string               `json:"filename" jsonschema:"Avatar filename (e.g. avatar.png),required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// UploadAvatar uploads or replaces the avatar for a group.
//
// It accepts either file_path (a local file the MCP server reads) or
// content_base64 (base64-encoded bytes), exactly one of which must be set, and
// streams the image to the GitLab Groups UploadAvatar endpoint as a multipart
// upload. The updated group is returned in the full group output shape.
func UploadAvatar(ctx context.Context, client *gitlabclient.Client, input UploadAvatarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupUploadAvatar: group_id is required")
	}
	if input.Filename == "" {
		return Output{}, errors.New("groupUploadAvatar: filename is required")
	}

	reader, _, cleanup, err := toolutil.OpenFileOrBase64Source("groupUploadAvatar", input.FilePath, input.ContentBase64)
	if err != nil {
		return Output{}, err
	}
	defer cleanup()

	g, _, err := client.GL().Groups.UploadAvatar(string(input.GroupID), reader, input.Filename, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("groupUploadAvatar", err,
				"avatar must be JPG/PNG/GIF and under 200 KB; verify filename has a valid image extension")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("groupUploadAvatar", err, http.StatusForbidden,
			"updating the group avatar requires Owner role")
	}
	return ToOutput(g), nil
}
