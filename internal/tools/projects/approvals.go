package projects

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetApprovalConfigInput defines parameters for getting approval configuration.
type GetApprovalConfigInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ApprovalConfigOutput holds project-level approval configuration.
type ApprovalConfigOutput struct {
	toolutil.HintableOutput
	ApprovalsBeforeMerge                      int64 `json:"approvals_before_merge"`
	ResetApprovalsOnPush                      bool  `json:"reset_approvals_on_push"`
	DisableOverridingApproversPerMergeRequest bool  `json:"disable_overriding_approvers_per_merge_request"`
	MergeRequestsAuthorApproval               bool  `json:"merge_requests_author_approval"`
	MergeRequestsDisableCommittersApproval    bool  `json:"merge_requests_disable_committers_approval"`
	RequireReauthenticationToApprove          bool  `json:"require_reauthentication_to_approve"`
	SelectiveCodeOwnerRemovals                bool  `json:"selective_code_owner_removals"`
}

func approvalConfigToOutput(a *gl.ProjectApprovals) ApprovalConfigOutput {
	return ApprovalConfigOutput{
		//lint:ignore SA1019 deprecated field still present in API response.
		ApprovalsBeforeMerge:                      a.ApprovalsBeforeMerge, //nolint:staticcheck // deprecated field still present in API response
		ResetApprovalsOnPush:                      a.ResetApprovalsOnPush,
		DisableOverridingApproversPerMergeRequest: a.DisableOverridingApproversPerMergeRequest,
		MergeRequestsAuthorApproval:               a.MergeRequestsAuthorApproval,
		MergeRequestsDisableCommittersApproval:    a.MergeRequestsDisableCommittersApproval,
		RequireReauthenticationToApprove:          a.RequireReauthenticationToApprove,
		SelectiveCodeOwnerRemovals:                a.SelectiveCodeOwnerRemovals,
	}
}

// GetApprovalConfig retrieves the project-level approval configuration.
func GetApprovalConfig(ctx context.Context, client *gitlabclient.Client, input GetApprovalConfigInput) (ApprovalConfigOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalConfigOutput{}, err
	}
	if input.ProjectID == "" {
		return ApprovalConfigOutput{}, errors.New("projectGetApprovalConfig: project_id is required")
	}
	approvals, _, err := client.GL().Projects.GetApprovalConfiguration(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ApprovalConfigOutput{}, toolutil.WrapErrWithStatusHint("projectGetApprovalConfig", err, http.StatusNotFound,
			"verify project_id with gitlab_project_list; approval rules are GitLab Premium/Ultimate; requires Reporter role minimum")
	}
	return approvalConfigToOutput(approvals), nil
}

// ChangeApprovalConfigInput defines parameters for changing approval configuration.
type ChangeApprovalConfigInput struct {
	ProjectID                                 toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ApprovalsBeforeMerge                      *int64               `json:"approvals_before_merge,omitempty" jsonschema:"Number of approvals required before merge"`
	ResetApprovalsOnPush                      *bool                `json:"reset_approvals_on_push,omitempty" jsonschema:"Reset approvals when new commits are pushed"`
	DisableOverridingApproversPerMergeRequest *bool                `json:"disable_overriding_approvers_per_merge_request,omitempty" jsonschema:"Prevent overriding approvers per MR"`
	MergeRequestsAuthorApproval               *bool                `json:"merge_requests_author_approval,omitempty" jsonschema:"Allow MR author to approve their own MR"`
	MergeRequestsDisableCommittersApproval    *bool                `json:"merge_requests_disable_committers_approval,omitempty" jsonschema:"Prevent MR committers from approving"`
	RequireReauthenticationToApprove          *bool                `json:"require_reauthentication_to_approve,omitempty" jsonschema:"Require reauthentication to approve"`
	SelectiveCodeOwnerRemovals                *bool                `json:"selective_code_owner_removals,omitempty" jsonschema:"Only remove code owner approvals when relevant files change"`
}

