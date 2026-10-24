package groupprotectedenvs

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// groupProtectedEnvironmentTierHint is what a protect GitLab refused with 400
// or 422 is wrapped with: what the handler cannot hold a call to itself. The
// shape of every entry and the values it takes are refused before anything is
// sent (toolutil.CheckEnvironmentProtect), so what is left is the tier and
// the entries GitLab keeps: ProtectedEnvironments::BaseService drops, without
// refusing it, an entry whose user is not a Maintainer or Owner of the group
// or whose group is neither the group, one of its subgroups nor a group it is
// shared with, and the model then refuses a protect left with no deploy
// access level with 422. The role and the license are refused with neither
// status, so the hint names neither. It is one literal, since gremlins
// mutates the + of a concatenated constant and no coverage profile reaches a
// constant.
const groupProtectedEnvironmentTierHint = "use one of GitLab's valid group protected environment tiers: production, staging, testing, development, other; a user_id must name a Maintainer or Owner of the group and a group_id the group, one of its subgroups or a group it is shared with: GitLab drops any other entry and refuses a protect left with no deploy access level"

// The operations a protect and an update are reported under, the refusals of
// their entries included.
const (
	opProtect = "protectGroupEnvironment"
	opUpdate  = "updateGroupProtectedEnvironment"
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

// ApprovalRuleInput represents an approval rule input. It mirrors
// gl.GroupEnvironmentApprovalRuleOptions field for field but for
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

// UpdateDeployAccessLevelInput represents an updated deploy access level.
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

// UpdateApprovalRuleInput represents an updated approval rule. It mirrors
// gl.UpdateGroupEnvironmentApprovalRuleOptions field for field but for
// access_level_description, which the update route does not declare, so
// GitLab drops it (upstream register row 139).
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

// ListInput defines parameters for the List action which retrieves group-level
// protected environments. It mirrors
// gl.ListGroupProtectedEnvironmentsOptions, which embeds gl.ListOptions,
// exposing keyset pagination (order_by, sort, pagination, page_token) in
// addition to offset pagination.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for the Get action which retrieves a single group-level protected environment.
type GetInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	Environment string               `json:"environment" jsonschema:"Environment tier (production, staging, testing, development, other),required"`
}

// ProtectInput defines parameters for the Protect action which creates a group-level protected environment.
type ProtectInput struct {
	GroupID               toolutil.StringOrInt     `json:"group_id"                          jsonschema:"Group ID or URL-encoded path,required"`
	Name                  string                   `json:"name"                              jsonschema:"Environment tier to protect (production, staging, testing, development, other),required"`
	DeployAccessLevels    []DeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Who may deploy: at least one entry, each naming exactly one of access_level, user_id or group_id,required"`
	RequiredApprovalCount *int64                   `json:"required_approval_count,omitempty" jsonschema:"Deprecated: GitLab refuses any value but 0. Require approvals with required_approvals on approval_rules instead."`
	ApprovalRules         []ApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Who must approve a deployment: each rule names at least one of access_level, user_id or group_id"`
}

// UpdateInput defines parameters for the Update action which modifies a group-level protected environment.
type UpdateInput struct {
	GroupID               toolutil.StringOrInt           `json:"group_id"                          jsonschema:"Group ID or URL-encoded path,required"`
	Environment           string                         `json:"environment"                       jsonschema:"Environment tier (production, staging, testing, development, other),required"`
	Name                  string                         `json:"name,omitempty"                    jsonschema:"New environment tier (production, staging, testing, development, other)"`
	DeployAccessLevels    []UpdateDeployAccessLevelInput `json:"deploy_access_levels,omitempty"    jsonschema:"Deploy access levels to add, change or remove: each entry names an existing one by id or exactly one of access_level, user_id or group_id, and never two of them"`
	RequiredApprovalCount *int64                         `json:"required_approval_count,omitempty" jsonschema:"Deprecated: GitLab refuses any value but 0. Require approvals with required_approvals on approval_rules instead."`
	ApprovalRules         []UpdateApprovalRuleInput      `json:"approval_rules,omitempty"          jsonschema:"Approval rules to add, change or remove: unless _destroy is true, each rule names at least one of access_level, user_id or group_id"`
}

// UnprotectInput defines parameters for the Unprotect action which removes a group-level protected environment.
type UnprotectInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	Environment string               `json:"environment" jsonschema:"Environment tier to unprotect (production, staging, testing, development, other),required"`
}

// toDeployAccessOpts converts the deploy access levels of a protect, which
// always carries at least one: toolutil.CheckEnvironmentProtect refuses one
// that carries none before this runs.
func toDeployAccessOpts(input []DeployAccessLevelInput) *[]*gl.GroupEnvironmentAccessOptions {
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
	opts := &gl.ListGroupProtectedEnvironmentsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	envs, resp, err := client.GL().GroupProtectedEnvironments.ListGroupProtectedEnvironments(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("listGroupProtectedEnvironments", err, http.StatusForbidden,
			"requires Owner role + Premium/Ultimate; verify group_id with group.list; group protected environments cascade to all subgroup projects")
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
		return Output{}, toolutil.WrapErrWithStatusHint("getGroupProtectedEnvironment", err, http.StatusNotFound,
			"verify group_id and name (tier: production, staging, testing, development, other); requires Owner + Premium/Ultimate")
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
	if err := toolutil.CheckEnvironmentProtect(opProtect, input.DeployAccessLevels, input.ApprovalRules, input.RequiredApprovalCount); err != nil {
		return Output{}, err
	}
	opts := &gl.ProtectGroupEnvironmentOptions{
		Name:                  new(input.Name),
		DeployAccessLevels:    toDeployAccessOpts(input.DeployAccessLevels),
		RequiredApprovalCount: input.RequiredApprovalCount,
		ApprovalRules:         toApprovalRuleOpts(input.ApprovalRules),
	}
	e, _, err := client.GL().GroupProtectedEnvironments.ProtectGroupEnvironment(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return Output{}, toolutil.WrapErrWithHint(opProtect, err, groupProtectedEnvironmentTierHint)
		}
		return Output{}, toolutil.WrapErrWithMessage(opProtect, err)
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
	if err := toolutil.CheckEnvironmentUpdate(opUpdate, input.DeployAccessLevels, input.ApprovalRules, input.RequiredApprovalCount); err != nil {
		return Output{}, err
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
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint(opUpdate, err,
				"use group.protected_env_list to verify the environment; valid tiers are production, staging, testing, development, other; provide _destroy=true on individual rule entries to remove them; partial updates merge with existing rules")
		}
		return Output{}, toolutil.WrapErrWithMessage(opUpdate, err)
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
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) || toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return toolutil.WrapErrWithHint("unprotectGroupEnvironment", err,
				"requires Owner + Premium/Ultimate; use group.protected_env_list to verify the environment; valid tiers are production, staging, testing, development, other; unprotection cascades and removes restrictions on subgroup projects")
		}
		return toolutil.WrapErrWithMessage("unprotectGroupEnvironment", err)
	}
	return nil
}
