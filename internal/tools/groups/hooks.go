package groups

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// HookInput defines common parameters for creating or editing a group hook.
type HookInput struct {
	URL                       string                  `json:"url,omitempty"                        jsonschema:"Webhook URL (required for add)"`
	Name                      string                  `json:"name,omitempty"                       jsonschema:"Hook name"`
	Description               string                  `json:"description,omitempty"                jsonschema:"Hook description"`
	Token                     string                  `json:"token,omitempty"                      jsonschema:"Secret token for payload validation"`
	SigningToken              string                  `json:"signing_token,omitempty"              jsonschema:"Write-only signing token for webhook signature validation"`
	PushEvents                *bool                   `json:"push_events,omitempty"                jsonschema:"Trigger on push events"`
	TagPushEvents             *bool                   `json:"tag_push_events,omitempty"            jsonschema:"Trigger on tag push events"`
	MergeRequestsEvents       *bool                   `json:"merge_requests_events,omitempty"      jsonschema:"Trigger on merge request events"`
	IssuesEvents              *bool                   `json:"issues_events,omitempty"              jsonschema:"Trigger on issue events"`
	NoteEvents                *bool                   `json:"note_events,omitempty"                jsonschema:"Trigger on comment events"`
	JobEvents                 *bool                   `json:"job_events,omitempty"                 jsonschema:"Trigger on job events"`
	PipelineEvents            *bool                   `json:"pipeline_events,omitempty"            jsonschema:"Trigger on pipeline events"`
	WikiPageEvents            *bool                   `json:"wiki_page_events,omitempty"           jsonschema:"Trigger on wiki page events"`
	DeploymentEvents          *bool                   `json:"deployment_events,omitempty"          jsonschema:"Trigger on deployment events"`
	ReleasesEvents            *bool                   `json:"releases_events,omitempty"            jsonschema:"Trigger on release events"`
	MilestoneEvents           *bool                   `json:"milestone_events,omitempty"           jsonschema:"Trigger on milestone events"`
	FeatureFlagEvents         *bool                   `json:"feature_flag_events,omitempty"        jsonschema:"Trigger on feature flag events"`
	SubGroupEvents            *bool                   `json:"subgroup_events,omitempty"            jsonschema:"Trigger on subgroup events"`
	MemberEvents              *bool                   `json:"member_events,omitempty"              jsonschema:"Trigger on member events"`
	VulnerabilityEvents       *bool                   `json:"vulnerability_events,omitempty"       jsonschema:"Trigger on vulnerability events"`
	ConfidentialIssuesEvents  *bool                   `json:"confidential_issues_events,omitempty" jsonschema:"Trigger on confidential issue events"`
	ConfidentialNoteEvents    *bool                   `json:"confidential_note_events,omitempty"   jsonschema:"Trigger on confidential note events"`
	EnableSSLVerification     *bool                   `json:"enable_ssl_verification,omitempty"    jsonschema:"Enable SSL verification for the hook endpoint"`
	PushEventsBranchFilter    string                  `json:"push_events_branch_filter,omitempty"  jsonschema:"Branch filter for push events (e.g. 'main')"`
	BranchFilterStrategy      string                  `json:"branch_filter_strategy,omitempty"      jsonschema:"Branch filter strategy (wildcard, regex, all_branches)"`
	EmojiEvents               *bool                   `json:"emoji_events,omitempty"                jsonschema:"Trigger on emoji events"`
	ResourceAccessTokenEvents *bool                   `json:"resource_access_token_events,omitempty" jsonschema:"Trigger on resource access token events"`
	ProjectEvents             *bool                   `json:"project_events,omitempty"               jsonschema:"Trigger on project events (group-level)"`
	CustomWebhookTemplate     string                  `json:"custom_webhook_template,omitempty"      jsonschema:"Custom payload template (JSON) sent instead of the default webhook body"`
	CustomHeaders             []HookCustomHeaderInput `json:"custom_headers,omitempty"  jsonschema:"Custom HTTP headers added to webhook requests"`
}

