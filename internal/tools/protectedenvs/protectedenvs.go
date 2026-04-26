// Package protectedenvs implements GitLab protected environment operations
// including list, get, protect, update, and unprotect.
package protectedenvs

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------- Input types ----------.

// ListInput holds parameters for listing protected environments.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput holds parameters for retrieving a single protected environment.
type GetInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	Environment string               `json:"environment"  jsonschema:"Environment name,required"`
}

// ProtectInput holds parameters for protecting a repository environment.
type ProtectInput struct {
	ProjectID             toolutil.StringOrInt     `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	Name                  string                   `json:"name"                              jsonschema:"Environment name to protect,required"`
	DeployAccessLevels    []DeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Deploy access levels"`
	RequiredApprovalCount *int64                   `json:"required_approval_count,omitempty" jsonschema:"Required number of approvals"`
	ApprovalRules         []ApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Approval rules"`
}

// DeployAccessLevelInput represents an access level for deployment.
type DeployAccessLevelInput struct {
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Access level (0=No access, 30=Developer, 40=Maintainer, 60=Admin)"`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type (0=direct, 1=inherited)"`
}

// ApprovalRuleInput represents an approval rule for an environment.
type ApprovalRuleInput struct {
	UserID                *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID               *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	AccessLevel           *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"     jsonschema:"Required number of approvals"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type (0=direct, 1=inherited)"`
}

// UpdateInput holds parameters for updating a protected environment.
type UpdateInput struct {
	ProjectID             toolutil.StringOrInt           `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	Environment           string                         `json:"environment"                       jsonschema:"Environment name,required"`
	Name                  string                         `json:"name,omitempty"                    jsonschema:"New environment name"`
	DeployAccessLevels    []UpdateDeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Updated deploy access levels"`
	RequiredApprovalCount *int64                         `json:"required_approval_count,omitempty" jsonschema:"Required number of approvals"`
	ApprovalRules         []UpdateApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Updated approval rules"`
}

// UpdateDeployAccessLevelInput represents an updated access level for deployment.
type UpdateDeployAccessLevelInput struct {
	ID                   *int64 `json:"id,omitempty"                     jsonschema:"Existing access level ID to update"`
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type"`
	Destroy              *bool  `json:"_destroy,omitempty"               jsonschema:"Set true to remove this access level"`
}

