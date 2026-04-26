// Package broadcastmessages implements MCP tool handlers for GitLab broadcast messages.
// It wraps the BroadcastMessagesService from client-go v2.
// These are admin-only endpoints requiring administrator access.
package broadcastmessages

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// MessageItem represents a broadcast message in output.
type MessageItem struct {
	ID                 int64   `json:"id"`
	Message            string  `json:"message"`
	StartsAt           string  `json:"starts_at,omitempty"`
	EndsAt             string  `json:"ends_at,omitempty"`
	Font               string  `json:"font,omitempty"`
	Active             bool    `json:"active"`
	TargetAccessLevels []int64 `json:"target_access_levels,omitempty"`
	TargetPath         string  `json:"target_path,omitempty"`
	BroadcastType      string  `json:"broadcast_type,omitempty"`
	Dismissable        bool    `json:"dismissable"`
	Theme              string  `json:"theme,omitempty"`
}

// toItem converts the GitLab API response to the tool output format.
func toItem(m *gl.BroadcastMessage) MessageItem {
	item := MessageItem{
		ID:            m.ID,
		Message:       m.Message,
		Font:          m.Font,
		Active:        m.Active,
		TargetPath:    m.TargetPath,
		BroadcastType: m.BroadcastType,
		Dismissable:   m.Dismissable,
		Theme:         m.Theme,
	}
	if m.StartsAt != nil {
		item.StartsAt = m.StartsAt.Format(time.RFC3339)
	}
	if m.EndsAt != nil {
		item.EndsAt = m.EndsAt.Format(time.RFC3339)
	}
	for _, level := range m.TargetAccessLevels {
		item.TargetAccessLevels = append(item.TargetAccessLevels, int64(level))
	}
	return item
}

// List.

// ListInput is the input for listing broadcast messages.
type ListInput struct {
	Page    int64 `json:"page,omitempty" jsonschema:"Page number"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// ListOutput is the output for listing broadcast messages.
type ListOutput struct {
	toolutil.HintableOutput
	Messages   []MessageItem             `json:"messages"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves all broadcast messages.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListBroadcastMessagesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}

	msgs, resp, err := client.GL().BroadcastMessage.ListBroadcastMessages(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("broadcast_message_list", err, http.StatusForbidden,
			"this is an instance-wide endpoint and may require administrator access on self-managed instances; not available on GitLab.com SaaS for non-admins")
	}

	items := make([]MessageItem, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, toItem(m))
	}
	return ListOutput{
		Messages:   items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get.

// GetInput is the input for getting a broadcast message.
type GetInput struct {
	ID int64 `json:"id" jsonschema:"Broadcast message ID,required"`
}

// GetOutput contains a single broadcast message.
type GetOutput struct {
	toolutil.HintableOutput
	Message MessageItem `json:"message"`
}

// Get retrieves a specific broadcast message by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("broadcast_message_get", "id")
	}
	m, _, err := client.GL().BroadcastMessage.GetBroadcastMessage(input.ID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("broadcast_message_get", err, http.StatusNotFound,
			"verify id with gitlab_broadcast_message_list; the message may have been deleted")
	}
	return GetOutput{Message: toItem(m)}, nil
}

// Create.

