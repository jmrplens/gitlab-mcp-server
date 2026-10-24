package protectedenvs

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Operation names used by error wrappers (kept as constants to satisfy S1192).
const (
	opProtectEnvironment   = "protect environment"
	opUpdateProtectedEnv   = "update protected environment"
	opUnprotectEnvironment = "unprotect environment"
)

// protectEntriesHint is what a protect GitLab refused with 422 is wrapped
// with. The handler refuses every entry GitLab's params and models refuse
// before anything is sent (toolutil.CheckEnvironmentProtect), so what is left
// is the entries GitLab keeps: ProtectedEnvironments::BaseService drops,
// without refusing it, an entry whose user has no Developer role or above on
// the project or whose group the project is not shared with, and the model
// then refuses a protect left with no deploy access level. It is one literal,
// since gremlins mutates the + of a concatenated constant and no coverage
// profile reaches a constant.
const protectEntriesHint = "a user_id must name a user with the Developer role or above on the project and a group_id a group the project is shared with: GitLab drops any other entry and refuses a protect left with no deploy access level"

// ---------- Input types ----------.

// ListInput holds parameters for listing protected environments. It mirrors
// gl.ListProtectedEnvironmentsOptions, which embeds gl.ListOptions, exposing
// keyset pagination (order_by, sort, pagination, page_token) in addition to
// offset pagination.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	DeployAccessLevels    []DeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Who may deploy: at least one entry, each naming exactly one of access_level, user_id or group_id,required"`
	RequiredApprovalCount *int64                   `json:"required_approval_count,omitempty" jsonschema:"Deprecated: GitLab refuses any value but 0. Require approvals with required_approvals on approval_rules instead."`
	ApprovalRules         []ApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Who must approve a deployment: each rule names at least one of access_level, user_id or group_id"`
}

// DeployAccessLevelInput represents an access level for deployment.
type DeployAccessLevelInput struct {
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Role allowed to deploy: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name exactly one of access_level, user_id or group_id."`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"ID of a user allowed to deploy. Name exactly one of access_level, user_id or group_id."`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"ID of a group whose members may deploy. Name exactly one of access_level, user_id or group_id."`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type for a group_id entry (0=direct members only, 1=inherited members too)"`
}

// EnvironmentAccess answers the entry as the checks of GitLab's rules read
// it.
func (d DeployAccessLevelInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{AccessLevel: d.AccessLevel, UserID: d.UserID, GroupID: d.GroupID, GroupInheritanceType: d.GroupInheritanceType}
}

// ApprovalRuleInput represents an approval rule for an environment. It mirrors
// gl.EnvironmentApprovalRuleOptions field for field but for
// access_level_description, which neither the protect nor the update route
// declares, so GitLab drops it (upstream register row 139).
type ApprovalRuleInput struct {
	UserID                *int64 `json:"user_id,omitempty"                  jsonschema:"ID of a user who may approve. Name at least one of access_level, user_id or group_id."`
	GroupID               *int64 `json:"group_id,omitempty"                 jsonschema:"ID of a group whose members may approve. Name at least one of access_level, user_id or group_id."`
	AccessLevel           *int   `json:"access_level,omitempty"             jsonschema:"Role whose members may approve: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at least one of access_level, user_id or group_id."`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"       jsonschema:"Approvals this rule requires, 1 to 5 (GitLab's default is 1)"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty"   jsonschema:"Group inheritance type for a group_id rule (0=direct members only, 1=inherited members too)"`
}

// EnvironmentAccess answers the rule as the checks of GitLab's rules read it.
func (r ApprovalRuleInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{AccessLevel: r.AccessLevel, UserID: r.UserID, GroupID: r.GroupID, GroupInheritanceType: r.GroupInheritanceType, RequiredApprovals: r.RequiredApprovalCount}
}

// UpdateInput holds parameters for updating a protected environment.
type UpdateInput struct {
	ProjectID             toolutil.StringOrInt           `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	Environment           string                         `json:"environment"                       jsonschema:"Environment name,required"`
	Name                  string                         `json:"name,omitempty"                    jsonschema:"New environment name"`
	DeployAccessLevels    []UpdateDeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Deploy access levels to add, change or remove: each entry names an existing one by id or exactly one of access_level, user_id or group_id, and never two of them"`
	RequiredApprovalCount *int64                         `json:"required_approval_count,omitempty" jsonschema:"Deprecated: GitLab refuses any value but 0. Require approvals with required_approvals on approval_rules instead."`
	ApprovalRules         []UpdateApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Approval rules to add, change or remove: unless _destroy is true, each rule names at least one of access_level, user_id or group_id"`
}