// HookCustomHeaderInput is a single custom HTTP header sent with webhook
// requests. It mirrors gl.HookCustomHeader on add/edit.
type HookCustomHeaderInput struct {
	Key   string `json:"key"   jsonschema:"Header name,required"`
	Value string `json:"value,omitempty" jsonschema:"Header value (write-only. Masked on read)"`
}

// ListHooksInput defines parameters for listing group hooks.
type ListHooksInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Order hooks by field (id)"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// HookURLVariable represents a templated webhook URL variable used to substitute
// placeholders like {var_name} in a webhook URL with secret values resolved
// server-side. Mirrors gl.HookURLVariable (Key + Value); the value is write-only
// and masked on read, so it is omitted unless GitLab returns it.
type HookURLVariable struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// HookCustomHeaderOutput mirrors gl.HookCustomHeader (a custom_headers entry).
// The value is write-only and masked on read.
type HookCustomHeaderOutput struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// HookOutput represents a GitLab group webhook.
type HookOutput struct {
	toolutil.HintableOutput
	ID                        int64                    `json:"id"`
	URL                       string                   `json:"url"`
	Name                      string                   `json:"name,omitempty"`
	Description               string                   `json:"description,omitempty"`
	GroupID                   int64                    `json:"group_id"`
	PushEvents                bool                     `json:"push_events"`
	TagPushEvents             bool                     `json:"tag_push_events"`
	MergeRequestsEvents       bool                     `json:"merge_requests_events"`
	IssuesEvents              bool                     `json:"issues_events"`
	NoteEvents                bool                     `json:"note_events"`
	JobEvents                 bool                     `json:"job_events"`
	PipelineEvents            bool                     `json:"pipeline_events"`
	WikiPageEvents            bool                     `json:"wiki_page_events"`
	DeploymentEvents          bool                     `json:"deployment_events"`
	ReleasesEvents            bool                     `json:"releases_events"`
	SubGroupEvents            bool                     `json:"subgroup_events"`
	MemberEvents              bool                     `json:"member_events"`
	ConfidentialIssuesEvents  bool                     `json:"confidential_issues_events"`
	ConfidentialNoteEvents    bool                     `json:"confidential_note_events"`
	EnableSSLVerification     bool                     `json:"enable_ssl_verification"`
	AlertStatus               string                   `json:"alert_status,omitempty"`
	DisabledUntil             string                   `json:"disabled_until,omitempty"`
	URLVariables              []HookURLVariable        `json:"url_variables,omitempty"`
	FeatureFlagEvents         bool                     `json:"feature_flag_events"`
	MilestoneEvents           bool                     `json:"milestone_events"`
	VulnerabilityEvents       bool                     `json:"vulnerability_events"`
	EmojiEvents               bool                     `json:"emoji_events"`
	ResourceAccessTokenEvents bool                     `json:"resource_access_token_events"`
	ProjectEvents             bool                     `json:"project_events"`
	PushEventsBranchFilter    string                   `json:"push_events_branch_filter,omitempty"`
	BranchFilterStrategy      string                   `json:"branch_filter_strategy,omitempty"`
	TokenPresent              bool                     `json:"token_present"`
	SigningTokenPresent       bool                     `json:"signing_token_present"`
	CreatedAt                 string                   `json:"created_at,omitempty"`
	CustomWebhookTemplate     string                   `json:"custom_webhook_template,omitempty"`
	CustomHeaders             []HookCustomHeaderOutput `json:"custom_headers,omitempty"`
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
		ID:                        h.ID,
		URL:                       h.URL,
		Name:                      h.Name,
		Description:               h.Description,
		GroupID:                   h.GroupID,
		PushEvents:                h.PushEvents,
		TagPushEvents:             h.TagPushEvents,
		MergeRequestsEvents:       h.MergeRequestsEvents,
		IssuesEvents:              h.IssuesEvents,
		NoteEvents:                h.NoteEvents,
		JobEvents:                 h.JobEvents,
		PipelineEvents:            h.PipelineEvents,
		WikiPageEvents:            h.WikiPageEvents,
		DeploymentEvents:          h.DeploymentEvents,
		ReleasesEvents:            h.ReleasesEvents,
		SubGroupEvents:            h.SubGroupEvents,
		MemberEvents:              h.MemberEvents,
		ConfidentialIssuesEvents:  h.ConfidentialIssuesEvents,
		ConfidentialNoteEvents:    h.ConfidentialNoteEvents,
		EnableSSLVerification:     h.EnableSSLVerification,
		AlertStatus:               h.AlertStatus,
		FeatureFlagEvents:         h.FeatureFlagEvents,
		MilestoneEvents:           h.MilestoneEvents,
		VulnerabilityEvents:       h.VulnerabilityEvents,
		EmojiEvents:               h.EmojiEvents,
		ResourceAccessTokenEvents: h.ResourceAccessTokenEvents,
		ProjectEvents:             h.ProjectEvents,
		PushEventsBranchFilter:    h.PushEventsBranchFilter,
		BranchFilterStrategy:      h.BranchFilterStrategy,
		TokenPresent:              h.TokenPresent,
		SigningTokenPresent:       h.SigningTokenPresent,
		CustomWebhookTemplate:     h.CustomWebhookTemplate,
	}
	if len(h.URLVariables) > 0 {
		// url_variables mirrors gl.HookURLVariable as a []object (Key + Value),
		// but the Value is a secret that GitLab masks on read; it is
		// deliberately not propagated so the output never carries secrets.
		out.URLVariables = make([]HookURLVariable, len(h.URLVariables))
		for i, v := range h.URLVariables {
			out.URLVariables[i] = HookURLVariable{Key: v.Key}
		}
	}
	if len(h.CustomHeaders) > 0 {
		// custom_headers mirrors gl.HookCustomHeader as a []object (Key +
		// Value); the Value is secret and masked on read, so only the key is
		// surfaced.
		out.CustomHeaders = make([]HookCustomHeaderOutput, 0, len(h.CustomHeaders))
		for _, c := range h.CustomHeaders {
			if c == nil {
				continue
			}
			out.CustomHeaders = append(out.CustomHeaders, HookCustomHeaderOutput{Key: c.Key})
		}
	}
	if h.CreatedAt != nil {
		out.CreatedAt = h.CreatedAt.Format(time.RFC3339)
	}
	if h.DisabledUntil != nil {
		out.DisabledUntil = h.DisabledUntil.Format(time.RFC3339)
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
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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

func applyGroupHookOptions(input HookInput, opts *gl.AddGroupHookOptions) {
	applyGroupHookIdentityOptions(input, opts)
	applyGroupHookEventOptions(input, opts)
}

func applyGroupHookIdentityOptions(input HookInput, opts *gl.AddGroupHookOptions) {
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
	if input.SigningToken != "" {
		opts.SigningToken = new(input.SigningToken)
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
	if input.BranchFilterStrategy != "" {
		opts.BranchFilterStrategy = new(input.BranchFilterStrategy)
	}
	if input.CustomWebhookTemplate != "" {
		opts.CustomWebhookTemplate = new(input.CustomWebhookTemplate)
	}
	if headers := customHeaderOptions(input.CustomHeaders); headers != nil {
		opts.CustomHeaders = headers
	}
}

// customHeaderOptions converts HookCustomHeaderInput values into the SDK
// pointer-slice shape, returning nil when no header was supplied.
func customHeaderOptions(headers []HookCustomHeaderInput) *[]*gl.HookCustomHeader {
	if len(headers) == 0 {
		return nil
	}
	out := make([]*gl.HookCustomHeader, len(headers))
	for i, h := range headers {
		out[i] = &gl.HookCustomHeader{Key: h.Key, Value: h.Value}
	}
	return &out
}

func applyGroupHookEventOptions(input HookInput, opts *gl.AddGroupHookOptions) {
	// Table-driven assignment keeps cyclomatic complexity flat as new
	// event-flag fields are added to HookInput. The slice pairs each
	// input pointer with the destination field pointer on the GitLab
	// options struct; the loop handles the nil-check once.
	pairs := []struct {
		src *bool
		dst **bool
	}{
		{input.PushEvents, &opts.PushEvents},
		{input.TagPushEvents, &opts.TagPushEvents},
		{input.MergeRequestsEvents, &opts.MergeRequestsEvents},
		{input.IssuesEvents, &opts.IssuesEvents},
		{input.NoteEvents, &opts.NoteEvents},
		{input.JobEvents, &opts.JobEvents},
		{input.PipelineEvents, &opts.PipelineEvents},
		{input.WikiPageEvents, &opts.WikiPageEvents},
		{input.DeploymentEvents, &opts.DeploymentEvents},
		{input.ReleasesEvents, &opts.ReleasesEvents},
		{input.MilestoneEvents, &opts.MilestoneEvents},
		{input.FeatureFlagEvents, &opts.FeatureFlagEvents},
		{input.SubGroupEvents, &opts.SubGroupEvents},
		{input.MemberEvents, &opts.MemberEvents},
		{input.VulnerabilityEvents, &opts.VulnerabilityEvents},
		{input.ConfidentialIssuesEvents, &opts.ConfidentialIssuesEvents},
		{input.ConfidentialNoteEvents, &opts.ConfidentialNoteEvents},
		{input.EmojiEvents, &opts.EmojiEvents},
		{input.ResourceAccessTokenEvents, &opts.ResourceAccessTokenEvents},
		{input.ProjectEvents, &opts.ProjectEvents},
	}
	for _, p := range pairs {
		if p.src != nil {
			*p.dst = p.src
		}
	}
}

// applyAddHookOpts builds the AddGroupHookOptions from HookInput.
func applyAddHookOpts(input HookInput) *gl.AddGroupHookOptions {
	opts := &gl.AddGroupHookOptions{}
	applyGroupHookOptions(input, opts)
	return opts
}

func groupEditHookOptionsFromAdd(opts *gl.AddGroupHookOptions) *gl.EditGroupHookOptions {
	return &gl.EditGroupHookOptions{
		URL:                       opts.URL,
		Name:                      opts.Name,
		Description:               opts.Description,
		PushEvents:                opts.PushEvents,
		PushEventsBranchFilter:    opts.PushEventsBranchFilter,
		BranchFilterStrategy:      opts.BranchFilterStrategy,
		IssuesEvents:              opts.IssuesEvents,
		ConfidentialIssuesEvents:  opts.ConfidentialIssuesEvents,
		MergeRequestsEvents:       opts.MergeRequestsEvents,
		TagPushEvents:             opts.TagPushEvents,
		NoteEvents:                opts.NoteEvents,
		ConfidentialNoteEvents:    opts.ConfidentialNoteEvents,
		JobEvents:                 opts.JobEvents,
		PipelineEvents:            opts.PipelineEvents,
		WikiPageEvents:            opts.WikiPageEvents,
		DeploymentEvents:          opts.DeploymentEvents,
		FeatureFlagEvents:         opts.FeatureFlagEvents,
		ReleasesEvents:            opts.ReleasesEvents,
		MilestoneEvents:           opts.MilestoneEvents,
		SubGroupEvents:            opts.SubGroupEvents,
		MemberEvents:              opts.MemberEvents,
		VulnerabilityEvents:       opts.VulnerabilityEvents,
		EmojiEvents:               opts.EmojiEvents,
		ResourceAccessTokenEvents: opts.ResourceAccessTokenEvents,
		ProjectEvents:             opts.ProjectEvents,
		EnableSSLVerification:     opts.EnableSSLVerification,
		Token:                     opts.Token,
		SigningToken:              opts.SigningToken,
		CustomWebhookTemplate:     opts.CustomWebhookTemplate,
		CustomHeaders:             opts.CustomHeaders,
	}
}

// applyEditHookOpts builds the EditGroupHookOptions from HookInput.
func applyEditHookOpts(input HookInput) *gl.EditGroupHookOptions {
	return groupEditHookOptionsFromAdd(applyAddHookOpts(input))
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
	if h.MilestoneEvents {
		events = append(events, "milestone")
	}
	if h.FeatureFlagEvents {
		events = append(events, "feature_flag")
	}
	if h.SubGroupEvents {
		events = append(events, "subgroup")
	}
	if h.MemberEvents {
		events = append(events, "member")
	}
	if h.VulnerabilityEvents {
		events = append(events, "vulnerability")
	}
	if len(events) == 0 {
		return "none"
	}
	return strings.Join(events, ", ")
}

// ---------------------------------------------------------------------------
// Hook sub-operations: custom headers, URL variables, test triggers, resends.
// These mirror the project-hook equivalents for input/output shape.
// ---------------------------------------------------------------------------.

// SetHookCustomHeaderInput defines parameters for setting a custom header on a group webhook.
type SetHookCustomHeaderInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Webhook ID,required"`
	Key     string               `json:"key"      jsonschema:"Custom header key name,required"`
	Value   string               `json:"value"    jsonschema:"Custom header value (write-only),required"`
}

// SetHookCustomHeader creates or updates a custom header on a group webhook.
func SetHookCustomHeader(ctx context.Context, client *gitlabclient.Client, input SetHookCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupSetHookCustomHeader: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupSetHookCustomHeader", "hook_id")
	}
	if input.Key == "" {
		return errors.New("groupSetHookCustomHeader: key is required")
	}
	opts := &gl.SetHookCustomHeaderOptions{Value: &input.Value}
	_, err := client.GL().Groups.SetGroupCustomHeader(string(input.GroupID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupSetHookCustomHeader", err, http.StatusNotFound,
			"webhook not found. Use gitlab_group_hook_list to verify hook_id; requires Owner role")
	}
	return nil
}

