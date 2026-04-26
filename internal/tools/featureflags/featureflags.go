// Package featureflags provides MCP tool handlers for GitLab project feature flag operations.
package featureflags

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ──────────────────────────────────────────────
// Shared output types
// ──────────────────────────────────────────────.

// ScopeOutput represents a feature flag scope.
type ScopeOutput struct {
	ID               int64  `json:"id"`
	EnvironmentScope string `json:"environment_scope"`
}

// StrategyParameterOutput represents strategy parameters.
type StrategyParameterOutput struct {
	GroupID    string `json:"group_id,omitempty"`
	UserIDs    string `json:"user_ids,omitempty"`
	Percentage string `json:"percentage,omitempty"`
	Rollout    string `json:"rollout,omitempty"`
	Stickiness string `json:"stickiness,omitempty"`
}

// StrategyOutput represents a feature flag strategy.
type StrategyOutput struct {
	ID         int64                    `json:"id"`
	Name       string                   `json:"name"`
	Parameters *StrategyParameterOutput `json:"parameters,omitempty"`
	Scopes     []ScopeOutput            `json:"scopes,omitempty"`
}

// Output represents a single project feature flag.
type Output struct {
	toolutil.HintableOutput
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Active      bool             `json:"active"`
	Version     string           `json:"version"`
	CreatedAt   string           `json:"created_at,omitempty"`
	UpdatedAt   string           `json:"updated_at,omitempty"`
	Strategies  []StrategyOutput `json:"strategies,omitempty"`
}

// ListOutput represents a paginated list of feature flags.
type ListOutput struct {
	toolutil.HintableOutput
	FeatureFlags []Output                  `json:"feature_flags"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// ──────────────────────────────────────────────
// Strategy input types (for create/update)
// ──────────────────────────────────────────────.

// ScopeInput represents a scope for strategy options.
type ScopeInput struct {
	EnvironmentScope string `json:"environment_scope"`
}

// StrategyParameterInput represents strategy parameters for create/update.
type StrategyParameterInput struct {
	GroupID    string `json:"group_id,omitempty"`
	UserIDs    string `json:"user_ids,omitempty"`
	Percentage string `json:"percentage,omitempty"`
	Rollout    string `json:"rollout,omitempty"`
	Stickiness string `json:"stickiness,omitempty"`
}

// StrategyInput represents a strategy for create/update operations.
type StrategyInput struct {
	ID         int64                   `json:"id,omitempty"`
	Name       string                  `json:"name"`
	Parameters *StrategyParameterInput `json:"parameters,omitempty"`
	Scopes     []ScopeInput            `json:"scopes,omitempty"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// ListInput contains parameters for listing feature flags.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Scope     string               `json:"scope,omitempty" jsonschema:"Filter by scope (enabled or disabled)"`
	toolutil.PaginationInput
}

// GetInput contains parameters for getting a feature flag.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name      string               `json:"name" jsonschema:"Feature flag name,required"`
}

// CreateInput contains parameters for creating a feature flag.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name        string               `json:"name" jsonschema:"Feature flag name,required"`
	Description string               `json:"description,omitempty" jsonschema:"Feature flag description"`
	Version     string               `json:"version,omitempty" jsonschema:"Version of the feature flag (new_version_flag)"`
	Active      *bool                `json:"active,omitempty" jsonschema:"Whether the flag is active"`
	Strategies  string               `json:"strategies,omitempty" jsonschema:"JSON array of strategy objects: [{name, parameters, scopes}]"`
}

// UpdateInput contains parameters for updating a feature flag.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name        string               `json:"name" jsonschema:"Current feature flag name,required"`
	NewName     string               `json:"new_name,omitempty" jsonschema:"New feature flag name"`
	Description string               `json:"description,omitempty" jsonschema:"Feature flag description"`
	Active      *bool                `json:"active,omitempty" jsonschema:"Whether the flag is active"`
	Strategies  string               `json:"strategies,omitempty" jsonschema:"JSON array of strategy objects: [{id, name, parameters, scopes}]"`
}

// DeleteInput contains parameters for deleting a feature flag.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name      string               `json:"name" jsonschema:"Feature flag name,required"`
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────.

