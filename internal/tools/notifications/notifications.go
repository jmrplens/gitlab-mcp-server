// Package notifications implements MCP tools for GitLab notification settings.
package notifications

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// GetGlobalInput is the input for getting global notification settings.
type GetGlobalInput struct{}

// GetProjectInput is the input for getting project notification settings.
type GetProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// GetGroupInput is the input for getting group notification settings.
type GetGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// UpdateGlobalInput is the input for updating global notification settings.
type UpdateGlobalInput struct {
	Level                     string `json:"level,omitempty" jsonschema:"Notification level: disabled, participating, watch, global, mention, custom"`
	NotificationEmail         string `json:"notification_email,omitempty" jsonschema:"Email address for notifications"`
	CloseIssue                *bool  `json:"close_issue,omitempty" jsonschema:"Notify on issue close"`
	CloseMergeRequest         *bool  `json:"close_merge_request,omitempty" jsonschema:"Notify on MR close"`
	FailedPipeline            *bool  `json:"failed_pipeline,omitempty" jsonschema:"Notify on pipeline failure"`
	FixedPipeline             *bool  `json:"fixed_pipeline,omitempty" jsonschema:"Notify on pipeline fix"`
	IssueDue                  *bool  `json:"issue_due,omitempty" jsonschema:"Notify on issue due date"`
	MergeMergeRequest         *bool  `json:"merge_merge_request,omitempty" jsonschema:"Notify on MR merge"`
	MergeWhenPipelineSucceeds *bool  `json:"merge_when_pipeline_succeeds,omitempty" jsonschema:"Notify on merge when pipeline succeeds"`
	MovedProject              *bool  `json:"moved_project,omitempty" jsonschema:"Notify on project move"`
	NewEpic                   *bool  `json:"new_epic,omitempty" jsonschema:"Notify on new epic"`
	NewIssue                  *bool  `json:"new_issue,omitempty" jsonschema:"Notify on new issue"`
	NewMergeRequest           *bool  `json:"new_merge_request,omitempty" jsonschema:"Notify on new MR"`
	NewNote                   *bool  `json:"new_note,omitempty" jsonschema:"Notify on new note"`
	PushToMergeRequest        *bool  `json:"push_to_merge_request,omitempty" jsonschema:"Notify on push to MR"`
	ReassignIssue             *bool  `json:"reassign_issue,omitempty" jsonschema:"Notify on issue reassign"`
	ReassignMergeRequest      *bool  `json:"reassign_merge_request,omitempty" jsonschema:"Notify on MR reassign"`
	ReopenIssue               *bool  `json:"reopen_issue,omitempty" jsonschema:"Notify on issue reopen"`
	ReopenMergeRequest        *bool  `json:"reopen_merge_request,omitempty" jsonschema:"Notify on MR reopen"`
	SuccessPipeline           *bool  `json:"success_pipeline,omitempty" jsonschema:"Notify on pipeline success"`
}

// UpdateProjectInput is the input for updating project notification settings.
type UpdateProjectInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Level                     string               `json:"level,omitempty" jsonschema:"Notification level: disabled, participating, watch, global, mention, custom"`
	NotificationEmail         string               `json:"notification_email,omitempty" jsonschema:"Email address for notifications"`
	CloseIssue                *bool                `json:"close_issue,omitempty" jsonschema:"Notify on issue close"`
	CloseMergeRequest         *bool                `json:"close_merge_request,omitempty" jsonschema:"Notify on MR close"`
	FailedPipeline            *bool                `json:"failed_pipeline,omitempty" jsonschema:"Notify on pipeline failure"`
	FixedPipeline             *bool                `json:"fixed_pipeline,omitempty" jsonschema:"Notify on pipeline fix"`
	IssueDue                  *bool                `json:"issue_due,omitempty" jsonschema:"Notify on issue due date"`
	MergeMergeRequest         *bool                `json:"merge_merge_request,omitempty" jsonschema:"Notify on MR merge"`
	MergeWhenPipelineSucceeds *bool                `json:"merge_when_pipeline_succeeds,omitempty" jsonschema:"Notify on merge when pipeline succeeds"`
	MovedProject              *bool                `json:"moved_project,omitempty" jsonschema:"Notify on project move"`
	NewEpic                   *bool                `json:"new_epic,omitempty" jsonschema:"Notify on new epic"`
	NewIssue                  *bool                `json:"new_issue,omitempty" jsonschema:"Notify on new issue"`
	NewMergeRequest           *bool                `json:"new_merge_request,omitempty" jsonschema:"Notify on new MR"`
	NewNote                   *bool                `json:"new_note,omitempty" jsonschema:"Notify on new note"`
	PushToMergeRequest        *bool                `json:"push_to_merge_request,omitempty" jsonschema:"Notify on push to MR"`
	ReassignIssue             *bool                `json:"reassign_issue,omitempty" jsonschema:"Notify on issue reassign"`
	ReassignMergeRequest      *bool                `json:"reassign_merge_request,omitempty" jsonschema:"Notify on MR reassign"`
	ReopenIssue               *bool                `json:"reopen_issue,omitempty" jsonschema:"Notify on issue reopen"`
	ReopenMergeRequest        *bool                `json:"reopen_merge_request,omitempty" jsonschema:"Notify on MR reopen"`
	SuccessPipeline           *bool                `json:"success_pipeline,omitempty" jsonschema:"Notify on pipeline success"`
}