// ChangeApprovalConfig updates the project-level approval configuration.
func ChangeApprovalConfig(ctx context.Context, client *gitlabclient.Client, input ChangeApprovalConfigInput) (ApprovalConfigOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalConfigOutput{}, err
	}
	if input.ProjectID == "" {
		return ApprovalConfigOutput{}, errors.New("projectChangeApprovalConfig: project_id is required")
	}
	opts := &gl.ChangeApprovalConfigurationOptions{
		ApprovalsBeforeMerge:                      input.ApprovalsBeforeMerge,
		ResetApprovalsOnPush:                      input.ResetApprovalsOnPush,
		DisableOverridingApproversPerMergeRequest: input.DisableOverridingApproversPerMergeRequest,
		MergeRequestsAuthorApproval:               input.MergeRequestsAuthorApproval,
		MergeRequestsDisableCommittersApproval:    input.MergeRequestsDisableCommittersApproval,
		RequireReauthenticationToApprove:          input.RequireReauthenticationToApprove,
		SelectiveCodeOwnerRemovals:                input.SelectiveCodeOwnerRemovals,
	}
	approvals, _, err := client.GL().Projects.ChangeApprovalConfiguration(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ApprovalConfigOutput{}, toolutil.WrapErrWithStatusHint("projectChangeApprovalConfig", err, http.StatusForbidden,
			"requires Maintainer role; approvals_before_merge is deprecated \u2014 use approval rules instead; approval features require Premium/Ultimate")
	}
	return approvalConfigToOutput(approvals), nil
}

// ListApprovalRulesInput defines parameters for listing project approval rules.
type ListApprovalRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ApprovalRuleOutput holds a single project approval rule.
type ApprovalRuleOutput struct {
	toolutil.HintableOutput
	ID                            int64    `json:"id"`
	Name                          string   `json:"name"`
	RuleType                      string   `json:"rule_type,omitempty"`
	ReportType                    string   `json:"report_type,omitempty"`
	ApprovalsRequired             int64    `json:"approvals_required"`
	EligibleApprovers             []string `json:"eligible_approvers,omitempty"`
	Users                         []string `json:"users,omitempty"`
	Groups                        []string `json:"groups,omitempty"`
	ContainsHiddenGroups          bool     `json:"contains_hidden_groups"`
	AppliesToAllProtectedBranches bool     `json:"applies_to_all_protected_branches"`
}