// UpdateDeployAccessLevelInput represents an updated access level for deployment.
type UpdateDeployAccessLevelInput struct {
	ID                   *int64 `json:"id,omitempty"                     jsonschema:"ID of the existing access level to change or remove. An entry without one adds an access level."`
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Role allowed to deploy: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at most one of access_level, user_id or group_id, and one of them without an id."`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"ID of a user allowed to deploy. Name at most one of access_level, user_id or group_id, and one of them without an id."`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"ID of a group whose members may deploy. Name at most one of access_level, user_id or group_id, and one of them without an id."`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type for a group_id entry (0=direct members only, 1=inherited members too)"`
	Destroy              *bool  `json:"_destroy,omitempty"               jsonschema:"Set true, with the entry's id, to remove this access level"`
}

// EnvironmentAccess answers the entry as the checks of GitLab's rules read
// it.
func (d UpdateDeployAccessLevelInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{ID: d.ID, AccessLevel: d.AccessLevel, UserID: d.UserID, GroupID: d.GroupID, GroupInheritanceType: d.GroupInheritanceType, Destroy: d.Destroy}
}

// UpdateApprovalRuleInput represents an updated approval rule for an
// environment. It mirrors gl.UpdateEnvironmentApprovalRuleOptions field for
// field but for access_level_description, which the update route does not
// declare, so GitLab drops it (upstream register row 139).
type UpdateApprovalRuleInput struct {
	ID                    *int64 `json:"id,omitempty"                       jsonschema:"ID of the existing approval rule to change or remove. A rule without one adds a rule."`
	UserID                *int64 `json:"user_id,omitempty"                  jsonschema:"ID of a user who may approve. Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	GroupID               *int64 `json:"group_id,omitempty"                 jsonschema:"ID of a group whose members may approve. Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	AccessLevel           *int   `json:"access_level,omitempty"             jsonschema:"Role whose members may approve: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"       jsonschema:"Approvals this rule requires, 1 to 5 unless _destroy is true"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty"   jsonschema:"Group inheritance type for a group_id rule (0=direct members only, 1=inherited members too)"`
	Destroy               *bool  `json:"_destroy,omitempty"                 jsonschema:"Set true, with the rule's id, to remove this rule"`
}

// EnvironmentAccess answers the rule as the checks of GitLab's rules read it.
func (r UpdateApprovalRuleInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{ID: r.ID, AccessLevel: r.AccessLevel, UserID: r.UserID, GroupID: r.GroupID, GroupInheritanceType: r.GroupInheritanceType, RequiredApprovals: r.RequiredApprovalCount, Destroy: r.Destroy}
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

	opts := &gl.ListProtectedEnvironmentsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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
			"the environment may not be protected. Use environment.protected_list first")
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
	if err := toolutil.CheckEnvironmentProtect(opProtectEnvironment, input.DeployAccessLevels, input.ApprovalRules, input.RequiredApprovalCount); err != nil {
		return Output{}, err
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opProtectEnvironment, err)
	}

	levels := protectEnvironmentAccessOptions(input.DeployAccessLevels)
	opts := &gl.ProtectRepositoryEnvironmentsOptions{
		Name:               &input.Name,
		DeployAccessLevels: &levels,
	}
	if input.RequiredApprovalCount != nil {
		opts.RequiredApprovalCount = input.RequiredApprovalCount
	}
	if len(input.ApprovalRules) > 0 {
		rules := protectEnvironmentApprovalRuleOptions(input.ApprovalRules)
		opts.ApprovalRules = &rules
	}

	pe, _, err := client.GL().ProtectedEnvironments.ProtectRepositoryEnvironments(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opProtectEnvironment, err,
				"protecting environments requires Maintainer role and GitLab Premium/Ultimate")
		}
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return Output{}, toolutil.WrapErrWithHint(opProtectEnvironment, err, protectEntriesHint)
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opProtectEnvironment, err, http.StatusConflict,
			"the environment is already protected. Use environment.protected_update to modify access levels")
	}
	return toOutput(pe), nil
}

