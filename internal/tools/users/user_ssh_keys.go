package users

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListSSHKeysForUserInput holds parameters for listing SSH keys for a specific user.
type ListSSHKeysForUserInput struct {
	UserID  int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListSSHKeysForUser retrieves SSH keys for a specific user.
func ListSSHKeysForUser(ctx context.Context, client *gitlabclient.Client, input ListSSHKeysForUserInput) (SSHKeyListOutput, error) {
	if input.UserID == 0 {
		return SSHKeyListOutput{}, errors.New("list_ssh_keys_for_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyListOutput{}, err
	}

	opts := &gl.ListSSHKeysForUserOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	keys, resp, err := client.GL().Users.ListSSHKeysForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithStatusHint("list_ssh_keys_for_user", err, http.StatusNotFound,
			"verify user_id with user.get; the user may have no SSH keys")
	}

	extras, err := toolutil.CapturedKeys(captured, len(keys))
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErr("list_ssh_keys_for_user", err)
	}

	out := make([]SSHKeyOutput, 0, len(keys))
	for i, k := range keys {
		out = append(out, toSSHKeyOutput(k, extras[i]))
	}
	return SSHKeyListOutput{
		Keys:       out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// GetSSHKeyInput holds parameters for retrieving a specific SSH key.
type GetSSHKeyInput struct {
	KeyID int64 `json:"key_id" jsonschema:"The ID of the SSH key,required"`
}

// GetSSHKey retrieves a specific SSH key for the current user.
func GetSSHKey(ctx context.Context, client *gitlabclient.Client, input GetSSHKeyInput) (SSHKeyOutput, error) {
	if input.KeyID == 0 {
		return SSHKeyOutput{}, errors.New("get_ssh_key: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyOutput{}, err
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	k, _, err := client.GL().Users.GetSSHKey(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("get_ssh_key", err, http.StatusNotFound,
			"verify key_id with user.ssh_keys; the key may have been deleted")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErr("get_ssh_key", err)
	}
	return toSSHKeyOutput(k, extra), nil
}

// GetSSHKeyForUserInput holds parameters for retrieving a specific SSH key for a user.
type GetSSHKeyForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
	KeyID  int64 `json:"key_id" jsonschema:"The ID of the SSH key,required"`
}

// GetSSHKeyForUser retrieves a specific SSH key for a user.
func GetSSHKeyForUser(ctx context.Context, client *gitlabclient.Client, input GetSSHKeyForUserInput) (SSHKeyOutput, error) {
	if input.UserID == 0 {
		return SSHKeyOutput{}, errors.New("get_ssh_key_for_user: user_id is required")
	}
	if input.KeyID == 0 {
		return SSHKeyOutput{}, errors.New("get_ssh_key_for_user: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyOutput{}, err
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	k, _, err := client.GL().Users.GetSSHKeyForUser(input.UserID, input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("get_ssh_key_for_user", err, http.StatusNotFound,
			"verify user_id and key_id; admin token may be required to view other users' keys")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErr("get_ssh_key_for_user", err)
	}
	return toSSHKeyOutput(k, extra), nil
}

// AddSSHKeyInput holds parameters for adding an SSH key to the current user.
type AddSSHKeyInput struct {
	Title     string `json:"title" jsonschema:"A descriptive title for the SSH key,required"`
	Key       string `json:"key" jsonschema:"The SSH public key content,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"Expiry time, an ISO 8601 timestamp such as 2026-12-31T23:00:00Z. A date such as 2026-12-31 is read as midnight UTC"`
	UsageType string `json:"usage_type,omitempty" jsonschema:"Usage type: auth or signing (default: auth)"`
}

// AddSSHKey adds an SSH key to the current authenticated user.
func AddSSHKey(ctx context.Context, client *gitlabclient.Client, input AddSSHKeyInput) (SSHKeyOutput, error) {
	if input.Title == "" {
		return SSHKeyOutput{}, errors.New("add_ssh_key: title is required")
	}
	if input.Key == "" {
		return SSHKeyOutput{}, errors.New("add_ssh_key: key is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyOutput{}, err
	}

	opts, sendExpiry, err := buildAddSSHKeyOptions(input)
	if err != nil {
		return SSHKeyOutput{}, fmt.Errorf("add_ssh_key: %w", err)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	k, _, err := client.GL().Users.AddSSHKey(opts, gl.WithContext(ctx), sendExpiry)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("add_ssh_key", err, http.StatusBadRequest,
			"key must be a valid SSH public key (ssh-rsa/ed25519/ecdsa) and not already used by another user; usage_type must be one of {auth, signing, auth_and_signing}; expires_at must be in the future")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErr("add_ssh_key", err)
	}
	return toSSHKeyOutput(k, extra), nil
}

// AddSSHKeyForUserInput holds parameters for adding an SSH key to a specific user.
type AddSSHKeyForUserInput struct {
	UserID    int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	Title     string `json:"title" jsonschema:"A descriptive title for the SSH key,required"`
	Key       string `json:"key" jsonschema:"The SSH public key content,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"Expiry time, an ISO 8601 timestamp such as 2026-12-31T23:00:00Z. A date such as 2026-12-31 is read as midnight UTC"`
	UsageType string `json:"usage_type,omitempty" jsonschema:"Usage type: auth or signing (default: auth)"`
}

// AddSSHKeyForUser adds an SSH key to a specific user (admin only).
func AddSSHKeyForUser(ctx context.Context, client *gitlabclient.Client, input AddSSHKeyForUserInput) (SSHKeyOutput, error) {
	if input.UserID == 0 {
		return SSHKeyOutput{}, errors.New("add_ssh_key_for_user: user_id is required")
	}
	if input.Title == "" {
		return SSHKeyOutput{}, errors.New("add_ssh_key_for_user: title is required")
	}
	if input.Key == "" {
		return SSHKeyOutput{}, errors.New("add_ssh_key_for_user: key is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyOutput{}, err
	}

	opts, sendExpiry, err := buildAddSSHKeyOptions(AddSSHKeyInput{
		Title:     input.Title,
		Key:       input.Key,
		ExpiresAt: input.ExpiresAt,
		UsageType: input.UsageType,
	})
	if err != nil {
		return SSHKeyOutput{}, fmt.Errorf("add_ssh_key_for_user: %w", err)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	k, _, err := client.GL().Users.AddSSHKeyForUser(input.UserID, opts, gl.WithContext(ctx), sendExpiry)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("add_ssh_key_for_user", err, http.StatusForbidden,
			"adding SSH keys for other users requires admin token; key must be valid SSH public key and unique; verify user_id with user.get")
	}
	extra, err := toolutil.CapturedKey(captured)
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErr("add_ssh_key_for_user", err)
	}
	return toSSHKeyOutput(k, extra), nil
}

// DeleteSSHKeyInput holds parameters for deleting an SSH key from the current user.
type DeleteSSHKeyInput struct {
	KeyID int64 `json:"key_id" jsonschema:"The ID of the SSH key to delete,required"`
}

// DeleteSSHKeyOutput represents the result of deleting an SSH key.
type DeleteSSHKeyOutput struct {
	KeyID   int64 `json:"key_id"`
	Deleted bool  `json:"deleted"`
}

// DeleteSSHKey deletes an SSH key from the current authenticated user.
func DeleteSSHKey(ctx context.Context, client *gitlabclient.Client, input DeleteSSHKeyInput) (DeleteSSHKeyOutput, error) {
	if input.KeyID == 0 {
		return DeleteSSHKeyOutput{}, errors.New("delete_ssh_key: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteSSHKeyOutput{}, err
	}

	_, err := client.GL().Users.DeleteSSHKey(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return DeleteSSHKeyOutput{}, toolutil.WrapErrWithStatusHint("delete_ssh_key", err, http.StatusNotFound,
			"verify key_id with user.ssh_keys; the key may already have been deleted")
	}
	return DeleteSSHKeyOutput{KeyID: input.KeyID, Deleted: true}, nil
}

// DeleteSSHKeyForUserInput holds parameters for deleting an SSH key from a specific user.
type DeleteSSHKeyForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
	KeyID  int64 `json:"key_id" jsonschema:"The ID of the SSH key to delete,required"`
}

// DeleteSSHKeyForUser deletes an SSH key from a specific user (admin only).
func DeleteSSHKeyForUser(ctx context.Context, client *gitlabclient.Client, input DeleteSSHKeyForUserInput) (DeleteSSHKeyOutput, error) {
	if input.UserID == 0 {
		return DeleteSSHKeyOutput{}, errors.New("delete_ssh_key_for_user: user_id is required")
	}
	if input.KeyID == 0 {
		return DeleteSSHKeyOutput{}, errors.New("delete_ssh_key_for_user: key_id is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteSSHKeyOutput{}, err
	}

	_, err := client.GL().Users.DeleteSSHKeyForUser(input.UserID, input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return DeleteSSHKeyOutput{}, toolutil.WrapErrWithStatusHint("delete_ssh_key_for_user", err, http.StatusForbidden,
			"deleting SSH keys for other users requires admin token; verify user_id and key_id")
	}
	return DeleteSSHKeyOutput{KeyID: input.KeyID, Deleted: true}, nil
}

// buildAddSSHKeyOptions builds SDK options from AddSSHKeyInput, and the
// request option that sends the expiry.
//
// An expiry that does not parse refuses the call. The guard this replaced
// read only YYYY-MM-DD and dropped anything else, an RFC 3339 timestamp
// included, so a key asked to expire was created with no expiry and reported
// as a success (issue 1308). GitLab takes a timestamp here, while client-go
// carries the field as a date, which would reach GitLab as midnight UTC and
// expire a key asked to last until 23:00 at the start of its day, or refuse
// one asked to expire later today as already expired (register row 126). The
// option struct's field is therefore left unset and the expiry is written as
// the instant given by [toolutil.TimestampParam].
func buildAddSSHKeyOptions(input AddSSHKeyInput) (*gl.AddSSHKeyOptions, gl.RequestOptionFunc, error) {
	expiresAt, err := toolutil.ParseTimeField("expires_at", input.ExpiresAt)
	if err != nil {
		return nil, nil, err
	}
	opts := &gl.AddSSHKeyOptions{
		Title: new(input.Title),
		Key:   new(input.Key),
	}
	if input.UsageType != "" {
		opts.UsageType = new(input.UsageType)
	}
	return opts, toolutil.TimestampParam("expires_at", expiresAt), nil
}