// UpdateGroupInput is the input for updating group notification settings.
type UpdateGroupInput struct {
	GroupID                   toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Level                     string               `json:"level,omitempty" jsonschema:"Notification level: disabled, participating, watch, global, mention, custom"`
	NotificationEmail         string               `json:"notification_email,omitempty" jsonschema:"Email address for notifications"`
	CloseIssue                *bool                `json:"close_issue,omitempty" jsonschema:"Notify on issue close"`
	CloseMergeRequest         *bool                `json:"close_merge_request,omitempty" jsonschema:"Notify on MR close"`
	FailedPipeline            *bool                `json:"failed_pipeline,omitempty" jsonschema:"Notify on pipeline failure"`
	FixedPipeline             *bool                `json:"fixed_pipeline,omitempty" jsonschema:"Notify on pipeline fix"`
	IssueDue                  *bool                `json:"issue_due,omitempty" jsonschema:"Notify on issue due date"`
	MergeMergeRequest         *bool                `json:"merge_merge_request,omitempty" jsonschema:"Notify on MR merge"`
	MergeWhenPipelineSucceeds *bool                `json:"merge_when_pipeline_succeeds,omitempty" jsonschema:"Notify on merge when pipeline succeeds"`
	MovedProject              *bool                `json:"moved_project,omitempty" jsonschema:"Notify on project move"`
	NewEpic                   *bool                `json:"new_epic,omitempty" jsonschema:"Notify on new epic"`
	NewIssue                  *bool                `json:"new_issue,omitempty" jsonschema:"Notify on new issue"`
	NewMergeRequest           *bool                `json:"new_merge_request,omitempty" jsonschema:"Notify on new MR"`
	NewNote                   *bool                `json:"new_note,omitempty" jsonschema:"Notify on new note"`
	PushToMergeRequest        *bool                `json:"push_to_merge_request,omitempty" jsonschema:"Notify on push to MR"`
	ReassignIssue             *bool                `json:"reassign_issue,omitempty" jsonschema:"Notify on issue reassign"`
	ReassignMergeRequest      *bool                `json:"reassign_merge_request,omitempty" jsonschema:"Notify on MR reassign"`
	ReopenIssue               *bool                `json:"reopen_issue,omitempty" jsonschema:"Notify on issue reopen"`
	ReopenMergeRequest        *bool                `json:"reopen_merge_request,omitempty" jsonschema:"Notify on MR reopen"`
	SuccessPipeline           *bool                `json:"success_pipeline,omitempty" jsonschema:"Notify on pipeline success"`
}

// Output type.

// Output represents notification settings.
type Output struct {
	toolutil.HintableOutput
	Level             string       `json:"level"`
	NotificationEmail string       `json:"notification_email,omitempty"`
	Events            *EventOutput `json:"events,omitempty"`
}