// UpdateApprovalRuleInput represents an updated approval rule for an environment.
type UpdateApprovalRuleInput struct {
	ID                    *int64 `json:"id,omitempty"                     jsonschema:"Existing approval rule ID to update"`
	UserID                *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID               *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	AccessLevel           *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"     jsonschema:"Required number of approvals"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type"`
	Destroy               *bool  `json:"_destroy,omitempty"               jsonschema:"Set true to remove this rule"`
}

// UnprotectInput holds parameters for unprotecting an environment.
type UnprotectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	Environment string               `json:"environment"  jsonschema:"Environment name to unprotect,required"`
}

// ---------- Output types ----------.

// AccessLevelOutput represents an access level on a protected environment.
type AccessLevelOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}

// ApprovalRuleOutput represents an approval rule on a protected environment.
type ApprovalRuleOutput struct {
	ID                     int64  `json:"id"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	RequiredApprovalCount  int64  `json:"required_approvals"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}

// Output represents a single protected environment.
type Output struct {
	toolutil.HintableOutput
	Name                  string               `json:"name"`
	DeployAccessLevels    []AccessLevelOutput  `json:"deploy_access_levels"`
	RequiredApprovalCount int64                `json:"required_approval_count"`
	ApprovalRules         []ApprovalRuleOutput `json:"approval_rules"`
}

// ListOutput represents a paginated list of protected environments.
type ListOutput struct {
	toolutil.HintableOutput
	Environments []Output                  `json:"environments"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// ---------- Converters ----------.

// toAccessLevelOutput converts the GitLab API response to the tool output format.
func toAccessLevelOutput(a *gl.EnvironmentAccessDescription) AccessLevelOutput {
	return AccessLevelOutput{
		ID:                     a.ID,
		AccessLevel:            int(a.AccessLevel),
		AccessLevelDescription: a.AccessLevelDescription,
		UserID:                 a.UserID,
		GroupID:                a.GroupID,
		GroupInheritanceType:   a.GroupInheritanceType,
	}
}

// toApprovalRuleOutput converts the GitLab API response to the tool output format.
func toApprovalRuleOutput(r *gl.EnvironmentApprovalRule) ApprovalRuleOutput {
	return ApprovalRuleOutput{
		ID:                     r.ID,
		UserID:                 r.UserID,
		GroupID:                r.GroupID,
		AccessLevel:            int(r.AccessLevel),
		AccessLevelDescription: r.AccessLevelDescription,
		RequiredApprovalCount:  r.RequiredApprovalCount,
		GroupInheritanceType:   r.GroupInheritanceType,
	}
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(pe *gl.ProtectedEnvironment) Output {
	out := Output{
		Name:                  pe.Name,
		RequiredApprovalCount: pe.RequiredApprovalCount,
	}
	out.DeployAccessLevels = make([]AccessLevelOutput, 0, len(pe.DeployAccessLevels))
	for _, a := range pe.DeployAccessLevels {
		out.DeployAccessLevels = append(out.DeployAccessLevels, toAccessLevelOutput(a))
	}
	out.ApprovalRules = make([]ApprovalRuleOutput, 0, len(pe.ApprovalRules))
	for _, r := range pe.ApprovalRules {
		out.ApprovalRules = append(out.ApprovalRules, toApprovalRuleOutput(r))
	}
	return out
}

// ---------- Handlers ----------.

// List retrieves a paginated list of protected environments for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list protected environments", err)
	}

	opts := &gl.ListProtectedEnvironmentsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	envs, resp, err := client.GL().ProtectedEnvironments.ListProtectedEnvironments(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list protected environments", err, http.StatusNotFound,
			"verify project_id; protected environments require GitLab Premium or Ultimate")
	}

	out := ListOutput{Environments: make([]Output, 0, len(envs))}
	for _, pe := range envs {
		out.Environments = append(out.Environments, toOutput(pe))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single protected environment by name.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get protected environment", err)
	}

	pe, _, err := client.GL().ProtectedEnvironments.GetProtectedEnvironment(string(input.ProjectID), input.Environment, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get protected environment", err, http.StatusNotFound,
			"the environment may not be protected \u2014 use gitlab_list_protected_environments first")
	}
	return toOutput(pe), nil
}

// Protect creates a new protected environment.
func Protect(ctx context.Context, client *gitlabclient.Client, input ProtectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("protect environment", err)
	}

	opts := &gl.ProtectRepositoryEnvironmentsOptions{
		Name: &input.Name,
	}
	if input.RequiredApprovalCount != nil {
		opts.RequiredApprovalCount = input.RequiredApprovalCount
	}
	if len(input.DeployAccessLevels) > 0 {
		levels := make([]*gl.EnvironmentAccessOptions, 0, len(input.DeployAccessLevels))
		for _, d := range input.DeployAccessLevels {
			eao := &gl.EnvironmentAccessOptions{}
			if d.AccessLevel != nil {
				alv := gl.AccessLevelValue(int64(*d.AccessLevel))
				eao.AccessLevel = &alv
			}
			if d.UserID != nil {
				eao.UserID = d.UserID
			}
			if d.GroupID != nil {
				eao.GroupID = d.GroupID
			}
			if d.GroupInheritanceType != nil {
				eao.GroupInheritanceType = d.GroupInheritanceType
			}
			levels = append(levels, eao)
		}
		opts.DeployAccessLevels = &levels
	}
	if len(input.ApprovalRules) > 0 {
		rules := make([]*gl.EnvironmentApprovalRuleOptions, 0, len(input.ApprovalRules))
		for _, r := range input.ApprovalRules {
			aro := &gl.EnvironmentApprovalRuleOptions{}
			if r.AccessLevel != nil {
				alv := gl.AccessLevelValue(int64(*r.AccessLevel))
				aro.AccessLevel = &alv
			}
			if r.UserID != nil {
				aro.UserID = r.UserID
			}
			if r.GroupID != nil {
				aro.GroupID = r.GroupID
			}
			if r.RequiredApprovalCount != nil {
				aro.RequiredApprovalCount = r.RequiredApprovalCount
			}
			if r.GroupInheritanceType != nil {
				aro.GroupInheritanceType = r.GroupInheritanceType
			}
			rules = append(rules, aro)
		}
		opts.ApprovalRules = &rules
	}

	pe, _, err := client.GL().ProtectedEnvironments.ProtectRepositoryEnvironments(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("protect environment", err,
				"protecting environments requires Maintainer role and GitLab Premium/Ultimate")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("protect environment", err, http.StatusConflict,
			"the environment is already protected \u2014 use gitlab_update_protected_environment to modify access levels")
	}
	return toOutput(pe), nil
}

