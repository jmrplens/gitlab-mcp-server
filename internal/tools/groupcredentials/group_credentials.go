package groupcredentials

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	groupCredentialInventoryHint = "verify group_id with gitlab_group_get; group credential inventory can return 404 when this GitLab instance does not expose the /groups/:id/manage endpoints; requires Ultimate and Owner or admin access"
	groupCredentialTokenHint     = "verify token_id with credential_list_pats; if credential_list_pats returns 404, group credential inventory may be unavailable on this GitLab instance; requires Ultimate and Owner or admin access"
	//nolint:gosec // Error hint text only; no secret material.
	groupCredentialSSHKeyHint = "verify key_id with credential_list_ssh_keys; if credential_list_ssh_keys returns 404, group credential inventory may be unavailable on this GitLab instance; requires Ultimate and Owner or admin access"
)

// ListPATsInput holds parameters for listing group personal access tokens.
type ListPATsInput struct {
	GroupID        toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	Search         string               `json:"search,omitempty"          jsonschema:"Filter tokens by name"`
	State          string               `json:"state,omitempty"           jsonschema:"Filter by token state: active or inactive"`
	Revoked        *bool                `json:"revoked,omitempty"         jsonschema:"Filter by revoked status"`
	CreatedAfter   string               `json:"created_after,omitempty"   jsonschema:"Return tokens created on or after this date (YYYY-MM-DD)"`
	CreatedBefore  string               `json:"created_before,omitempty"  jsonschema:"Return tokens created on or before this date (YYYY-MM-DD)"`
	LastUsedAfter  string               `json:"last_used_after,omitempty" jsonschema:"Return tokens last used on or after this date (YYYY-MM-DD)"`
	LastUsedBefore string               `json:"last_used_before,omitempty" jsonschema:"Return tokens last used on or before this date (YYYY-MM-DD)"`
	OrderBy        string               `json:"order_by,omitempty"        jsonschema:"Column to order keyset-paginated results by"`
	Sort           string               `json:"sort,omitempty"            jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListSSHKeysInput holds parameters for listing group SSH keys.
type ListSSHKeysInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"                 jsonschema:"Group ID or URL-encoded path,required"`
	CreatedAfter  string               `json:"created_after,omitempty"  jsonschema:"Return keys created on or after this date (YYYY-MM-DD)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return keys created on or before this date (YYYY-MM-DD)"`
	ExpiresAfter  string               `json:"expires_after,omitempty"  jsonschema:"Return keys expiring on or after this date (YYYY-MM-DD)"`
	ExpiresBefore string               `json:"expires_before,omitempty" jsonschema:"Return keys expiring on or before this date (YYYY-MM-DD)"`
	OrderBy       string               `json:"order_by,omitempty"       jsonschema:"Column to order keyset-paginated results by"`
	Sort          string               `json:"sort,omitempty"           jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// RevokePATInput holds parameters for revoking a group personal access token.
type RevokePATInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	TokenID int64                `json:"token_id" jsonschema:"Personal access token ID,required"`
}

// DeleteSSHKeyInput holds parameters for deleting a group SSH key.
type DeleteSSHKeyInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	KeyID   int64                `json:"key_id"   jsonschema:"SSH key ID,required"`
}

// PATOutput represents a group personal access token.
type PATOutput struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Revoked     bool     `json:"revoked"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	UserID      int64    `json:"user_id"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
	Active      bool     `json:"active"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
}

// SSHKeyOutput represents a group SSH key.
type SSHKeyOutput struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	CreatedAt  string `json:"created_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	UsageType  string `json:"usage_type,omitempty"`
	UserID     int64  `json:"user_id"`
}

// PATListOutput holds the list response for PATs.
type PATListOutput struct {
	toolutil.HintableOutput
	Tokens     []PATOutput               `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// SSHKeyListOutput holds the list response for SSH keys.
