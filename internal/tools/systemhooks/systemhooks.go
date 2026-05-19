package systemhooks

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Types.

// HookItem represents a system hook.
type HookItem struct {
	ID                     int64             `json:"id"`
	URL                    string            `json:"url"`
	Name                   string            `json:"name,omitempty"`
	Description            string            `json:"description,omitempty"`
	CreatedAt              string            `json:"created_at,omitempty"`
	PushEvents             bool              `json:"push_events"`
	TagPushEvents          bool              `json:"tag_push_events"`
	MergeRequestsEvents    bool              `json:"merge_requests_events"`
	RepositoryUpdateEvents bool              `json:"repository_update_events"`
	EnableSSLVerification  bool              `json:"enable_ssl_verification"`
	URLVariables           []HookURLVariable `json:"url_variables,omitempty"`
	TokenPresent           bool              `json:"token_present"`
	SigningTokenPresent    bool              `json:"signing_token_present"`
}

// HookURLVariable represents a masked URL variable configured on a system hook.
type HookURLVariable struct {
	Key string `json:"key"`
}

// HookEventItem represents a hook test event.
type HookEventItem struct {
	EventName  string `json:"event_name"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ProjectID  int64  `json:"project_id"`
	OwnerName  string `json:"owner_name"`
	OwnerEmail string `json:"owner_email"`
}

// ListInput is empty (no params).
type ListInput struct{}

// ListOutput contains the list of system hooks.
type ListOutput struct {
	toolutil.HintableOutput
	Hooks []HookItem `json:"hooks"`
}

// GetInput is the input for getting a system hook.
type GetInput struct {
	ID int64 `json:"id" jsonschema:"System hook ID,required"`
}

// GetOutput wraps a single hook.
type GetOutput struct {
	toolutil.HintableOutput
	Hook HookItem `json:"hook"`
}

// AddInput is the input for adding a system hook.
type AddInput struct {
	URL                    string `json:"url"                       jsonschema:"Hook URL to receive events,required"`
	Name                   string `json:"name,omitempty"            jsonschema:"Descriptive name for the hook"`
	Description            string `json:"description,omitempty"     jsonschema:"Description for the hook"`
	Token                  string `json:"token,omitempty"           jsonschema:"Secret token for payload validation"`
	SigningToken           string `json:"signing_token,omitempty"   jsonschema:"Write-only signing token for webhook signature validation"`
	PushEvents             *bool  `json:"push_events,omitempty"             jsonschema:"Trigger on push events"`
	PushEventsBranchFilter string `json:"push_events_branch_filter,omitempty" jsonschema:"Branch filter for push events (wildcard, regex, or branch name)"`
	BranchFilterStrategy   string `json:"branch_filter_strategy,omitempty" jsonschema:"Branch filter strategy: wildcard, regex, or all_branches"`
	TagPushEvents          *bool  `json:"tag_push_events,omitempty"         jsonschema:"Trigger on tag push events"`
	MergeRequestsEvents    *bool  `json:"merge_requests_events,omitempty"   jsonschema:"Trigger on merge request events"`
	RepositoryUpdateEvents *bool  `json:"repository_update_events,omitempty" jsonschema:"Trigger on repository update events"`
	EnableSSLVerification  *bool  `json:"enable_ssl_verification,omitempty" jsonschema:"Enable SSL verification for the hook URL"`
}

// AddOutput wraps the added hook.
type AddOutput struct {
	toolutil.HintableOutput
	Hook HookItem `json:"hook"`
}

// EditInput is the input for editing a system hook.
type EditInput struct {
	ID                     int64  `json:"id"                        jsonschema:"System hook ID to edit,required"`
	URL                    string `json:"url,omitempty"             jsonschema:"Updated hook URL"`
	Name                   string `json:"name,omitempty"            jsonschema:"Updated hook name"`
	Description            string `json:"description,omitempty"     jsonschema:"Updated hook description"`
	Token                  string `json:"token,omitempty"           jsonschema:"Updated secret token for payload validation"`
	SigningToken           string `json:"signing_token,omitempty"   jsonschema:"Updated write-only signing token for webhook signature validation"`
	PushEvents             *bool  `json:"push_events,omitempty"             jsonschema:"Trigger on push events"`
	PushEventsBranchFilter string `json:"push_events_branch_filter,omitempty" jsonschema:"Branch filter for push events (wildcard, regex, or branch name)"`
	BranchFilterStrategy   string `json:"branch_filter_strategy,omitempty" jsonschema:"Branch filter strategy: wildcard, regex, or all_branches"`
	TagPushEvents          *bool  `json:"tag_push_events,omitempty"         jsonschema:"Trigger on tag push events"`
	MergeRequestsEvents    *bool  `json:"merge_requests_events,omitempty"   jsonschema:"Trigger on merge request events"`
	RepositoryUpdateEvents *bool  `json:"repository_update_events,omitempty" jsonschema:"Trigger on repository update events"`
	EnableSSLVerification  *bool  `json:"enable_ssl_verification,omitempty" jsonschema:"Enable SSL verification for the hook URL"`
}

// EditOutput wraps the edited hook.
type EditOutput struct {
	toolutil.HintableOutput
	Hook HookItem `json:"hook"`
}

// TestInput is the input for testing a system hook.
type TestInput struct {
	ID int64 `json:"id" jsonschema:"System hook ID to test,required"`
}

// TestOutput wraps the test event result.
type TestOutput struct {
	toolutil.HintableOutput
	Event HookEventItem `json:"event"`
}

// DeleteInput is the input for deleting a system hook.
type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"System hook ID to delete,required"`
}

