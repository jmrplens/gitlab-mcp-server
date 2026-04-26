// Package accesstokens implements GitLab Access Token operations as MCP tools.
// It supports project access tokens, group access tokens, and personal access tokens,
// including listing, getting, creating, rotating, and revoking tokens.
package accesstokens

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	errTokenIDInvalid      = "token_id is required and must be > 0" //#nosec G101 -- false positive: error message, not a credential
	errInvalidExpiresAtFmt = "invalid expires_at format (expected YYYY-MM-DD): %w"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a GitLab access token in responses.
type Output struct {
	toolutil.HintableOutput
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Revoked     bool     `json:"revoked"`
	Active      bool     `json:"active"`
	Scopes      []string `json:"scopes,omitempty"`
	UserID      int64    `json:"user_id,omitempty"`
	AccessLevel int      `json:"access_level,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	Token       string   `json:"token,omitempty"`
}

// ListOutput holds a paginated list of access tokens.
type ListOutput struct {
	toolutil.HintableOutput
	Tokens     []Output                  `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// fromProjectToken is an internal helper for the accesstokens package.
func fromProjectToken(t *gl.ProjectAccessToken) Output {
	out := Output{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Revoked:     t.Revoked,
		Active:      t.Active,
		Scopes:      t.Scopes,
		UserID:      t.UserID,
		AccessLevel: int(t.AccessLevel),
		Token:       t.Token,
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.LastUsedAt != nil {
		out.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		out.ExpiresAt = time.Time(*t.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	return out
}

// fromGroupToken is an internal helper for the accesstokens package.
func fromGroupToken(t *gl.GroupAccessToken) Output {
	out := Output{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Revoked:     t.Revoked,
		Active:      t.Active,
		Scopes:      t.Scopes,
		UserID:      t.UserID,
		AccessLevel: int(t.AccessLevel),
		Token:       t.Token,
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.LastUsedAt != nil {
		out.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		out.ExpiresAt = time.Time(*t.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	return out
}

// fromPersonalToken is an internal helper for the accesstokens package.
func fromPersonalToken(t *gl.PersonalAccessToken) Output {
	out := Output{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Revoked:     t.Revoked,
		Active:      t.Active,
		Scopes:      t.Scopes,
		UserID:      t.UserID,
		Token:       t.Token,
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.LastUsedAt != nil {
		out.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		out.ExpiresAt = time.Time(*t.ExpiresAt).Format(toolutil.DateFormatISO)
	}
	return out
}

// ---------------------------------------------------------------------------
// Project Access Tokens
// ---------------------------------------------------------------------------.

// ProjectListInput defines parameters for listing project access tokens.
type ProjectListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	State     string               `json:"state,omitempty" jsonschema:"Token state filter: active, inactive"`
	toolutil.PaginationInput
}

// ProjectList returns access tokens for a project.
func ProjectList(ctx context.Context, client *gitlabclient.Client, input ProjectListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListProjectAccessTokensOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	tokens, resp, err := client.GL().ProjectAccessTokens.ListProjectAccessTokens(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list project access tokens", err, http.StatusForbidden,
			"listing project access tokens requires Maintainer or Owner role on the project")
	}

	items := make([]Output, len(tokens))
	for i, t := range tokens {
		items[i] = fromProjectToken(t)
	}
	return ListOutput{Tokens: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ProjectGetInput defines parameters for getting a project access token.
type ProjectGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TokenID   int64                `json:"token_id"   jsonschema:"Access token ID,required"`
}

// ProjectGet returns a specific project access token.
func ProjectGet(ctx context.Context, client *gitlabclient.Client, input ProjectGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.TokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().ProjectAccessTokens.GetProjectAccessToken(string(input.ProjectID), input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get project access token", err, http.StatusNotFound,
			"token_id not found on this project (already revoked or never existed) \u2014 use gitlab_access_token_project_list to discover current token IDs")
	}
	return fromProjectToken(t), nil
}

// ProjectCreateInput defines parameters for creating a project access token.
type ProjectCreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	Name        string               `json:"name"                    jsonschema:"Token name,required"`
	Description string               `json:"description,omitempty"   jsonschema:"Token description"`
	Scopes      []string             `json:"scopes"                  jsonschema:"Token scopes: api, read_api, read_repository, write_repository, etc.,required"`
	AccessLevel int                  `json:"access_level,omitempty"  jsonschema:"Access level: 10 (guest), 20 (reporter), 30 (developer), 40 (maintainer)"`
	ExpiresAt   string               `json:"expires_at,omitempty"    jsonschema:"Expiry date in YYYY-MM-DD format"`
}

// ProjectCreate creates a new project access token.
func ProjectCreate(ctx context.Context, client *gitlabclient.Client, input ProjectCreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return Output{}, toolutil.ErrFieldRequired("scopes")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.CreateProjectAccessTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.AccessLevel > 0 {
		opts.AccessLevel = new(gl.AccessLevelValue(input.AccessLevel))
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().ProjectAccessTokens.CreateProjectAccessToken(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("create project access token", err,
				"validate scopes (api|read_api|read_repository|write_repository|read_registry|write_registry), access_level (10|20|30|40|50), and expires_at format (YYYY-MM-DD, must be within instance-configured maximum lifetime)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("create project access token", err, http.StatusForbidden,
			"creating project access tokens requires Maintainer or Owner role; the requested access_level cannot exceed the caller's role")
	}
	return fromProjectToken(token), nil
}

// ProjectRotateInput defines parameters for rotating a project access token.
type ProjectRotateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	TokenID   int64                `json:"token_id"             jsonschema:"Access token ID,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// ProjectRotate rotates a project access token and returns the new token.
func ProjectRotate(ctx context.Context, client *gitlabclient.Client, input ProjectRotateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.TokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotateProjectAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().ProjectAccessTokens.RotateProjectAccessToken(string(input.ProjectID), input.TokenID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("rotate project access token", err,
				"token may already be revoked/expired; expires_at must be YYYY-MM-DD and within instance maximum lifetime")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("rotate project access token", err, http.StatusNotFound,
			"token_id not found \u2014 use gitlab_access_token_project_list to verify")
	}
	return fromProjectToken(token), nil
}

// ProjectRevokeInput defines parameters for revoking a project access token.
type ProjectRevokeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TokenID   int64                `json:"token_id"   jsonschema:"Access token ID to revoke,required"`
}

// ProjectRevoke revokes a project access token.
func ProjectRevoke(ctx context.Context, client *gitlabclient.Client, input ProjectRevokeInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.TokenID == 0 {
		return errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().ProjectAccessTokens.RevokeProjectAccessToken(string(input.ProjectID), input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("revoke project access token", err, http.StatusNotFound,
			"token already revoked or never existed \u2014 nothing to do")
	}
	return nil
}

// ProjectRotateSelfInput defines parameters for self-rotating a project access token.
type ProjectRotateSelfInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// ProjectRotateSelf rotates the project access token used for the current request.
func ProjectRotateSelf(ctx context.Context, client *gitlabclient.Client, input ProjectRotateSelfInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotateProjectAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().ProjectAccessTokens.RotateProjectAccessTokenSelf(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("self-rotate project access token", err, http.StatusUnauthorized,
			"the calling token is not a project access token for this project, or has already been rotated/revoked")
	}
	return fromProjectToken(token), nil
}

// ---------------------------------------------------------------------------
// Group Access Tokens
// ---------------------------------------------------------------------------.

// GroupListInput defines parameters for listing group access tokens.
type GroupListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	State   string               `json:"state,omitempty" jsonschema:"Token state filter: active, inactive"`
	toolutil.PaginationInput
}

// GroupList returns access tokens for a group.
func GroupList(ctx context.Context, client *gitlabclient.Client, input GroupListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListGroupAccessTokensOptions{}
	if input.State != "" {
		st := gl.AccessTokenState(input.State)
		opts.State = &st
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	tokens, resp, err := client.GL().GroupAccessTokens.ListGroupAccessTokens(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list group access tokens", err, http.StatusForbidden,
			"listing group access tokens requires Owner role on the group")
	}

	items := make([]Output, len(tokens))
	for i, t := range tokens {
		items[i] = fromGroupToken(t)
	}
	return ListOutput{Tokens: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GroupGetInput defines parameters for getting a group access token.
type GroupGetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"  jsonschema:"Group ID or URL-encoded path,required"`
	TokenID int64                `json:"token_id"  jsonschema:"Access token ID,required"`
}

// GroupGet returns a specific group access token.
func GroupGet(ctx context.Context, client *gitlabclient.Client, input GroupGetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.TokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().GroupAccessTokens.GetGroupAccessToken(string(input.GroupID), input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get group access token", err, http.StatusNotFound,
			"token_id not found on this group \u2014 use gitlab_access_token_group_list to discover current token IDs")
	}
	return fromGroupToken(t), nil
}

// GroupCreateInput defines parameters for creating a group access token.
type GroupCreateInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	Name        string               `json:"name"                    jsonschema:"Token name,required"`
	Description string               `json:"description,omitempty"   jsonschema:"Token description"`
	Scopes      []string             `json:"scopes"                  jsonschema:"Token scopes: api, read_api, read_repository, write_repository, etc.,required"`
	AccessLevel int                  `json:"access_level,omitempty"  jsonschema:"Access level: 10 (guest), 20 (reporter), 30 (developer), 40 (maintainer), 50 (owner)"`
	ExpiresAt   string               `json:"expires_at,omitempty"    jsonschema:"Expiry date in YYYY-MM-DD format"`
}

// GroupCreate creates a new group access token.
func GroupCreate(ctx context.Context, client *gitlabclient.Client, input GroupCreateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return Output{}, toolutil.ErrFieldRequired("scopes")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.CreateGroupAccessTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.AccessLevel > 0 {
		opts.AccessLevel = new(gl.AccessLevelValue(input.AccessLevel))
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().GroupAccessTokens.CreateGroupAccessToken(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("create group access token", err,
				"validate scopes (api|read_api|read_repository|write_repository|read_registry|write_registry), access_level (10|20|30|40|50), and expires_at format (YYYY-MM-DD)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("create group access token", err, http.StatusForbidden,
			"creating group access tokens requires Owner role; the requested access_level cannot exceed the caller's role")
	}
	return fromGroupToken(token), nil
}

// GroupRotateInput defines parameters for rotating a group access token.
type GroupRotateInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id"             jsonschema:"Group ID or URL-encoded path,required"`
	TokenID   int64                `json:"token_id"             jsonschema:"Access token ID,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// GroupRotate rotates a group access token and returns the new token.
func GroupRotate(ctx context.Context, client *gitlabclient.Client, input GroupRotateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.TokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotateGroupAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().GroupAccessTokens.RotateGroupAccessToken(string(input.GroupID), input.TokenID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("rotate group access token", err,
				"token may already be revoked/expired; expires_at must be YYYY-MM-DD")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("rotate group access token", err, http.StatusNotFound,
			"token_id not found \u2014 use gitlab_access_token_group_list to verify")
	}
	return fromGroupToken(token), nil
}

// GroupRevokeInput defines parameters for revoking a group access token.
type GroupRevokeInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	TokenID int64                `json:"token_id" jsonschema:"Access token ID to revoke,required"`
}

// GroupRevoke revokes a group access token.
func GroupRevoke(ctx context.Context, client *gitlabclient.Client, input GroupRevokeInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.TokenID == 0 {
		return errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().GroupAccessTokens.RevokeGroupAccessToken(string(input.GroupID), input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("revoke group access token", err, http.StatusNotFound,
			"token already revoked or never existed \u2014 nothing to do")
	}
	return nil
}

// GroupRotateSelfInput defines parameters for self-rotating a group access token.
type GroupRotateSelfInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id"             jsonschema:"Group ID or URL-encoded path,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// GroupRotateSelf rotates the group access token used for the current request.
func GroupRotateSelf(ctx context.Context, client *gitlabclient.Client, input GroupRotateSelfInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotateGroupAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().GroupAccessTokens.RotateGroupAccessTokenSelf(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("self-rotate group access token", err, http.StatusUnauthorized,
			"the calling token is not a group access token for this group, or has already been rotated/revoked")
	}
	return fromGroupToken(token), nil
}

// ---------------------------------------------------------------------------
// Personal Access Tokens
// ---------------------------------------------------------------------------.

// PersonalListInput defines parameters for listing personal access tokens.
type PersonalListInput struct {
	State  string `json:"state,omitempty"  jsonschema:"Token state filter: active, inactive"`
	Search string `json:"search,omitempty" jsonschema:"Search by token name"`
	UserID int64  `json:"user_id,omitempty" jsonschema:"Filter by user ID (admin only)"`
	toolutil.PaginationInput
}

// PersonalList returns personal access tokens.
func PersonalList(ctx context.Context, client *gitlabclient.Client, input PersonalListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListPersonalAccessTokensOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.UserID > 0 {
		opts.UserID = new(input.UserID)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	tokens, resp, err := client.GL().PersonalAccessTokens.ListPersonalAccessTokens(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list personal access tokens", err, http.StatusForbidden,
			"listing all personal access tokens (across all users) requires an admin token; without admin, the result is filtered to the authenticated user's own tokens")
	}

	items := make([]Output, len(tokens))
	for i, t := range tokens {
		items[i] = fromPersonalToken(t)
	}
	return ListOutput{Tokens: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// PersonalGetInput defines parameters for getting a personal access token.
type PersonalGetInput struct {
	TokenID int64 `json:"token_id" jsonschema:"Access token ID (required, use 0 for current token)"`
}

// PersonalGet returns a specific personal access token by ID, or the current token if ID is 0.
func PersonalGet(ctx context.Context, client *gitlabclient.Client, input PersonalGetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	if input.TokenID == 0 {
		t, _, err := client.GL().PersonalAccessTokens.GetSinglePersonalAccessToken(gl.WithContext(ctx))
		if err != nil {
			return Output{}, toolutil.WrapErrWithStatusHint("get current personal access token", err, http.StatusUnauthorized,
				"the calling credential is not a personal access token (e.g. OAuth or job token) \u2014 supply token_id to introspect a specific PAT instead")
		}
		return fromPersonalToken(t), nil
	}

	t, _, err := client.GL().PersonalAccessTokens.GetSinglePersonalAccessTokenByID(input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get personal access token", err, http.StatusNotFound,
			"token_id not found, already revoked, or owned by another user (admin token required to inspect other users' tokens)")
	}
	return fromPersonalToken(t), nil
}

// PersonalRotateInput defines parameters for rotating a personal access token.
type PersonalRotateInput struct {
	TokenID   int64  `json:"token_id"             jsonschema:"Access token ID,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// PersonalRotate rotates a personal access token and returns the new token.
func PersonalRotate(ctx context.Context, client *gitlabclient.Client, input PersonalRotateInput) (Output, error) {
	if input.TokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotatePersonalAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().PersonalAccessTokens.RotatePersonalAccessToken(input.TokenID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("rotate personal access token", err,
				"token may already be revoked/expired; expires_at must be YYYY-MM-DD")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("rotate personal access token", err, http.StatusNotFound,
			"token_id not found \u2014 use gitlab_access_token_personal_list to verify")
	}
	return fromPersonalToken(token), nil
}

// PersonalRevokeInput defines parameters for revoking a personal access token.
type PersonalRevokeInput struct {
	TokenID int64 `json:"token_id" jsonschema:"Access token ID to revoke,required"`
}

// PersonalRevoke revokes a personal access token by ID.
func PersonalRevoke(ctx context.Context, client *gitlabclient.Client, input PersonalRevokeInput) error {
	if input.TokenID == 0 {
		return errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().PersonalAccessTokens.RevokePersonalAccessTokenByID(input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("revoke personal access token", err, http.StatusNotFound,
			"token already revoked or never existed \u2014 nothing to do")
	}
	return nil
}

// PersonalRotateSelfInput defines parameters for self-rotating the current personal access token.
type PersonalRotateSelfInput struct {
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// PersonalRotateSelf rotates the personal access token used for the current request.
func PersonalRotateSelf(ctx context.Context, client *gitlabclient.Client, input PersonalRotateSelfInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RotatePersonalAccessTokenOptions{}
	if input.ExpiresAt != "" {
		t, err := time.Parse(toolutil.DateFormatISO, input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf(errInvalidExpiresAtFmt, err)
		}
		isoT := gl.ISOTime(t)
		opts.ExpiresAt = &isoT
	}

	token, _, err := client.GL().PersonalAccessTokens.RotatePersonalAccessTokenSelf(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("self-rotate personal access token", err, http.StatusUnauthorized,
			"the calling credential is not a personal access token (e.g. OAuth or job token) or has already been rotated/revoked")
	}
	return fromPersonalToken(token), nil
}

// PersonalRevokeSelfInput is an empty struct for self-revoking the current PAT.
type PersonalRevokeSelfInput struct{}

// PersonalRevokeSelf revokes the personal access token used for the current request.
func PersonalRevokeSelf(ctx context.Context, client *gitlabclient.Client, _ PersonalRevokeSelfInput) error {
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().PersonalAccessTokens.RevokePersonalAccessTokenSelf(gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("self-revoke personal access token", err, http.StatusUnauthorized,
			"the calling credential is not a personal access token or has already been revoked")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// tokenAccessLevelNames maps GitLab numeric access levels to role names.
var tokenAccessLevelNames = map[int]string{
	10: "Guest",
	20: "Reporter",
	30: "Developer",
	40: "Maintainer",
	50: "Owner",
}

// accessLevelName maps GitLab numeric access levels to human-readable role names.
func accessLevelName(level int) string {
	if name, ok := tokenAccessLevelNames[level]; ok {
		return name
	}
	return fmt.Sprintf("Unknown (%d)", level)
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
