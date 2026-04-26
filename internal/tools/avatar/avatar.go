// Package avatar implements MCP tools for GitLab avatar retrieval.
package avatar

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetInput contains parameters for getting an avatar URL.
type GetInput struct {
	Email string `json:"email" jsonschema:"Email address to look up avatar for,required"`
	Size  int64  `json:"size" jsonschema:"Desired avatar size in pixels"`
}

// GetOutput contains the avatar URL.
type GetOutput struct {
	toolutil.HintableOutput
	AvatarURL string `json:"avatar_url"`
}

// Get retrieves the avatar URL for an email address.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	opts := &gl.GetAvatarOptions{
		Email: new(input.Email),
	}
	if input.Size > 0 {
		opts.Size = new(input.Size)
	}
	a, _, err := client.GL().Avatar.GetAvatar(opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_avatar", err, http.StatusBadRequest, "verify email address format")
	}
	return GetOutput{AvatarURL: a.AvatarURL}, nil
}