// ListFeatureFlags lists feature flags for a project.
func ListFeatureFlags(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("feature_flag_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListProjectFeatureFlagOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	flags, resp, err := client.GL().ProjectFeatureFlags.ListProjectFeatureFlags(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ListOutput{}, toolutil.WrapErrWithHint("feature_flag_list", err,
				"feature flags require GitLab Premium/Ultimate \u2014 verify the project's tier and that you have Developer+ role")
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("feature_flag_list", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	out := ListOutput{
		FeatureFlags: make([]Output, 0, len(flags)),
		Pagination:   toolutil.PaginationFromResponse(resp),
	}
	for _, f := range flags {
		out.FeatureFlags = append(out.FeatureFlags, convertFeatureFlag(f))
	}
	return out, nil
}

// GetFeatureFlag gets a single feature flag by name.
func GetFeatureFlag(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_get", toolutil.ErrFieldRequired("name"))
	}
	flag, _, err := client.GL().ProjectFeatureFlags.GetProjectFeatureFlag(
		string(input.ProjectID), input.Name, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("feature_flag_get", err, http.StatusNotFound,
			"verify the flag name with gitlab_feature_flag_list \u2014 names are case-sensitive")
	}
	return convertFeatureFlag(flag), nil
}

// CreateFeatureFlag creates a new feature flag.
func CreateFeatureFlag(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_create", toolutil.ErrFieldRequired("name"))
	}
	opts := &gl.CreateProjectFeatureFlagOptions{
		Name: new(input.Name),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Version != "" {
		opts.Version = new(input.Version)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Strategies != "" {
		strategies, err := parseStrategyInputs(input.Strategies)
		if err != nil {
			return Output{}, toolutil.WrapErrWithMessage("feature_flag_create", fmt.Errorf("invalid strategies JSON: %w", err))
		}
		opts.Strategies = toStrategyOptions(strategies)
	}
	flag, _, err := client.GL().ProjectFeatureFlags.CreateProjectFeatureFlag(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("feature_flag_create", err,
				"creating feature flags requires GitLab Premium/Ultimate and Developer+ role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("feature_flag_create", err,
				"name may already exist or strategies JSON is malformed \u2014 valid strategy names: 'default', 'gradualRolloutUserId', 'userWithId', 'gitlabUserList', 'flexibleRollout'")
		}
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_create", err)
	}
	return convertFeatureFlag(flag), nil
}

// UpdateFeatureFlag updates an existing feature flag.
func UpdateFeatureFlag(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return Output{}, toolutil.WrapErrWithMessage("feature_flag_update", toolutil.ErrFieldRequired("name"))
	}
	opts := &gl.UpdateProjectFeatureFlagOptions{}
	if input.NewName != "" {
		opts.Name = new(input.NewName)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Strategies != "" {
		strategies, err := parseStrategyInputs(input.Strategies)
		if err != nil {
			return Output{}, toolutil.WrapErrWithMessage("feature_flag_update", fmt.Errorf("invalid strategies JSON: %w", err))
		}
		opts.Strategies = toStrategyOptions(strategies)
	}
	flag, _, err := client.GL().ProjectFeatureFlags.UpdateProjectFeatureFlag(
		string(input.ProjectID), input.Name, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("feature_flag_update", err,
				"updating feature flags requires Developer+ role on a Premium/Ultimate project")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("feature_flag_update", err, http.StatusNotFound,
			"verify the flag name with gitlab_feature_flag_list \u2014 names are case-sensitive")
	}
	return convertFeatureFlag(flag), nil
}

// DeleteFeatureFlag deletes a feature flag.
func DeleteFeatureFlag(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("feature_flag_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return toolutil.WrapErrWithMessage("feature_flag_delete", toolutil.ErrFieldRequired("name"))
	}
	_, err := client.GL().ProjectFeatureFlags.DeleteProjectFeatureFlag(
		string(input.ProjectID), input.Name, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("feature_flag_delete", err,
				"deleting feature flags requires Maintainer+ role on a Premium/Ultimate project")
		}
		return toolutil.WrapErrWithStatusHint("feature_flag_delete", err, http.StatusNotFound,
			"verify the flag name with gitlab_feature_flag_list")
	}
	return nil
}

// ──────────────────────────────────────────────
// Converters
// ──────────────────────────────────────────────.

