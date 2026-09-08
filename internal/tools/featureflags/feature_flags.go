package featureflags

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// StrategyUserListOutput represents the user list a strategy targets, returned
// for strategies of the gitlabUserList type.
type StrategyUserListOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	UserXIDs  string `json:"user_xids"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// StrategyOutput represents a feature flag strategy.
type StrategyOutput struct {
	ID         int64                    `json:"id"`
	Name       string                   `json:"name"`
	Parameters *StrategyParameterOutput `json:"parameters,omitempty"`
	UserList   *StrategyUserListOutput  `json:"user_list,omitempty"`
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
	Scopes      []ScopeOutput    `json:"scopes,omitempty"`
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

// CreateScopeInput represents a scope of a strategy being created. Creation
// carries no scope identity: GitLab documents environment_scope alone for
// POST, and id and _destroy only for PUT.
type CreateScopeInput struct {
	EnvironmentScope string `json:"environment_scope" jsonschema:"Environment scope this strategy applies to (e.g. production, staging). Omit for the default * scope"`
}

// UpdateScopeInput represents a scope of a strategy being updated, where a
// scope already on the flag can be addressed by id and removed with _destroy.
type UpdateScopeInput struct {
	ID               *int64 `json:"id,omitempty" jsonschema:"Environment scope ID, to update or remove a scope already on the strategy. Read it from the scopes of a prior feature flag get or list"`
	EnvironmentScope string `json:"environment_scope,omitempty" jsonschema:"Environment scope this strategy applies to (e.g. production, staging). Omit for the default * scope"`
	Destroy          *bool  `json:"_destroy,omitempty" jsonschema:"Set true together with id to delete this scope from the strategy"`
}

// StrategyParameterInput represents strategy parameters for create/update.
type StrategyParameterInput struct {
	GroupID    string `json:"group_id,omitempty" jsonschema:"Group ID for the gradualRolloutUserId strategy"`
	UserIDs    string `json:"user_ids,omitempty" jsonschema:"Comma-separated user IDs for the userWithId strategy"`
	Percentage string `json:"percentage,omitempty" jsonschema:"Percentage of users to include for the gradualRolloutUserId strategy (e.g. 25)"`
	Rollout    string `json:"rollout,omitempty" jsonschema:"Rollout duration (0-100) for the gradualRolloutUserId strategy"`
	Stickiness string `json:"stickiness,omitempty" jsonschema:"Stickiness attribute used to bucket users for the flexibleRollout strategy: default, userId, sessionId, or random"`
}

// CreateStrategyInput represents a strategy of a flag being created. A
// strategy that does not exist yet has no id to address and nothing to
// remove, which is why GitLab documents neither for POST.
type CreateStrategyInput struct {
	Name       string                  `json:"name" jsonschema:"Strategy name (e.g. default, gradualRolloutUserId, userWithId, flexibleRollout),required"`
	Parameters *StrategyParameterInput `json:"parameters,omitempty" jsonschema:"Strategy-specific parameters (group_id, user_ids, percentage, rollout, stickiness)"`
	UserListID *int64                  `json:"user_list_id,omitempty" jsonschema:"ID of the feature flag user list this strategy targets (for gitlabUserList strategies). Use gitlab_ff_user_list_list to find it"`
	Scopes     []CreateScopeInput      `json:"scopes,omitempty" jsonschema:"Environment scopes to which this strategy applies"`
}

// UpdateStrategyInput represents a strategy of a flag being updated, where a
// strategy already on the flag can be addressed by id and removed with
// _destroy.
type UpdateStrategyInput struct {
	ID         int64                   `json:"id,omitempty" jsonschema:"Strategy ID, to update or remove a strategy already on the flag. Read it from the strategies of a prior feature flag get or list"`
	Name       string                  `json:"name,omitempty" jsonschema:"Strategy name (e.g. default, gradualRolloutUserId, userWithId, flexibleRollout). Required except when removing a strategy with _destroy"`
	Parameters *StrategyParameterInput `json:"parameters,omitempty" jsonschema:"Strategy-specific parameters (group_id, user_ids, percentage, rollout, stickiness)"`
	UserListID *int64                  `json:"user_list_id,omitempty" jsonschema:"ID of the feature flag user list this strategy targets (for gitlabUserList strategies). Use gitlab_ff_user_list_list to find it"`
	Destroy    *bool                   `json:"_destroy,omitempty" jsonschema:"Set true together with id (and no name) to delete this strategy from the flag"`
	Scopes     []UpdateScopeInput      `json:"scopes,omitempty" jsonschema:"Environment scopes to which this strategy applies"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// ListInput contains parameters for listing feature flags.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Scope     string               `json:"scope,omitempty" jsonschema:"Filter by scope (enabled or disabled)"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. name, created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput contains parameters for getting a feature flag.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name      string               `json:"name" jsonschema:"Feature flag name,required"`
}

