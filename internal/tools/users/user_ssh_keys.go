// user_ssh_keys.go implements SSH key CRUD operations for users:
// list for user, get, get for user, add, add for user, delete, delete for user.
package users

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListSSHKeysForUserInput holds parameters for listing SSH keys for a specific user.
type ListSSHKeysForUserInput struct {
	UserID  int64 `json:"user_id" jsonschema:"The ID of the user,required"`
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// ListSSHKeysForUser retrieves SSH keys for a specific user.
func ListSSHKeysForUser(ctx context.Context, client *gitlabclient.Client, input ListSSHKeysForUserInput) (SSHKeyListOutput, error) {
	if input.UserID == 0 {
		return SSHKeyListOutput{}, errors.New("list_ssh_keys_for_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return SSHKeyListOutput{}, err
	}

	opts := &gl.ListSSHKeysForUserOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}

	keys, resp, err := client.GL().Users.ListSSHKeysForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithStatusHint("list_ssh_keys_for_user", err, http.StatusNotFound,
			"verify user_id with gitlab_get_user; the user may have no SSH keys")
	}

	out := make([]SSHKeyOutput, 0, len(keys))
	for _, k := range keys {
		out = append(out, toSSHKeyOutput(k))
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

	k, _, err := client.GL().Users.GetSSHKey(input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("get_ssh_key", err, http.StatusNotFound,
			"verify key_id with gitlab_user_ssh_keys_list; the key may have been deleted")
	}
	return toSSHKeyOutput(k), nil
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

	k, _, err := client.GL().Users.GetSSHKeyForUser(input.UserID, input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("get_ssh_key_for_user", err, http.StatusNotFound,
			"verify user_id and key_id; admin token may be required to view other users' keys")
	}
	return toSSHKeyOutput(k), nil
}

// AddSSHKeyInput holds parameters for adding an SSH key to the current user.
type AddSSHKeyInput struct {
	Title     string `json:"title" jsonschema:"A descriptive title for the SSH key,required"`
	Key       string `json:"key" jsonschema:"The SSH public key content,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"Expiration date in ISO 8601 format (YYYY-MM-DD)"`
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

	opts := buildAddSSHKeyOptions(input)

	k, _, err := client.GL().Users.AddSSHKey(opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("add_ssh_key", err, http.StatusBadRequest,
			"key must be a valid SSH public key (ssh-rsa/ed25519/ecdsa) and not already used by another user; usage_type must be one of {auth, signing, auth_and_signing}; expires_at format YYYY-MM-DD")
	}
	return toSSHKeyOutput(k), nil
}

// AddSSHKeyForUserInput holds parameters for adding an SSH key to a specific user.
type AddSSHKeyForUserInput struct {
	UserID    int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	Title     string `json:"title" jsonschema:"A descriptive title for the SSH key,required"`
	Key       string `json:"key" jsonschema:"The SSH public key content,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"Expiration date in ISO 8601 format (YYYY-MM-DD)"`
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

	opts := buildAddSSHKeyOptions(AddSSHKeyInput{
		Title:     input.Title,
		Key:       input.Key,
		ExpiresAt: input.ExpiresAt,
		UsageType: input.UsageType,
	})

	k, _, err := client.GL().Users.AddSSHKeyForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyOutput{}, toolutil.WrapErrWithStatusHint("add_ssh_key_for_user", err, http.StatusForbidden,
			"adding SSH keys for other users requires admin token; key must be valid SSH public key and unique; verify user_id with gitlab_get_user")
	}
	return toSSHKeyOutput(k), nil
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
			"verify key_id with gitlab_user_ssh_keys_list; the key may already have been deleted")
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

// buildAddSSHKeyOptions builds SDK options from AddSSHKeyInput.
func buildAddSSHKeyOptions(input AddSSHKeyInput) *gl.AddSSHKeyOptions {
	opts := &gl.AddSSHKeyOptions{
		Title: new(input.Title),
		Key:   new(input.Key),
	}
	if input.ExpiresAt != "" {
		if t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt); err == nil {
			isoT := gl.ISOTime(t)
			opts.ExpiresAt = &isoT
		}
	}
	if input.UsageType != "" {
		opts.UsageType = new(input.UsageType)
	}
	return opts
}
