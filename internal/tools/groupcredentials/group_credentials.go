// Package groupcredentials implements GitLab group credential operations including
// listing and revoking personal access tokens and SSH keys for groups.
package groupcredentials

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListPATsInput holds parameters for listing group personal access tokens.
type ListPATsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"           jsonschema:"Group ID or URL-encoded path,required"`
	Search  string               `json:"search,omitempty"    jsonschema:"Filter tokens by name"`
	State   string               `json:"state,omitempty"     jsonschema:"Filter by state (active, inactive)"`
	Revoked *bool                `json:"revoked,omitempty"   jsonschema:"Filter by revoked status"`
	toolutil.PaginationInput
}

// ListSSHKeysInput holds parameters for listing group SSH keys.
type ListSSHKeysInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
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
	State       string   `json:"state,omitempty"`
}

// SSHKeyOutput represents a group SSH key.
type SSHKeyOutput struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Key        string `json:"key,omitempty"`
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
	switch {
	case t.Revoked:
		o.State = "revoked"
	case t.Active:
		o.State = "active"
	default:
		o.State = "inactive"
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
	if in.Page > 0 {
		opts.Page = int64(in.Page)
	}
	if in.PerPage > 0 {
		opts.PerPage = int64(in.PerPage)
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
	tokens, resp, err := client.GL().GroupCredentials.ListGroupPersonalAccessTokens(in.GroupID.String(), opts)
	if err != nil {
		return PATListOutput{}, toolutil.WrapErrWithMessage("list group PATs", err)
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
	if in.Page > 0 {
		opts.Page = int64(in.Page)
	}
	if in.PerPage > 0 {
		opts.PerPage = int64(in.PerPage)
	}
	keys, resp, err := client.GL().GroupCredentials.ListGroupSSHKeys(in.GroupID.String(), opts)
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithMessage("list group SSH keys", err)
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
		return toolutil.WrapErrWithMessage("revoke group PAT", err)
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
		return toolutil.WrapErrWithMessage("delete group SSH key", err)
	}
	return nil
}
