package projectserviceaccounts

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const projectServiceAccountTokenHint = "token_id must be the project service account personal access token ID returned by service_account_pat_list or service_account_pat_create; do not use service_account_id as token_id; requires Premium/Ultimate and sufficient project permissions"

// Output represents a project service account.
type Output struct {
	toolutil.HintableOutput
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	UnconfirmedEmail string `json:"unconfirmed_email,omitempty"`
}

// ListOutput holds a paginated list of project service accounts.
type ListOutput struct {
	toolutil.HintableOutput
	Accounts   []Output                  `json:"accounts"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// PATOutput represents a personal access token for a project service account.
type PATOutput struct {
	toolutil.HintableOutput
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Revoked     bool     `json:"revoked"`
	CreatedAt   string   `json:"created_at,omitempty"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	UserID      int64    `json:"user_id"`
	Active      bool     `json:"active"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	Token       string   `json:"token,omitempty"`
}

// ListPATOutput holds a paginated list of project service account PATs.
type ListPATOutput struct {
	toolutil.HintableOutput
	Tokens     []PATOutput               `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(account *gl.ProjectServiceAccount) Output {
	return Output{
		ID:               account.ID,
		Name:             account.Name,
		Username:         account.Username,
		Email:            account.Email,
		UnconfirmedEmail: account.UnconfirmedEmail,
	}
}

func toPATOutput(token *gl.PersonalAccessToken) PATOutput {
	out := PATOutput{
		ID:          token.ID,
		Name:        token.Name,
		Revoked:     token.Revoked,
		Description: token.Description,
		Scopes:      token.Scopes,
		UserID:      token.UserID,
		Active:      token.Active,
		Token:       token.Token,
	}
	if token.CreatedAt != nil {
		out.CreatedAt = token.CreatedAt.Format(time.RFC3339)
	}
	if token.LastUsedAt != nil {
		out.LastUsedAt = token.LastUsedAt.Format(time.RFC3339)
	}
	if token.ExpiresAt != nil {
		out.ExpiresAt = time.Time(*token.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	return out
}

// ListInput holds parameters for listing project service accounts.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
	OrderBy string `json:"order_by,omitempty" jsonschema:"Order by id or username"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
}

// List retrieves service accounts for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListProjectServiceAccountsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}

	accounts, resp, err := client.GL().Projects.ListProjectServiceAccounts(input.ProjectID.String(), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("list project service accounts", err)
	}
	out := make([]Output, len(accounts))
	for i, account := range accounts {
		out[i] = toOutput(account)
	}
	return ListOutput{Accounts: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// CreateInput holds parameters for creating a project service account.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name,omitempty" jsonschema:"Service account name"`
	Username  string               `json:"username,omitempty" jsonschema:"Service account username"`
	Email     string               `json:"email,omitempty" jsonschema:"Service account email"`
}

// Create creates a new project service account.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.CreateProjectServiceAccountOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.Username != "" {
		opts.Username = &input.Username
	}
	if input.Email != "" {
		opts.Email = &input.Email
	}
	account, _, err := client.GL().Projects.CreateProjectServiceAccount(input.ProjectID.String(), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create project service account", err)
	}
	return toOutput(account), nil
}

// UpdateInput holds parameters for updating a project service account.
type UpdateInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	Name             string               `json:"name,omitempty" jsonschema:"New name"`
	Username         string               `json:"username,omitempty" jsonschema:"New username"`
	Email            string               `json:"email,omitempty" jsonschema:"New email"`
}

// Update updates a project service account.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ServiceAccountID == 0 {
		return Output{}, toolutil.ErrFieldRequired("service_account_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.UpdateProjectServiceAccountOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.Username != "" {
		opts.Username = &input.Username
	}
	if input.Email != "" {
		opts.Email = &input.Email
	}
	account, _, err := client.GL().Projects.UpdateProjectServiceAccount(input.ProjectID.String(), input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("update project service account", err)
	}
	return toOutput(account), nil
}

// DeleteInput holds parameters for deleting a project service account.
type DeleteInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	HardDelete       *bool                `json:"hard_delete,omitempty" jsonschema:"Hard delete the service account"`
}

// Delete deletes a project service account.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.ServiceAccountID == 0 {
		return toolutil.ErrFieldRequired("service_account_id")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.DeleteProjectServiceAccountOptions{HardDelete: input.HardDelete}
	_, err := client.GL().Projects.DeleteProjectServiceAccount(input.ProjectID.String(), input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("delete project service account", err)
	}
	return nil
}

