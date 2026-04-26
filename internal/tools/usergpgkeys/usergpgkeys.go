// Package usergpgkeys implements GitLab GPG key management operations for users.
package usergpgkeys

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents a GPG key.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListOutput holds a list of GPG keys.
type ListOutput struct {
	toolutil.HintableOutput
	Keys []Output `json:"keys"`
}

// ListInput is empty — lists GPG keys for the current user.
type ListInput struct{}

// List retrieves GPG keys for the current authenticated user.
func List(ctx context.Context, client *gitlabclient.Client, _ ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	keys, _, err := client.GL().Users.ListGPGKeys(gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_gpg_keys", err, http.StatusUnauthorized,
			"GPG key listing requires an authenticated user; verify your token is valid (api or read_user scope)")
	}
	return ListOutput{Keys: toOutputList(keys)}, nil
}

// ListForUserInput holds parameters for listing GPG keys for a specific user.
type ListForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
}

// ListForUser retrieves GPG keys for a specific user.
func ListForUser(ctx context.Context, client *gitlabclient.Client, input ListForUserInput) (ListOutput, error) {
	if input.UserID == 0 {
		return ListOutput{}, errors.New("list_gpg_keys_for_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	keys, _, err := client.GL().Users.ListGPGKeysForUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_gpg_keys_for_user", err, http.StatusNotFound,
			"verify user_id with gitlab_user_get; viewing other users' GPG keys may require admin token")
	}
	return ListOutput{Keys: toOutputList(keys)}, nil
}

// GetInput holds parameters for retrieving a specific GPG key.
type GetInput struct {
	KeyID int64 `json:"key_id" jsonschema:"The ID of the GPG key,required"`
}

// Get retrieves a specific GPG key for the current user.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.KeyID == 0 {
		return Output{}, errors.New("get_gpg_key: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	k, _, err := client.GL().Users.GetGPGKey(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get_gpg_key", err, http.StatusNotFound,
			"verify key_id with gitlab_user_gpg_keys_list; the key may have been deleted")
	}
	return toOutput(k), nil
}

// GetForUserInput holds parameters for retrieving a specific GPG key for a user.
type GetForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
	KeyID  int64 `json:"key_id" jsonschema:"The ID of the GPG key,required"`
}

// GetForUser retrieves a specific GPG key for a specific user.
func GetForUser(ctx context.Context, client *gitlabclient.Client, input GetForUserInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("get_gpg_key_for_user: user_id is required")
	}
	if input.KeyID == 0 {
		return Output{}, errors.New("get_gpg_key_for_user: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	k, _, err := client.GL().Users.GetGPGKeyForUser(input.UserID, input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get_gpg_key_for_user", err, http.StatusNotFound,
			"verify user_id and key_id with gitlab_user_gpg_keys_list_for_user; admin token may be required")
	}
	return toOutput(k), nil
}

// AddInput holds parameters for adding a GPG key to the current user.
type AddInput struct {
	Key string `json:"key" jsonschema:"The armored GPG public key content,required"`
}

// Add adds a GPG key to the current authenticated user.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.Key == "" {
		return Output{}, errors.New("add_gpg_key: key is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	opts := &gl.AddGPGKeyOptions{Key: new(input.Key)}
	k, _, err := client.GL().Users.AddGPGKey(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("add_gpg_key", err, http.StatusBadRequest,
			"key must be an ASCII-armored OpenPGP public key block beginning with '-----BEGIN PGP PUBLIC KEY BLOCK-----'; the key fingerprint must be unique across GitLab")
	}
	return toOutput(k), nil
}

// AddForUserInput holds parameters for adding a GPG key to a specific user.
type AddForUserInput struct {
	UserID int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	Key    string `json:"key" jsonschema:"The armored GPG public key content,required"`
}

