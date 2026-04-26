// Package civariables implements MCP tool handlers for GitLab project-level
// CI/CD variables. It supports list, get, create, update, and delete operations
// via the ProjectVariables API.
package civariables

import (
	"context"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------.

// ListInput holds parameters for listing project CI/CD variables.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput holds parameters for retrieving a single project CI/CD variable.
type GetInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key              string               `json:"key" jsonschema:"Variable key name,required"`
	EnvironmentScope string               `json:"environment_scope" jsonschema:"Filter by environment scope"`
}

// CreateInput holds parameters for creating a project CI/CD variable.
type CreateInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key              string               `json:"key" jsonschema:"Variable key name,required"`
	Value            string               `json:"value" jsonschema:"Variable value,required"`
	Description      string               `json:"description" jsonschema:"Variable description"`
	VariableType     string               `json:"variable_type" jsonschema:"Variable type: env_var or file"`
	Protected        *bool                `json:"protected" jsonschema:"Only expose in protected branches/tags"`
	Masked           *bool                `json:"masked" jsonschema:"Mask variable value in job logs"`
	MaskedAndHidden  *bool                `json:"masked_and_hidden" jsonschema:"Mask and hide variable value"`
	Raw              *bool                `json:"raw" jsonschema:"Treat variable value as raw string"`
	EnvironmentScope string               `json:"environment_scope" jsonschema:"Environment scope (default: *)"`
}

// UpdateInput holds parameters for updating a project CI/CD variable.
type UpdateInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key              string               `json:"key" jsonschema:"Variable key name,required"`
	Value            string               `json:"value" jsonschema:"Updated variable value"`
	Description      string               `json:"description" jsonschema:"Updated variable description"`
	VariableType     string               `json:"variable_type" jsonschema:"Variable type: env_var or file"`
	Protected        *bool                `json:"protected" jsonschema:"Only expose in protected branches/tags"`
	Masked           *bool                `json:"masked" jsonschema:"Mask variable value in job logs"`
	Raw              *bool                `json:"raw" jsonschema:"Treat variable value as raw string"`
	EnvironmentScope string               `json:"environment_scope" jsonschema:"Filter by environment scope"`
}

// DeleteInput holds parameters for deleting a project CI/CD variable.
type DeleteInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key              string               `json:"key" jsonschema:"Variable key name,required"`
	EnvironmentScope string               `json:"environment_scope" jsonschema:"Filter by environment scope"`
}

// Output represents a single CI/CD variable.
type Output struct {
	toolutil.HintableOutput
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
	Hidden           bool   `json:"hidden"`
	Raw              bool   `json:"raw"`
	EnvironmentScope string `json:"environment_scope"`
	Description      string `json:"description"`
}