// convertFeatureFlag is an internal helper for the featureflags package.
func convertFeatureFlag(f *gl.ProjectFeatureFlag) Output {
	out := Output{
		Name:        f.Name,
		Description: f.Description,
		Active:      f.Active,
		Version:     f.Version,
	}
	if f.CreatedAt != nil {
		out.CreatedAt = f.CreatedAt.Format(time.RFC3339)
	}
	if f.UpdatedAt != nil {
		out.UpdatedAt = f.UpdatedAt.Format(time.RFC3339)
	}
	for _, s := range f.Strategies {
		out.Strategies = append(out.Strategies, convertStrategy(s))
	}
	return out
}

// convertStrategy is an internal helper for the featureflags package.
func convertStrategy(s *gl.ProjectFeatureFlagStrategy) StrategyOutput {
	out := StrategyOutput{
		ID:   s.ID,
		Name: s.Name,
	}
	if s.Parameters != nil {
		out.Parameters = &StrategyParameterOutput{
			GroupID:    s.Parameters.GroupID,
			UserIDs:    s.Parameters.UserIDs,
			Percentage: s.Parameters.Percentage,
			Rollout:    s.Parameters.Rollout,
			Stickiness: s.Parameters.Stickiness,
		}
	}
	for _, sc := range s.Scopes {
		out.Scopes = append(out.Scopes, ScopeOutput{
			ID:               sc.ID,
			EnvironmentScope: sc.EnvironmentScope,
		})
	}
	return out
}

// ──────────────────────────────────────────────
// Strategy parsing helpers
// ──────────────────────────────────────────────.

// parseStrategyInputs performs the parse strategy inputs operation using the GitLab API and returns [[]StrategyInput].
func parseStrategyInputs(jsonStr string) ([]StrategyInput, error) {
	var strategies []StrategyInput
	if err := json.Unmarshal([]byte(jsonStr), &strategies); err != nil {
		return nil, err
	}
	return strategies, nil
}

// toStrategyOptions converts the GitLab API response to the tool output format.
func toStrategyOptions(strategies []StrategyInput) *[]*gl.FeatureFlagStrategyOptions {
	opts := make([]*gl.FeatureFlagStrategyOptions, 0, len(strategies))
	for _, s := range strategies {
		o := &gl.FeatureFlagStrategyOptions{
			Name: new(s.Name),
		}
		if s.ID != 0 {
			o.ID = new(s.ID)
		}
		if s.Parameters != nil {
			o.Parameters = &gl.ProjectFeatureFlagStrategyParameter{
				GroupID:    s.Parameters.GroupID,
				UserIDs:    s.Parameters.UserIDs,
				Percentage: s.Parameters.Percentage,
				Rollout:    s.Parameters.Rollout,
				Stickiness: s.Parameters.Stickiness,
			}
		}
		if len(s.Scopes) > 0 {
			scopes := make([]*gl.ProjectFeatureFlagScope, 0, len(s.Scopes))
			for _, sc := range s.Scopes {
				scopes = append(scopes, &gl.ProjectFeatureFlagScope{
					EnvironmentScope: sc.EnvironmentScope,
				})
			}
			o.Scopes = &scopes
		}
		opts = append(opts, o)
	}
	return &opts
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.

// formatParameters renders the result as a formatted string.
func formatParameters(p *StrategyParameterOutput) string {
	if p == nil {
		return "-"
	}
	var parts []string
	if p.Percentage != "" {
		parts = append(parts, "percentage="+p.Percentage)
	}
	if p.GroupID != "" {
		parts = append(parts, "groupId="+p.GroupID)
	}
	if p.UserIDs != "" {
		parts = append(parts, "userIds="+p.UserIDs)
	}
	if p.Rollout != "" {
		parts = append(parts, "rollout="+p.Rollout)
	}
	if p.Stickiness != "" {
		parts = append(parts, "stickiness="+p.Stickiness)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// formatScopes renders the result as a formatted string.
func formatScopes(scopes []ScopeOutput) string {
	if len(scopes) == 0 {
		return "-"
	}
	var parts []string
	for _, s := range scopes {
		parts = append(parts, s.EnvironmentScope)
	}
	return strings.Join(parts, ", ")
}