// AddForUser adds a GPG key to a specific user (admin only).
func AddForUser(ctx context.Context, client *gitlabclient.Client, input AddForUserInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("add_gpg_key_for_user: user_id is required")
	}
	if input.Key == "" {
		return Output{}, errors.New("add_gpg_key_for_user: key is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	opts := &gl.AddGPGKeyOptions{Key: new(input.Key)}
	k, _, err := client.GL().Users.AddGPGKeyForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("add_gpg_key_for_user", err, http.StatusForbidden,
			"adding GPG keys for other users requires admin token; key must be ASCII-armored and unique")
	}
	return toOutput(k), nil
}

// DeleteInput holds parameters for deleting a GPG key from the current user.
type DeleteInput struct {
	KeyID int64 `json:"key_id" jsonschema:"The ID of the GPG key to delete,required"`
}

// DeleteOutput represents the result of deleting a GPG key.
type DeleteOutput struct {
	KeyID   int64 `json:"key_id"`
	Deleted bool  `json:"deleted"`
}

// Delete deletes a GPG key from the current authenticated user.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if input.KeyID == 0 {
		return DeleteOutput{}, errors.New("delete_gpg_key: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	_, err := client.GL().Users.DeleteGPGKey(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithStatusHint("delete_gpg_key", err, http.StatusNotFound,
			"verify key_id with gitlab_user_gpg_keys_list; the key may already have been deleted")
	}
	return DeleteOutput{KeyID: input.KeyID, Deleted: true}, nil
}

// DeleteForUserInput holds parameters for deleting a GPG key from a specific user.
type DeleteForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
	KeyID  int64 `json:"key_id" jsonschema:"The ID of the GPG key to delete,required"`
}

// DeleteForUser deletes a GPG key from a specific user (admin only).
func DeleteForUser(ctx context.Context, client *gitlabclient.Client, input DeleteForUserInput) (DeleteOutput, error) {
	if input.UserID == 0 {
		return DeleteOutput{}, errors.New("delete_gpg_key_for_user: user_id is required")
	}
	if input.KeyID == 0 {
		return DeleteOutput{}, errors.New("delete_gpg_key_for_user: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	_, err := client.GL().Users.DeleteGPGKeyForUser(input.UserID, input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithStatusHint("delete_gpg_key_for_user", err, http.StatusForbidden,
			"deleting GPG keys for other users requires admin token; verify user_id and key_id with gitlab_user_gpg_keys_list_for_user")
	}
	return DeleteOutput{KeyID: input.KeyID, Deleted: true}, nil
}

// Conversion helpers.

func toOutput(k *gl.GPGKey) Output {
	o := Output{ID: k.ID, Key: k.Key}
	if k.CreatedAt != nil {
		o.CreatedAt = k.CreatedAt.Format(time.RFC3339)
	}
	return o
}

func toOutputList(keys []*gl.GPGKey) []Output {
	out := make([]Output, 0, len(keys))
	for _, k := range keys {
		out = append(out, toOutput(k))
	}
	return out
}

// Markdown formatters.

// FormatListMarkdownString renders a GPG key list as a Markdown string.
func FormatListMarkdownString(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GPG Keys (%d)\n\n", len(o.Keys))
	if len(o.Keys) == 0 {
		b.WriteString("No GPG keys found.\n")
	} else {
		b.WriteString("| ID | Key (truncated) | Created At |\n")
		b.WriteString("|---|---|---|\n")
		for _, k := range o.Keys {
			keyPreview := k.Key
			if len(keyPreview) > 40 {
				keyPreview = keyPreview[:40] + "..."
			}
			fmt.Fprintf(&b, "| %d | `%s` | %s |\n",
				k.ID, toolutil.EscapeMdTableCell(keyPreview), toolutil.FormatTime(k.CreatedAt))
		}
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_get_gpg_key` to view full key details",
	)
	return b.String()
}

// FormatMarkdownString renders a single GPG key as a Markdown string.
func FormatMarkdownString(o Output) string {
	var b strings.Builder
	b.WriteString("## GPG Key\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	keyPreview := o.Key
	if len(keyPreview) > 80 {
		keyPreview = keyPreview[:80] + "..."
	}
	fmt.Fprintf(&b, "- **Key**: `%s`\n", keyPreview)
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(o.CreatedAt))
	}
	return b.String()
}