// CreateInput contains parameters for creating a feature flag.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt  `json:"project_id" jsonschema:"Project ID or path,required"`
	Name        string                `json:"name" jsonschema:"Feature flag name,required"`
	Description string                `json:"description,omitempty" jsonschema:"Feature flag description"`
	Version     string                `json:"version,omitempty" jsonschema:"Version of the feature flag (new_version_flag)"`
	Active      *bool                 `json:"active,omitempty" jsonschema:"Whether the flag is active"`
	Strategies  []CreateStrategyInput `json:"strategies,omitempty" jsonschema:"Activation strategies for the flag. Each has a name, optional parameters, and optional environment scopes"`
}

// UpdateInput contains parameters for updating a feature flag.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt  `json:"project_id" jsonschema:"Project ID or path,required"`
	Name        string                `json:"name" jsonschema:"Current feature flag name,required"`
	NewName     string                `json:"new_name,omitempty" jsonschema:"New feature flag name"`
	Description string                `json:"description,omitempty" jsonschema:"Feature flag description"`
	Active      *bool                 `json:"active,omitempty" jsonschema:"Whether the flag is active"`
	Strategies  []UpdateStrategyInput `json:"strategies,omitempty" jsonschema:"Activation strategies for the flag. Each has an optional id (to update an existing strategy), a name, optional parameters, and optional environment scopes carrying their own id and _destroy"`
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
	opts := &gl.ListProjectFeatureFlagOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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
				"feature flags require GitLab Premium/Ultimate. Verify the project's tier and that you have Developer+ role")
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
			"verify the flag name with gitlab_feature_flag_list. Names are case-sensitive")
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
	if len(input.Strategies) > 0 {
		if err := validateCreateStrategies(input.Strategies); err != nil {
			return Output{}, err
		}
		opts.Strategies = toCreateStrategyOptions(input.Strategies)
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
				"name may already exist or a strategy is invalid. Valid strategy names: 'default', 'gradualRolloutUserId', 'userWithId', 'gitlabUserList', 'flexibleRollout'")
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
	if len(input.Strategies) > 0 {
		if err := validateUpdateStrategies(input.Strategies); err != nil {
			return Output{}, err
		}
		opts.Strategies = toUpdateStrategyOptions(input.Strategies)
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
			"verify the flag name with gitlab_feature_flag_list. Names are case-sensitive")
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

// convertFeatureFlag maps a GitLab feature flag into the MCP output shape.
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
	for _, sc := range f.Scopes {
		if sc == nil {
			continue
		}
		out.Scopes = append(out.Scopes, ScopeOutput{
			ID:               sc.ID,
			EnvironmentScope: sc.EnvironmentScope,
		})
	}
	for _, s := range f.Strategies {
		out.Strategies = append(out.Strategies, convertStrategy(s))
	}
	return out
}

