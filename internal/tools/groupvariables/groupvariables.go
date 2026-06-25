package groupvariables

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Operation names used by error wrappers (kept as constants to satisfy S1192).
const (
	opCreateGroupVariable = "create group variable"
	opUpdateGroupVariable = "update group variable"
	opDeleteGroupVariable = "delete group variable"
)

// ---------- Input types ----------.

// ListInput holds parameters for listing group CI/CD variables. The OrderBy,
// Sort, and embedded KeysetPaginationInput fields mirror the SDK
// gitlab.ListGroupVariablesOptions (its embedded gitlab.ListOptions: order_by,
// sort, pagination, page_token) so every supported list filter is exposed 1:1.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column by which to order keyset-paginated results"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset-paginated results (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Filter mirrors gitlab.VariableFilter: the available filter on group variable
// get/update/delete operations, selecting the variable instance by its
// environment scope when several variables share a key.
type Filter struct {
	EnvironmentScope string `json:"environment_scope,omitempty" jsonschema:"Filter the variable by its environment scope (e.g. * or production)"`
}

// GetInput holds parameters for retrieving a single group CI/CD variable.
type GetInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"          jsonschema:"Group ID or URL-encoded path,required"`
	Key              string               `json:"key"               jsonschema:"Variable key name,required"`
	EnvironmentScope string               `json:"environment_scope,omitempty" jsonschema:"Filter by environment scope (shorthand for filter.environment_scope)"`
	Filter           *Filter              `json:"filter,omitempty" jsonschema:"Filter selecting the variable by environment scope; mirrors the GitLab variable filter object"`
}

// CreateInput holds parameters for creating a group CI/CD variable.
type CreateInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	Key              string               `json:"key"                            jsonschema:"Variable key name,required"`
	Value            string               `json:"value"                          jsonschema:"Variable value"`
	Description      string               `json:"description,omitempty"          jsonschema:"Variable description"`
	VariableType     string               `json:"variable_type,omitempty"        jsonschema:"Variable type: env_var or file"`
	Protected        *bool                `json:"protected,omitempty"            jsonschema:"Only expose in protected branches/tags"`
	Masked           *bool                `json:"masked,omitempty"               jsonschema:"Mask variable value in job logs"`
	MaskedAndHidden  *bool                `json:"masked_and_hidden,omitempty"    jsonschema:"Mask and hide variable value"`
	Raw              *bool                `json:"raw,omitempty"                  jsonschema:"Treat variable value as raw string"`
	EnvironmentScope string               `json:"environment_scope,omitempty"    jsonschema:"Environment scope (default: *)"`
}

// UpdateInput holds parameters for updating a group CI/CD variable.
type UpdateInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	Key              string               `json:"key"                            jsonschema:"Variable key name,required"`
	Value            string               `json:"value,omitempty"                jsonschema:"Updated variable value"`
	Description      string               `json:"description,omitempty"          jsonschema:"Updated variable description"`
	VariableType     string               `json:"variable_type,omitempty"        jsonschema:"Variable type: env_var or file"`
	Protected        *bool                `json:"protected,omitempty"            jsonschema:"Only expose in protected branches/tags"`
	Masked           *bool                `json:"masked,omitempty"               jsonschema:"Mask variable value in job logs"`
	Raw              *bool                `json:"raw,omitempty"                  jsonschema:"Treat variable value as raw string"`
	EnvironmentScope string               `json:"environment_scope,omitempty"    jsonschema:"Filter by environment scope (shorthand for filter.environment_scope)"`
	Filter           *Filter              `json:"filter,omitempty"               jsonschema:"Filter selecting the variable by environment scope; mirrors the GitLab variable filter object"`
}

// DeleteInput holds parameters for deleting a group CI/CD variable.
type DeleteInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	Key              string               `json:"key"                            jsonschema:"Variable key name,required"`
	EnvironmentScope string               `json:"environment_scope,omitempty"    jsonschema:"Filter by environment scope (shorthand for filter.environment_scope)"`
	Filter           *Filter              `json:"filter,omitempty"               jsonschema:"Filter selecting the variable by environment scope; mirrors the GitLab variable filter object"`
}

// ---------- Output types ----------.

// Output represents a single group CI/CD variable.
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

