// Package mrapprovalsettings implements MCP tool handlers for GitLab
// merge request approval settings at project and group level.
// It wraps the MergeRequestApprovalSettings API.
package mrapprovalsettings

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// GroupGetInput defines parameters for retrieving group-level MR approval settings.
type GroupGetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// GroupUpdateInput defines parameters for updating group-level MR approval settings.
type GroupUpdateInput struct {
	GroupID                                     toolutil.StringOrInt `json:"group_id"                                        jsonschema:"Group ID or URL-encoded path,required"`
	AllowAuthorApproval                         *bool                `json:"allow_author_approval,omitempty"                  jsonschema:"Allow merge request authors to approve their own MRs"`
	AllowCommitterApproval                      *bool                `json:"allow_committer_approval,omitempty"               jsonschema:"Allow committers to approve MRs they contributed to"`
	AllowOverridesToApproverListPerMergeRequest *bool                `json:"allow_overrides_approver_list_per_mr,omitempty"    jsonschema:"Allow overriding approver list per merge request"`
	RetainApprovalsOnPush                       *bool                `json:"retain_approvals_on_push,omitempty"               jsonschema:"Retain approvals when new commits are pushed"`
	RequireReauthenticationToApprove            *bool                `json:"require_reauthentication_to_approve,omitempty"    jsonschema:"Require password re-entry to approve"`
}

// ProjectGetInput defines parameters for retrieving project-level MR approval settings.
type ProjectGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ProjectUpdateInput defines parameters for updating project-level MR approval settings.
type ProjectUpdateInput struct {
	ProjectID                                   toolutil.StringOrInt `json:"project_id"                                      jsonschema:"Project ID or URL-encoded path,required"`
	AllowAuthorApproval                         *bool                `json:"allow_author_approval,omitempty"                  jsonschema:"Allow merge request authors to approve their own MRs"`
	AllowCommitterApproval                      *bool                `json:"allow_committer_approval,omitempty"               jsonschema:"Allow committers to approve MRs they contributed to"`
	AllowOverridesToApproverListPerMergeRequest *bool                `json:"allow_overrides_approver_list_per_mr,omitempty"    jsonschema:"Allow overriding approver list per merge request"`
	RetainApprovalsOnPush                       *bool                `json:"retain_approvals_on_push,omitempty"               jsonschema:"Retain approvals when new commits are pushed"`
	RequireReauthenticationToApprove            *bool                `json:"require_reauthentication_to_approve,omitempty"    jsonschema:"Require password re-entry to approve"`
	SelectiveCodeOwnerRemovals                  *bool                `json:"selective_code_owner_removals,omitempty"          jsonschema:"Only remove Code Owner approvals for changed files (project-only)"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// SettingOutput represents a single approval setting with its value,
// lock status, and inheritance source.
type SettingOutput struct {
	Value         bool   `json:"value"`
	Locked        bool   `json:"locked"`
	InheritedFrom string `json:"inherited_from,omitempty"`
}

// Output represents the full set of MR approval settings for a group or project.
type Output struct {
	toolutil.HintableOutput
	AllowAuthorApproval                         SettingOutput `json:"allow_author_approval"`
	AllowCommitterApproval                      SettingOutput `json:"allow_committer_approval"`
	AllowOverridesToApproverListPerMergeRequest SettingOutput `json:"allow_overrides_approver_list_per_mr"`
	RetainApprovalsOnPush                       SettingOutput `json:"retain_approvals_on_push"`
	SelectiveCodeOwnerRemovals                  SettingOutput `json:"selective_code_owner_removals"`
	RequirePasswordToApprove                    SettingOutput `json:"require_password_to_approve"`
	RequireReauthenticationToApprove            SettingOutput `json:"require_reauthentication_to_approve"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

func settingToOutput(s gl.MergeRequestApprovalSetting) SettingOutput {
	return SettingOutput{
		Value:         s.Value,
		Locked:        s.Locked,
		InheritedFrom: s.InheritedFrom,
	}
}

func toOutput(s *gl.MergeRequestApprovalSettings) Output {
	return Output{
		AllowAuthorApproval:                         settingToOutput(s.AllowAuthorApproval),
		AllowCommitterApproval:                      settingToOutput(s.AllowCommitterApproval),
		AllowOverridesToApproverListPerMergeRequest: settingToOutput(s.AllowOverridesToApproverListPerMergeRequest),
		RetainApprovalsOnPush:                       settingToOutput(s.RetainApprovalsOnPush),
		SelectiveCodeOwnerRemovals:                  settingToOutput(s.SelectiveCodeOwnerRemovals),
		RequirePasswordToApprove:                    settingToOutput(s.RequirePasswordToApprove),
		RequireReauthenticationToApprove:            settingToOutput(s.RequireReauthenticationToApprove),
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// GetGroupSettings retrieves the MR approval settings for a group.
func GetGroupSettings(ctx context.Context, client *gitlabclient.Client, input GroupGetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	settings, _, err := client.GL().MergeRequestApprovalSettings.GetGroupMergeRequestApprovalSettings(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_get_group_mr_approval_settings", err, http.StatusNotFound, "verify group_id \u2014 requires Owner or Maintainer role")
	}
	return toOutput(settings), nil
}

// UpdateGroupSettings updates the MR approval settings for a group.
func UpdateGroupSettings(ctx context.Context, client *gitlabclient.Client, input GroupUpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.UpdateGroupMergeRequestApprovalSettingsOptions{
		AllowAuthorApproval:                         input.AllowAuthorApproval,
		AllowCommitterApproval:                      input.AllowCommitterApproval,
		AllowOverridesToApproverListPerMergeRequest: input.AllowOverridesToApproverListPerMergeRequest,
		RetainApprovalsOnPush:                       input.RetainApprovalsOnPush,
		RequireReauthenticationToApprove:            input.RequireReauthenticationToApprove,
	}
	settings, _, err := client.GL().MergeRequestApprovalSettings.UpdateGroupMergeRequestApprovalSettings(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_update_group_mr_approval_settings", err, http.StatusNotFound, "verify group_id \u2014 requires Owner role to update approval settings")
	}
	return toOutput(settings), nil
}

// GetProjectSettings retrieves the MR approval settings for a project.
func GetProjectSettings(ctx context.Context, client *gitlabclient.Client, input ProjectGetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	settings, _, err := client.GL().MergeRequestApprovalSettings.GetProjectMergeRequestApprovalSettings(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_get_project_mr_approval_settings", err, http.StatusNotFound, "verify project_id \u2014 requires Maintainer or Owner role")
	}
	return toOutput(settings), nil
}

// UpdateProjectSettings updates the MR approval settings for a project.
func UpdateProjectSettings(ctx context.Context, client *gitlabclient.Client, input ProjectUpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.UpdateProjectMergeRequestApprovalSettingsOptions{
		AllowAuthorApproval:                         input.AllowAuthorApproval,
		AllowCommitterApproval:                      input.AllowCommitterApproval,
		AllowOverridesToApproverListPerMergeRequest: input.AllowOverridesToApproverListPerMergeRequest,
		RetainApprovalsOnPush:                       input.RetainApprovalsOnPush,
		RequireReauthenticationToApprove:            input.RequireReauthenticationToApprove,
		SelectiveCodeOwnerRemovals:                  input.SelectiveCodeOwnerRemovals,
	}
	settings, _, err := client.GL().MergeRequestApprovalSettings.UpdateProjectMergeRequestApprovalSettings(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_update_project_mr_approval_settings", err, http.StatusNotFound, "verify project_id \u2014 requires Maintainer or Owner role")
	}
	return toOutput(settings), nil
}