func protectEnvironmentAccessOptions(inputs []DeployAccessLevelInput) []*gl.EnvironmentAccessOptions {
	levels := make([]*gl.EnvironmentAccessOptions, 0, len(inputs))
	for _, input := range inputs {
		accessOptions := &gl.EnvironmentAccessOptions{UserID: input.UserID, GroupID: input.GroupID, GroupInheritanceType: input.GroupInheritanceType}
		if input.AccessLevel != nil {
			accessLevel := gl.AccessLevelValue(int64(*input.AccessLevel))
			accessOptions.AccessLevel = &accessLevel
		}
		levels = append(levels, accessOptions)
	}
	return levels
}

func protectEnvironmentApprovalRuleOptions(inputs []ApprovalRuleInput) []*gl.EnvironmentApprovalRuleOptions {
	rules := make([]*gl.EnvironmentApprovalRuleOptions, 0, len(inputs))
	for _, input := range inputs {
		approvalOptions := &gl.EnvironmentApprovalRuleOptions{UserID: input.UserID, GroupID: input.GroupID, RequiredApprovalCount: input.RequiredApprovalCount, GroupInheritanceType: input.GroupInheritanceType}
		if input.AccessLevel != nil {
			accessLevel := gl.AccessLevelValue(int64(*input.AccessLevel))
			approvalOptions.AccessLevel = &accessLevel
		}
		rules = append(rules, approvalOptions)
	}
	return rules
}

// Update modifies an existing protected environment.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	if err := toolutil.CheckEnvironmentUpdate(opUpdateProtectedEnv, input.DeployAccessLevels, input.ApprovalRules, input.RequiredApprovalCount); err != nil {
		return Output{}, err
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(opUpdateProtectedEnv, err)
	}

	opts := &gl.UpdateProtectedEnvironmentsOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.RequiredApprovalCount != nil {
		opts.RequiredApprovalCount = input.RequiredApprovalCount
	}
	if len(input.DeployAccessLevels) > 0 {
		levels := updateEnvironmentAccessOptions(input.DeployAccessLevels)
		opts.DeployAccessLevels = &levels
	}
	if len(input.ApprovalRules) > 0 {
		rules := updateEnvironmentApprovalRuleOptions(input.ApprovalRules)
		opts.ApprovalRules = &rules
	}

	pe, _, err := client.GL().ProtectedEnvironments.UpdateProtectedEnvironments(string(input.ProjectID), input.Environment, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint(opUpdateProtectedEnv, err,
				"updating protected environments requires Maintainer role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint(opUpdateProtectedEnv, err, http.StatusNotFound,
			"the environment may not be protected. Use environment.protected_protect first")
	}
	return toOutput(pe), nil
}

func updateEnvironmentAccessOptions(inputs []UpdateDeployAccessLevelInput) []*gl.UpdateEnvironmentAccessOptions {
	levels := make([]*gl.UpdateEnvironmentAccessOptions, 0, len(inputs))
	for _, input := range inputs {
		eao := &gl.UpdateEnvironmentAccessOptions{ID: input.ID, UserID: input.UserID, GroupID: input.GroupID, GroupInheritanceType: input.GroupInheritanceType, Destroy: input.Destroy}
		if input.AccessLevel != nil {
			accessLevel := gl.AccessLevelValue(int64(*input.AccessLevel))
			eao.AccessLevel = &accessLevel
		}
		levels = append(levels, eao)
	}
	return levels
}

func updateEnvironmentApprovalRuleOptions(inputs []UpdateApprovalRuleInput) []*gl.UpdateEnvironmentApprovalRuleOptions {
	rules := make([]*gl.UpdateEnvironmentApprovalRuleOptions, 0, len(inputs))
	for _, input := range inputs {
		aro := &gl.UpdateEnvironmentApprovalRuleOptions{ID: input.ID, UserID: input.UserID, GroupID: input.GroupID, RequiredApprovalCount: input.RequiredApprovalCount, GroupInheritanceType: input.GroupInheritanceType, Destroy: input.Destroy}
		if input.AccessLevel != nil {
			accessLevel := gl.AccessLevelValue(int64(*input.AccessLevel))
			aro.AccessLevel = &accessLevel
		}
		rules = append(rules, aro)
	}
	return rules
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
		return toolutil.WrapErrWithMessage(opUnprotectEnvironment, err)
	}

	_, err := client.GL().ProtectedEnvironments.UnprotectEnvironment(string(input.ProjectID), input.Environment, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint(opUnprotectEnvironment, err,
				"unprotecting environments requires Maintainer role")
		}
		return toolutil.WrapErrWithStatusHint(opUnprotectEnvironment, err, http.StatusNotFound,
			"the environment may already be unprotected. Use environment.protected_list to verify")
	}
	return nil
}

// ---------- Formatters ----------.
