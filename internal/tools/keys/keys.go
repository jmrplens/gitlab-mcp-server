// Package keys implements MCP tools for GitLab SSH key lookup operations.
package keys

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// GetByIDInput is the input for getting a key by its ID.
type GetByIDInput struct {
	KeyID int64 `json:"key_id" jsonschema:"SSH key ID,required"`
}

// GetByFingerprintInput is the input for getting a key by fingerprint.
type GetByFingerprintInput struct {
	Fingerprint string `json:"fingerprint" jsonschema:"SSH key fingerprint (e.g. SHA256:abc123 or MD5:aa:bb:cc),required"`
}

// Output types.

// UserOutput represents the user associated with a key.
type UserOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Output represents an SSH key with its associated user.
type Output struct {
	toolutil.HintableOutput
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Key       string     `json:"key"`
	CreatedAt string     `json:"created_at,omitempty"`
	User      UserOutput `json:"user"`
}

// Handlers.

// GetKeyWithUser retrieves an SSH key and the user it belongs to.
func GetKeyWithUser(ctx context.Context, client *gitlabclient.Client, input GetByIDInput) (Output, error) {
	if input.KeyID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("key_get", toolutil.ErrFieldRequired("key_id"))
	}
	key, _, err := client.GL().Keys.GetKeyWithUser(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("key_get", err)
	}
	return toOutput(key), nil
}

// GetKeyByFingerprint retrieves an SSH key by its fingerprint.
func GetKeyByFingerprint(ctx context.Context, client *gitlabclient.Client, input GetByFingerprintInput) (Output, error) {
	if input.Fingerprint == "" {
		return Output{}, toolutil.WrapErrWithMessage("key_get_by_fingerprint", toolutil.ErrFieldRequired("fingerprint"))
	}
	opts := &gl.GetKeyByFingerprintOptions{Fingerprint: input.Fingerprint}
	key, _, err := client.GL().Keys.GetKeyByFingerprint(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("key_get_by_fingerprint", err)
	}
	return toOutput(key), nil
}

// Converters.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(k *gl.Key) Output {
	out := Output{
		ID:    k.ID,
		Title: k.Title,
		Key:   k.Key,
		User: UserOutput{
			ID:       k.User.ID,
			Username: k.User.Username,
			Name:     k.User.Name,
		},
	}
	if k.CreatedAt != nil {
		out.CreatedAt = k.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// Formatters.

// truncateKey is an internal helper for the keys package.
func truncateKey(key string) string {
	if len(key) > 60 {
		return key[:57] + "..."
	}
	return key
}
