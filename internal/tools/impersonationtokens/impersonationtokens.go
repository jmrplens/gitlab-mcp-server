// Package impersonationtokens implements GitLab impersonation token
// and personal access token management operations.
package impersonationtokens

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

const errUserIDPositive = "user_id must be a positive integer"

// Output represents an impersonation token.
type Output struct {
	toolutil.HintableOutput
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Active     bool     `json:"active"`
	Token      string   `json:"token,omitempty"`
	Scopes     []string `json:"scopes"`
	Revoked    bool     `json:"revoked"`
	CreatedAt  string   `json:"created_at,omitempty"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	LastUsedAt string   `json:"last_used_at,omitempty"`
}

// ListOutput holds a list of impersonation tokens.
type ListOutput struct {
	toolutil.HintableOutput
	Tokens []Output `json:"tokens"`
}

// PATOutput represents a personal access token.
type PATOutput struct {
	toolutil.HintableOutput
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Active      bool     `json:"active"`
	Token       string   `json:"token,omitempty"`
	Scopes      []string `json:"scopes"`
	Revoked     bool     `json:"revoked"`
	Description string   `json:"description,omitempty"`
	UserID      int64    `json:"user_id"`
	CreatedAt   string   `json:"created_at,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
}

// RevokeOutput confirms a token revocation.
type RevokeOutput struct {
	toolutil.HintableOutput
	UserID  int64 `json:"user_id"`
	TokenID int64 `json:"token_id"`
	Revoked bool  `json:"revoked"`
}

// --- Input types ---.

// ListInput holds parameters for listing impersonation tokens.
type ListInput struct {
	UserID  int64  `json:"user_id" jsonschema:"GitLab user ID,required"`
	State   string `json:"state,omitempty" jsonschema:"Filter by state: all/active/inactive"`
	Page    int    `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"Items per page (max 100)"`
}

// GetInput identifies a specific impersonation token.
type GetInput struct {
	UserID  int64 `json:"user_id" jsonschema:"GitLab user ID,required"`
	TokenID int64 `json:"token_id" jsonschema:"Impersonation token ID,required"`
}

// CreateInput holds parameters for creating an impersonation token.
type CreateInput struct {
	UserID    int64    `json:"user_id" jsonschema:"GitLab user ID,required"`
	Name      string   `json:"name" jsonschema:"Name of the impersonation token,required"`
	Scopes    []string `json:"scopes" jsonschema:"Array of scopes (api/read_user/read_api/read_repository/write_repository/read_registry/write_registry/sudo/admin_mode/create_runner/manage_runner/ai_features/k8s_proxy),required"`
	ExpiresAt string   `json:"expires_at,omitempty" jsonschema:"Token expiration date (YYYY-MM-DD)"`
}

// RevokeInput identifies a token to revoke.
type RevokeInput struct {
	UserID  int64 `json:"user_id" jsonschema:"GitLab user ID,required"`
	TokenID int64 `json:"token_id" jsonschema:"Impersonation token ID to revoke,required"`
}

// CreatePATInput holds parameters for creating a personal access token for a user.
type CreatePATInput struct {
	UserID      int64    `json:"user_id" jsonschema:"GitLab user ID,required"`
	Name        string   `json:"name" jsonschema:"Name of the personal access token,required"`
	Scopes      []string `json:"scopes" jsonschema:"Array of scopes,required"`
	Description string   `json:"description,omitempty" jsonschema:"Description for the token"`
	ExpiresAt   string   `json:"expires_at,omitempty" jsonschema:"Token expiration date (YYYY-MM-DD)"`
}

// --- Conversion helpers ---.

func toOutput(t *gl.ImpersonationToken) Output {
	o := Output{
		ID:      t.ID,
		Name:    t.Name,
		Active:  t.Active,
		Token:   t.Token,
		Scopes:  t.Scopes,
		Revoked: t.Revoked,
	}
	if t.CreatedAt != nil {
		o.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		o.ExpiresAt = time.Time(*t.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	if t.LastUsedAt != nil {
		o.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	return o
}

func toPATOutput(t *gl.PersonalAccessToken) PATOutput {
	o := PATOutput{
		ID:          t.ID,
		Name:        t.Name,
		Active:      t.Active,
		Token:       t.Token,
		Scopes:      t.Scopes,
		Revoked:     t.Revoked,
		Description: t.Description,
		UserID:      t.UserID,
	}
	if t.CreatedAt != nil {
		o.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		o.ExpiresAt = time.Time(*t.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	if t.LastUsedAt != nil {
		o.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	return o
}

// --- Handlers ---.

// List retrieves all impersonation tokens for a user.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.UserID <= 0 {
		return ListOutput{}, errors.New(errUserIDPositive)
	}
	opts := &gl.GetAllImpersonationTokensOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	tokens, _, err := client.GL().Users.GetAllImpersonationTokens(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_impersonation_tokens", err, http.StatusForbidden,
			"impersonation tokens require admin token; verify user_id with gitlab_get_user; state must be one of {all, active, inactive}")
	}
	out := make([]Output, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toOutput(t))
	}
	return ListOutput{Tokens: out}, nil
}

// Get retrieves a specific impersonation token.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.UserID <= 0 {
		return Output{}, errors.New(errUserIDPositive)
	}
	if input.TokenID <= 0 {
		return Output{}, errors.New("token_id must be a positive integer")
	}
	token, _, err := client.GL().Users.GetImpersonationToken(input.UserID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get_impersonation_token", err, http.StatusNotFound,
			"verify token_id with gitlab_list_impersonation_tokens; admin token required; the token may have been revoked")
	}
	return toOutput(token), nil
}

