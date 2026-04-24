// Package groupprotectedenvs implements MCP tool handlers for GitLab
// group-level protected environment operations.
package groupprotectedenvs

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AccessLevelOutput represents a deploy access level on a group protected environment.
type AccessLevelOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}

// ApprovalRuleOutput represents an approval rule on a group protected environment.
type ApprovalRuleOutput struct {
	ID                     int64  `json:"id"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	RequiredApprovalCount  int64  `json:"required_approvals"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}

// Output represents a single group-level protected environment.
type Output struct {
	toolutil.HintableOutput
	Name                  string               `json:"name"`
	DeployAccessLevels    []AccessLevelOutput  `json:"deploy_access_levels"`
	RequiredApprovalCount int64                `json:"required_approval_count"`
	ApprovalRules         []ApprovalRuleOutput `json:"approval_rules"`
}

// ListOutput holds a paginated list of group protected environments.
type ListOutput struct {
	toolutil.HintableOutput
	Environments []Output                  `json:"environments"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

func toAccessLevels(src []*gl.GroupEnvironmentAccessDescription) []AccessLevelOutput {
	out := make([]AccessLevelOutput, len(src))
	for i, a := range src {
		out[i] = AccessLevelOutput{
			ID:                     a.ID,
			AccessLevel:            int(a.AccessLevel),
			AccessLevelDescription: a.AccessLevelDescription,
			UserID:                 a.UserID,
			GroupID:                a.GroupID,
			GroupInheritanceType:   a.GroupInheritanceType,
		}
	}
	return out
}

func toApprovalRules(src []*gl.GroupEnvironmentApprovalRule) []ApprovalRuleOutput {
	out := make([]ApprovalRuleOutput, len(src))
	for i, r := range src {
		out[i] = ApprovalRuleOutput{
			ID:                     r.ID,
			UserID:                 r.UserID,
			GroupID:                r.GroupID,
			AccessLevel:            int(r.AccessLevel),
			AccessLevelDescription: r.AccessLevelDescription,
			RequiredApprovalCount:  r.RequiredApprovalCount,
			GroupInheritanceType:   r.GroupInheritanceType,
		}
	}
	return out
}

func toOutput(e *gl.GroupProtectedEnvironment) Output {
	return Output{
		Name:                  e.Name,
		DeployAccessLevels:    toAccessLevels(e.DeployAccessLevels),
		RequiredApprovalCount: e.RequiredApprovalCount,
		ApprovalRules:         toApprovalRules(e.ApprovalRules),
	}
}

// DeployAccessLevelInput represents an access level for deployment.
type DeployAccessLevelInput struct {
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Access level (0=No access, 30=Developer, 40=Maintainer, 60=Admin)"`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type (0=direct, 1=inherited)"`
}

// ApprovalRuleInput represents an approval rule input.
type ApprovalRuleInput struct {
	UserID                *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID               *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	AccessLevel           *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"     jsonschema:"Required number of approvals"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type (0=direct, 1=inherited)"`
}

// UpdateDeployAccessLevelInput represents an updated deploy access level.
type UpdateDeployAccessLevelInput struct {
	ID                   *int64 `json:"id,omitempty"                     jsonschema:"Existing access level ID to update"`
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type"`
	Destroy              *bool  `json:"_destroy,omitempty"               jsonschema:"Set true to remove this access level"`
}

// UpdateApprovalRuleInput represents an updated approval rule.
type UpdateApprovalRuleInput struct {
	ID                    *int64 `json:"id,omitempty"                     jsonschema:"Existing approval rule ID to update"`
	UserID                *int64 `json:"user_id,omitempty"                jsonschema:"User ID"`
	GroupID               *int64 `json:"group_id,omitempty"               jsonschema:"Group ID"`
	AccessLevel           *int   `json:"access_level,omitempty"           jsonschema:"Access level"`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"     jsonschema:"Required number of approvals"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type"`
	Destroy               *bool  `json:"_destroy,omitempty"               jsonschema:"Set true to remove this rule"`
}

// ListInput defines parameters for the List action which retrieves group-level protected environments.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput defines parameters for the Get action which retrieves a single group-level protected environment.
type GetInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	Environment string               `json:"environment" jsonschema:"Environment name,required"`
}

// ProtectInput defines parameters for the Protect action which creates a group-level protected environment.
type ProtectInput struct {
	GroupID               toolutil.StringOrInt     `json:"group_id"                          jsonschema:"Group ID or URL-encoded path,required"`
	Name                  string                   `json:"name"                              jsonschema:"Environment name to protect,required"`
	DeployAccessLevels    []DeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Deploy access levels"`
	RequiredApprovalCount *int64                   `json:"required_approval_count,omitempty" jsonschema:"Required number of approvals"`
	ApprovalRules         []ApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Approval rules"`
}

// UpdateInput defines parameters for the Update action which modifies a group-level protected environment.
type UpdateInput struct {
	GroupID               toolutil.StringOrInt           `json:"group_id"                          jsonschema:"Group ID or URL-encoded path,required"`
	Environment           string                         `json:"environment"                       jsonschema:"Environment name,required"`
	Name                  string                         `json:"name,omitempty"                    jsonschema:"New environment name"`
	DeployAccessLevels    []UpdateDeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Updated deploy access levels"`
	RequiredApprovalCount *int64                         `json:"required_approval_count,omitempty" jsonschema:"Required number of approvals"`
	ApprovalRules         []UpdateApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Updated approval rules"`
}

