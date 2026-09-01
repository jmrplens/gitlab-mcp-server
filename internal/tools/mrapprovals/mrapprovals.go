package mrapprovals

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// StateInput defines parameters for retrieving the approval state
// of a merge request.
type StateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// RulesInput defines parameters for listing the approval rules
// of a merge request.
type RulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// ConfigInput defines parameters for getting approval configuration.
type ConfigInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// ResetInput defines parameters for resetting approvals on a merge request.
type ResetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// CreateRuleInput defines parameters for creating an MR approval rule.
type CreateRuleInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"merge_request_iid"                   jsonschema:"Merge request internal ID,required"`
	Name                  string               `json:"name"                     jsonschema:"Rule name,required"`
	ApprovalsRequired     int64                `json:"approvals_required"       jsonschema:"Number of approvals required,required"`
	ApprovalProjectRuleID int64                `json:"approval_project_rule_id" jsonschema:"Project-level approval rule ID to inherit from"`
	UserIDs               []int64              `json:"user_ids"                 jsonschema:"User IDs eligible to approve"`
	GroupIDs              []int64              `json:"group_ids"                jsonschema:"Group IDs eligible to approve"`
}

// UpdateRuleInput defines parameters for updating an MR approval rule.
type UpdateRuleInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	MRIID             int64                `json:"merge_request_iid"             jsonschema:"Merge request internal ID,required"`
	ApprovalRuleID    int64                `json:"approval_rule_id"   jsonschema:"Approval rule ID,required"`
	Name              string               `json:"name"               jsonschema:"Rule name"`
	ApprovalsRequired *int64               `json:"approvals_required" jsonschema:"Number of approvals required"`
	UserIDs           []int64              `json:"user_ids"           jsonschema:"User IDs eligible to approve"`
	GroupIDs          []int64              `json:"group_ids"          jsonschema:"Group IDs eligible to approve"`
}

// DeleteRuleInput defines parameters for deleting an MR approval rule.
type DeleteRuleInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	MRIID          int64                `json:"merge_request_iid"           jsonschema:"Merge request internal ID,required"`
	ApprovalRuleID int64                `json:"approval_rule_id" jsonschema:"Approval rule ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// RuleOutput represents a single approval rule for a merge request. It mirrors
// gl.MergeRequestApprovalRule, surfacing the full approver/eligible/user/group
// objects and the project-level source rule on their canonical keys.
type RuleOutput struct {
	toolutil.HintableOutput
	ID                   int64                      `json:"id"`
	Name                 string                     `json:"name"`
	RuleType             string                     `json:"rule_type"`
	ReportType           string                     `json:"report_type,omitempty"`
	Section              string                     `json:"section,omitempty"`
	ApprovalsRequired    int                        `json:"approvals_required"`
	Approved             bool                       `json:"approved"`
	ContainsHiddenGroups bool                       `json:"contains_hidden_groups,omitempty"`
	Overridden           bool                       `json:"overridden,omitempty"`
	ApprovedBy           []*BasicUserOutput         `json:"approved_by,omitempty"`
	EligibleApprovers    []*BasicUserOutput         `json:"eligible_approvers,omitempty"`
	Users                []*BasicUserOutput         `json:"users,omitempty"`
	Groups               []*GroupOutput             `json:"groups,omitempty"`
	SourceRule           *ProjectApprovalRuleOutput `json:"source_rule,omitempty"`
}

// StateOutput holds the overall approval state for a merge request,
// including whether rules have been overridden and the list of applicable rules.
type StateOutput struct {
	toolutil.HintableOutput
	ApprovalRulesOverwritten bool         `json:"approval_rules_overwritten"`
	Rules                    []RuleOutput `json:"rules"`
}

// RulesOutput holds the list of approval rules for a merge request.
type RulesOutput struct {
	toolutil.HintableOutput
	Rules []RuleOutput `json:"rules"`
}