// Update modifies an existing protected environment.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("update protected environment", err)
	}

	opts := &gl.UpdateProtectedEnvironmentsOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.RequiredApprovalCount != nil {
		opts.RequiredApprovalCount = input.RequiredApprovalCount
	}
	if len(input.DeployAccessLevels) > 0 {
		levels := make([]*gl.UpdateEnvironmentAccessOptions, 0, len(input.DeployAccessLevels))
		for _, d := range input.DeployAccessLevels {
			eao := &gl.UpdateEnvironmentAccessOptions{}
			if d.ID != nil {
				eao.ID = d.ID
			}
			if d.AccessLevel != nil {
				alv := gl.AccessLevelValue(int64(*d.AccessLevel))
				eao.AccessLevel = &alv
			}
			if d.UserID != nil {
				eao.UserID = d.UserID
			}
			if d.GroupID != nil {
				eao.GroupID = d.GroupID
			}
			if d.GroupInheritanceType != nil {
				eao.GroupInheritanceType = d.GroupInheritanceType
			}
			if d.Destroy != nil {
				eao.Destroy = d.Destroy
			}
			levels = append(levels, eao)
		}
		opts.DeployAccessLevels = &levels
	}
	if len(input.ApprovalRules) > 0 {
		rules := make([]*gl.UpdateEnvironmentApprovalRuleOptions, 0, len(input.ApprovalRules))
		for _, r := range input.ApprovalRules {
			aro := &gl.UpdateEnvironmentApprovalRuleOptions{}
			if r.ID != nil {
				aro.ID = r.ID
			}
			if r.AccessLevel != nil {
				alv := gl.AccessLevelValue(int64(*r.AccessLevel))
				aro.AccessLevel = &alv
			}
			if r.UserID != nil {
				aro.UserID = r.UserID
			}
			if r.GroupID != nil {
				aro.GroupID = r.GroupID
			}
			if r.RequiredApprovalCount != nil {
				aro.RequiredApprovalCount = r.RequiredApprovalCount
			}
			if r.GroupInheritanceType != nil {
				aro.GroupInheritanceType = r.GroupInheritanceType
			}
			if r.Destroy != nil {
				aro.Destroy = r.Destroy
			}
			rules = append(rules, aro)
		}
		opts.ApprovalRules = &rules
	}

	pe, _, err := client.GL().ProtectedEnvironments.UpdateProtectedEnvironments(string(input.ProjectID), input.Environment, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("update protected environment", err,
				"updating protected environments requires Maintainer role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("update protected environment", err, http.StatusNotFound,
			"the environment may not be protected \u2014 use gitlab_protect_environment first")
	}
	return toOutput(pe), nil
}

// Unprotect removes protection from an environment.
func Unprotect(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.Environment == "" {
		return toolutil.ErrFieldRequired("environment")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage("unprotect environment", err)
	}

	_, err := client.GL().ProtectedEnvironments.UnprotectEnvironment(string(input.ProjectID), input.Environment, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("unprotect environment", err,
				"unprotecting environments requires Maintainer role")
		}
		return toolutil.WrapErrWithStatusHint("unprotect environment", err, http.StatusNotFound,
			"the environment may already be unprotected \u2014 use gitlab_list_protected_environments to verify")
	}
	return nil
}

// ---------- Formatters ----------.
