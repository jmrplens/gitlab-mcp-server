package keys

import (
	"context"
	"net/http"
	"strings"
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
	Fingerprint string `json:"fingerprint" jsonschema:"SSH key fingerprint: SHA256: and the 43 base64 characters ssh-keygen -l prints, or the MD5 form of 16 colon-separated hex pairs, with or without the MD5: prefix ssh-keygen -l -E md5 prints,required"`
}

// md5Prefix is the label ssh-keygen -l -E md5 writes before an MD5
// fingerprint. GitLab reads an MD5 fingerprint as the bare hex pairs alone
// (KeysFinder#valid_fingerprint_param? matches them from the first
// character), so a fingerprint copied with the label is refused there.
const md5Prefix = "MD5:"

// fingerprintHint is what a lookup GitLab answered 400 or 404 is told: the
// two forms GitLab reads a fingerprint in, since it refuses any other with
// 400, and that a fingerprint in either form that names no key answers 404.
const fingerprintHint = "GitLab reads a fingerprint as SHA256: and the 43 base64 characters ssh-keygen -l prints, or as the 16 colon-separated hex pairs of the MD5 form. It answers 400 to any other form and 404 when no SSH key or deploy key has the fingerprint"

// Output types.

// UserOutput represents the user associated with a key.
type UserOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Output represents an SSH key with its associated user, as [gl.Key] decodes
// it, the expiry, last use and usage type lib/api/entities/ssh_key.rb sends on
// every key included.
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
	key, _, err := client.GL().Keys.GetKeyWithUser(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("key_get", err, http.StatusNotFound,
			"verify key_id with user.ssh_keys_for_user. This admin endpoint requires administrator access on self-managed instances")
	}
	return toOutput(key), nil
}

// GetKeyByFingerprint retrieves an SSH key by its fingerprint via the
// GitLab admin Keys API (GET /keys). The fingerprint may be in the
// modern SHA256:base64 form or the legacy MD5 hex-pairs form, whose MD5:
// label is removed before the request, since GitLab reads the pairs alone.
func GetKeyByFingerprint(ctx context.Context, client *gitlabclient.Client, input GetByFingerprintInput) (Output, error) {
	if input.Fingerprint == "" {
		return Output{}, toolutil.WrapErrWithMessage("key_get_by_fingerprint", toolutil.ErrFieldRequired("fingerprint"))
	}
	opts := &gl.GetKeyByFingerprintOptions{Fingerprint: strings.TrimPrefix(input.Fingerprint, md5Prefix)}
	key, _, err := client.GL().Keys.GetKeyByFingerprint(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("key_get_by_fingerprint", err, fingerprintHint)
		}
		return Output{}, toolutil.WrapErrWithMessage("key_get_by_fingerprint", err)
	}
	return toOutput(key), nil
}

// Converters.

// toOutput converts a [gl.Key] (with embedded user) into the package's
// [Output], formatting the timestamps as RFC 3339.
func toOutput(k *gl.Key) Output {
	out := Output{
		ID:         k.ID,
		Title:      k.Title,
		Key:        k.Key,
		ExpiresAt:  toolutil.FormatTimePtr(k.ExpiresAt),
		LastUsedAt: toolutil.FormatTimePtr(k.LastUsedAt),
		UsageType:  k.UsageType,
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