// ListPATInput holds parameters for listing project service account PATs.
type ListPATInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	toolutil.PaginationInput
	CreatedAfter   string `json:"created_after,omitempty" jsonschema:"Filter tokens created after RFC3339 datetime or YYYY-MM-DD date"`
	CreatedBefore  string `json:"created_before,omitempty" jsonschema:"Filter tokens created before RFC3339 datetime or YYYY-MM-DD date"`
	ExpiresAfter   string `json:"expires_after,omitempty" jsonschema:"Filter tokens expiring after YYYY-MM-DD date"`
	ExpiresBefore  string `json:"expires_before,omitempty" jsonschema:"Filter tokens expiring before YYYY-MM-DD date"`
	LastUsedAfter  string `json:"last_used_after,omitempty" jsonschema:"Filter tokens last used after RFC3339 datetime or YYYY-MM-DD date"`
	LastUsedBefore string `json:"last_used_before,omitempty" jsonschema:"Filter tokens last used before RFC3339 datetime or YYYY-MM-DD date"`
	Revoked        *bool  `json:"revoked,omitempty" jsonschema:"Filter by revoked state"`
	UserID         int64  `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	Search         string `json:"search,omitempty" jsonschema:"Search token names"`
	Sort           string `json:"sort,omitempty" jsonschema:"Sort expression"`
	State          string `json:"state,omitempty" jsonschema:"Token state filter: active or inactive"`
}

// ListPATs retrieves personal access tokens for a project service account.
func ListPATs(ctx context.Context, client *gitlabclient.Client, input ListPATInput) (ListPATOutput, error) {
	if input.ProjectID == "" {
		return ListPATOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ServiceAccountID == 0 {
		return ListPATOutput{}, toolutil.ErrFieldRequired("service_account_id")
	}
	if err := ctx.Err(); err != nil {
		return ListPATOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts, err := listPATOptions(input)
	if err != nil {
		return ListPATOutput{}, err
	}
	tokens, resp, err := client.GL().Projects.ListProjectServiceAccountPersonalAccessTokens(input.ProjectID.String(), input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListPATOutput{}, toolutil.WrapErr("list project service account PATs", err)
	}
	out := make([]PATOutput, len(tokens))
	for i, token := range tokens {
		out[i] = toPATOutput(token)
	}
	return ListPATOutput{Tokens: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// CreatePATInput holds parameters for creating a project service account PAT.
type CreatePATInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	Name             string               `json:"name" jsonschema:"Token name,required"`
	Scopes           []string             `json:"scopes" jsonschema:"Token scopes (e.g. api, read_api, read_user),required"`
	Description      string               `json:"description,omitempty" jsonschema:"Token description"`
	ExpiresAt        string               `json:"expires_at,omitempty" jsonschema:"Expiration date in YYYY-MM-DD format"`
}

// CreatePAT creates a new personal access token for a project service account.
func CreatePAT(ctx context.Context, client *gitlabclient.Client, input CreatePATInput) (PATOutput, error) {
	if input.ProjectID == "" {
		return PATOutput{}, toolutil.ErrFieldRequired("project_id")
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
	if err := ctx.Err(); err != nil {
		return PATOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	expiresAt, hasExpiresAt, err := parseISODate(input.ExpiresAt, "expires_at")
	if err != nil {
		return PATOutput{}, err
	}
	opts := &gl.CreateProjectServiceAccountPersonalAccessTokenOptions{
		Name:   &input.Name,
		Scopes: &input.Scopes,
	}
	if hasExpiresAt {
		opts.ExpiresAt = expiresAt
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	token, _, err := client.GL().Projects.CreateProjectServiceAccountPersonalAccessToken(input.ProjectID.String(), input.ServiceAccountID, opts, gl.WithContext(ctx))
	if err != nil {
		return PATOutput{}, toolutil.WrapErrWithMessage("create project service account PAT", err)
	}
	return toPATOutput(token), nil
}

// RevokePATInput holds parameters for revoking a project service account PAT.
type RevokePATInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	TokenID          int64                `json:"token_id" jsonschema:"Personal access token ID to revoke,required"`
}

// RevokePAT revokes a personal access token for a project service account.
func RevokePAT(ctx context.Context, client *gitlabclient.Client, input RevokePATInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.ServiceAccountID == 0 {
		return toolutil.ErrFieldRequired("service_account_id")
	}
	if input.TokenID == 0 {
		return toolutil.ErrFieldRequired("token_id")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().Projects.RevokeProjectServiceAccountPersonalAccessToken(input.ProjectID.String(), input.ServiceAccountID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return toolutil.WrapErrWithHint("revoke project service account PAT", err, projectServiceAccountTokenHint)
		}
		return toolutil.WrapErrWithMessage("revoke project service account PAT", err)
	}
	return nil
}

// RotatePATInput holds parameters for rotating a project service account PAT.
type RotatePATInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ServiceAccountID int64                `json:"service_account_id" jsonschema:"Service account user ID,required"`
	TokenID          int64                `json:"token_id" jsonschema:"Personal access token ID to rotate,required"`
	ExpiresAt        string               `json:"expires_at,omitempty" jsonschema:"New expiration date in YYYY-MM-DD format"`
}

// RotatePAT rotates a personal access token for a project service account.
func RotatePAT(ctx context.Context, client *gitlabclient.Client, input RotatePATInput) (PATOutput, error) {
	if input.ProjectID == "" {
		return PATOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ServiceAccountID == 0 {
		return PATOutput{}, toolutil.ErrFieldRequired("service_account_id")
	}
	if input.TokenID == 0 {
		return PATOutput{}, toolutil.ErrFieldRequired("token_id")
	}
	if err := ctx.Err(); err != nil {
		return PATOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	expiresAt, hasExpiresAt, err := parseISODate(input.ExpiresAt, "expires_at")
	if err != nil {
		return PATOutput{}, err
	}
	opts := &gl.RotateProjectServiceAccountPersonalAccessTokenOptions{}
	if hasExpiresAt {
		opts.ExpiresAt = expiresAt
	}
	token, _, err := client.GL().Projects.RotateProjectServiceAccountPersonalAccessToken(input.ProjectID.String(), input.ServiceAccountID, input.TokenID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return PATOutput{}, toolutil.WrapErrWithHint("rotate project service account PAT", err, projectServiceAccountTokenHint)
		}
		return PATOutput{}, toolutil.WrapErrWithMessage("rotate project service account PAT", err)
	}
	return toPATOutput(token), nil
}

func listPATOptions(input ListPATInput) (*gl.ListProjectServiceAccountPersonalAccessTokensOptions, error) {
	opts := &gl.ListProjectServiceAccountPersonalAccessTokensOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
		Revoked:     input.Revoked,
	}
	createdAfter, hasCreatedAfter, err := parseTimeFilter(input.CreatedAfter, "created_after")
	if err != nil {
		return nil, err
	}
	if hasCreatedAfter {
		opts.CreatedAfter = createdAfter
	}
	createdBefore, hasCreatedBefore, err := parseTimeFilter(input.CreatedBefore, "created_before")
	if err != nil {
		return nil, err
	}
	if hasCreatedBefore {
		opts.CreatedBefore = createdBefore
	}
	lastUsedAfter, hasLastUsedAfter, err := parseTimeFilter(input.LastUsedAfter, "last_used_after")
	if err != nil {
		return nil, err
	}
	if hasLastUsedAfter {
		opts.LastUsedAfter = lastUsedAfter
	}
	lastUsedBefore, hasLastUsedBefore, err := parseTimeFilter(input.LastUsedBefore, "last_used_before")
	if err != nil {
		return nil, err
	}
	if hasLastUsedBefore {
		opts.LastUsedBefore = lastUsedBefore
	}
	expiresAfter, hasExpiresAfter, err := parseISODate(input.ExpiresAfter, "expires_after")
	if err != nil {
		return nil, err
	}
	if hasExpiresAfter {
		opts.ExpiresAfter = expiresAfter
	}
	expiresBefore, hasExpiresBefore, err := parseISODate(input.ExpiresBefore, "expires_before")
	if err != nil {
		return nil, err
	}
	if hasExpiresBefore {
		opts.ExpiresBefore = expiresBefore
	}
	if input.UserID != 0 {
		opts.UserID = &input.UserID
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.State != "" {
		opts.State = &input.State
	}
	return opts, nil
}

func parseISODate(value, field string) (*gl.ISOTime, bool, error) {
	if value == "" {
		return nil, false, nil
	}
	date, err := time.Parse(toolutil.DateFormatISO, value)
	if err != nil {
		return nil, false, fmt.Errorf("invalid %s format (expected YYYY-MM-DD): %w", field, err)
	}
	isoDate := gl.ISOTime(date)
	return &isoDate, true, nil
}

func parseTimeFilter(value, field string) (*time.Time, bool, error) {
	if value == "" {
		return nil, false, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, true, nil
	}
	parsed, err := time.Parse(toolutil.DateFormatISO, value)
	if err != nil {
		return nil, false, fmt.Errorf("invalid %s format (expected RFC3339 or YYYY-MM-DD): %w", field, err)
	}
	return &parsed, true, nil
}
