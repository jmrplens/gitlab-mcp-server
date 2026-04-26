// Package instancevariables implements GitLab instance-level CI/CD variable
// operations including list, get, create, update, and delete.
package instancevariables

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Operation names used by error wrappers (kept as constants to satisfy S1192).
const (
	opCreateInstanceVariable = "create instance variable"
	opUpdateInstanceVariable = "update instance variable"
	opDeleteInstanceVariable = "delete instance variable"
)

// ---------- Input types ----------.

// ListInput holds parameters for listing instance CI/CD variables.
type ListInput struct {
	toolutil.PaginationInput
}

// GetInput holds parameters for retrieving a single instance CI/CD variable.
type GetInput struct {
	Key string `json:"key" jsonschema:"Variable key name,required"`
}

// CreateInput holds parameters for creating an instance CI/CD variable.
type CreateInput struct {
	Key          string `json:"key"                        jsonschema:"Variable key name,required"`
	Value        string `json:"value"                      jsonschema:"Variable value"`
	Description  string `json:"description,omitempty"      jsonschema:"Variable description"`
	VariableType string `json:"variable_type,omitempty"    jsonschema:"Variable type: env_var or file"`
	Protected    *bool  `json:"protected,omitempty"        jsonschema:"Only expose in protected branches/tags"`
	Masked       *bool  `json:"masked,omitempty"           jsonschema:"Mask variable value in job logs"`
	Raw          *bool  `json:"raw,omitempty"              jsonschema:"Treat variable value as raw string"`
}

// UpdateInput holds parameters for updating an instance CI/CD variable.
type UpdateInput struct {
	Key          string `json:"key"                        jsonschema:"Variable key name,required"`
	Value        string `json:"value,omitempty"            jsonschema:"Updated variable value"`
	Description  string `json:"description,omitempty"      jsonschema:"Updated variable description"`
	VariableType string `json:"variable_type,omitempty"    jsonschema:"Variable type: env_var or file"`
	Protected    *bool  `json:"protected,omitempty"        jsonschema:"Only expose in protected branches/tags"`
	Masked       *bool  `json:"masked,omitempty"           jsonschema:"Mask variable value in job logs"`
	Raw          *bool  `json:"raw,omitempty"              jsonschema:"Treat variable value as raw string"`
}

// DeleteInput holds parameters for deleting an instance CI/CD variable.
type DeleteInput struct {
	Key string `json:"key" jsonschema:"Variable key name,required"`
}

// ---------- Output types ----------.

// Output represents a single instance CI/CD variable.
type Output struct {
	toolutil.HintableOutput
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
	Protected    bool   `json:"protected"`
	Masked       bool   `json:"masked"`
	Raw          bool   `json:"raw"`
	Description  string `json:"description"`
}

// ListOutput represents a paginated list of instance CI/CD variables.
type ListOutput struct {
	toolutil.HintableOutput
	Variables  []Output                  `json:"variables"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------- Converter ----------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(v *gl.InstanceVariable) Output {
	return Output{
		Key:          v.Key,
		Value:        v.Value,
		VariableType: string(v.VariableType),
		Protected:    v.Protected,
		Masked:       v.Masked,
		Raw:          v.Raw,
		Description:  v.Description,
	}
}

// ---------- Handlers ----------.

// List retrieves a paginated list of instance-level CI/CD variables.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list instance variables", err)
	}

	opts := &gl.ListInstanceVariablesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	vars, resp, err := client.GL().InstanceVariables.ListVariables(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list instance variables", err, http.StatusForbidden,
			"instance-level CI/CD variables are admin-only \u2014 verify your token has admin scope")
	}

	out := ListOutput{Variables: make([]Output, 0, len(vars))}
	for _, v := range vars {
		out.Variables = append(out.Variables, toOutput(v))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single instance-level CI/CD variable by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get instance variable", err)
	}

	v, _, err := client.GL().InstanceVariables.GetVariable(input.Key, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get instance variable", err, http.StatusNotFound,
			"verify the variable key exists with gitlab_instance_variable_list; admin-only API")
	}
	return toOutput(v), nil
}

// Create creates a new instance-level CI/CD variable.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if input.Value == "" {
		return Output{}, toolutil.ErrFieldRequired("value")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opCreateInstanceVariable, err)
	}

	opts := &gl.CreateInstanceVariableOptions{
		Key:   &input.Key,
		Value: &input.Value,
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.VariableType != "" {
		vt := gl.VariableTypeValue(input.VariableType)
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

	v, _, err := client.GL().InstanceVariables.CreateVariable(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opCreateInstanceVariable, err,
				"creating instance variables requires admin privileges")
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opCreateInstanceVariable, err, http.StatusBadRequest,
			"key must match /^[A-Za-z0-9_]{1,255}$/; valid variable_type: env_var (default) or file; the key may already exist")
	}
	return toOutput(v), nil
}

// Update modifies an existing instance-level CI/CD variable.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opUpdateInstanceVariable, err)
	}

	opts := &gl.UpdateInstanceVariableOptions{}
	if input.Value != "" {
		opts.Value = &input.Value
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.VariableType != "" {
		vt := gl.VariableTypeValue(input.VariableType)
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

	v, _, err := client.GL().InstanceVariables.UpdateVariable(input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opUpdateInstanceVariable, err,
				"updating instance variables requires admin privileges")
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opUpdateInstanceVariable, err, http.StatusNotFound,
			"verify the variable key exists with gitlab_instance_variable_list")
	}
	return toOutput(v), nil
}

// Delete removes an instance-level CI/CD variable by key.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.Key == "" {
		return toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(opDeleteInstanceVariable, err)
	}

	_, err := client.GL().InstanceVariables.RemoveVariable(input.Key, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint(opDeleteInstanceVariable, err,
				"deleting instance variables requires admin privileges")
		}
		return toolutil.WrapErrWithStatusHint(opDeleteInstanceVariable, err, http.StatusNotFound,
			"the variable may already be deleted \u2014 verify with gitlab_instance_variable_list")
	}
	return nil
}

// ---------- Formatters ----------.
