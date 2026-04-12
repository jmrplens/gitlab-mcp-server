// Package groupserviceaccounts implements MCP tool handlers for GitLab group service account operations.
package groupserviceaccounts

import (
	"context"
	"fmt"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const dateFormatISO = "2006-01-02"

// Output represents a group service account.
type Output struct {
	toolutil.HintableOutput
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// ListOutput holds a paginated list of group service accounts.
type ListOutput struct {
	toolutil.HintableOutput
	Accounts   []Output                  `json:"accounts"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// PATOutput represents a personal access token for a service account.
type PATOutput struct {
	toolutil.HintableOutput
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Revoked     bool     `json:"revoked"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	UserID      int64    `json:"user_id"`
	Active      bool     `json:"active"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	Token       string   `json:"token,omitempty"`
}

// ListPATOutput holds a paginated list of service account PATs.
type ListPATOutput struct {
	toolutil.HintableOutput
	Tokens     []PATOutput               `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// DeleteOutput confirms the deletion of a resource.
type DeleteOutput = toolutil.DeleteOutput

func toOutput(sa *gl.GroupServiceAccount) Output {
	return Output{
		ID:       sa.ID,
		Name:     sa.Name,
		Username: sa.UserName,
		Email:    sa.Email,
	}
}

func toPATOutput(pat *gl.PersonalAccessToken) PATOutput {
	out := PATOutput{
		ID:          pat.ID,
		Name:        pat.Name,
		Revoked:     pat.Revoked,
		Description: pat.Description,
		Scopes:      pat.Scopes,
		UserID:      pat.UserID,
		Active:      pat.Active,
		Token:       pat.Token,
	}
	if pat.CreatedAt != nil {
		out.CreatedAt = pat.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	if pat.ExpiresAt != nil {
		out.ExpiresAt = time.Time(*pat.ExpiresAt).Format(dateFormatISO)
	}
	return out
}

// ListInput holds parameters for listing group service accounts.
type ListInput struct {
	GroupID string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
	OrderBy string `json:"order_by,omitempty" jsonschema:"Order by id or username"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
}

// List retrieves service accounts for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListServiceAccountsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	accounts, resp, err := client.GL().Groups.ListServiceAccounts(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, fmt.Errorf("list group service accounts: %w", err)
	}
	out := make([]Output, len(accounts))
	for i, sa := range accounts {
		out[i] = toOutput(sa)
	}
	return ListOutput{
		Accounts:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// CreateInput holds parameters for creating a group service account.
type CreateInput struct {
	GroupID  string `json:"group_id" jsonschema:"Group ID or URL-encoded path (top-level only),required"`
	Name     string `json:"name,omitempty" jsonschema:"Service account name"`
	Username string `json:"username,omitempty" jsonschema:"Service account username"`
	Email    string `json:"email,omitempty" jsonschema:"Service account email"`
}

// Create creates a new group service account.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.CreateServiceAccountOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.Username != "" {
		opts.Username = &input.Username
	}
	if input.Email != "" {
		opts.Email = &input.Email
	}
	sa, _, err := client.GL().Groups.CreateServiceAccount(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, fmt.Errorf("create group service account: %w", err)
	}
	return toOutput(sa), nil
}

// UpdateInput holds parameters for updating a group service account.
type UpdateInput struct {
	GroupID          string `json:"group_id" jsonschema:"Group ID or URL-encoded path (top-level only),required"`
	ServiceAccountID int64  `json:"service_account_id" jsonschema:"Service account user ID,required"`
	Name             string `json:"name,omitempty" jsonschema:"New name"`
	Username         string `json:"username,omitempty" jsonschema:"New username"`
	Email            string `json:"email,omitempty" jsonschema:"New email"`
}

// Update updates a group service account.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.ServiceAccountID == 0 {
		return Output{}, toolutil.ErrFieldRequired("service_account_id")
	}
	opts := &gl.UpdateServiceAccountOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.Username != "" {
		opts.Username = &input.Username
	}
	if input.Email != "" {
		opts.Email = &input.Email
	}
	sa, _, err := client.GL().Groups.UpdateServiceAccount(input.GroupID, input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, fmt.Errorf("update group service account: %w", err)
	}
	return toOutput(sa), nil
}

// DeleteInput holds parameters for deleting a group service account.
type DeleteInput struct {
	GroupID          string `json:"group_id" jsonschema:"Group ID or URL-encoded path (top-level only),required"`
	ServiceAccountID int64  `json:"service_account_id" jsonschema:"Service account user ID,required"`
	HardDelete       bool   `json:"hard_delete,omitempty" jsonschema:"Hard delete the service account"`
}

// Delete deletes a group service account.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.ServiceAccountID == 0 {
		return toolutil.ErrFieldRequired("service_account_id")
	}
	opts := &gl.DeleteServiceAccountOptions{}
	if input.HardDelete {
		opts.HardDelete = &input.HardDelete
	}
	_, err := client.GL().Groups.DeleteServiceAccount(input.GroupID, input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("delete group service account: %w", err)
	}
	return nil
}

// ListPATInput holds parameters for listing service account PATs.
type ListPATInput struct {
	GroupID          string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ServiceAccountID int64  `json:"service_account_id" jsonschema:"Service account user ID,required"`
	toolutil.PaginationInput
}

// ListPATs retrieves personal access tokens for a group service account.
func ListPATs(ctx context.Context, client *gitlabclient.Client, input ListPATInput) (ListPATOutput, error) {
	if input.GroupID == "" {
		return ListPATOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.ServiceAccountID == 0 {
		return ListPATOutput{}, toolutil.ErrFieldRequired("service_account_id")
	}
	opts := &gl.ListServiceAccountPersonalAccessTokensOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	tokens, resp, err := client.GL().Groups.ListServiceAccountPersonalAccessTokens(input.GroupID, input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListPATOutput{}, fmt.Errorf("list service account PATs: %w", err)
	}
	out := make([]PATOutput, len(tokens))
	for i, t := range tokens {
		out[i] = toPATOutput(t)
	}
	return ListPATOutput{
		Tokens:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// CreatePATInput holds parameters for creating a service account PAT.
type CreatePATInput struct {
	GroupID          string   `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ServiceAccountID int64    `json:"service_account_id" jsonschema:"Service account user ID,required"`
	Name             string   `json:"name" jsonschema:"Token name,required"`
	Scopes           []string `json:"scopes" jsonschema:"Token scopes (e.g. api read_api read_user),required"`
	Description      string   `json:"description,omitempty" jsonschema:"Token description"`
	ExpiresAt        string   `json:"expires_at,omitempty" jsonschema:"Expiration date (YYYY-MM-DD)"`
}

// CreatePAT creates a new personal access token for a group service account.
func CreatePAT(ctx context.Context, client *gitlabclient.Client, input CreatePATInput) (PATOutput, error) {
	if input.GroupID == "" {
		return PATOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.ServiceAccountID == 0 {
		return PATOutput{}, toolutil.ErrFieldRequired("service_account_id")
	}
	if input.Name == "" {
		return PATOutput{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return PATOutput{}, toolutil.ErrFieldRequired("scopes")
	}
	opts := &gl.CreateServiceAccountPersonalAccessTokenOptions{
		Name:   &input.Name,
		Scopes: &input.Scopes,
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(dateFormatISO, input.ExpiresAt)
		if err != nil {
			return PATOutput{}, fmt.Errorf("invalid expires_at format (expected YYYY-MM-DD): %w", err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}
	pat, _, err := client.GL().Groups.CreateServiceAccountPersonalAccessToken(input.GroupID, input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return PATOutput{}, fmt.Errorf("create service account PAT: %w", err)
	}
	return toPATOutput(pat), nil
}

// RevokePATInput holds parameters for revoking a service account PAT.
type RevokePATInput struct {
	GroupID          string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ServiceAccountID int64  `json:"service_account_id" jsonschema:"Service account user ID,required"`
	TokenID          int64  `json:"token_id" jsonschema:"Personal access token ID to revoke,required"`
}

// RevokePAT revokes a personal access token for a group service account.
func RevokePAT(ctx context.Context, client *gitlabclient.Client, input RevokePATInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.ServiceAccountID == 0 {
		return toolutil.ErrFieldRequired("service_account_id")
	}
	if input.TokenID == 0 {
		return toolutil.ErrFieldRequired("token_id")
	}
	_, err := client.GL().Groups.RevokeServiceAccountPersonalAccessToken(input.GroupID, input.ServiceAccountID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("revoke service account PAT: %w", err)
	}
	return nil
}

// FormatOutputMarkdown renders a single service account as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Service Account: %s\n\n", toolutil.EscapeMdHeading(out.Username))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.Name)
	fmt.Fprintf(&b, toolutil.FmtMdUsername, out.Username)
	fmt.Fprintf(&b, toolutil.FmtMdEmail, out.Email)
	toolutil.WriteHints(&b,
		"Use gitlab_group_service_account_update to modify this account",
		"Use gitlab_group_service_account_pat_create to create a token",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of service accounts as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Accounts) == 0 {
		return "No group service accounts found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Accounts), out.Pagination)
	b.WriteString("| ID | Name | Username | Email |\n| --- | --- | --- | --- |\n")
	for _, a := range out.Accounts {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
			a.ID,
			toolutil.EscapeMdTableCell(a.Name),
			toolutil.EscapeMdTableCell(a.Username),
			toolutil.EscapeMdTableCell(a.Email),
		)
	}
	return b.String()
}

// FormatPATOutputMarkdown renders a single PAT as Markdown.
func FormatPATOutputMarkdown(out PATOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## PAT: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.Name)
	fmt.Fprintf(&b, "- **Active**: %t\n", out.Active)
	fmt.Fprintf(&b, "- **Revoked**: %t\n", out.Revoked)
	fmt.Fprintf(&b, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, out.CreatedAt)
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires**: %s\n", out.ExpiresAt)
	}
	if out.Token != "" {
		fmt.Fprintf(&b, "- **Token**: `%s`\n", out.Token)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_service_account_pat_revoke to revoke this token",
	)
	return b.String()
}

// FormatListPATMarkdown renders a paginated list of PATs as Markdown.
func FormatListPATMarkdown(out ListPATOutput) string {
	if len(out.Tokens) == 0 {
		return "No personal access tokens found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Tokens), out.Pagination)
	b.WriteString("| ID | Name | Active | Revoked | Scopes |\n| --- | --- | --- | --- | --- |\n")
	for _, t := range out.Tokens {
		fmt.Fprintf(&b, "| %d | %s | %t | %t | %s |\n",
			t.ID,
			toolutil.EscapeMdTableCell(t.Name),
			t.Active,
			t.Revoked,
			strings.Join(t.Scopes, ", "),
		)
	}
	return b.String()
}
