package mrapprovals

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
//
// The three optional fields carry omitempty because the schema generator
// marks a field required unless the tag says it may be absent: without it
// the individual surface, which validates arguments against the schema
// before any handler runs, refused every create that named no source rule
// and no approvers, which is the ordinary one. The dispatcher surfaces
// decode the arguments directly and never saw the refusal.
type CreateRuleInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                         jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"merge_request_iid"                  jsonschema:"Merge request internal ID,required"`
	Name                  string               `json:"name"                               jsonschema:"Rule name,required"`
	ApprovalsRequired     int64                `json:"approvals_required"                 jsonschema:"Number of approvals required,required"`
	ApprovalProjectRuleID int64                `json:"approval_project_rule_id,omitempty" jsonschema:"Project-level approval rule ID to inherit from"`
	UserIDs               []int64              `json:"user_ids,omitempty"                 jsonschema:"User IDs eligible to approve"`
	GroupIDs              []int64              `json:"group_ids,omitempty"                jsonschema:"Group IDs eligible to approve"`
}

// UpdateRuleInput defines parameters for updating an MR approval rule. Every
// field but the three that name the rule is optional, and says so for the
// reason [CreateRuleInput] gives.
type UpdateRuleInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id"                   jsonschema:"Project ID or URL-encoded path,required"`
	MRIID             int64                `json:"merge_request_iid"            jsonschema:"Merge request internal ID,required"`
	ApprovalRuleID    int64                `json:"approval_rule_id"             jsonschema:"Approval rule ID,required"`
	Name              string               `json:"name,omitempty"               jsonschema:"Rule name"`
	ApprovalsRequired *int64               `json:"approvals_required,omitempty" jsonschema:"Number of approvals required"`
	UserIDs           []int64              `json:"user_ids,omitempty"           jsonschema:"User IDs eligible to approve"`
	GroupIDs          []int64              `json:"group_ids,omitempty"          jsonschema:"Group IDs eligible to approve"`
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
	ContainsHiddenGroups bool                       `json:"contains_hidden_groups,omitempty"`
	Overridden           bool                       `json:"overridden,omitempty"`
	EligibleApprovers    []*BasicUserOutput         `json:"eligible_approvers,omitempty"`
	Users                []*BasicUserOutput         `json:"users,omitempty"`
	Groups               []*GroupOutput             `json:"groups,omitempty"`
	SourceRule           *ProjectApprovalRuleOutput `json:"source_rule,omitempty"`
}

// StateRuleOutput is a rule as the approval-state endpoint renders it: the
// rule, plus whether it is satisfied and by whom.
//
// It exists because one Go type used to serve two GitLab entities.
// ee/lib/api/entities/merge_request_approval_state_rule.rb adds approved and
// approved_by, and only GET .../approval_state presents it; the three
// approval_rules routes present MergeRequestApprovalRule, which adds section,
// source_rule and overridden and neither of these. Publishing both on one type
// meant every rules row asserted "approved": false, since that field carried no
// omitempty, which is the shape of issue 580 one entity over.
type StateRuleOutput struct {
	RuleOutput
	Approved   bool               `json:"approved"`
	ApprovedBy []*BasicUserOutput `json:"approved_by,omitempty"`
}

// StateOutput holds the overall approval state for a merge request,
// including whether rules have been overridden and the list of applicable rules.
type StateOutput struct {
	toolutil.HintableOutput
	ApprovalRulesOverwritten bool              `json:"approval_rules_overwritten"`
	Rules                    []StateRuleOutput `json:"rules"`
}

// RulesOutput holds the list of approval rules for a merge request.
type RulesOutput struct {
	toolutil.HintableOutput
	Rules []RuleOutput `json:"rules"`
}

// ConfigOutput holds what GitLab answers at
// GET /projects/:id/merge_requests/:merge_request_iid/approvals, which is four
// fields on every tier.
//
// It used to carry all twenty-four of gl.MergeRequestApprovals, because the 1:1
// norm mirrors the SDK type and that type models the response of the POST at
// the same path, deprecated in GitLab 16.0, which this action does not call.
// GitLab's own generated OpenAPI document separates the two: the GET declares
// approved, approved_by, user_can_approve and user_has_approved, and every one
// of the other twenty appears only under the POST. So a model was told to
// expect an id, a title, a state and an approvals_required it would never
// receive, and thirteen of those arrived in the payload as zeroes because they
// carry no omitempty. Mirroring the SDK is mirroring a second model of the API,
// not the API, which is the whole reason cmd/gen_api_live exists.
type ConfigOutput struct {
	toolutil.HintableOutput
	Approved        bool                              `json:"approved"`
	UserHasApproved bool                              `json:"user_has_approved"`
	UserCanApprove  bool                              `json:"user_can_approve"`
	ApprovedBy      []*MergeRequestApproverUserOutput `json:"approved_by,omitempty"`
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
		ContainsHiddenGroups: r.ContainsHiddenGroups,
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
		ContainsHiddenGroups: r.ContainsHiddenGroups,
		Overridden:           r.Overridden,
		EligibleApprovers:    basicUserOutputs(r.EligibleApprovers),
		Users:                basicUserOutputs(r.Users),
		Groups:               groupOutputs(r.Groups),
		SourceRule:           projectApprovalRuleOutput(r.SourceRule),
	}
}

// rawStateRuleToOutput converts a raw-fetch rule as the approval-state
// endpoint renders it, wrapping [rawRuleToOutput] with the two fields only
// that entity adds.
func rawStateRuleToOutput(r *mergeRequestApprovalRuleAPI) StateRuleOutput {
	return StateRuleOutput{
		RuleOutput: rawRuleToOutput(r),
		Approved:   r.Approved,
		ApprovedBy: basicUserOutputs(r.ApprovedBy),
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
// taking the four fields the GET actually answers with.
//
// The SDK type carries twenty more, and they are deliberately dropped rather
// than forwarded: they belong to the deprecated POST at the same path, so on a
// GET they are the zero value whatever the tier, and publishing a zero is worse
// than publishing nothing. [ConfigOutput] records the whole reasoning.
func configToOutput(c *gl.MergeRequestApprovals) ConfigOutput {
	return ConfigOutput{
		Approved:        c.Approved,
		UserHasApproved: c.UserHasApproved,
		UserCanApprove:  c.UserCanApprove,
		ApprovedBy:      approverUserOutputs(c.ApprovedBy),
	}
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
			out.Rules = append(out.Rules, rawStateRuleToOutput(r))
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

// Config reports who has approved a merge request and whether the calling user
// can and has.
//
// It is not a Premium action, though it long said so: GitLab answers this
// endpoint on Community Edition, and the approval *rules* that do need Premium
// are [Rules] and [State].
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
			return ConfigOutput{}, toolutil.WrapErrWithHint("mrApprovalConfig", err,
				"verify project_id + merge_request_iid with gitlab_mr_list; this endpoint is available on every tier, so a 404 is a wrong identifier rather than a missing license")
		}
		return ConfigOutput{}, toolutil.WrapErrWithStatusHint("mrApprovalConfig", err, http.StatusForbidden,
			"the caller must be able to see the merge request; verify project_id + merge_request_iid")
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
