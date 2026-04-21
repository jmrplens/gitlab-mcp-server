// Package environments implements MCP tool handlers for GitLab environment
// lifecycle management including list, get, create, update, delete, and stop.
// It wraps the Environments service from client-go v2.
package environments

import (
	"context"
	"errors"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing project environments.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name,omitempty"   jsonschema:"Filter by exact environment name"`
	Search    string               `json:"search,omitempty" jsonschema:"Search environments by name (fuzzy)"`
	States    string               `json:"states,omitempty" jsonschema:"Filter by state: available, stopping, stopped"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single environment.
type GetInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	EnvironmentID int64                `json:"environment_id"  jsonschema:"Environment ID,required"`
}

// CreateInput defines parameters for creating an environment.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"             jsonschema:"Project ID or URL-encoded path,required"`
	Name        string               `json:"name"                   jsonschema:"Environment name (e.g. production, staging),required"`
	Description string               `json:"description,omitempty"  jsonschema:"Description of the environment"`
	ExternalURL string               `json:"external_url,omitempty" jsonschema:"URL of the environment's external deployment"`
	Tier        string               `json:"tier,omitempty"         jsonschema:"Deployment tier: production, staging, testing, development, other"`
}

// UpdateInput defines parameters for updating an environment.
type UpdateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"             jsonschema:"Project ID or URL-encoded path,required"`
	EnvironmentID int64                `json:"environment_id"         jsonschema:"Environment ID,required"`
	Name          string               `json:"name,omitempty"         jsonschema:"New environment name"`
	Description   string               `json:"description,omitempty"  jsonschema:"Updated description"`
	ExternalURL   string               `json:"external_url,omitempty" jsonschema:"Updated external URL"`
	Tier          string               `json:"tier,omitempty"         jsonschema:"Updated tier: production, staging, testing, development, other"`
}

// DeleteInput defines parameters for deleting an environment.
type DeleteInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	EnvironmentID int64                `json:"environment_id"  jsonschema:"Environment ID,required"`
}

// StopInput defines parameters for stopping an environment.
type StopInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"          jsonschema:"Project ID or URL-encoded path,required"`
	EnvironmentID int64                `json:"environment_id"      jsonschema:"Environment ID,required"`
	Force         *bool                `json:"force,omitempty"     jsonschema:"Force stop even if environment has active deployments"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single GitLab environment.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	State       string `json:"state"`
	Tier        string `json:"tier,omitempty"`
	ExternalURL string `json:"external_url,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	AutoStopAt  string `json:"auto_stop_at,omitempty"`
}

// ListOutput holds a paginated list of environments.
type ListOutput struct {
	toolutil.HintableOutput
	Environments []Output                  `json:"environments"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// environmentToOutput converts a client-go Environment to the MCP output type.
func toOutput(e *gl.Environment) Output {
	out := Output{
		ID:          e.ID,
		Name:        e.Name,
		Slug:        e.Slug,
		Description: e.Description,
		State:       e.State,
		Tier:        e.Tier,
		ExternalURL: e.ExternalURL,
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	if e.UpdatedAt != nil {
		out.UpdatedAt = e.UpdatedAt.Format(time.RFC3339)
	}
	if e.AutoStopAt != nil {
		out.AutoStopAt = e.AutoStopAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List retrieves all environments for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("environmentList: project_id is required")
	}
	opts := &gl.ListEnvironmentsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.States != "" {
		opts.States = new(input.States)
	}
	envs, resp, err := client.GL().Environments.ListEnvironments(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("environmentList", err)
	}
	out := ListOutput{
		Environments: make([]Output, 0, len(envs)),
		Pagination:   toolutil.PaginationFromResponse(resp),
	}
	for _, e := range envs {
		out.Environments = append(out.Environments, toOutput(e))
	}
	return out, nil
}

// Get retrieves a single environment by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("environmentGet: project_id is required")
	}
	if input.EnvironmentID == 0 {
		return Output{}, errors.New("environmentGet: environment_id is required")
	}
	env, _, err := client.GL().Environments.GetEnvironment(string(input.ProjectID), input.EnvironmentID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("environmentGet", err)
	}
	return toOutput(env), nil
}

// Create creates a new environment in a project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("environmentCreate: project_id is required")
	}
	if input.Name == "" {
		return Output{}, errors.New("environmentCreate: name is required")
	}
	opts := &gl.CreateEnvironmentOptions{
		Name: new(input.Name),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ExternalURL != "" {
		opts.ExternalURL = new(input.ExternalURL)
	}
	if input.Tier != "" {
		opts.Tier = new(input.Tier)
	}
	env, _, err := client.GL().Environments.CreateEnvironment(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("environmentCreate", err)
	}
	return toOutput(env), nil
}

// Update updates an existing environment.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("environmentUpdate: project_id is required")
	}
	if input.EnvironmentID == 0 {
		return Output{}, errors.New("environmentUpdate: environment_id is required")
	}
	opts := &gl.EditEnvironmentOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ExternalURL != "" {
		opts.ExternalURL = new(input.ExternalURL)
	}
	if input.Tier != "" {
		opts.Tier = new(input.Tier)
	}
	env, _, err := client.GL().Environments.EditEnvironment(string(input.ProjectID), input.EnvironmentID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("environmentUpdate", err)
	}
	return toOutput(env), nil
}

// Delete deletes an environment from a project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("environmentDelete: project_id is required")
	}
	if input.EnvironmentID == 0 {
		return errors.New("environmentDelete: environment_id is required")
	}
	_, err := client.GL().Environments.DeleteEnvironment(string(input.ProjectID), input.EnvironmentID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("environmentDelete", err)
	}
	return nil
}

// Stop stops an active environment.
func Stop(ctx context.Context, client *gitlabclient.Client, input StopInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("environmentStop: project_id is required")
	}
	if input.EnvironmentID == 0 {
		return Output{}, errors.New("environmentStop: environment_id is required")
	}
	opts := &gl.StopEnvironmentOptions{}
	if input.Force != nil {
		opts.Force = input.Force
	}
	env, _, err := client.GL().Environments.StopEnvironment(string(input.ProjectID), input.EnvironmentID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("environmentStop", err)
	}
	return toOutput(env), nil
}