// EventOutput represents which events trigger notifications.
type EventOutput struct {
	CloseIssue                bool `json:"close_issue"`
	CloseMergeRequest         bool `json:"close_merge_request"`
	FailedPipeline            bool `json:"failed_pipeline"`
	FixedPipeline             bool `json:"fixed_pipeline"`
	IssueDue                  bool `json:"issue_due"`
	MergeWhenPipelineSucceeds bool `json:"merge_when_pipeline_succeeds"`
	MergeMergeRequest         bool `json:"merge_merge_request"`
	MovedProject              bool `json:"moved_project"`
	NewIssue                  bool `json:"new_issue"`
	NewMergeRequest           bool `json:"new_merge_request"`
	NewEpic                   bool `json:"new_epic"`
	NewNote                   bool `json:"new_note"`
	PushToMergeRequest        bool `json:"push_to_merge_request"`
	ReassignIssue             bool `json:"reassign_issue"`
	ReassignMergeRequest      bool `json:"reassign_merge_request"`
	ReopenIssue               bool `json:"reopen_issue"`
	ReopenMergeRequest        bool `json:"reopen_merge_request"`
	SuccessPipeline           bool `json:"success_pipeline"`
}

// Handlers.

// GetGlobalSettings gets global notification settings.
func GetGlobalSettings(ctx context.Context, client *gitlabclient.Client, _ GetGlobalInput) (Output, error) {
	settings, _, err := client.GL().NotificationSettings.GetGlobalSettings(gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_global_get", err)
	}
	return toOutput(settings), nil
}

// GetSettingsForProject gets notification settings for a project.
func GetSettingsForProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("notification_project_get", toolutil.ErrFieldRequired("project_id"))
	}
	settings, _, err := client.GL().NotificationSettings.GetSettingsForProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_project_get", err)
	}
	return toOutput(settings), nil
}

// GetSettingsForGroup gets notification settings for a group.
func GetSettingsForGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("notification_group_get", toolutil.ErrFieldRequired("group_id"))
	}
	settings, _, err := client.GL().NotificationSettings.GetSettingsForGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_group_get", err)
	}
	return toOutput(settings), nil
}

// UpdateGlobalSettings updates global notification settings.
func UpdateGlobalSettings(ctx context.Context, client *gitlabclient.Client, input UpdateGlobalInput) (Output, error) {
	opts := buildUpdateOpts(input.Level, input.NotificationEmail, input.CloseIssue, input.CloseMergeRequest,
		input.FailedPipeline, input.FixedPipeline, input.IssueDue, input.MergeMergeRequest,
		input.MergeWhenPipelineSucceeds, input.MovedProject, input.NewEpic, input.NewIssue,
		input.NewMergeRequest, input.NewNote, input.PushToMergeRequest, input.ReassignIssue,
		input.ReassignMergeRequest, input.ReopenIssue, input.ReopenMergeRequest, input.SuccessPipeline)
	settings, _, err := client.GL().NotificationSettings.UpdateGlobalSettings(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_global_update", err)
	}
	return toOutput(settings), nil
}

// UpdateSettingsForProject updates notification settings for a project.
func UpdateSettingsForProject(ctx context.Context, client *gitlabclient.Client, input UpdateProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("notification_project_update", toolutil.ErrFieldRequired("project_id"))
	}
	opts := buildUpdateOpts(input.Level, input.NotificationEmail, input.CloseIssue, input.CloseMergeRequest,
		input.FailedPipeline, input.FixedPipeline, input.IssueDue, input.MergeMergeRequest,
		input.MergeWhenPipelineSucceeds, input.MovedProject, input.NewEpic, input.NewIssue,
		input.NewMergeRequest, input.NewNote, input.PushToMergeRequest, input.ReassignIssue,
		input.ReassignMergeRequest, input.ReopenIssue, input.ReopenMergeRequest, input.SuccessPipeline)
	settings, _, err := client.GL().NotificationSettings.UpdateSettingsForProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_project_update", err)
	}
	return toOutput(settings), nil
}