// SetURLVariableInput is the input for creating or updating a system hook URL variable.
type SetURLVariableInput struct {
	ID    int64  `json:"id"    jsonschema:"System hook ID,required"`
	Key   string `json:"key"   jsonschema:"URL variable key name,required"`
	Value string `json:"value" jsonschema:"URL variable value,required"`
}

// DeleteURLVariableInput is the input for deleting a system hook URL variable.
type DeleteURLVariableInput struct {
	ID  int64  `json:"id"  jsonschema:"System hook ID,required"`
	Key string `json:"key" jsonschema:"URL variable key name to delete,required"`
}

// Helpers.

// toItem converts the GitLab API response to the tool output format.
func toItem(h *gl.Hook) HookItem {
	createdAt := ""
	if h.CreatedAt != nil {
		createdAt = h.CreatedAt.Format(time.RFC3339)
	}
	return HookItem{
		ID:                     h.ID,
		URL:                    h.URL,
		Name:                   h.Name,
		Description:            h.Description,
		CreatedAt:              createdAt,
		PushEvents:             h.PushEvents,
		TagPushEvents:          h.TagPushEvents,
		MergeRequestsEvents:    h.MergeRequestsEvents,
		RepositoryUpdateEvents: h.RepositoryUpdateEvents,
		EnableSSLVerification:  h.EnableSSLVerification,
		URLVariables:           hookURLVariablesToOutput(h.URLVariables),
		TokenPresent:           h.TokenPresent,
		SigningTokenPresent:    h.SigningTokenPresent,
	}
}

func hookURLVariablesToOutput(variables []gl.HookURLVariable) []HookURLVariable {
	if len(variables) == 0 {
		return nil
	}
	out := make([]HookURLVariable, len(variables))
	for i, variable := range variables {
		out[i] = HookURLVariable{Key: variable.Key}
	}
	return out
}

type hookOptions struct {
	URL                    string
	Name                   string
	Description            string
	Token                  string
	SigningToken           string
	PushEvents             *bool
	PushEventsBranchFilter string
	BranchFilterStrategy   string
	TagPushEvents          *bool
	MergeRequestsEvents    *bool
	RepositoryUpdateEvents *bool
	EnableSSLVerification  *bool
}

func hookOptionsFromAdd(input AddInput) hookOptions {
	return hookOptions(input)
}

func hookOptionsFromEdit(input EditInput) hookOptions {
	return hookOptions{
		URL:                    input.URL,
		Name:                   input.Name,
		Description:            input.Description,
		Token:                  input.Token,
		SigningToken:           input.SigningToken,
		PushEvents:             input.PushEvents,
		PushEventsBranchFilter: input.PushEventsBranchFilter,
		BranchFilterStrategy:   input.BranchFilterStrategy,
		TagPushEvents:          input.TagPushEvents,
		MergeRequestsEvents:    input.MergeRequestsEvents,
		RepositoryUpdateEvents: input.RepositoryUpdateEvents,
		EnableSSLVerification:  input.EnableSSLVerification,
	}
}

func applyHookOptions(input hookOptions, opts *gl.AddHookOptions) {
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
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
	if input.BranchFilterStrategy != "" {
		opts.BranchFilterStrategy = new(gl.BranchFilterStrategy(input.BranchFilterStrategy))
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.RepositoryUpdateEvents != nil {
		opts.RepositoryUpdateEvents = input.RepositoryUpdateEvents
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
}

func hookAddOptions(input AddInput) *gl.AddHookOptions {
	opts := &gl.AddHookOptions{}
	applyHookOptions(hookOptionsFromAdd(input), opts)
	return opts
}

func hookEditOptions(input EditInput) *gl.EditHookOptions {
	addOptions := &gl.AddHookOptions{}
	applyHookOptions(hookOptionsFromEdit(input), addOptions)
	editOptions := gl.EditHookOptions(*addOptions)
	return &editOptions
}

// Handlers.

// List retrieves all system hooks.
func List(ctx context.Context, client *gitlabclient.Client, _ ListInput) (ListOutput, error) {
	hooks, _, err := client.GL().SystemHooks.ListHooks(gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("system_hook_list", err, http.StatusForbidden,
			"requires administrator access; system hooks are instance-wide and only available on self-managed instances")
	}
	items := make([]HookItem, 0, len(hooks))
	for _, h := range hooks {
		items = append(items, toItem(h))
	}
	return ListOutput{Hooks: items}, nil
}

// Get retrieves a single system hook.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("system_hook_get", "id")
	}
	hook, _, err := client.GL().SystemHooks.GetHook(input.ID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("system_hook_get", err, http.StatusNotFound,
			"verify hook_id with gitlab_list_system_hooks; admin-only on self-managed instances")
	}
	return GetOutput{Hook: toItem(hook)}, nil
}