// CreateInput is the input for creating a broadcast message.
type CreateInput struct {
	Message            string  `json:"message" jsonschema:"Message text. Supports Markdown.,required"`
	StartsAt           string  `json:"starts_at,omitempty" jsonschema:"Start time in ISO 8601 format"`
	EndsAt             string  `json:"ends_at,omitempty" jsonschema:"End time in ISO 8601 format"`
	Font               string  `json:"font,omitempty" jsonschema:"Font for the message"`
	TargetAccessLevels []int64 `json:"target_access_levels,omitempty" jsonschema:"Access levels to target (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	TargetPath         string  `json:"target_path,omitempty" jsonschema:"Target path to show message on"`
	BroadcastType      string  `json:"broadcast_type,omitempty" jsonschema:"Type: banner or notification"`
	Dismissable        *bool   `json:"dismissable,omitempty" jsonschema:"Whether message can be dismissed"`
	Theme              string  `json:"theme,omitempty" jsonschema:"Theme: indigo, light-indigo, blue, light-blue, green, light-green, red, light-red"`
}

// CreateOutput contains the created broadcast message.
type CreateOutput struct {
	toolutil.HintableOutput
	Message MessageItem `json:"message"`
}

// Create creates a new broadcast message (admin-only).
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	opts := &gl.CreateBroadcastMessageOptions{
		Message: new(input.Message),
	}
	if input.StartsAt != "" {
		t, err := time.Parse(time.RFC3339, input.StartsAt)
		if err != nil {
			return CreateOutput{}, toolutil.WrapErrWithMessage("broadcast_message_create", fmt.Errorf("invalid starts_at: %w", err))
		}
		opts.StartsAt = &t
	}
	if input.EndsAt != "" {
		t, err := time.Parse(time.RFC3339, input.EndsAt)
		if err != nil {
			return CreateOutput{}, toolutil.WrapErrWithMessage("broadcast_message_create", fmt.Errorf("invalid ends_at: %w", err))
		}
		opts.EndsAt = &t
	}
	if input.Font != "" {
		opts.Font = new(input.Font)
	}
	if len(input.TargetAccessLevels) > 0 {
		levels := make([]gl.AccessLevelValue, len(input.TargetAccessLevels))
		for i, l := range input.TargetAccessLevels {
			levels[i] = gl.AccessLevelValue(l)
		}
		opts.TargetAccessLevels = levels
	}
	if input.TargetPath != "" {
		opts.TargetPath = new(input.TargetPath)
	}
	if input.BroadcastType != "" {
		opts.BroadcastType = new(input.BroadcastType)
	}
	if input.Dismissable != nil {
		opts.Dismissable = input.Dismissable
	}
	if input.Theme != "" {
		opts.Theme = new(input.Theme)
	}

	m, _, err := client.GL().BroadcastMessage.CreateBroadcastMessage(opts, gl.WithContext(ctx))
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithStatusHint("broadcast_message_create", err, http.StatusBadRequest,
			"requires administrator access; broadcast_type must be 'banner' or 'notification'; theme must be one of indigo, light-indigo, blue, light-blue, green, light-green, red, light-red; starts_at < ends_at; access levels: 10/20/30/40/50")
	}
	return CreateOutput{Message: toItem(m)}, nil
}

// Update.

// UpdateInput is the input for updating a broadcast message.
type UpdateInput struct {
	ID                 int64   `json:"id" jsonschema:"Broadcast message ID,required"`
	Message            string  `json:"message,omitempty" jsonschema:"Message text. Supports Markdown."`
	StartsAt           string  `json:"starts_at,omitempty" jsonschema:"Start time in ISO 8601 format"`
	EndsAt             string  `json:"ends_at,omitempty" jsonschema:"End time in ISO 8601 format"`
	Font               string  `json:"font,omitempty" jsonschema:"Font for the message"`
	TargetAccessLevels []int64 `json:"target_access_levels,omitempty" jsonschema:"Access levels to target"`
	TargetPath         string  `json:"target_path,omitempty" jsonschema:"Target path to show message on"`
	BroadcastType      string  `json:"broadcast_type,omitempty" jsonschema:"Type: banner or notification"`
	Dismissable        *bool   `json:"dismissable,omitempty" jsonschema:"Whether message can be dismissed"`
	Theme              string  `json:"theme,omitempty" jsonschema:"Theme color"`
}

// UpdateOutput contains the updated broadcast message.
type UpdateOutput struct {
	toolutil.HintableOutput
	Message MessageItem `json:"message"`
}

// Update modifies a broadcast message (admin-only).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	if input.ID <= 0 {
		return UpdateOutput{}, toolutil.ErrRequiredInt64("broadcast_message_update", "id")
	}
	opts, err := buildUpdateOpts(input)
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithMessage("broadcast_message_update", err)
	}

	m, _, err := client.GL().BroadcastMessage.UpdateBroadcastMessage(input.ID, opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("broadcast_message_update", err, http.StatusNotFound,
			"verify id with gitlab_broadcast_message_list; requires administrator access; broadcast_type must be 'banner' or 'notification'")
	}
	return UpdateOutput{Message: toItem(m)}, nil
}

// buildUpdateOpts maps UpdateInput fields to the GitLab API update options.
func buildUpdateOpts(input UpdateInput) (*gl.UpdateBroadcastMessageOptions, error) {
	opts := &gl.UpdateBroadcastMessageOptions{}
	if input.Message != "" {
		opts.Message = new(input.Message)
	}
	if input.StartsAt != "" {
		t, err := time.Parse(time.RFC3339, input.StartsAt)
		if err != nil {
			return nil, fmt.Errorf("invalid starts_at: %w", err)
		}
		opts.StartsAt = &t
	}
	if input.EndsAt != "" {
		t, err := time.Parse(time.RFC3339, input.EndsAt)
		if err != nil {
			return nil, fmt.Errorf("invalid ends_at: %w", err)
		}
		opts.EndsAt = &t
	}
	if input.Font != "" {
		opts.Font = new(input.Font)
	}
	if len(input.TargetAccessLevels) > 0 {
		levels := make([]gl.AccessLevelValue, len(input.TargetAccessLevels))
		for i, l := range input.TargetAccessLevels {
			levels[i] = gl.AccessLevelValue(l)
		}
		opts.TargetAccessLevels = levels
	}
	if input.TargetPath != "" {
		opts.TargetPath = new(input.TargetPath)
	}
	if input.BroadcastType != "" {
		opts.BroadcastType = new(input.BroadcastType)
	}
	if input.Dismissable != nil {
		opts.Dismissable = input.Dismissable
	}
	if input.Theme != "" {
		opts.Theme = new(input.Theme)
	}
	return opts, nil
}

// Delete.

// DeleteInput is the input for deleting a broadcast message.
type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"Broadcast message ID,required"`
}

// Delete removes a broadcast message (admin-only).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("broadcast_message_delete", "id")
	}
	_, err := client.GL().BroadcastMessage.DeleteBroadcastMessage(input.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("broadcast_message_delete", err, http.StatusForbidden,
			"requires administrator access; verify id with gitlab_broadcast_message_list; deletion is irreversible")
	}
	return nil
}
