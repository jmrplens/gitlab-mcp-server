package accesstokens

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errTokenIDInvalid identifies the err token ID invalid constant used by this package.
	errTokenIDInvalid = "token_id is required and must be > 0" //#nosec G101 -- false positive: error message, not a credential
	// errInvalidExpiresAtFmt identifies the err invalid expires at fmt constant used by this package.
	errInvalidExpiresAtFmt = "invalid expires_at format (expected YYYY-MM-DD): %w"
	// hintTokenAlreadyRevoked is returned when revoking a token that the API
	// reports as not found.
	hintTokenAlreadyRevoked = "token already revoked or never existed \u2014 nothing to do" //#nosec G101 -- error hint, not a credential
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

// fromProjectToken maps from project token between API and evaluator models.
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

// fromGroupToken maps from group token between API and evaluator models.
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

// fromPersonalToken maps from personal token between API and evaluator models.
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
			"token_id not found on this project (already revoked or never existed) - use gitlab_access_token_project_list to discover current token IDs")
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
	return createAccessToken(ctx, accessTokenCreateArgs{
		scopeID:        input.ProjectID,
		requiredField:  "project_id",
		operation:      "create project access token",
		validationHint: "validate scopes (api|read_api|read_repository|write_repository|read_registry|write_registry), access_level (10|20|30|40|50), and expires_at format (YYYY-MM-DD, must be within instance-configured maximum lifetime)",
		forbiddenHint:  "creating project access tokens requires Maintainer or Owner role; the requested access_level cannot exceed the caller's role",
		req:            accessTokenCreateRequest{Name: input.Name, Description: input.Description, Scopes: input.Scopes, AccessLevel: input.AccessLevel, ExpiresAt: input.ExpiresAt},
		create: func(scopeID string, req accessTokenCreateRequest, expiresAt *gl.ISOTime) (Output, error) {
			return createProjectAccessToken(ctx, client, scopeID, req, expiresAt)
		},
	})
}

func createProjectAccessToken(ctx context.Context, client *gitlabclient.Client, projectID string, req accessTokenCreateRequest, expiresAt *gl.ISOTime) (Output, error) {
	opts := &gl.CreateProjectAccessTokenOptions{Name: new(req.Name), Scopes: &req.Scopes, ExpiresAt: expiresAt}
	applyAccessTokenCreateOptions(req, func(value *string) { opts.Description = value }, func(value *gl.AccessLevelValue) { opts.AccessLevel = value })
	token, _, err := client.GL().ProjectAccessTokens.CreateProjectAccessToken(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, err
	}
	return fromProjectToken(token), nil
}

type accessTokenCreateRequest struct {
	Name        string
	Description string
	Scopes      []string
	AccessLevel int
	ExpiresAt   string
}

type accessTokenCreateArgs struct {
	scopeID        toolutil.StringOrInt
	requiredField  string
	operation      string
	validationHint string
	forbiddenHint  string
	req            accessTokenCreateRequest
	create         func(string, accessTokenCreateRequest, *gl.ISOTime) (Output, error)
}

func createAccessToken(ctx context.Context, args accessTokenCreateArgs) (Output, error) {
	if args.scopeID == "" {
		return Output{}, toolutil.ErrFieldRequired(args.requiredField)
	}
	if args.req.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(args.req.Scopes) == 0 {
		return Output{}, toolutil.ErrFieldRequired("scopes")
	}
	if err := validateAccessTokenScopes(args.req.Scopes); err != nil {
		return Output{}, err
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}
	expiresAtValue, hasExpiresAt, err := parseAccessTokenExpiresAt(args.req.ExpiresAt)
	if err != nil {
		return Output{}, err
	}
	var expiresAt *gl.ISOTime
	if hasExpiresAt {
		expiresAt = &expiresAtValue
	}
	out, err := args.create(string(args.scopeID), args.req, expiresAt)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint(args.operation, err, args.validationHint)
		}
		return Output{}, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusForbidden, args.forbiddenHint)
	}
	return out, nil
}