// Add creates a new system hook.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (AddOutput, error) {
	if input.URL == "" {
		return AddOutput{}, toolutil.ErrRequiredString("system_hook_add", "url")
	}
	opts := hookAddOptions(input)

	hook, _, err := client.GL().SystemHooks.AddHook(opts, gl.WithContext(ctx))
	if err != nil {
		return AddOutput{}, toolutil.WrapErrWithStatusHint("system_hook_add", err, http.StatusBadRequest,
			"requires administrator; url must be HTTP(S) and reachable from the instance; token is shared secret for X-Gitlab-Token header; enable specific event flags (push_events, tag_push_events, merge_requests_events, etc.)")
	}
	return AddOutput{Hook: toItem(hook)}, nil
}

// Edit updates an existing system hook.
func Edit(ctx context.Context, client *gitlabclient.Client, input EditInput) (EditOutput, error) {
	if input.ID <= 0 {
		return EditOutput{}, toolutil.ErrRequiredInt64("system_hook_edit", "id")
	}
	opts := hookEditOptions(input)

	hook, _, err := client.GL().SystemHooks.EditHook(input.ID, opts, gl.WithContext(ctx))
	if err != nil {
		return EditOutput{}, toolutil.WrapErrWithStatusHint("system_hook_edit", err, http.StatusNotFound,
			"verify hook_id with gitlab_list_system_hooks; admin-only on self-managed instances; unset fields keep current values")
	}
	return EditOutput{Hook: toItem(hook)}, nil
}

// Test triggers a test event for a system hook.
func Test(ctx context.Context, client *gitlabclient.Client, input TestInput) (TestOutput, error) {
	if input.ID <= 0 {
		return TestOutput{}, toolutil.ErrRequiredInt64("system_hook_test", "id")
	}
	event, _, err := client.GL().SystemHooks.TestHook(input.ID, gl.WithContext(ctx))
	if err != nil {
		return TestOutput{}, toolutil.WrapErrWithStatusHint("system_hook_test", err, http.StatusNotFound,
			"verify hook_id with gitlab_list_system_hooks; test triggers a sample push event \u2014 verify the receiving endpoint is reachable")
	}
	return TestOutput{Event: HookEventItem{
		EventName:  event.EventName,
		Name:       event.Name,
		Path:       event.Path,
		ProjectID:  event.ProjectID,
		OwnerName:  event.OwnerName,
		OwnerEmail: event.OwnerEmail,
	}}, nil
}

// Delete removes a system hook.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("system_hook_delete", "id")
	}
	_, err := client.GL().SystemHooks.DeleteHook(input.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("system_hook_delete", err, http.StatusForbidden,
			"requires administrator access; deletion is irreversible \u2014 verify hook_id with gitlab_list_system_hooks before deleting")
	}
	return nil
}

// SetURLVariable creates or updates a URL variable for a system hook.
func SetURLVariable(ctx context.Context, client *gitlabclient.Client, input SetURLVariableInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("system_hook_set_url_variable", "id")
	}
	if input.Key == "" {
		return toolutil.ErrRequiredString("system_hook_set_url_variable", "key")
	}
	if input.Value == "" {
		return toolutil.ErrRequiredString("system_hook_set_url_variable", "value")
	}
	opts := &gl.SetHookURLVariableOptions{Value: &input.Value}
	_, err := client.GL().SystemHooks.SetHookURLVariable(input.ID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("system_hook_set_url_variable", err, http.StatusNotFound,
			"verify hook_id with gitlab_list_system_hooks; URL variable keys are case-sensitive and referenced by placeholders in the hook URL")
	}
	return nil
}

// DeleteURLVariable deletes a URL variable from a system hook.
func DeleteURLVariable(ctx context.Context, client *gitlabclient.Client, input DeleteURLVariableInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("system_hook_delete_url_variable", "id")
	}
	if input.Key == "" {
		return toolutil.ErrRequiredString("system_hook_delete_url_variable", "key")
	}
	_, err := client.GL().SystemHooks.DeleteHookURLVariable(input.ID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("system_hook_delete_url_variable", err, http.StatusNotFound,
			"variable key not currently set on this hook, or hook not found; use gitlab_get_system_hook to inspect configured URL variables")
	}
	return nil
}

// Markdown formatters.