// Create creates an impersonation token for a user.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.UserID <= 0 {
		return Output{}, errors.New(errUserIDPositive)
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return Output{}, errors.New("scopes is required and must not be empty")
	}
	opts := &gl.CreateImpersonationTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf("invalid expires_at format, expected YYYY-MM-DD: %w", err)
		}
		opts.ExpiresAt = &t
	}
	token, _, err := client.GL().Users.CreateImpersonationToken(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("create_impersonation_token", err, http.StatusForbidden,
			"creating impersonation tokens requires admin token; scopes must be from {api, read_user, read_api, read_repository, write_repository, read_registry, write_registry, sudo, admin_mode, create_runner, manage_runner, ai_features, k8s_proxy}; expires_at format YYYY-MM-DD")
	}
	return toOutput(token), nil
}

// Revoke revokes an impersonation token.
func Revoke(ctx context.Context, client *gitlabclient.Client, input RevokeInput) (RevokeOutput, error) {
	if input.UserID <= 0 {
		return RevokeOutput{}, errors.New(errUserIDPositive)
	}
	if input.TokenID <= 0 {
		return RevokeOutput{}, errors.New("token_id must be a positive integer")
	}
	_, err := client.GL().Users.RevokeImpersonationToken(input.UserID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return RevokeOutput{}, toolutil.WrapErrWithStatusHint("revoke_impersonation_token", err, http.StatusNotFound,
			"verify token_id with gitlab_list_impersonation_tokens; admin token required; the token may already be revoked")
	}
	return RevokeOutput{UserID: input.UserID, TokenID: input.TokenID, Revoked: true}, nil
}

// CreatePAT creates a personal access token for a user (admin only).
func CreatePAT(ctx context.Context, client *gitlabclient.Client, input CreatePATInput) (PATOutput, error) {
	if input.UserID <= 0 {
		return PATOutput{}, errors.New(errUserIDPositive)
	}
	if input.Name == "" {
		return PATOutput{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return PATOutput{}, errors.New("scopes is required and must not be empty")
	}
	opts := &gl.CreatePersonalAccessTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return PATOutput{}, fmt.Errorf("invalid expires_at format, expected YYYY-MM-DD: %w", err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}
	token, _, err := client.GL().Users.CreatePersonalAccessToken(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return PATOutput{}, toolutil.WrapErrWithStatusHint("create_personal_access_token", err, http.StatusForbidden,
			"creating PAT for another user requires admin token; scopes must include valid PAT scopes; expires_at format YYYY-MM-DD")
	}
	return toPATOutput(token), nil
}

// --- Markdown formatters ---.

// FormatListMarkdownString formats a list of impersonation tokens as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return fmt.Sprintf("## Impersonation Tokens\n\n%s No tokens found.\n", toolutil.EmojiWarning)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Impersonation Tokens (%d)\n\n", len(out.Tokens))
	sb.WriteString("| ID | Name | Active | Scopes | Expires At |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, t := range out.Tokens {
		expires := "-"
		if t.ExpiresAt != "" {
			expires = t.ExpiresAt
		}
		fmt.Fprintf(&sb, "| %d | %s | %v | %s | %s |\n",
			t.ID, t.Name, t.Active, strings.Join(t.Scopes, ", "), expires)
	}
	return sb.String()
}

// FormatMarkdownString formats a single impersonation token as Markdown.
func FormatMarkdownString(out Output) string {
	var sb strings.Builder
	sb.WriteString("## Impersonation Token\n\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "- **Name**: %s\n", out.Name)
	fmt.Fprintf(&sb, "- **Active**: %v\n", out.Active)
	fmt.Fprintf(&sb, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "- **Expires At**: %s\n", out.ExpiresAt)
	}
	if out.Token != "" {
		fmt.Fprintf(&sb, "- **Token**: `%s`\n", out.Token)
	}
	return sb.String()
}

// FormatPATMarkdownString formats a personal access token as Markdown.
func FormatPATMarkdownString(out PATOutput) string {
	var sb strings.Builder
	sb.WriteString("## Personal Access Token\n\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "- **Name**: %s\n", out.Name)
	fmt.Fprintf(&sb, "- **Active**: %v\n", out.Active)
	fmt.Fprintf(&sb, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	if out.Description != "" {
		fmt.Fprintf(&sb, "- **Description**: %s\n", out.Description)
	}
	fmt.Fprintf(&sb, "- **User ID**: %d\n", out.UserID)
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "- **Expires At**: %s\n", out.ExpiresAt)
	}
	if out.Token != "" {
		fmt.Fprintf(&sb, "- **Token**: `%s`\n", out.Token)
	}
	return sb.String()
}