func applyAccessTokenCreateOptions(req accessTokenCreateRequest, setDescription func(*string), setAccessLevel func(*gl.AccessLevelValue)) {
	if req.Description != "" {
		setDescription(&req.Description)
	}
	if req.AccessLevel > 0 {
		level := gl.AccessLevelValue(req.AccessLevel)
		setAccessLevel(&level)
	}
}

// ProjectRotateInput defines parameters for rotating a project access token.
type ProjectRotateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	TokenID   int64                `json:"token_id"             jsonschema:"Access token ID,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"New expiry date in YYYY-MM-DD format"`
}

// ProjectRotate rotates a project access token and returns the new token.
func ProjectRotate(ctx context.Context, client *gitlabclient.Client, input ProjectRotateInput) (Output, error) {
	return rotateAccessToken(ctx, accessTokenRotateArgs{
		scopeID:        input.ProjectID,
		tokenID:        input.TokenID,
		expiresAtValue: input.ExpiresAt,
		requiredField:  "project_id",
		operation:      "rotate project access token",
		validationHint: "token may already be revoked/expired; expires_at must be YYYY-MM-DD and within instance maximum lifetime",
		notFoundHint:   "token_id not found - use gitlab_access_token_project_list to verify",
		rotate: func(scopeID string, tokenID int64, expiresAt *gl.ISOTime) (Output, error) {
			opts := &gl.RotateProjectAccessTokenOptions{ExpiresAt: expiresAt}
			token, _, err := client.GL().ProjectAccessTokens.RotateProjectAccessToken(scopeID, tokenID, opts, gl.WithContext(ctx))
			if err != nil {
				return Output{}, err
			}
			return fromProjectToken(token), nil
		},
	})
}

type accessTokenRotateArgs struct {
	scopeID        toolutil.StringOrInt
	tokenID        int64
	expiresAtValue string
	requiredField  string
	operation      string
	validationHint string
	notFoundHint   string
	rotate         func(string, int64, *gl.ISOTime) (Output, error)
}

func rotateAccessToken(ctx context.Context, args accessTokenRotateArgs) (Output, error) {
	if args.scopeID == "" {
		return Output{}, toolutil.ErrFieldRequired(args.requiredField)
	}
	if args.tokenID == 0 {
		return Output{}, errors.New(errTokenIDInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}
	expiresAtParsed, hasExpiresAt, err := parseAccessTokenExpiresAt(args.expiresAtValue)
	if err != nil {
		return Output{}, err
	}
	var expiresAt *gl.ISOTime
	if hasExpiresAt {
		expiresAt = &expiresAtParsed
	}
	out, err := args.rotate(string(args.scopeID), args.tokenID, expiresAt)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint(args.operation, err, args.validationHint)
		}
		return Output{}, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusNotFound, args.notFoundHint)
	}
	return out, nil
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
			hintTokenAlreadyRevoked)
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
	return rotateSelfAccessToken(ctx, input.ProjectID, input.ExpiresAt, "project_id", "self-rotate project access token",
		"the calling token is not a project access token for this project, or has already been rotated/revoked",
		func(scopeID string, expiresAt *gl.ISOTime) (Output, error) {
			opts := &gl.RotateProjectAccessTokenOptions{ExpiresAt: expiresAt}
			token, _, err := client.GL().ProjectAccessTokens.RotateProjectAccessTokenSelf(scopeID, opts, gl.WithContext(ctx))
			if err != nil {
				return Output{}, err
			}
			return fromProjectToken(token), nil
		})
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
	return createAccessToken(ctx, accessTokenCreateArgs{
		scopeID:        input.GroupID,
		requiredField:  "group_id",
		operation:      "create group access token",
		validationHint: "validate scopes (api|read_api|read_repository|write_repository|read_registry|write_registry), access_level (10|20|30|40|50), and expires_at format (YYYY-MM-DD)",
		forbiddenHint:  "creating group access tokens requires Owner role; the requested access_level cannot exceed the caller's role",
		req:            accessTokenCreateRequest{Name: input.Name, Description: input.Description, Scopes: input.Scopes, AccessLevel: input.AccessLevel, ExpiresAt: input.ExpiresAt},
		create: func(scopeID string, req accessTokenCreateRequest, expiresAt *gl.ISOTime) (Output, error) {
			return createGroupAccessToken(ctx, client, scopeID, req, expiresAt)
		},
	})
}