// ListApprovalRulesOutput holds a paginated list of project approval rules.
type ListApprovalRulesOutput struct {
	toolutil.HintableOutput
	Rules      []ApprovalRuleOutput      `json:"rules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func approvalRuleToOutput(r *gl.ProjectApprovalRule) ApprovalRuleOutput {
	out := ApprovalRuleOutput{
		ID:                            r.ID,
		Name:                          r.Name,
		RuleType:                      r.RuleType,
		ReportType:                    r.ReportType,
		ApprovalsRequired:             r.ApprovalsRequired,
		ContainsHiddenGroups:          r.ContainsHiddenGroups,
		AppliesToAllProtectedBranches: r.AppliesToAllProtectedBranches,
	}
	for _, u := range r.EligibleApprovers {
		out.EligibleApprovers = append(out.EligibleApprovers, u.Username)
	}
	for _, u := range r.Users {
		out.Users = append(out.Users, u.Username)
	}
	for _, g := range r.Groups {
		out.Groups = append(out.Groups, g.Name)
	}
	return out
}

// ListApprovalRules retrieves all project-level approval rules.
func ListApprovalRules(ctx context.Context, client *gitlabclient.Client, input ListApprovalRulesInput) (ListApprovalRulesOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListApprovalRulesOutput{}, err
	}
	if input.ProjectID == "" {
		return ListApprovalRulesOutput{}, errors.New("projectListApprovalRules: project_id is required")
	}
	opts := &gl.GetProjectApprovalRulesListsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	rules, resp, err := client.GL().Projects.GetProjectApprovalRules(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListApprovalRulesOutput{}, toolutil.WrapErrWithStatusHint("projectListApprovalRules", err, http.StatusNotFound,
			"verify project_id with gitlab_project_list; approval rules require Premium/Ultimate license")
	}
	out := make([]ApprovalRuleOutput, len(rules))
	for i, r := range rules {
		out[i] = approvalRuleToOutput(r)
	}
	return ListApprovalRulesOutput{Rules: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetApprovalRuleInput defines parameters for getting a specific approval rule.
type GetApprovalRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RuleID    int64                `json:"rule_id" jsonschema:"Approval rule ID,required"`
}

// GetApprovalRule retrieves a single project-level approval rule.
func GetApprovalRule(ctx context.Context, client *gitlabclient.Client, input GetApprovalRuleInput) (ApprovalRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRuleOutput{}, err
	}
	if input.ProjectID == "" {
		return ApprovalRuleOutput{}, errors.New("projectGetApprovalRule: project_id is required")
	}
	if input.RuleID == 0 {
		return ApprovalRuleOutput{}, errors.New("projectGetApprovalRule: rule_id is required")
	}
	rule, _, err := client.GL().Projects.GetProjectApprovalRule(string(input.ProjectID), input.RuleID, gl.WithContext(ctx))
	if err != nil {
		return ApprovalRuleOutput{}, toolutil.WrapErrWithStatusHint("projectGetApprovalRule", err, http.StatusNotFound,
			"verify approval_rule_id with gitlab_project_approval_rule_list; rule may have been deleted")
	}
	return approvalRuleToOutput(rule), nil
}

// CreateApprovalRuleInput defines parameters for creating an approval rule.
type CreateApprovalRuleInput struct {
	ProjectID                     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name                          string               `json:"name" jsonschema:"Rule name,required"`
	ApprovalsRequired             int64                `json:"approvals_required" jsonschema:"Number of approvals required,required"`
	RuleType                      string               `json:"rule_type,omitempty" jsonschema:"Rule type (regular, code_owner)"`
	UserIDs                       []int64              `json:"user_ids,omitempty" jsonschema:"User IDs to assign as approvers"`
	GroupIDs                      []int64              `json:"group_ids,omitempty" jsonschema:"Group IDs to assign as approvers"`
	ProtectedBranchIDs            []int64              `json:"protected_branch_ids,omitempty" jsonschema:"Protected branch IDs to scope the rule to"`
	Usernames                     []string             `json:"usernames,omitempty" jsonschema:"Usernames to assign as approvers"`
	AppliesToAllProtectedBranches *bool                `json:"applies_to_all_protected_branches,omitempty" jsonschema:"Apply this rule to all protected branches"`
}

// CreateApprovalRule creates a new project-level approval rule.
func CreateApprovalRule(ctx context.Context, client *gitlabclient.Client, input CreateApprovalRuleInput) (ApprovalRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRuleOutput{}, err
	}
	if input.ProjectID == "" {
		return ApprovalRuleOutput{}, errors.New("projectCreateApprovalRule: project_id is required")
	}
	if input.Name == "" {
		return ApprovalRuleOutput{}, errors.New("projectCreateApprovalRule: name is required")
	}
	opts := &gl.CreateProjectLevelRuleOptions{
		Name:              &input.Name,
		ApprovalsRequired: &input.ApprovalsRequired,
	}
	if input.RuleType != "" {
		opts.RuleType = &input.RuleType
	}
	if len(input.UserIDs) > 0 {
		opts.UserIDs = &input.UserIDs
	}
	if len(input.GroupIDs) > 0 {
		opts.GroupIDs = &input.GroupIDs
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	if len(input.Usernames) > 0 {
		opts.Usernames = &input.Usernames
	}
	if input.AppliesToAllProtectedBranches != nil {
		opts.AppliesToAllProtectedBranches = input.AppliesToAllProtectedBranches
	}
	rule, _, err := client.GL().Projects.CreateProjectApprovalRule(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ApprovalRuleOutput{}, toolutil.WrapErrWithStatusHint("projectCreateApprovalRule", err, http.StatusBadRequest,
			"requires Maintainer role + Premium/Ultimate; rule_type must be 'regular' or 'any_approver'; user_ids/group_ids must reference existing project members; protected_branch_ids require Premium")
	}
	return approvalRuleToOutput(rule), nil
}

// UpdateApprovalRuleInput defines parameters for updating an approval rule.
type UpdateApprovalRuleInput struct {
	ProjectID                     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RuleID                        int64                `json:"rule_id" jsonschema:"Approval rule ID,required"`
	Name                          string               `json:"name,omitempty" jsonschema:"Updated rule name"`
	ApprovalsRequired             *int64               `json:"approvals_required,omitempty" jsonschema:"Updated number of approvals required"`
	UserIDs                       []int64              `json:"user_ids,omitempty" jsonschema:"Updated user IDs to assign as approvers"`
	GroupIDs                      []int64              `json:"group_ids,omitempty" jsonschema:"Updated group IDs to assign as approvers"`
	ProtectedBranchIDs            []int64              `json:"protected_branch_ids,omitempty" jsonschema:"Updated protected branch IDs"`
	Usernames                     []string             `json:"usernames,omitempty" jsonschema:"Updated usernames to assign as approvers"`
	AppliesToAllProtectedBranches *bool                `json:"applies_to_all_protected_branches,omitempty" jsonschema:"Apply this rule to all protected branches"`
}

// UpdateApprovalRule updates an existing project-level approval rule.
func UpdateApprovalRule(ctx context.Context, client *gitlabclient.Client, input UpdateApprovalRuleInput) (ApprovalRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRuleOutput{}, err
	}
	if input.ProjectID == "" {
		return ApprovalRuleOutput{}, errors.New("projectUpdateApprovalRule: project_id is required")
	}
	if input.RuleID == 0 {
		return ApprovalRuleOutput{}, errors.New("projectUpdateApprovalRule: rule_id is required")
	}
	opts := &gl.UpdateProjectLevelRuleOptions{}
	if input.Name != "" {
		opts.Name = &input.Name
	}
	if input.ApprovalsRequired != nil {
		opts.ApprovalsRequired = input.ApprovalsRequired
	}
	if len(input.UserIDs) > 0 {
		opts.UserIDs = &input.UserIDs
	}
	if len(input.GroupIDs) > 0 {
		opts.GroupIDs = &input.GroupIDs
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	if len(input.Usernames) > 0 {
		opts.Usernames = &input.Usernames
	}
	if input.AppliesToAllProtectedBranches != nil {
		opts.AppliesToAllProtectedBranches = input.AppliesToAllProtectedBranches
	}
	rule, _, err := client.GL().Projects.UpdateProjectApprovalRule(string(input.ProjectID), input.RuleID, opts, gl.WithContext(ctx))
	if err != nil {
		return ApprovalRuleOutput{}, toolutil.WrapErrWithStatusHint("projectUpdateApprovalRule", err, http.StatusNotFound,
			"verify approval_rule_id with gitlab_project_approval_rule_list; requires Maintainer role; cannot change rule_type after creation")
	}
	return approvalRuleToOutput(rule), nil
}

// DeleteApprovalRuleInput defines parameters for deleting an approval rule.
type DeleteApprovalRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RuleID    int64                `json:"rule_id" jsonschema:"Approval rule ID to delete,required"`
}

// DeleteApprovalRule deletes a project-level approval rule.
func DeleteApprovalRule(ctx context.Context, client *gitlabclient.Client, input DeleteApprovalRuleInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteApprovalRule: project_id is required")
	}
	if input.RuleID == 0 {
		return errors.New("projectDeleteApprovalRule: rule_id is required")
	}
	_, err := client.GL().Projects.DeleteProjectApprovalRule(string(input.ProjectID), input.RuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteApprovalRule", err, http.StatusForbidden,
			"requires Maintainer role; verify approval_rule_id with gitlab_project_approval_rule_list; deletion is irreversible")
	}
	return nil
}