// DeleteHookCustomHeaderInput defines parameters for deleting a custom header from a group webhook.
type DeleteHookCustomHeaderInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Webhook ID,required"`
	Key     string               `json:"key"      jsonschema:"Custom header key name to delete,required"`
}

// DeleteHookCustomHeader removes a custom header from a group webhook.
func DeleteHookCustomHeader(ctx context.Context, client *gitlabclient.Client, input DeleteHookCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupDeleteHookCustomHeader: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupDeleteHookCustomHeader", "hook_id")
	}
	if input.Key == "" {
		return errors.New("groupDeleteHookCustomHeader: key is required")
	}
	_, err := client.GL().Groups.DeleteGroupCustomHeader(string(input.GroupID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupDeleteHookCustomHeader", err, http.StatusNotFound,
			"header key not currently set on this hook (or hook not found). Use gitlab_group_hook_get to inspect configured custom headers")
	}
	return nil
}

// SetHookURLVariableInput defines parameters for setting a URL variable on a group webhook.
type SetHookURLVariableInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Webhook ID,required"`
	Key     string               `json:"key"      jsonschema:"URL variable key name. Letters and underscores only. GitLab rejects keys containing digits,required"`
	Value   string               `json:"value"    jsonschema:"URL variable value (write-only). Must be non-empty,required"`
}

