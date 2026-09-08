package keys

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Input types.

// GetByIDInput is the input for getting a key by its ID.
type GetByIDInput struct {
	KeyID int64 `json:"key_id" jsonschema:"SSH key ID,required"`
}

// GetByFingerprintInput is the input for getting a key by fingerprint.
type GetByFingerprintInput struct {
	Fingerprint string `json:"fingerprint" jsonschema:"SSH key fingerprint, either the SHA256 form (for example SHA256:abc123) or the legacy MD5 hex-pair form (for example MD5:aa:bb:cc),required"`
}

// Output types.

// UserOutput represents the user associated with a key.
type UserOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Output represents an SSH key with its associated user: what [gl.Key]
// decodes, plus the expiry, last use and usage type lib/api/entities/ssh_key.rb
// sends on every key and the SDK does not carry, read from the captured
// response (ADR-0021).
type Output struct {
	toolutil.HintableOutput
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Key        string     `json:"key"`
	CreatedAt  string     `json:"created_at,omitempty"`
	ExpiresAt  string     `json:"expires_at,omitempty"`
	LastUsedAt string     `json:"last_used_at,omitempty"`
	UsageType  string     `json:"usage_type,omitempty"`
	User       UserOutput `json:"user"`
}

// Handlers.

// GetKeyWithUser retrieves an SSH key and its owning user via the
// GitLab admin Keys API (GET /keys/:key_id). Requires administrator
// access on self-managed GitLab instances.
func GetKeyWithUser(ctx context.Context, client *gitlabclient.Client, input GetByIDInput) (Output, error) {
	if input.KeyID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("key_get", toolutil.ErrFieldRequired("key_id"))
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	key, _, err := client.GL().Keys.GetKeyWithUser(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("key_get", err, http.StatusNotFound,
			"verify key_id with gitlab_list_ssh_keys_for_user. This admin endpoint requires administrator access on self-managed instances")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("key_get", err)
	}
	return toOutput(key, extra), nil
}

// GetKeyByFingerprint retrieves an SSH key by its fingerprint via the
// GitLab admin Keys API (GET /keys). The fingerprint may be in the
// modern SHA256:base64 form or the legacy MD5:hex-pairs form.
func GetKeyByFingerprint(ctx context.Context, client *gitlabclient.Client, input GetByFingerprintInput) (Output, error) {
	if input.Fingerprint == "" {
		return Output{}, toolutil.WrapErrWithMessage("key_get_by_fingerprint", toolutil.ErrFieldRequired("fingerprint"))
	}
	opts := &gl.GetKeyByFingerprintOptions{Fingerprint: input.Fingerprint}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	key, _, err := client.GL().Keys.GetKeyByFingerprint(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("key_get_by_fingerprint", err, http.StatusNotFound,
			"fingerprint format must be SHA256:base64 (43 chars, no padding) or MD5:aa:bb:cc:... (32 hex pairs); requires administrator access")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("key_get_by_fingerprint", err)
	}
	return toOutput(key, extra), nil
}

// Converters.

// toOutput converts a [gl.Key] (with embedded user) into the package's
// [Output], formatting the timestamps as RFC 3339, and takes the fields the
// capture read beside the SDK.
func toOutput(k *gl.Key, extra toolutil.KeyExtra) Output {
	out := Output{
		ID:         k.ID,
		Title:      k.Title,
		Key:        k.Key,
		ExpiresAt:  toolutil.FormatTimePtr(extra.ExpiresAt),
		LastUsedAt: toolutil.FormatTimePtr(extra.LastUsedAt),
		UsageType:  extra.UsageType,
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

// truncateKey shortens long public keys for readable Markdown tables.
func truncateKey(key string) string {
	if len(key) > 60 {
		return key[:57] + "..."
	}
	return key
}
