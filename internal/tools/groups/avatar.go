// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groups

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// AvatarInput is an image a group create uploads as the new group's avatar,
// as [gl.GroupAvatar] carries it: the file name GitLab reads the image type
// from, and the content, from a local file or as base64.
type AvatarInput struct {
	Filename      string `json:"filename" jsonschema:"Avatar file name such as avatar.png, whose extension GitLab reads the image type from,required"`
	FilePath      string `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// openAvatar opens the image an avatar input names, nil for no avatar, with
// the function that releases it.
func openAvatar(op string, in *AvatarInput) (*gl.GroupAvatar, func(), error) {
	if in == nil {
		return nil, func() { /* no avatar: nothing to release */ }, nil
	}
	if in.Filename == "" {
		return nil, nil, fmt.Errorf("%s: avatar.filename is required with an avatar, since GitLab reads the image type from its extension", op)
	}
	reader, _, release, err := toolutil.OpenFileOrBase64Source(op+" avatar", in.FilePath, in.ContentBase64)
	if err != nil {
		return nil, nil, err
	}
	return &gl.GroupAvatar{Filename: in.Filename, Image: reader}, release, nil
}

// attachAvatar puts an avatar on the create options. With one, client-go
// sends the create as a multipart upload (UploadRequest), which writes only
// the first value of each field, so the usernames excluded from the unique
// project download limit go as one comma-separated value, which GitLab
// splits back into the list (CommaSeparatedToArray in
// ee/lib/ee/api/helpers/groups_helpers.rb) a JSON body would have carried.
func attachAvatar(opts *gl.CreateGroupOptions, avatar *gl.GroupAvatar) {
	if avatar == nil {
		return
	}
	opts.Avatar = avatar
	if opts.UniqueProjectDownloadLimitAllowlist != nil {
		opts.UniqueProjectDownloadLimitAllowlist = new([]string{strings.Join(*opts.UniqueProjectDownloadLimitAllowlist, ",")})
	}
}

// UploadAvatar uploads or replaces the avatar for a group.
//
// It accepts either file_path (a local file the MCP server reads) or
// content_base64 (base64-encoded bytes), exactly one of which must be set, and
// streams the image to the GitLab Groups UploadAvatar endpoint as a multipart
// upload. The updated group is returned in the full group output shape.
func UploadAvatar(ctx context.Context, client *gitlabclient.Client, input UploadAvatarInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.GroupID == "" {
		return DetailOutput{}, errors.New("groupUploadAvatar: group_id is required")
	}
	if input.Filename == "" {
		return DetailOutput{}, errors.New("groupUploadAvatar: filename is required")
	}

	reader, _, cleanup, err := toolutil.OpenFileOrBase64Source("groupUploadAvatar", input.FilePath, input.ContentBase64)
	if err != nil {
		return DetailOutput{}, err
	}
	defer cleanup()

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	g, _, err := client.GL().Groups.UploadAvatar(string(input.GroupID), reader, input.Filename, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return DetailOutput{}, toolutil.WrapErrWithHint("groupUploadAvatar", err,
				"avatar must be JPG/PNG/GIF and under 200 KB; verify filename has a valid image extension")
		}
		return DetailOutput{}, toolutil.WrapErrWithStatusHint("groupUploadAvatar", err, http.StatusForbidden,
			"updating the group avatar requires Owner role")
	}
	return groupDetail("UploadAvatar", g, captured)
}