// UpdateSettingsForGroup updates notification settings for a group.
func UpdateSettingsForGroup(ctx context.Context, client *gitlabclient.Client, input UpdateGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("notification_group_update", toolutil.ErrFieldRequired("group_id"))
	}
	opts := buildUpdateOpts(input.Level, input.NotificationEmail, input.CloseIssue, input.CloseMergeRequest,
		input.FailedPipeline, input.FixedPipeline, input.IssueDue, input.MergeMergeRequest,
		input.MergeWhenPipelineSucceeds, input.MovedProject, input.NewEpic, input.NewIssue,
		input.NewMergeRequest, input.NewNote, input.PushToMergeRequest, input.ReassignIssue,
		input.ReassignMergeRequest, input.ReopenIssue, input.ReopenMergeRequest, input.SuccessPipeline)
	settings, _, err := client.GL().NotificationSettings.UpdateSettingsForGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("notification_group_update", err)
	}
	return toOutput(settings), nil
}

// Helpers.

var levelMap = map[string]gl.NotificationLevelValue{
	"disabled":      gl.DisabledNotificationLevel,
	"participating": gl.ParticipatingNotificationLevel,
	"watch":         gl.WatchNotificationLevel,
	"global":        gl.GlobalNotificationLevel,
	"mention":       gl.MentionNotificationLevel,
	"custom":        gl.CustomNotificationLevel,
}

// buildUpdateOpts constructs the request parameters from the input.
func buildUpdateOpts(level, email string, closeIssue, closeMR, failedPipeline, fixedPipeline, issueDue, mergeMR, mergeWhenPipeline, movedProject, newEpic, newIssue, newMR, newNote, pushToMR, reassignIssue, reassignMR, reopenIssue, reopenMR, successPipeline *bool) *gl.NotificationSettingsOptions {
	opts := &gl.NotificationSettingsOptions{}
	if level != "" {
		if lv, ok := levelMap[level]; ok {
			opts.Level = &lv
		}
	}
	if email != "" {
		opts.NotificationEmail = &email
	}
	opts.CloseIssue = closeIssue
	opts.CloseMergeRequest = closeMR
	opts.FailedPipeline = failedPipeline
	opts.FixedPipeline = fixedPipeline
	opts.IssueDue = issueDue
	opts.MergeMergeRequest = mergeMR
	opts.MergeWhenPipelineSucceeds = mergeWhenPipeline
	opts.MovedProject = movedProject
	opts.NewEpic = newEpic
	opts.NewIssue = newIssue
	opts.NewMergeRequest = newMR
	opts.NewNote = newNote
	opts.PushToMergeRequest = pushToMR
	opts.ReassignIssue = reassignIssue
	opts.ReassignMergeRequest = reassignMR
	opts.ReopenIssue = reopenIssue
	opts.ReopenMergeRequest = reopenMR
	opts.SuccessPipeline = successPipeline
	return opts
}

// Converters.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(s *gl.NotificationSettings) Output {
	out := Output{
		Level:             s.Level.String(),
		NotificationEmail: s.NotificationEmail,
	}
	if s.Events != nil {
		out.Events = &EventOutput{
			CloseIssue:                s.Events.CloseIssue,
			CloseMergeRequest:         s.Events.CloseMergeRequest,
			FailedPipeline:            s.Events.FailedPipeline,
			FixedPipeline:             s.Events.FixedPipeline,
			IssueDue:                  s.Events.IssueDue,
			MergeWhenPipelineSucceeds: s.Events.MergeWhenPipelineSucceeds,
			MergeMergeRequest:         s.Events.MergeMergeRequest,
			MovedProject:              s.Events.MovedProject,
			NewIssue:                  s.Events.NewIssue,
			NewMergeRequest:           s.Events.NewMergeRequest,
			NewEpic:                   s.Events.NewEpic,
			NewNote:                   s.Events.NewNote,
			PushToMergeRequest:        s.Events.PushToMergeRequest,
			ReassignIssue:             s.Events.ReassignIssue,
			ReassignMergeRequest:      s.Events.ReassignMergeRequest,
			ReopenIssue:               s.Events.ReopenIssue,
			ReopenMergeRequest:        s.Events.ReopenMergeRequest,
			SuccessPipeline:           s.Events.SuccessPipeline,
		}
	}
	return out
}

// Formatters.

// eventLine is an internal helper for the notifications package.
func eventLine(name string, enabled bool) string {
	return fmt.Sprintf("- %s %s\n", toolutil.BoolEmoji(enabled), name)
}