// SetHookURLVariable creates or updates a templated URL variable on a group webhook.
func SetHookURLVariable(ctx context.Context, client *gitlabclient.Client, input SetHookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupSetHookURLVariable: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupSetHookURLVariable", "hook_id")
	}
	if input.Key == "" {
		return errors.New("groupSetHookURLVariable: key is required")
	}
	opts := &gl.SetHookURLVariableOptions{Value: &input.Value}
	_, err := client.GL().Groups.SetGroupHookURLVariable(string(input.GroupID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return toolutil.WrapErrWithHint("groupSetHookURLVariable", err,
				"URL variable keys accept only letters and underscores (digits are rejected) and the value must be non-empty")
		}
		return toolutil.WrapErrWithStatusHint("groupSetHookURLVariable", err, http.StatusNotFound,
			"webhook not found. Use gitlab_group_hook_list to verify hook_id; requires Owner role")
	}
	return nil
}

// DeleteHookURLVariableInput defines parameters for deleting a URL variable from a group webhook.
type DeleteHookURLVariableInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Webhook ID,required"`
	Key     string               `json:"key"      jsonschema:"URL variable key name to delete,required"`
}

// DeleteHookURLVariable removes a templated URL variable from a group webhook.
func DeleteHookURLVariable(ctx context.Context, client *gitlabclient.Client, input DeleteHookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupDeleteHookURLVariable: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupDeleteHookURLVariable", "hook_id")
	}
	if input.Key == "" {
		return errors.New("groupDeleteHookURLVariable: key is required")
	}
	_, err := client.GL().Groups.DeleteGroupHookURLVariable(string(input.GroupID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupDeleteHookURLVariable", err, http.StatusNotFound,
			"variable key not currently set on this hook (or hook not found). Use gitlab_group_hook_get to inspect configured URL variables")
	}
	return nil
}