// ListOutput represents a paginated list of group CI/CD variables.
type ListOutput struct {
	toolutil.HintableOutput
	Variables  []Output                  `json:"variables"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------- Converter ----------.

// resolveScope returns the effective environment-scope filter for a variable
// get/update/delete request. The nested filter object takes precedence over the
// flat environment_scope shorthand; both mirror gitlab.VariableFilter.
func resolveScope(filter *Filter, flat string) string {
	if filter != nil && filter.EnvironmentScope != "" {
		return filter.EnvironmentScope
	}
	return flat
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(v *gl.GroupVariable) Output {
	return Output{
		Key:              v.Key,
		Value:            v.Value,
		VariableType:     string(v.VariableType),
		Protected:        v.Protected,
		Masked:           v.Masked,
		Hidden:           v.Hidden,
		Raw:              v.Raw,
		EnvironmentScope: v.EnvironmentScope,
		Description:      v.Description,
	}
}

// ---------- Handlers ----------.

// List retrieves a paginated list of CI/CD variables for a GitLab group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list group variables", err)
	}

	opts := &gl.ListGroupVariablesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	vars, resp, err := client.GL().GroupVariables.ListVariables(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list group variables", err, http.StatusNotFound,
			"verify group_id; listing variables requires Maintainer or Owner role")
	}

	out := ListOutput{Variables: make([]Output, 0, len(vars))}
	for _, v := range vars {
		out.Variables = append(out.Variables, toOutput(v))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single group CI/CD variable by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get group variable", err)
	}

	var opts *gl.GetGroupVariableOptions
	if scope := resolveScope(input.Filter, input.EnvironmentScope); scope != "" {
		opts = &gl.GetGroupVariableOptions{
			Filter: &gl.VariableFilter{EnvironmentScope: scope},
		}
	}

	v, _, err := client.GL().GroupVariables.GetVariable(string(input.GroupID), input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get group variable", err, http.StatusNotFound,
			"verify the variable key with gitlab_group_variable_list; for scoped vars supply matching environment_scope filter")
	}
	return toOutput(v), nil
}

// Create creates a new CI/CD variable in a GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if input.Value == "" {
		return Output{}, toolutil.ErrFieldRequired("value")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opCreateGroupVariable, err)
	}

	opts := &gl.CreateGroupVariableOptions{
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
	if input.MaskedAndHidden != nil {
		opts.MaskedAndHidden = input.MaskedAndHidden
	}
	if input.Raw != nil {
		opts.Raw = input.Raw
	}
	if input.EnvironmentScope != "" {
		opts.EnvironmentScope = &input.EnvironmentScope
	}

	v, _, err := client.GL().GroupVariables.CreateVariable(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opCreateGroupVariable, err,
				"creating group variables requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opCreateGroupVariable, err, http.StatusBadRequest,
			"key must match /^[A-Za-z0-9_]{1,255}$/; valid variable_type: env_var (default) or file; the (key, environment_scope) pair may already exist")
	}
	return toOutput(v), nil
}

// Update modifies an existing group CI/CD variable.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opUpdateGroupVariable, err)
	}

	opts := &gl.UpdateGroupVariableOptions{}
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
	if scope := resolveScope(input.Filter, input.EnvironmentScope); scope != "" {
		opts.Filter = &gl.VariableFilter{EnvironmentScope: scope}
	}

	v, _, err := client.GL().GroupVariables.UpdateVariable(string(input.GroupID), input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opUpdateGroupVariable, err,
				"updating group variables requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opUpdateGroupVariable, err, http.StatusNotFound,
			"verify the variable key and environment_scope with gitlab_group_variable_list")
	}
	return toOutput(v), nil
}

// Delete removes a group CI/CD variable by key.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.Key == "" {
		return toolutil.ErrFieldRequired("key")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(opDeleteGroupVariable, err)
	}

	var opts *gl.RemoveGroupVariableOptions
	if scope := resolveScope(input.Filter, input.EnvironmentScope); scope != "" {
		opts = &gl.RemoveGroupVariableOptions{
			Filter: &gl.VariableFilter{EnvironmentScope: scope},
		}
	}

	_, err := client.GL().GroupVariables.RemoveVariable(string(input.GroupID), input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint(opDeleteGroupVariable, err,
				"deleting group variables requires Maintainer or Owner role")
		}
		return toolutil.WrapErrWithStatusHint(opDeleteGroupVariable, err, http.StatusNotFound,
			"the variable may already be deleted \u2014 verify with gitlab_group_variable_list")
	}
	return nil
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group CI/CD variable."}, nil
}

// ---------- Formatters ----------.
