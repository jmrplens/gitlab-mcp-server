// user_service_accounts.go implements service account and current-user PAT operations.
package users

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

// ServiceAccountOutput represents a service account.
type ServiceAccountOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// ServiceAccountListOutput holds a list of service accounts.
type ServiceAccountListOutput struct {
	Accounts []ServiceAccountOutput `json:"accounts"`
}

// CreateServiceAccountInput holds parameters for creating a service account.
type CreateServiceAccountInput struct {
	Name     string `json:"name,omitempty" jsonschema:"Name for the service account"`
	Username string `json:"username,omitempty" jsonschema:"Username for the service account"`
	Email    string `json:"email,omitempty" jsonschema:"Email for the service account"`
}

// ListServiceAccountsInput holds parameters for listing service accounts.
type ListServiceAccountsInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Field to order by (id/username/name)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort direction (asc/desc)"`
	Page    int    `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"Items per page (max 100)"`
}

// CurrentUserPATOutput represents a personal access token.
type CurrentUserPATOutput struct {
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

// CreateCurrentUserPATInput holds parameters for creating a PAT for the current user.
type CreateCurrentUserPATInput struct {
	Name        string   `json:"name" jsonschema:"Name of the personal access token,required"`
	Scopes      []string `json:"scopes" jsonschema:"Array of scopes,required"`
	Description string   `json:"description,omitempty" jsonschema:"Description for the token"`
	ExpiresAt   string   `json:"expires_at,omitempty" jsonschema:"Token expiration date (YYYY-MM-DD)"`
}

// --- Handlers ---.

// CreateServiceAccount creates a new service account user.
func CreateServiceAccount(ctx context.Context, client *gitlabclient.Client, input CreateServiceAccountInput) (Output, error) {
	opts := &gl.CreateServiceAccountUserOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	user, _, err := client.GL().Users.CreateServiceAccountUser(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("create_service_account", err, http.StatusForbidden,
			"creating service accounts requires admin token; service accounts are GitLab Premium/Ultimate; username must be unique")
	}
	return toOutput(user), nil
}

// ListServiceAccounts lists all service accounts.
func ListServiceAccounts(ctx context.Context, client *gitlabclient.Client, input ListServiceAccountsInput) (ServiceAccountListOutput, error) {
	opts := &gl.ListServiceAccountsOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	accounts, _, err := client.GL().Users.ListServiceAccounts(opts, gl.WithContext(ctx))
	if err != nil {
		return ServiceAccountListOutput{}, toolutil.WrapErrWithStatusHint("list_service_accounts", err, http.StatusForbidden,
			"listing service accounts requires admin token; service accounts are GitLab Premium/Ultimate")
	}
	out := make([]ServiceAccountOutput, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, ServiceAccountOutput{
			ID:       a.ID,
			Username: a.Username,
			Name:     a.Name,
		})
	}
	return ServiceAccountListOutput{Accounts: out}, nil
}

// CreateCurrentUserPAT creates a personal access token for the currently authenticated user.
func CreateCurrentUserPAT(ctx context.Context, client *gitlabclient.Client, input CreateCurrentUserPATInput) (CurrentUserPATOutput, error) {
	if input.Name == "" {
		return CurrentUserPATOutput{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return CurrentUserPATOutput{}, errors.New("scopes is required and must not be empty")
	}
	opts := &gl.CreatePersonalAccessTokenForCurrentUserOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return CurrentUserPATOutput{}, fmt.Errorf("invalid expires_at format, expected YYYY-MM-DD: %w", err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}
	token, _, err := client.GL().Users.CreatePersonalAccessTokenForCurrentUser(opts, gl.WithContext(ctx))
	if err != nil {
		return CurrentUserPATOutput{}, toolutil.WrapErrWithStatusHint("create_personal_access_token_for_current_user", err, http.StatusBadRequest,
			"name is required; scopes must include valid PAT scopes (e.g. api, read_user, read_repository, write_repository); expires_at format YYYY-MM-DD")
	}
	return toCurrentUserPATOutput(token), nil
}

func toCurrentUserPATOutput(t *gl.PersonalAccessToken) CurrentUserPATOutput {
	o := CurrentUserPATOutput{
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

// --- Markdown formatters ---.

// FormatServiceAccountListMarkdownString formats a list of service accounts as Markdown.
func FormatServiceAccountListMarkdownString(out ServiceAccountListOutput) string {
	if len(out.Accounts) == 0 {
		return fmt.Sprintf("## Service Accounts\n\n%s No service accounts found.\n", toolutil.EmojiWarning)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Service Accounts (%d)\n\n", len(out.Accounts))
	sb.WriteString("| ID | Username | Name |\n")
	sb.WriteString("|---|---|---|\n")
	for _, a := range out.Accounts {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", a.ID, a.Username, a.Name)
	}
	return sb.String()
}

// FormatCurrentUserPATMarkdownString formats a PAT as Markdown.
func FormatCurrentUserPATMarkdownString(out CurrentUserPATOutput) string {
	var sb strings.Builder
	sb.WriteString("## Personal Access Token\n\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "- **Name**: %s\n", out.Name)
	fmt.Fprintf(&sb, "- **Active**: %v\n", out.Active)
	fmt.Fprintf(&sb, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	if out.Description != "" {
		fmt.Fprintf(&sb, "- **Description**: %s\n", out.Description)
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "- **Expires At**: %s\n", out.ExpiresAt)
	}
	if out.Token != "" {
		fmt.Fprintf(&sb, "- **Token**: `%s`\n", out.Token)
	}
	return sb.String()
}