type SSHKeyListOutput struct {
	toolutil.HintableOutput
	Keys       []SSHKeyOutput            `json:"keys"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// parseISODate converts a YYYY-MM-DD string into a *gl.ISOTime for date-only
// list filters. It returns nil for empty or unparseable input so callers can
// assign the result unconditionally.
func parseISODate(s string) *gl.ISOTime {
	if s == "" {
		return nil
	}
	t, err := time.Parse(toolutil.DateFormatISO, s)
	if err != nil {
		return nil
	}
	d := gl.ISOTime(t)
	return &d
}

func toPATOutput(t *gl.GroupPersonalAccessToken) PATOutput {
	if t == nil {
		return PATOutput{}
	}
	o := PATOutput{
		ID:          t.ID,
		Name:        t.Name,
		Revoked:     t.Revoked,
		Description: t.Description,
		Scopes:      t.Scopes,
		UserID:      t.UserID,
		Active:      t.Active,
	}
	if t.CreatedAt != nil {
		o.CreatedAt = t.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	if t.LastUsedAt != nil {
		o.LastUsedAt = t.LastUsedAt.Format(toolutil.DateTimeFormat)
	}
	if t.ExpiresAt != nil {
		o.ExpiresAt = t.ExpiresAt.String()
	}
	return o
}

func toSSHKeyOutput(k *gl.GroupSSHKey) SSHKeyOutput {
	if k == nil {
		return SSHKeyOutput{}
	}
	o := SSHKeyOutput{
		ID:        k.ID,
		Title:     k.Title,
		UsageType: k.UsageType,
		UserID:    k.UserID,
	}
	if k.CreatedAt != nil {
		o.CreatedAt = k.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	if k.ExpiresAt != nil {
		o.ExpiresAt = k.ExpiresAt.Format(toolutil.DateTimeFormat)
	}
	if k.LastUsedAt != nil {
		o.LastUsedAt = k.LastUsedAt.Format(toolutil.DateTimeFormat)
	}
	return o
}

// ListPATs returns all personal access tokens for a group.
func ListPATs(ctx context.Context, client *gitlabclient.Client, in ListPATsInput) (PATListOutput, error) {
	if err := ctx.Err(); err != nil {
		return PATListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return PATListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupPersonalAccessTokensOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	if in.Search != "" {
		opts.Search = new(in.Search)
	}
	if in.State != "" {
		opts.State = new(in.State)
	}
	if in.Revoked != nil {
		opts.Revoked = in.Revoked
	}
	opts.CreatedAfter = parseISODate(in.CreatedAfter)
	opts.CreatedBefore = parseISODate(in.CreatedBefore)
	opts.LastUsedAfter = parseISODate(in.LastUsedAfter)
	opts.LastUsedBefore = parseISODate(in.LastUsedBefore)
	tokens, resp, err := client.GL().GroupCredentials.ListGroupPersonalAccessTokens(in.GroupID.String(), opts)
	if err != nil {
		return PATListOutput{}, toolutil.WrapErrWithStatusHint("list group PATs", err, http.StatusNotFound, groupCredentialInventoryHint)
	}
	out := PATListOutput{Tokens: make([]PATOutput, 0, len(tokens))}
	for _, t := range tokens {
		out.Tokens = append(out.Tokens, toPATOutput(t))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// ListSSHKeys returns all SSH keys for a group.
func ListSSHKeys(ctx context.Context, client *gitlabclient.Client, in ListSSHKeysInput) (SSHKeyListOutput, error) {
	if err := ctx.Err(); err != nil {
		return SSHKeyListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return SSHKeyListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupSSHKeysOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	opts.CreatedAfter = parseISODate(in.CreatedAfter)
	opts.CreatedBefore = parseISODate(in.CreatedBefore)
	opts.ExpiresAfter = parseISODate(in.ExpiresAfter)
	opts.ExpiresBefore = parseISODate(in.ExpiresBefore)
	keys, resp, err := client.GL().GroupCredentials.ListGroupSSHKeys(in.GroupID.String(), opts)
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithStatusHint("list group SSH keys", err, http.StatusNotFound, groupCredentialInventoryHint)
	}
	out := SSHKeyListOutput{Keys: make([]SSHKeyOutput, 0, len(keys))}
	for _, k := range keys {
		out.Keys = append(out.Keys, toSSHKeyOutput(k))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// RevokePAT revokes a personal access token for a group.
func RevokePAT(ctx context.Context, client *gitlabclient.Client, in RevokePATInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.TokenID == 0 {
		return toolutil.ErrFieldRequired("token_id")
	}
	_, err := client.GL().GroupCredentials.RevokeGroupPersonalAccessToken(in.GroupID.String(), in.TokenID)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("revoke group PAT", err, http.StatusNotFound, groupCredentialTokenHint)
	}
	return nil
}

// DeleteSSHKey deletes an SSH key from a group.
func DeleteSSHKey(ctx context.Context, client *gitlabclient.Client, in DeleteSSHKeyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.KeyID == 0 {
		return toolutil.ErrFieldRequired("key_id")
	}
	_, err := client.GL().GroupCredentials.DeleteGroupSSHKey(in.GroupID.String(), in.KeyID)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete group SSH key", err, http.StatusNotFound, groupCredentialSSHKeyHint)
	}
	return nil
}
