// hooks.go implements GitLab group webhook operations including list, get,
// add, edit, and delete. It exposes typed input/output structs and handler
// functions that interact with the GitLab Group Webhooks API v4.

package groups

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// HookInput defines common parameters for creating or editing a group hook.
type HookInput struct {
	URL                      string `json:"url,omitempty"                        jsonschema:"Webhook URL (required for add)"`
	Name                     string `json:"name,omitempty"                       jsonschema:"Hook name"`
	Description              string `json:"description,omitempty"                jsonschema:"Hook description"`
	Token                    string `json:"token,omitempty"                      jsonschema:"Secret token for payload validation"`
	PushEvents               *bool  `json:"push_events,omitempty"                jsonschema:"Trigger on push events"`
	TagPushEvents            *bool  `json:"tag_push_events,omitempty"            jsonschema:"Trigger on tag push events"`
	MergeRequestsEvents      *bool  `json:"merge_requests_events,omitempty"      jsonschema:"Trigger on merge request events"`
	IssuesEvents             *bool  `json:"issues_events,omitempty"              jsonschema:"Trigger on issue events"`
	NoteEvents               *bool  `json:"note_events,omitempty"                jsonschema:"Trigger on comment events"`
	JobEvents                *bool  `json:"job_events,omitempty"                 jsonschema:"Trigger on job events"`
	PipelineEvents           *bool  `json:"pipeline_events,omitempty"            jsonschema:"Trigger on pipeline events"`
	WikiPageEvents           *bool  `json:"wiki_page_events,omitempty"           jsonschema:"Trigger on wiki page events"`
	DeploymentEvents         *bool  `json:"deployment_events,omitempty"          jsonschema:"Trigger on deployment events"`
	ReleasesEvents           *bool  `json:"releases_events,omitempty"            jsonschema:"Trigger on release events"`
	SubGroupEvents           *bool  `json:"subgroup_events,omitempty"            jsonschema:"Trigger on subgroup events"`
	MemberEvents             *bool  `json:"member_events,omitempty"              jsonschema:"Trigger on member events"`
	ConfidentialIssuesEvents *bool  `json:"confidential_issues_events,omitempty" jsonschema:"Trigger on confidential issue events"`
	ConfidentialNoteEvents   *bool  `json:"confidential_note_events,omitempty"   jsonschema:"Trigger on confidential note events"`
	EnableSSLVerification    *bool  `json:"enable_ssl_verification,omitempty"    jsonschema:"Enable SSL verification for the hook endpoint"`
	PushEventsBranchFilter   string `json:"push_events_branch_filter,omitempty"  jsonschema:"Branch filter for push events (e.g. 'main')"`
}

// ListHooksInput defines parameters for listing group hooks.
type ListHooksInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetHookInput defines parameters for retrieving a single group hook.
type GetHookInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Hook ID,required"`
}

// AddHookInput defines parameters for adding a new group hook.
type AddHookInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookInput
}

// EditHookInput defines parameters for editing an existing group hook.
type EditHookInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Hook ID to edit,required"`
	HookInput
}

// DeleteHookInput defines parameters for deleting a group hook.
type DeleteHookInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Hook ID to delete,required"`
}

// HookOutput represents a GitLab group webhook.
type HookOutput struct {
	toolutil.HintableOutput
	ID                       int64  `json:"id"`
	URL                      string `json:"url"`
	Name                     string `json:"name,omitempty"`
	Description              string `json:"description,omitempty"`
	GroupID                  int64  `json:"group_id"`
	PushEvents               bool   `json:"push_events"`
	TagPushEvents            bool   `json:"tag_push_events"`
	MergeRequestsEvents      bool   `json:"merge_requests_events"`
	IssuesEvents             bool   `json:"issues_events"`
	NoteEvents               bool   `json:"note_events"`
	JobEvents                bool   `json:"job_events"`
	PipelineEvents           bool   `json:"pipeline_events"`
	WikiPageEvents           bool   `json:"wiki_page_events"`
	DeploymentEvents         bool   `json:"deployment_events"`
	ReleasesEvents           bool   `json:"releases_events"`
	SubGroupEvents           bool   `json:"subgroup_events"`
	MemberEvents             bool   `json:"member_events"`
	ConfidentialIssuesEvents bool   `json:"confidential_issues_events"`
	ConfidentialNoteEvents   bool   `json:"confidential_note_events"`
	EnableSSLVerification    bool   `json:"enable_ssl_verification"`
	AlertStatus              string `json:"alert_status,omitempty"`
	CreatedAt                string `json:"created_at,omitempty"`
}