// TestHookInput defines parameters for triggering a test group hook event.
type TestHookInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	HookID  int64                `json:"hook_id"  jsonschema:"Webhook ID,required"`
	Trigger string               `json:"trigger"  jsonschema:"Event type to test (push_events, tag_push_events, issues_events, confidential_issues_events, note_events, merge_requests_events, job_events, pipeline_events, wiki_page_events, releases_events, emoji_events, resource_access_token_events),required"`
}

// TestHook triggers a test event for a group webhook.
func TestHook(ctx context.Context, client *gitlabclient.Client, input TestHookInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupTestHook: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupTestHook", "hook_id")
	}
	if input.Trigger == "" {
		return errors.New("groupTestHook: trigger is required (e.g. push_events, pipeline_events)")
	}
	_, err := client.GL().Groups.TriggerTestGroupHook(string(input.GroupID), input.HookID, gl.GroupHookTrigger(input.Trigger), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupTestHook", err, http.StatusNotFound,
			"webhook not found, or trigger is not a valid event type. Use gitlab_group_hook_list to verify hook_id; requires Owner role")
	}
	return nil
}

// ResendHookEventInput defines parameters for resending a group hook event.
type ResendHookEventInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"      jsonschema:"Group ID or URL-encoded path,required"`
	HookID      int64                `json:"hook_id"       jsonschema:"Webhook ID,required"`
	HookEventID int64                `json:"hook_event_id" jsonschema:"ID of the hook event to resend,required"`
}

// ResendHookEvent resends a specific previously-delivered group hook event.
func ResendHookEvent(ctx context.Context, client *gitlabclient.Client, input ResendHookEventInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupResendHookEvent: group_id is required")
	}
	if input.HookID <= 0 {
		return toolutil.ErrRequiredInt64("groupResendHookEvent", "hook_id")
	}
	if input.HookEventID <= 0 {
		return toolutil.ErrRequiredInt64("groupResendHookEvent", "hook_event_id")
	}
	_, err := client.GL().Groups.ResendGroupHookEvent(string(input.GroupID), input.HookID, input.HookEventID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupResendHookEvent", err, http.StatusNotFound,
			"webhook or hook event not found. Verify hook_id and hook_event_id; requires Owner role")
	}
	return nil
}