// ConfigOutput holds the approval configuration for a merge request. It mirrors
// gl.MergeRequestApprovals, surfacing every SDK field including the full
// approver/suggested-approver/approver-group objects and the per-rule
// approval_rules_left list on their canonical keys.
type ConfigOutput struct {
	toolutil.HintableOutput
	ID                             int64                              `json:"id"`
	IID                            int64                              `json:"iid"`
	ProjectID                      int64                              `json:"project_id"`
	Title                          string                             `json:"title"`
	Description                    string                             `json:"description,omitempty"`
	State                          string                             `json:"state"`
	CreatedAt                      string                             `json:"created_at,omitempty"`
	UpdatedAt                      string                             `json:"updated_at,omitempty"`
	MergeStatus                    string                             `json:"merge_status,omitempty"`
	Approved                       bool                               `json:"approved"`
	ApprovalsRequired              int64                              `json:"approvals_required" tier:"premium"`
	ApprovalsLeft                  int64                              `json:"approvals_left" tier:"premium"`
	ApprovalsBeforeMerge           int64                              `json:"approvals_before_merge" tier:"premium"`
	RequirePasswordToApprove       bool                               `json:"require_password_to_approve" tier:"premium"`
	HasApprovalRules               bool                               `json:"has_approval_rules" tier:"premium"`
	UserHasApproved                bool                               `json:"user_has_approved"`
	UserCanApprove                 bool                               `json:"user_can_approve"`
	MergeRequestApproversAvailable bool                               `json:"merge_request_approvers_available" tier:"premium"`
	MultipleApprovalRulesAvailable bool                               `json:"multiple_approval_rules_available" tier:"premium"`
	ApprovedBy                     []*MergeRequestApproverUserOutput  `json:"approved_by,omitempty"`
	SuggestedApprovers             []*BasicUserOutput                 `json:"suggested_approvers,omitempty" tier:"premium"`
	Approvers                      []*MergeRequestApproverUserOutput  `json:"approvers,omitempty" tier:"premium"`
	ApproverGroups                 []*MergeRequestApproverGroupOutput `json:"approver_groups,omitempty" tier:"premium"`
	ApprovalRulesLeft              []RuleOutput                       `json:"approval_rules_left,omitempty" tier:"premium"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// RuleToOutput converts a client-go MergeRequestApprovalRule to the
// MCP output representation, surfacing the full approver/eligible/user/group
// objects and the project-level source rule on their canonical keys.
func RuleToOutput(r *gl.MergeRequestApprovalRule) RuleOutput {
	return RuleOutput{
		ID:                   r.ID,
		Name:                 r.Name,
		RuleType:             r.RuleType,
		ReportType:           r.ReportType,
		Section:              r.Section,
		ApprovalsRequired:    int(r.ApprovalsRequired),
		Approved:             r.Approved,
		ContainsHiddenGroups: r.ContainsHiddenGroups,
		ApprovedBy:           basicUserOutputs(r.ApprovedBy),
		EligibleApprovers:    basicUserOutputs(r.EligibleApprovers),
		Users:                basicUserOutputs(r.Users),
		Groups:               groupOutputs(r.Groups),
		SourceRule:           projectApprovalRuleOutput(r.SourceRule),
	}
}

// rawRuleToOutput converts a raw-fetch [mergeRequestApprovalRuleAPI] to the MCP
// output representation, surfacing the documented "overridden" boolean alongside
// the documented-reference-subset approver/user/group objects and the
// project-level source rule on their canonical keys.
func rawRuleToOutput(r *mergeRequestApprovalRuleAPI) RuleOutput {
	return RuleOutput{
		ID:                   r.ID,
		Name:                 r.Name,
		RuleType:             r.RuleType,
		ReportType:           r.ReportType,
		Section:              r.Section,
		ApprovalsRequired:    int(r.ApprovalsRequired),
		Approved:             r.Approved,
		ContainsHiddenGroups: r.ContainsHiddenGroups,
		Overridden:           r.Overridden,
		ApprovedBy:           basicUserOutputs(r.ApprovedBy),
		EligibleApprovers:    basicUserOutputs(r.EligibleApprovers),
		Users:                basicUserOutputs(r.Users),
		Groups:               groupOutputs(r.Groups),
		SourceRule:           projectApprovalRuleOutput(r.SourceRule),
	}
}

// rawListApprovalRules issues a raw REST GET against an MR approval-rules list
// path, decoding the documented response (including the SDK-missing "overridden"
// field) into a slice of [mergeRequestApprovalRuleAPI].
func rawListApprovalRules(ctx context.Context, client *gitlabclient.Client, path string) ([]*mergeRequestApprovalRuleAPI, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var rules []*mergeRequestApprovalRuleAPI
	_, err = client.GL().Do(req, &rules)
	return rules, err
}

// rawApprovalState issues a raw REST GET against an MR approval_state path,
// decoding the documented response (including the SDK-missing per-rule
// "overridden" field) into a [mergeRequestApprovalStateAPI].
func rawApprovalState(ctx context.Context, client *gitlabclient.Client, path string) (*mergeRequestApprovalStateAPI, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var state mergeRequestApprovalStateAPI
	_, err = client.GL().Do(req, &state)
	return &state, err
}

// rawMutateApprovalRule issues a raw REST request (POST/PUT) against an MR
// approval-rule path with the supplied options, decoding the documented response
// (including the SDK-missing "overridden" field) into a single
// [mergeRequestApprovalRuleAPI].
func rawMutateApprovalRule(ctx context.Context, client *gitlabclient.Client, method, path string, opt any) (*mergeRequestApprovalRuleAPI, error) {
	req, err := client.GL().NewRequest(method, path, opt, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var rule mergeRequestApprovalRuleAPI
	_, err = client.GL().Do(req, &rule)
	return &rule, err
}

// configToOutput converts a client-go MergeRequestApprovals to ConfigOutput,
// surfacing every SDK field including the full approver, suggested-approver,
// approver-group, and per-rule approval_rules_left objects.
func configToOutput(c *gl.MergeRequestApprovals) ConfigOutput {
	out := ConfigOutput{
		ID:                             c.ID,
		IID:                            c.IID,
		ProjectID:                      c.ProjectID,
		Title:                          c.Title,
		Description:                    c.Description,
		State:                          c.State,
		CreatedAt:                      toolutil.FormatTimePtr(c.CreatedAt),
		UpdatedAt:                      toolutil.FormatTimePtr(c.UpdatedAt),
		MergeStatus:                    c.MergeStatus,
		Approved:                       c.Approved,
		ApprovalsRequired:              c.ApprovalsRequired,
		ApprovalsLeft:                  c.ApprovalsLeft,
		ApprovalsBeforeMerge:           c.ApprovalsBeforeMerge,
		RequirePasswordToApprove:       c.RequirePasswordToApprove,
		HasApprovalRules:               c.HasApprovalRules,
		UserHasApproved:                c.UserHasApproved,
		UserCanApprove:                 c.UserCanApprove,
		MergeRequestApproversAvailable: c.MergeRequestApproversAvailable,
		MultipleApprovalRulesAvailable: c.MultipleApprovalRulesAvailable,
		ApprovedBy:                     approverUserOutputs(c.ApprovedBy),
		SuggestedApprovers:             basicUserOutputs(c.SuggestedApprovers),
		Approvers:                      approverUserOutputs(c.Approvers),
		ApproverGroups:                 approverGroupOutputs(c.ApproverGroups),
	}
	for _, r := range c.ApprovalRulesLeft {
		if r != nil {
			out.ApprovalRulesLeft = append(out.ApprovalRulesLeft, RuleToOutput(r))
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// State retrieves the approval state of a merge request, including
// whether approval rules have been overridden and the list of rules with their
// current approval status.
func State(ctx context.Context, client *gitlabclient.Client, input StateInput) (StateOutput, error) {
	if err := ctx.Err(); err != nil {
		return StateOutput{}, err
	}
	if input.ProjectID == "" {
		return StateOutput{}, errors.New("mrApprovalState: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return StateOutput{}, toolutil.ErrRequiredInt64("mrApprovalState", "merge_request_iid")
	}
	path := fmt.Sprintf("projects/%s/merge_requests/%d/approval_state", gl.PathEscape(string(input.ProjectID)), input.MRIID)
	state, err := rawApprovalState(ctx, client, path)
	if err != nil {
		if toolutil.IsNotFound(err) {
			return StateOutput{}, fmt.Errorf("mrApprovalState: merge request approval features require GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return StateOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalState", err, http.StatusNotFound,
			"verify project_id + merge_request_iid with gitlab_mr_list; approval rules require Premium/Ultimate license")
	}
	out := StateOutput{
		ApprovalRulesOverwritten: state.ApprovalRulesOverwritten,
	}
	for _, r := range state.Rules {
		if r != nil {
			out.Rules = append(out.Rules, rawRuleToOutput(r))
		}
	}
	return out, nil
}

// Rules lists the approval rules configured for a merge request.
func Rules(ctx context.Context, client *gitlabclient.Client, input RulesInput) (RulesOutput, error) {
	if err := ctx.Err(); err != nil {
		return RulesOutput{}, err
	}
	if input.ProjectID == "" {
		return RulesOutput{}, errors.New("mrApprovalRules: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return RulesOutput{}, toolutil.ErrRequiredInt64("mrApprovalRules", "merge_request_iid")
	}
	path := fmt.Sprintf("projects/%s/merge_requests/%d/approval_rules", gl.PathEscape(string(input.ProjectID)), input.MRIID)
	rules, err := rawListApprovalRules(ctx, client, path)
	if err != nil {
		if toolutil.IsNotFound(err) {
			return RulesOutput{}, fmt.Errorf("mrApprovalRules: merge request approval rules require GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return RulesOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalRules", err, http.StatusNotFound,
			"verify project_id + merge_request_iid with gitlab_mr_list; rules-per-MR require Premium/Ultimate")
	}
	out := RulesOutput{}
	for _, r := range rules {
		if r != nil {
			out.Rules = append(out.Rules, rawRuleToOutput(r))
		}
	}
	return out, nil
}

// Config retrieves the approval configuration (approvals required, current
// approvers, suggested approvers) for a merge request.
func Config(ctx context.Context, client *gitlabclient.Client, input ConfigInput) (ConfigOutput, error) {
	if err := ctx.Err(); err != nil {
		return ConfigOutput{}, err
	}
	if input.ProjectID == "" {
		return ConfigOutput{}, errors.New("mrApprovalConfig: project_id is required")
	}
	if input.MRIID <= 0 {
		return ConfigOutput{}, toolutil.ErrRequiredInt64("mrApprovalConfig", "merge_request_iid")
	}
	cfg, _, err := client.GL().MergeRequestApprovals.GetConfiguration(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsNotFound(err) {
			return ConfigOutput{}, fmt.Errorf("mrApprovalConfig: merge request approval configuration requires GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return ConfigOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalConfig", err, http.StatusForbidden,
			"requires Maintainer role; approvals_required is deprecated. Prefer approval rules; verify project_id + merge_request_iid")
	}
	return configToOutput(cfg), nil
}

// Reset clears all existing approvals on a merge request.
func Reset(ctx context.Context, client *gitlabclient.Client, input ResetInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrApprovalReset: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrApprovalReset", "merge_request_iid")
	}
	_, err := client.GL().MergeRequestApprovals.ResetApprovalsOfMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return toolutil.WrapErrWithHint("mrApprovalReset", err,
				"endpoint requires a bot user backed by a project/group access token; verify project_id + merge_request_iid and that the caller authenticates with a project or group access token (PATs from human users are not accepted)")
		}
		return toolutil.WrapErrWithStatusHint("mrApprovalReset", err, http.StatusForbidden,
			"requires Maintainer role; resets all approvals on the MR. Cannot be undone; verify project_id + merge_request_iid")
	}
	return nil
}

// CreateRule creates a new approval rule on a merge request.
func CreateRule(ctx context.Context, client *gitlabclient.Client, input CreateRuleInput) (RuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return RuleOutput{}, err
	}
	if input.ProjectID == "" {
		return RuleOutput{}, errors.New("mrApprovalRuleCreate: project_id is required")
	}
	if input.MRIID <= 0 {
		return RuleOutput{}, toolutil.ErrRequiredInt64("mrApprovalRuleCreate", "merge_request_iid")
	}
	if input.Name == "" {
		return RuleOutput{}, errors.New("mrApprovalRuleCreate: name is required")
	}

	opts := &gl.CreateMergeRequestApprovalRuleOptions{
		Name:              new(input.Name),
		ApprovalsRequired: new(input.ApprovalsRequired),
	}
	if input.ApprovalProjectRuleID != 0 {
		opts.ApprovalProjectRuleID = new(input.ApprovalProjectRuleID)
	}
	if len(input.UserIDs) > 0 {
		opts.UserIDs = new(input.UserIDs)
	}
	if len(input.GroupIDs) > 0 {
		opts.GroupIDs = new(input.GroupIDs)
	}

	path := fmt.Sprintf("projects/%s/merge_requests/%d/approval_rules", gl.PathEscape(string(input.ProjectID)), input.MRIID)
	rule, err := rawMutateApprovalRule(ctx, client, http.MethodPost, path, opts)
	if err != nil {
		return RuleOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalRuleCreate", err, http.StatusBadRequest,
			"requires Maintainer + Premium/Ultimate; user_ids/group_ids must be project members; rule_type must be 'regular' or 'any_approver'; cannot have multiple any_approver rules")
	}
	return rawRuleToOutput(rule), nil
}

// UpdateRule updates an existing approval rule on a merge request.
func UpdateRule(ctx context.Context, client *gitlabclient.Client, input UpdateRuleInput) (RuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return RuleOutput{}, err
	}
	if input.ProjectID == "" {
		return RuleOutput{}, errors.New("mrApprovalRuleUpdate: project_id is required")
	}
	if input.MRIID <= 0 {
		return RuleOutput{}, toolutil.ErrRequiredInt64("mrApprovalRuleUpdate", "merge_request_iid")
	}
	if input.ApprovalRuleID <= 0 {
		return RuleOutput{}, toolutil.ErrRequiredInt64("mrApprovalRuleUpdate", "approval_rule_id")
	}

	opts := &gl.UpdateMergeRequestApprovalRuleOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.ApprovalsRequired != nil {
		opts.ApprovalsRequired = input.ApprovalsRequired
	}
	if len(input.UserIDs) > 0 {
		opts.UserIDs = new(input.UserIDs)
	}
	if len(input.GroupIDs) > 0 {
		opts.GroupIDs = new(input.GroupIDs)
	}

	path := fmt.Sprintf("projects/%s/merge_requests/%d/approval_rules/%d", gl.PathEscape(string(input.ProjectID)), input.MRIID, input.ApprovalRuleID)
	rule, err := rawMutateApprovalRule(ctx, client, http.MethodPut, path, opts)
	if err != nil {
		return RuleOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalRuleUpdate", err, http.StatusNotFound,
			"verify approval_rule_id with gitlab_mr_approval_rules; requires Maintainer; cannot change rule_type after creation")
	}
	return rawRuleToOutput(rule), nil
}

// DeleteRule removes an approval rule from a merge request.
func DeleteRule(ctx context.Context, client *gitlabclient.Client, input DeleteRuleInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrApprovalRuleDelete: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrApprovalRuleDelete", "merge_request_iid")
	}
	if input.ApprovalRuleID <= 0 {
		return toolutil.ErrRequiredInt64("mrApprovalRuleDelete", "approval_rule_id")
	}
	_, err := client.GL().MergeRequestApprovals.DeleteApprovalRule(string(input.ProjectID), input.MRIID, input.ApprovalRuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("mrApprovalRuleDelete", err, http.StatusForbidden,
			"requires Maintainer role; verify approval_rule_id with gitlab_mr_approval_rules; deletion is irreversible")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