// ListOutput represents a paginated list of CI/CD variables.
type ListOutput struct {
	toolutil.HintableOutput
	Variables  []Output                  `json:"variables"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// maskedPlaceholder replaces real values for masked or hidden CI/CD variables.
const maskedPlaceholder = "[masked]"

// toOutput converts the GitLab API response to the tool output format.
// Masked or hidden variable values are redacted to prevent accidental exposure.
func toOutput(v *gitlab.ProjectVariable) Output {
	value := v.Value
	if v.Masked || v.Hidden {
		value = maskedPlaceholder
	}

	return Output{
		Key:              v.Key,
		Value:            value,
		VariableType:     string(v.VariableType),
		Protected:        v.Protected,
		Masked:           v.Masked,
		Hidden:           v.Hidden,
		Raw:              v.Raw,
		EnvironmentScope: v.EnvironmentScope,
		Description:      v.Description,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List lists resources for the civariables package.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list CI/CD variables", err)
	}

	opts := &gitlab.ListProjectVariablesOptions{
		ListOptions: gitlab.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	vars, resp, err := client.GL().ProjectVariables.ListVariables(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list CI/CD variables", err, http.StatusNotFound,
			"verify project_id; listing CI/CD variables requires Maintainer or Owner role")
	}

	out := ListOutput{
		Variables: make([]Output, 0, len(vars)),
	}
	for _, v := range vars {
		out.Variables = append(out.Variables, toOutput(v))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves resources for the civariables package.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get CI/CD variable", err)
	}

	var opts *gitlab.GetProjectVariableOptions
	if input.EnvironmentScope != "" {
		opts = &gitlab.GetProjectVariableOptions{
			Filter: &gitlab.VariableFilter{EnvironmentScope: input.EnvironmentScope},
		}
	}

	v, _, err := client.GL().ProjectVariables.GetVariable(string(input.ProjectID), input.Key, opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get CI/CD variable", err, http.StatusNotFound,
			"verify the variable key with gitlab_list_ci_variables; for scoped vars supply matching environment_scope filter")
	}
	return toOutput(v), nil
}

// Create creates resources for the civariables package.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if input.Value == "" {
		return Output{}, toolutil.ErrFieldRequired("value")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create CI/CD variable", err)
	}

	opts := &gitlab.CreateProjectVariableOptions{
		Key:   &input.Key,
		Value: &input.Value,
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.VariableType != "" {
		vt := gitlab.VariableTypeValue(input.VariableType)
		opts.VariableType = &vt
	}
	if input.Protected != nil {
		opts.Protected = input.Protected
	}
	if input.Masked != nil {
		opts.Masked = input.Masked
	}
	if input.MaskedAndHidden != nil {
		opts.MaskedAndHidden = input.MaskedAndHidden
	}
	if input.Raw != nil {
		opts.Raw = input.Raw
	}
	if input.EnvironmentScope != "" {
		opts.EnvironmentScope = &input.EnvironmentScope
	}

	v, _, err := client.GL().ProjectVariables.CreateVariable(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("create CI/CD variable", err,
				"creating CI/CD variables requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("create CI/CD variable", err, http.StatusBadRequest,
			"key must match /^[A-Za-z0-9_]{1,255}$/; valid variable_type: env_var (default) or file; the (key, environment_scope) pair may already exist; masked vars require values without newlines and minimum 8 chars")
	}
	return toOutput(v), nil
}

// Update updates resources for the civariables package.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("update CI/CD variable", err)
	}

	opts := &gitlab.UpdateProjectVariableOptions{}
	if input.Value != "" {
		opts.Value = &input.Value
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.VariableType != "" {
		vt := gitlab.VariableTypeValue(input.VariableType)
		opts.VariableType = &vt
	}
	if input.Protected != nil {
		opts.Protected = input.Protected
	}
	if input.Masked != nil {
		opts.Masked = input.Masked
	}
	if input.Raw != nil {
		opts.Raw = input.Raw
	}
	if input.EnvironmentScope != "" {
		opts.EnvironmentScope = &input.EnvironmentScope
	}

	v, _, err := client.GL().ProjectVariables.UpdateVariable(string(input.ProjectID), input.Key, opts, gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("update CI/CD variable", err,
				"updating CI/CD variables requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("update CI/CD variable", err, http.StatusNotFound,
			"verify the variable key and environment_scope with gitlab_list_ci_variables")
	}
	return toOutput(v), nil
}

// Delete deletes resources for the civariables package.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.Key == "" {
		return toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage("delete CI/CD variable", err)
	}

	var opts *gitlab.RemoveProjectVariableOptions
	if input.EnvironmentScope != "" {
		opts = &gitlab.RemoveProjectVariableOptions{
			Filter: &gitlab.VariableFilter{EnvironmentScope: input.EnvironmentScope},
		}
	}

	_, err := client.GL().ProjectVariables.RemoveVariable(string(input.ProjectID), input.Key, opts, gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("delete CI/CD variable", err,
				"deleting CI/CD variables requires Maintainer or Owner role")
		}
		return toolutil.WrapErrWithStatusHint("delete CI/CD variable", err, http.StatusNotFound,
			"the variable may already be deleted \u2014 verify with gitlab_list_ci_variables")
	}
	return nil
}
