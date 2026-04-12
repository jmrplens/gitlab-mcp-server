// Package deploytokens implements GitLab Deploy Tokens API operations as MCP tools.
// It supports listing, getting, creating, and deleting deploy tokens at
// project, group, and instance levels.
package deploytokens

import (
	"context"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a GitLab deploy token.
type Output struct {
	toolutil.HintableOutput
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Username  string   `json:"username"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Revoked   bool     `json:"revoked"`
	Expired   bool     `json:"expired"`
	Token     string   `json:"token,omitempty"`
	Scopes    []string `json:"scopes"`
}

// ListOutput holds a paginated list of deploy tokens.
type ListOutput struct {
	toolutil.HintableOutput
	DeployTokens []Output                  `json:"deploy_tokens"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// timeStr is an internal helper for the deploytokens package.
func timeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(t *gl.DeployToken) Output {
	return Output{
		ID:        t.ID,
		Name:      t.Name,
		Username:  t.Username,
		ExpiresAt: timeStr(t.ExpiresAt),
		Revoked:   t.Revoked,
		Expired:   t.Expired,
		Token:     t.Token,
		Scopes:    t.Scopes,
	}
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListAllInput represents parameters for listing all instance deploy tokens.
type ListAllInput struct{}

// ListProjectInput represents parameters for listing project deploy tokens.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int                  `json:"per_page,omitempty" jsonschema:"Results per page (max 100)"`
}

// ListGroupInput represents parameters for listing group deploy tokens.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Page    int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int                  `json:"per_page,omitempty" jsonschema:"Results per page (max 100)"`
}

// GetProjectInput represents parameters for getting a project deploy token.
type GetProjectInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	DeployTokenID int64                `json:"deploy_token_id" jsonschema:"Deploy token ID,required"`
}

// GetGroupInput represents parameters for getting a group deploy token.
type GetGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	DeployTokenID int64                `json:"deploy_token_id" jsonschema:"Deploy token ID,required"`
}

// CreateProjectInput represents parameters for creating a project deploy token.
type CreateProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Deploy token name,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"Expiry date (YYYY-MM-DD)"`
	Username  string               `json:"username,omitempty" jsonschema:"Username for the deploy token"`
	Scopes    []string             `json:"scopes" jsonschema:"Array of scopes (read_repository, read_registry, write_registry, read_package_registry, write_package_registry)"`
}

// CreateGroupInput represents parameters for creating a group deploy token.
type CreateGroupInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Deploy token name,required"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"Expiry date (YYYY-MM-DD)"`
	Username  string               `json:"username,omitempty" jsonschema:"Username for the deploy token"`
	Scopes    []string             `json:"scopes" jsonschema:"Array of scopes (read_repository, read_registry, write_registry, read_package_registry, write_package_registry)"`
}

// DeleteProjectInput represents parameters for deleting a project deploy token.
type DeleteProjectInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	DeployTokenID int64                `json:"deploy_token_id" jsonschema:"Deploy token ID,required"`
}

// DeleteGroupInput represents parameters for deleting a group deploy token.
type DeleteGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	DeployTokenID int64                `json:"deploy_token_id" jsonschema:"Deploy token ID,required"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// ListAll lists all instance-level deploy tokens.
func ListAll(ctx context.Context, client *gitlabclient.Client, _ ListAllInput) (ListOutput, error) {
	tokens, resp, err := client.GL().DeployTokens.ListAllDeployTokens(gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("deploy_token_list_all", err)
	}

	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, t := range tokens {
		out.DeployTokens = append(out.DeployTokens, toOutput(t))
	}
	return out, nil
}

// ListProject lists deploy tokens for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := &gl.ListProjectDeployTokensOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	tokens, resp, err := client.GL().DeployTokens.ListProjectDeployTokens(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("deploy_token_list_project", err)
	}

	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, t := range tokens {
		out.DeployTokens = append(out.DeployTokens, toOutput(t))
	}
	return out, nil
}

// ListGroup lists deploy tokens for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}

	opts := &gl.ListGroupDeployTokensOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	tokens, resp, err := client.GL().DeployTokens.ListGroupDeployTokens(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("deploy_token_list_group", err)
	}

	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, t := range tokens {
		out.DeployTokens = append(out.DeployTokens, toOutput(t))
	}
	return out, nil
}

// GetProject retrieves a specific project deploy token.
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployTokenID == 0 {
		return Output{}, toolutil.ErrFieldRequired("deploy_token_id")
	}

	token, _, err := client.GL().DeployTokens.GetProjectDeployToken(string(input.ProjectID), input.DeployTokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("deploy_token_get_project", err)
	}

	return toOutput(token), nil
}

// GetGroup retrieves a specific group deploy token.
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.DeployTokenID == 0 {
		return Output{}, toolutil.ErrFieldRequired("deploy_token_id")
	}

	token, _, err := client.GL().DeployTokens.GetGroupDeployToken(string(input.GroupID), input.DeployTokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("deploy_token_get_group", err)
	}

	return toOutput(token), nil
}

// CreateProject creates a deploy token for a project.
func CreateProject(ctx context.Context, client *gitlabclient.Client, input CreateProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return Output{}, toolutil.ErrFieldRequired("scopes")
	}

	opts := &gl.CreateProjectDeployTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf("invalid expires_at format, use YYYY-MM-DD: %w", err)
		}
		opts.ExpiresAt = &t
	}

	token, _, err := client.GL().DeployTokens.CreateProjectDeployToken(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("deploy_token_create_project", err)
	}

	return toOutput(token), nil
}

// CreateGroup creates a deploy token for a group.
func CreateGroup(ctx context.Context, client *gitlabclient.Client, input CreateGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if len(input.Scopes) == 0 {
		return Output{}, toolutil.ErrFieldRequired("scopes")
	}

	opts := &gl.CreateGroupDeployTokenOptions{
		Name:   new(input.Name),
		Scopes: &input.Scopes,
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf("invalid expires_at format, use YYYY-MM-DD: %w", err)
		}
		opts.ExpiresAt = &t
	}

	token, _, err := client.GL().DeployTokens.CreateGroupDeployToken(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("deploy_token_create_group", err)
	}

	return toOutput(token), nil
}

// DeleteProject deletes a project deploy token.
func DeleteProject(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployTokenID == 0 {
		return toolutil.ErrFieldRequired("deploy_token_id")
	}

	_, err := client.GL().DeployTokens.DeleteProjectDeployToken(string(input.ProjectID), input.DeployTokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deploy_token_delete_project", err)
	}

	return nil
}

// DeleteGroup deletes a group deploy token.
func DeleteGroup(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.DeployTokenID == 0 {
		return toolutil.ErrFieldRequired("deploy_token_id")
	}

	_, err := client.GL().DeployTokens.DeleteGroupDeployToken(string(input.GroupID), input.DeployTokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deploy_token_delete_group", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
