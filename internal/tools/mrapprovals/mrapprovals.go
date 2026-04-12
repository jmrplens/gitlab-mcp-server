// Package mrapprovals implements MCP tool handlers for GitLab merge request
// approval operations including approval state, rules CRUD, configuration,
// approve, unapprove, and reset. It wraps the MergeRequestApprovals API.
package mrapprovals

import (
	"context"
	"errors"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// StateInput defines parameters for retrieving the approval state
// of a merge request.
type StateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// RulesInput defines parameters for listing the approval rules
// of a merge request.
type RulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// ConfigInput defines parameters for getting approval configuration.
type ConfigInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// ResetInput defines parameters for resetting approvals on a merge request.
type ResetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// CreateRuleInput defines parameters for creating an MR approval rule.
type CreateRuleInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"mr_iid"                   jsonschema:"Merge request internal ID,required"`
	Name                  string               `json:"name"                     jsonschema:"Rule name,required"`
	ApprovalsRequired     int64                `json:"approvals_required"       jsonschema:"Number of approvals required,required"`
	ApprovalProjectRuleID int64                `json:"approval_project_rule_id" jsonschema:"Project-level approval rule ID to inherit from"`
	UserIDs               []int64              `json:"user_ids"                 jsonschema:"User IDs eligible to approve"`
	GroupIDs              []int64              `json:"group_ids"                jsonschema:"Group IDs eligible to approve"`
}

// UpdateRuleInput defines parameters for updating an MR approval rule.
type UpdateRuleInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	MRIID             int64                `json:"mr_iid"             jsonschema:"Merge request internal ID,required"`
	ApprovalRuleID    int64                `json:"approval_rule_id"   jsonschema:"Approval rule ID,required"`
	Name              string               `json:"name"               jsonschema:"Rule name"`
	ApprovalsRequired *int64               `json:"approvals_required" jsonschema:"Number of approvals required"`
	UserIDs           []int64              `json:"user_ids"           jsonschema:"User IDs eligible to approve"`
	GroupIDs          []int64              `json:"group_ids"          jsonschema:"Group IDs eligible to approve"`
}

// DeleteRuleInput defines parameters for deleting an MR approval rule.
type DeleteRuleInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	MRIID          int64                `json:"mr_iid"           jsonschema:"Merge request internal ID,required"`
	ApprovalRuleID int64                `json:"approval_rule_id" jsonschema:"Approval rule ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// RuleOutput represents a single approval rule for a merge request.
type RuleOutput struct {
	toolutil.HintableOutput
	ID                   int64    `json:"id"`
	Name                 string   `json:"name"`
	RuleType             string   `json:"rule_type"`
	ReportType           string   `json:"report_type,omitempty"`
	Section              string   `json:"section,omitempty"`
	ApprovalsRequired    int      `json:"approvals_required"`
	Approved             bool     `json:"approved"`
	ContainsHiddenGroups bool     `json:"contains_hidden_groups,omitempty"`
	ApprovedByNames      []string `json:"approved_by_names,omitempty"`
	EligibleNames        []string `json:"eligible_names,omitempty"`
	UserNames            []string `json:"user_names,omitempty"`
	GroupNames           []string `json:"group_names,omitempty"`
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

// Approver holds approver identity and the timestamp of approval.
type Approver struct {
	Name       string `json:"name"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

// ConfigOutput holds the approval configuration for a merge request.
type ConfigOutput struct {
	toolutil.HintableOutput
	ID                   int64      `json:"id"`
	IID                  int64      `json:"mr_iid"`
	ProjectID            int64      `json:"project_id"`
	Title                string     `json:"title"`
	State                string     `json:"state"`
	Approved             bool       `json:"approved"`
	ApprovalsRequired    int64      `json:"approvals_required"`
	ApprovalsLeft        int64      `json:"approvals_left"`
	ApprovalsBeforeMerge int64      `json:"approvals_before_merge"`
	HasApprovalRules     bool       `json:"has_approval_rules"`
	UserHasApproved      bool       `json:"user_has_approved"`
	UserCanApprove       bool       `json:"user_can_approve"`
	ApprovedBy           []Approver `json:"approved_by,omitempty"`
	SuggestedNames       []string   `json:"suggested_approvers,omitempty"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// RuleToOutput converts a client-go MergeRequestApprovalRule to the
// MCP output representation.
func RuleToOutput(r *gl.MergeRequestApprovalRule) RuleOutput {
	out := RuleOutput{
		ID:                   r.ID,
		Name:                 r.Name,
		RuleType:             r.RuleType,
		ReportType:           r.ReportType,
		Section:              r.Section,
		ApprovalsRequired:    int(r.ApprovalsRequired),
		Approved:             r.Approved,
		ContainsHiddenGroups: r.ContainsHiddenGroups,
	}
	for _, u := range r.ApprovedBy {
		if u != nil {
			out.ApprovedByNames = append(out.ApprovedByNames, u.Name)
		}
	}
	for _, u := range r.EligibleApprovers {
		if u != nil {
			out.EligibleNames = append(out.EligibleNames, u.Name)
		}
	}
	for _, u := range r.Users {
		if u != nil {
			out.UserNames = append(out.UserNames, u.Name)
		}
	}
	for _, g := range r.Groups {
		if g != nil {
			out.GroupNames = append(out.GroupNames, g.Name)
		}
	}
	return out
}

// configToOutput converts a client-go MergeRequestApprovals to ConfigOutput.
func configToOutput(c *gl.MergeRequestApprovals) ConfigOutput {
	out := ConfigOutput{
		ID:                   c.ID,
		IID:                  c.IID,
		ProjectID:            c.ProjectID,
		Title:                c.Title,
		State:                c.State,
		Approved:             c.Approved,
		ApprovalsRequired:    c.ApprovalsRequired,
		ApprovalsLeft:        c.ApprovalsLeft,
		ApprovalsBeforeMerge: c.ApprovalsBeforeMerge,
		HasApprovalRules:     c.HasApprovalRules,
		UserHasApproved:      c.UserHasApproved,
		UserCanApprove:       c.UserCanApprove,
	}
	for _, u := range c.ApprovedBy {
		if u != nil && u.User != nil {
			a := Approver{Name: u.User.Name}
			if u.ApprovedAt != nil {
				a.ApprovedAt = u.ApprovedAt.Format("2006-01-02T15:04:05Z")
			}
			out.ApprovedBy = append(out.ApprovedBy, a)
		}
	}
	for _, u := range c.SuggestedApprovers {
		if u != nil {
			out.SuggestedNames = append(out.SuggestedNames, u.Name)
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
		return StateOutput{}, toolutil.ErrRequiredInt64("mrApprovalState", "mr_iid")
	}
	state, _, err := client.GL().MergeRequestApprovals.GetApprovalState(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 404) || toolutil.ContainsAny(err, "404") {
			return StateOutput{}, fmt.Errorf("mrApprovalState: merge request approval features require GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return StateOutput{}, toolutil.WrapErrWithMessage("mrApprovalState", err)
	}
	out := StateOutput{
		ApprovalRulesOverwritten: state.ApprovalRulesOverwritten,
	}
	for _, r := range state.Rules {
		out.Rules = append(out.Rules, RuleToOutput(r))
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
		return RulesOutput{}, toolutil.ErrRequiredInt64("mrApprovalRules", "mr_iid")
	}
	rules, _, err := client.GL().MergeRequestApprovals.GetApprovalRules(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 404) || toolutil.ContainsAny(err, "404") {
			return RulesOutput{}, fmt.Errorf("mrApprovalRules: merge request approval rules require GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return RulesOutput{}, toolutil.WrapErrWithMessage("mrApprovalRules", err)
	}
	out := RulesOutput{}
	for _, r := range rules {
		out.Rules = append(out.Rules, RuleToOutput(r))
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
		return ConfigOutput{}, toolutil.ErrRequiredInt64("mrApprovalConfig", "mr_iid")
	}
	cfg, _, err := client.GL().MergeRequestApprovals.GetConfiguration(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 404) || toolutil.ContainsAny(err, "404") {
			return ConfigOutput{}, fmt.Errorf("mrApprovalConfig: merge request approval configuration requires GitLab Premium or higher. This instance appears to be running Community Edition: %w", err)
		}
		return ConfigOutput{}, toolutil.WrapErrWithMessage("mrApprovalConfig", err)
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
		return toolutil.ErrRequiredInt64("mrApprovalReset", "mr_iid")
	}
	_, err := client.GL().MergeRequestApprovals.ResetApprovalsOfMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mrApprovalReset", err)
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
		return RuleOutput{}, toolutil.ErrRequiredInt64("mrApprovalRuleCreate", "mr_iid")
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

	rule, _, err := client.GL().MergeRequestApprovals.CreateApprovalRule(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return RuleOutput{}, toolutil.WrapErrWithMessage("mrApprovalRuleCreate", err)
	}
	return RuleToOutput(rule), nil
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
		return RuleOutput{}, toolutil.ErrRequiredInt64("mrApprovalRuleUpdate", "mr_iid")
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

	rule, _, err := client.GL().MergeRequestApprovals.UpdateApprovalRule(string(input.ProjectID), input.MRIID, input.ApprovalRuleID, opts, gl.WithContext(ctx))
	if err != nil {
		return RuleOutput{}, toolutil.WrapErrWithMessage("mrApprovalRuleUpdate", err)
	}
	return RuleToOutput(rule), nil
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
		return toolutil.ErrRequiredInt64("mrApprovalRuleDelete", "mr_iid")
	}
	if input.ApprovalRuleID <= 0 {
		return toolutil.ErrRequiredInt64("mrApprovalRuleDelete", "approval_rule_id")
	}
	_, err := client.GL().MergeRequestApprovals.DeleteApprovalRule(string(input.ProjectID), input.MRIID, input.ApprovalRuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mrApprovalRuleDelete", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