// UnprotectInput defines parameters for the Unprotect action which removes a group-level protected environment.
type UnprotectInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	Environment string               `json:"environment" jsonschema:"Environment name to unprotect,required"`
}

func toDeployAccessOpts(input []DeployAccessLevelInput) *[]*gl.GroupEnvironmentAccessOptions {
	if len(input) == 0 {
		return nil
	}
	opts := make([]*gl.GroupEnvironmentAccessOptions, len(input))
	for i, d := range input {
		o := &gl.GroupEnvironmentAccessOptions{
			UserID:               d.UserID,
			GroupID:              d.GroupID,
			GroupInheritanceType: d.GroupInheritanceType,
		}
		if d.AccessLevel != nil {
			v := gl.AccessLevelValue(*d.AccessLevel)
			o.AccessLevel = &v
		}
		opts[i] = o
	}
	return &opts
}

func toApprovalRuleOpts(input []ApprovalRuleInput) *[]*gl.GroupEnvironmentApprovalRuleOptions {
	if len(input) == 0 {
		return nil
	}
	opts := make([]*gl.GroupEnvironmentApprovalRuleOptions, len(input))
	for i, r := range input {
		o := &gl.GroupEnvironmentApprovalRuleOptions{
			UserID:                r.UserID,
			GroupID:               r.GroupID,
			RequiredApprovalCount: r.RequiredApprovalCount,
			GroupInheritanceType:  r.GroupInheritanceType,
		}
		if r.AccessLevel != nil {
			v := gl.AccessLevelValue(*r.AccessLevel)
			o.AccessLevel = &v
		}
		opts[i] = o
	}
	return &opts
}

func toUpdateDeployAccessOpts(input []UpdateDeployAccessLevelInput) *[]*gl.UpdateGroupEnvironmentAccessOptions {
	if len(input) == 0 {
		return nil
	}
	opts := make([]*gl.UpdateGroupEnvironmentAccessOptions, len(input))
	for i, d := range input {
		o := &gl.UpdateGroupEnvironmentAccessOptions{
			ID:                   d.ID,
			UserID:               d.UserID,
			GroupID:              d.GroupID,
			GroupInheritanceType: d.GroupInheritanceType,
			Destroy:              d.Destroy,
		}
		if d.AccessLevel != nil {
			v := gl.AccessLevelValue(*d.AccessLevel)
			o.AccessLevel = &v
		}
		opts[i] = o
	}
	return &opts
}

func toUpdateApprovalRuleOpts(input []UpdateApprovalRuleInput) *[]*gl.UpdateGroupEnvironmentApprovalRuleOptions {
	if len(input) == 0 {
		return nil
	}
	opts := make([]*gl.UpdateGroupEnvironmentApprovalRuleOptions, len(input))
	for i, r := range input {
		o := &gl.UpdateGroupEnvironmentApprovalRuleOptions{
			ID:                    r.ID,
			UserID:                r.UserID,
			GroupID:               r.GroupID,
			RequiredApprovalCount: r.RequiredApprovalCount,
			GroupInheritanceType:  r.GroupInheritanceType,
			Destroy:               r.Destroy,
		}
		if r.AccessLevel != nil {
			v := gl.AccessLevelValue(*r.AccessLevel)
			o.AccessLevel = &v
		}
		opts[i] = o
	}
	return &opts
}

// List retrieves all group-level protected environments.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupProtectedEnvironmentsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	envs, resp, err := client.GL().GroupProtectedEnvironments.ListGroupProtectedEnvironments(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("listGroupProtectedEnvironments", err)
	}
	out := make([]Output, len(envs))
	for i, e := range envs {
		out[i] = toOutput(e)
	}
	return ListOutput{
		Environments: out,
		Pagination:   toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get retrieves a single group-level protected environment.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	e, _, err := client.GL().GroupProtectedEnvironments.GetGroupProtectedEnvironment(string(input.GroupID), input.Environment, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("getGroupProtectedEnvironment", err)
	}
	return toOutput(e), nil
}

// Protect creates a new group-level protected environment.
func Protect(ctx context.Context, client *gitlabclient.Client, input ProtectInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	opts := &gl.ProtectGroupEnvironmentOptions{
		Name:                  new(input.Name),
		DeployAccessLevels:    toDeployAccessOpts(input.DeployAccessLevels),
		RequiredApprovalCount: input.RequiredApprovalCount,
		ApprovalRules:         toApprovalRuleOpts(input.ApprovalRules),
	}
	e, _, err := client.GL().GroupProtectedEnvironments.ProtectGroupEnvironment(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("protectGroupEnvironment", err)
	}
	return toOutput(e), nil
}

// Update modifies a group-level protected environment.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	opts := &gl.UpdateGroupProtectedEnvironmentOptions{
		DeployAccessLevels:    toUpdateDeployAccessOpts(input.DeployAccessLevels),
		RequiredApprovalCount: input.RequiredApprovalCount,
		ApprovalRules:         toUpdateApprovalRuleOpts(input.ApprovalRules),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	e, _, err := client.GL().GroupProtectedEnvironments.UpdateGroupProtectedEnvironment(string(input.GroupID), input.Environment, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("updateGroupProtectedEnvironment", err)
	}
	return toOutput(e), nil
}

// Unprotect removes a group-level protected environment.
func Unprotect(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.Environment == "" {
		return toolutil.ErrFieldRequired("environment")
	}
	_, err := client.GL().GroupProtectedEnvironments.UnprotectGroupEnvironment(string(input.GroupID), input.Environment, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("unprotectGroupEnvironment", err)
	}
	return nil
}