// convertStrategy maps a GitLab feature flag strategy into MCP output.
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
	if s.UserList != nil {
		out.UserList = &StrategyUserListOutput{
			ID:        s.UserList.ID,
			IID:       s.UserList.IID,
			ProjectID: s.UserList.ProjectID,
			Name:      s.UserList.Name,
			UserXIDs:  s.UserList.UserXIDs,
			CreatedAt: toolutil.FormatTimePtr(s.UserList.CreatedAt),
			UpdatedAt: toolutil.FormatTimePtr(s.UserList.UpdatedAt),
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
// Strategy conversion helpers
// ──────────────────────────────────────────────.

// validateCreateStrategies rejects a strategy the create endpoint cannot act
// on. Creation names every strategy it adds, since there is nothing on the
// flag yet to address or remove.
func validateCreateStrategies(strategies []CreateStrategyInput) error {
	for i, s := range strategies {
		if s.Name == "" {
			return fmt.Errorf("strategies[%d]: name is required when creating a feature flag strategy", i)
		}
	}
	return nil
}

// validateUpdateStrategies rejects strategy entries the GitLab API cannot act
// on: a removal needs the id of the strategy to remove, and anything else
// needs a name. The same holds one level down for the scopes of a strategy.
// Catching it here turns an opaque API rejection into a usable message.
func validateUpdateStrategies(strategies []UpdateStrategyInput) error {
	for i, s := range strategies {
		destroying := s.Destroy != nil && *s.Destroy
		switch {
		case destroying && s.ID == 0:
			return fmt.Errorf("strategies[%d]: _destroy requires the id of the strategy to remove; read it from the strategies of a prior feature flag get or list", i)
		case !destroying && s.Name == "":
			return fmt.Errorf("strategies[%d]: name is required unless the entry removes a strategy with _destroy and id", i)
		}
		if err := validateUpdateScopes(i, s.Scopes); err != nil {
			return err
		}
	}
	return nil
}

// validateUpdateScopes holds a strategy's scopes to the same rule its
// strategies obey: a removal addresses what it removes.
func validateUpdateScopes(strategy int, scopes []UpdateScopeInput) error {
	for j, sc := range scopes {
		destroying := sc.Destroy != nil && *sc.Destroy
		switch {
		case destroying && (sc.ID == nil || *sc.ID == 0):
			return fmt.Errorf("strategies[%d].scopes[%d]: _destroy requires the id of the scope to remove; read it from the scopes of a prior feature flag get or list", strategy, j)
		case !destroying && sc.EnvironmentScope == "" && sc.ID == nil:
			return fmt.Errorf("strategies[%d].scopes[%d]: environment_scope is required unless the entry addresses a scope by id", strategy, j)
		}
	}
	return nil
}

// toCreateStrategyOptions converts typed strategy inputs into the client-go
// CreateFeatureFlagStrategyOptions shape the create request takes.
func toCreateStrategyOptions(strategies []CreateStrategyInput) *[]*gl.CreateFeatureFlagStrategyOptions {
	opts := make([]*gl.CreateFeatureFlagStrategyOptions, 0, len(strategies))
	for _, s := range strategies {
		o := &gl.CreateFeatureFlagStrategyOptions{
			Name:       new(s.Name),
			Parameters: toStrategyParameters(s.Parameters),
			UserListID: s.UserListID,
		}
		if len(s.Scopes) > 0 {
			scopes := make([]*gl.CreateProjectFeatureFlagScopeOptions, 0, len(s.Scopes))
			for _, sc := range s.Scopes {
				scopes = append(scopes, &gl.CreateProjectFeatureFlagScopeOptions{
					EnvironmentScope: new(sc.EnvironmentScope),
				})
			}
			o.Scopes = &scopes
		}
		opts = append(opts, o)
	}
	return &opts
}

// toUpdateStrategyOptions converts typed strategy inputs into the client-go
// UpdateFeatureFlagStrategyOptions shape the update request takes, which is
// the create shape plus the identity a strategy already on the flag has.
func toUpdateStrategyOptions(strategies []UpdateStrategyInput) *[]*gl.UpdateFeatureFlagStrategyOptions {
	opts := make([]*gl.UpdateFeatureFlagStrategyOptions, 0, len(strategies))
	for _, s := range strategies {
		o := &gl.UpdateFeatureFlagStrategyOptions{
			Parameters: toStrategyParameters(s.Parameters),
			UserListID: s.UserListID,
			Destroy:    s.Destroy,
		}
		// name is omittable, and a destroy-only entry has none: sending
		// "name":"" would fail the update schema instead of removing the
		// strategy.
		if s.Name != "" {
			o.Name = new(s.Name)
		}
		if s.ID != 0 {
			o.ID = new(s.ID)
		}
		if len(s.Scopes) > 0 {
			scopes := make([]*gl.UpdateProjectFeatureFlagScopeOptions, 0, len(s.Scopes))
			for _, sc := range s.Scopes {
				scope := &gl.UpdateProjectFeatureFlagScopeOptions{
					ID:      sc.ID,
					Destroy: sc.Destroy,
				}
				if sc.EnvironmentScope != "" {
					scope.EnvironmentScope = new(sc.EnvironmentScope)
				}
				scopes = append(scopes, scope)
			}
			o.Scopes = &scopes
		}
		opts = append(opts, o)
	}
	return &opts
}

// toStrategyParameters converts the typed parameters both requests share.
func toStrategyParameters(p *StrategyParameterInput) *gl.ProjectFeatureFlagStrategyParameter {
	if p == nil {
		return nil
	}
	return &gl.ProjectFeatureFlagStrategyParameter{
		GroupID:    p.GroupID,
		UserIDs:    p.UserIDs,
		Percentage: p.Percentage,
		Rollout:    p.Rollout,
		Stickiness: p.Stickiness,
	}
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