// HookListOutput holds a paginated list of group hooks.
type HookListOutput struct {
	toolutil.HintableOutput
	Hooks      []HookOutput              `json:"hooks"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// hookToOutput converts a GitLab API [gl.GroupHook] to the MCP tool output format.
func hookToOutput(h *gl.GroupHook) HookOutput {
	out := HookOutput{
		ID:                       h.ID,
		URL:                      h.URL,
		Name:                     h.Name,
		Description:              h.Description,
		GroupID:                  h.GroupID,
		PushEvents:               h.PushEvents,
		TagPushEvents:            h.TagPushEvents,
		MergeRequestsEvents:      h.MergeRequestsEvents,
		IssuesEvents:             h.IssuesEvents,
		NoteEvents:               h.NoteEvents,
		JobEvents:                h.JobEvents,
		PipelineEvents:           h.PipelineEvents,
		WikiPageEvents:           h.WikiPageEvents,
		DeploymentEvents:         h.DeploymentEvents,
		ReleasesEvents:           h.ReleasesEvents,
		SubGroupEvents:           h.SubGroupEvents,
		MemberEvents:             h.MemberEvents,
		ConfidentialIssuesEvents: h.ConfidentialIssuesEvents,
		ConfidentialNoteEvents:   h.ConfidentialNoteEvents,
		EnableSSLVerification:    h.EnableSSLVerification,
		AlertStatus:              h.AlertStatus,
	}
	if h.CreatedAt != nil {
		out.CreatedAt = h.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// ListHooks retrieves a paginated list of webhooks for a group.
func ListHooks(ctx context.Context, client *gitlabclient.Client, input ListHooksInput) (HookListOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookListOutput{}, err
	}
	if input.GroupID == "" {
		return HookListOutput{}, errors.New("ListHooks: group_id is required")
	}

	opts := &gl.ListGroupHooksOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	hooks, resp, err := client.GL().Groups.ListGroupHooks(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return HookListOutput{}, toolutil.WrapErrWithStatusHint("ListHooks", err, http.StatusForbidden,
			"requires Owner role on the group; verify group_id with gitlab_group_list; group webhooks fire for events in the group and all its subgroups/projects")
	}

	out := HookListOutput{
		Hooks:      make([]HookOutput, len(hooks)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, h := range hooks {
		out.Hooks[i] = hookToOutput(h)
	}
	return out, nil
}

// GetHook retrieves a single group webhook by its ID.
func GetHook(ctx context.Context, client *gitlabclient.Client, input GetHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.GroupID == "" {
		return HookOutput{}, errors.New("GetHook: group_id is required")
	}
	if input.HookID <= 0 {
		return HookOutput{}, toolutil.ErrRequiredInt64("GetHook", "hook_id")
	}

	h, _, err := client.GL().Groups.GetGroupHook(string(input.GroupID), input.HookID, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithStatusHint("GetHook", err, http.StatusNotFound,
			"verify group_id + hook_id with gitlab_group_hook_list; requires Owner role")
	}
	return hookToOutput(h), nil
}

// applyAddHookOpts builds the AddGroupHookOptions from HookInput.
func applyAddHookOpts(input HookInput) *gl.AddGroupHookOptions {
	opts := &gl.AddGroupHookOptions{}
	applyAddHookIdentity(input, opts)
	applyAddHookEvents(input, opts)
	return opts
}

// applyAddHookIdentity is an internal helper for the groups package.
func applyAddHookIdentity(input HookInput, opts *gl.AddGroupHookOptions) {
	if input.URL != "" {
		opts.URL = new(input.URL)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Token != "" {
		opts.Token = new(input.Token)
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
}

// applyAddHookEvents is an internal helper for the groups package.
func applyAddHookEvents(input HookInput, opts *gl.AddGroupHookOptions) {
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.IssuesEvents != nil {
		opts.IssuesEvents = input.IssuesEvents
	}
	if input.NoteEvents != nil {
		opts.NoteEvents = input.NoteEvents
	}
	if input.JobEvents != nil {
		opts.JobEvents = input.JobEvents
	}
	if input.PipelineEvents != nil {
		opts.PipelineEvents = input.PipelineEvents
	}
	if input.WikiPageEvents != nil {
		opts.WikiPageEvents = input.WikiPageEvents
	}
	if input.DeploymentEvents != nil {
		opts.DeploymentEvents = input.DeploymentEvents
	}
	if input.ReleasesEvents != nil {
		opts.ReleasesEvents = input.ReleasesEvents
	}
	if input.SubGroupEvents != nil {
		opts.SubGroupEvents = input.SubGroupEvents
	}
	if input.MemberEvents != nil {
		opts.MemberEvents = input.MemberEvents
	}
	if input.ConfidentialIssuesEvents != nil {
		opts.ConfidentialIssuesEvents = input.ConfidentialIssuesEvents
	}
	if input.ConfidentialNoteEvents != nil {
		opts.ConfidentialNoteEvents = input.ConfidentialNoteEvents
	}
}

// applyEditHookOpts builds the EditGroupHookOptions from HookInput.
func applyEditHookOpts(input HookInput) *gl.EditGroupHookOptions {
	opts := &gl.EditGroupHookOptions{}
	applyEditHookIdentity(input, opts)
	applyEditHookEvents(input, opts)
	return opts
}

// applyEditHookIdentity is an internal helper for the groups package.
func applyEditHookIdentity(input HookInput, opts *gl.EditGroupHookOptions) {
	if input.URL != "" {
		opts.URL = new(input.URL)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Token != "" {
		opts.Token = new(input.Token)
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
}

// applyEditHookEvents is an internal helper for the groups package.
func applyEditHookEvents(input HookInput, opts *gl.EditGroupHookOptions) {
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.IssuesEvents != nil {
		opts.IssuesEvents = input.IssuesEvents
	}
	if input.NoteEvents != nil {
		opts.NoteEvents = input.NoteEvents
	}
	if input.JobEvents != nil {
		opts.JobEvents = input.JobEvents
	}
	if input.PipelineEvents != nil {
		opts.PipelineEvents = input.PipelineEvents
	}
	if input.WikiPageEvents != nil {
		opts.WikiPageEvents = input.WikiPageEvents
	}
	if input.DeploymentEvents != nil {
		opts.DeploymentEvents = input.DeploymentEvents
	}
	if input.ReleasesEvents != nil {
		opts.ReleasesEvents = input.ReleasesEvents
	}
	if input.SubGroupEvents != nil {
		opts.SubGroupEvents = input.SubGroupEvents
	}
	if input.MemberEvents != nil {
		opts.MemberEvents = input.MemberEvents
	}
	if input.ConfidentialIssuesEvents != nil {
		opts.ConfidentialIssuesEvents = input.ConfidentialIssuesEvents
	}
	if input.ConfidentialNoteEvents != nil {
		opts.ConfidentialNoteEvents = input.ConfidentialNoteEvents
	}
}

// AddHook adds a new webhook to a group. Requires the webhook URL.
func AddHook(ctx context.Context, client *gitlabclient.Client, input AddHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.GroupID == "" {
		return HookOutput{}, errors.New("AddHook: group_id is required")
	}
	if input.URL == "" {
		return HookOutput{}, errors.New("AddHook: url is required")
	}

	opts := applyAddHookOpts(input.HookInput)

	h, _, err := client.GL().Groups.AddGroupHook(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithStatusHint("AddHook", err, http.StatusBadRequest,
			"requires Owner role; url must be HTTP(S) and reachable; token is shared secret for X-Gitlab-Token header; enable specific event flags (push_events, merge_requests_events, etc.); enable_ssl_verification recommended")
	}
	return hookToOutput(h), nil
}

// EditHook updates an existing group webhook configuration.
func EditHook(ctx context.Context, client *gitlabclient.Client, input EditHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.GroupID == "" {
		return HookOutput{}, errors.New("EditHook: group_id is required")
	}
	if input.HookID <= 0 {
		return HookOutput{}, toolutil.ErrRequiredInt64("EditHook", "hook_id")
	}

	opts := applyEditHookOpts(input.HookInput)

	h, _, err := client.GL().Groups.EditGroupHook(string(input.GroupID), input.HookID, opts, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithStatusHint("EditHook", err, http.StatusNotFound,
			"verify hook_id with gitlab_group_hook_list; requires Owner role; updates merge with existing config \u2014 unset fields keep current values")
	}
	return hookToOutput(h), nil
}

// DeleteHook removes a webhook from a group.
func DeleteHook(ctx context.Context, client *gitlabclient.Client, input DeleteHookInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("DeleteHook: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("DeleteHook", "hook_id")
	}

	_, err := client.GL().Groups.DeleteGroupHook(string(input.GroupID), input.HookID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("DeleteHook", err, http.StatusForbidden,
			"requires Owner role; verify hook_id with gitlab_group_hook_list; deletion is irreversible")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Hook Markdown formatters
// ---------------------------------------------------------------------------.

// enabledEvents returns a comma-separated list of enabled event types.
func enabledEvents(h HookOutput) string {
	var events []string
	if h.PushEvents {
		events = append(events, "push")
	}
	if h.TagPushEvents {
		events = append(events, "tag_push")
	}
	if h.MergeRequestsEvents {
		events = append(events, "merge_request")
	}
	if h.IssuesEvents {
		events = append(events, "issues")
	}
	if h.NoteEvents {
		events = append(events, "note")
	}
	if h.JobEvents {
		events = append(events, "job")
	}
	if h.PipelineEvents {
		events = append(events, "pipeline")
	}
	if h.WikiPageEvents {
		events = append(events, "wiki")
	}
	if h.DeploymentEvents {
		events = append(events, "deployment")
	}
	if h.ReleasesEvents {
		events = append(events, "releases")
	}
	if h.SubGroupEvents {
		events = append(events, "subgroup")
	}
	if h.MemberEvents {
		events = append(events, "member")
	}
	if len(events) == 0 {
		return "none"
	}
	return strings.Join(events, ", ")
}