func createGroupAccessToken(ctx context.Context, client *gitlabclient.Client, groupID string, req accessTokenCreateRequest, expiresAt *gl.ISOTime) (Output, error) {
	opts := &gl.CreateGroupAccessTokenOptions{Name: new(req.Name), Scopes: &req.Scopes, ExpiresAt: expiresAt}
	applyAccessTokenCreateOptions(req, func(value *string) { opts.Description = value }, func(value *gl.AccessLevelValue) { opts.AccessLevel = value })
	token, _, err := client.GL().GroupAccessTokens.CreateGroupAccessToken(groupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, err
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
			hintTokenAlreadyRevoked)
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
	return rotateSelfAccessToken(ctx, input.GroupID, input.ExpiresAt, "group_id", "self-rotate group access token",
		"the calling token is not a group access token for this group, or has already been rotated/revoked",
		func(scopeID string, expiresAt *gl.ISOTime) (Output, error) {
			opts := &gl.RotateGroupAccessTokenOptions{ExpiresAt: expiresAt}
			token, _, err := client.GL().GroupAccessTokens.RotateGroupAccessTokenSelf(scopeID, opts, gl.WithContext(ctx))
			if err != nil {
				return Output{}, err
			}
			return fromGroupToken(token), nil
		})
}

func rotateSelfAccessToken(ctx context.Context, scopeID toolutil.StringOrInt, expiresAtValue, requiredField, operation, unauthorizedHint string, rotate func(string, *gl.ISOTime) (Output, error)) (Output, error) {
	if scopeID == "" {
		return Output{}, toolutil.ErrFieldRequired(requiredField)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}
	expiresAtParsed, hasExpiresAt, err := parseAccessTokenExpiresAt(expiresAtValue)
	if err != nil {
		return Output{}, err
	}
	var expiresAt *gl.ISOTime
	if hasExpiresAt {
		expiresAt = &expiresAtParsed
	}
	out, err := rotate(string(scopeID), expiresAt)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(operation, err, http.StatusUnauthorized, unauthorizedHint)
	}
	return out, nil
}

func parseAccessTokenExpiresAt(value string) (gl.ISOTime, bool, error) {
	if value == "" {
		return gl.ISOTime{}, false, nil
	}
	t, err := time.Parse(toolutil.DateFormatISO, value)
	if err != nil {
		return gl.ISOTime{}, false, fmt.Errorf(errInvalidExpiresAtFmt, err)
	}
	return gl.ISOTime(t), true, nil
}

var validAccessTokenScopeNames = []string{
	"admin_mode",
	"ai_features",
	"api",
	"create_runner",
	"k8s_proxy",
	"manage_runner",
	"read_api",
	"read_observability",
	"read_registry",
	"read_repository",
	"read_service_ping",
	"read_user",
	"read_virtual_registry",
	"self_rotate",
	"sudo",
	"write_observability",
	"write_registry",
	"write_repository",
	"write_virtual_registry",
}

var validAccessTokenScopes = func() map[string]struct{} {
	out := make(map[string]struct{}, len(validAccessTokenScopeNames))
	for _, scope := range validAccessTokenScopeNames {
		out[scope] = struct{}{}
	}
	return out
}()

func validateAccessTokenScopes(scopes []string) error {
	seen := make(map[string]struct{}, len(scopes))
	for index, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			return fmt.Errorf("scopes[%d] must not be empty", index)
		}
		if trimmed != scope {
			return fmt.Errorf("scopes[%d] %q must not contain surrounding whitespace", index, scope)
		}
		if _, ok := validAccessTokenScopes[scope]; !ok {
			return fmt.Errorf("scopes[%d] %q is not supported; expected one of %s", index, scope, strings.Join(validAccessTokenScopeNames, ", "))
		}
		if _, duplicate := seen[scope]; duplicate {
			return fmt.Errorf("scopes[%d] %q is duplicated", index, scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
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
			hintTokenAlreadyRevoked)
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
