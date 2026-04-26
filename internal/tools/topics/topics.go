// Package topics implements MCP tool handlers for GitLab project topics.
// It wraps the TopicsService from client-go v2.
package topics

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TopicItem represents a topic in output.
type TopicItem struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Title              string `json:"title"`
	Description        string `json:"description,omitempty"`
	TotalProjectsCount uint64 `json:"total_projects_count"`
	AvatarURL          string `json:"avatar_url,omitempty"`
}

// topicToItem converts the GitLab API response to the tool output format.
func topicToItem(t *gl.Topic) TopicItem {
	return TopicItem{
		ID:                 t.ID,
		Name:               t.Name,
		Title:              t.Title,
		Description:        t.Description,
		TotalProjectsCount: t.TotalProjectsCount,
		AvatarURL:          t.AvatarURL,
	}
}

// List.

// ListInput is the input for listing topics.
type ListInput struct {
	Search  string `json:"search,omitempty" jsonschema:"Filter topics by search query"`
	Page    int64  `json:"page,omitempty" jsonschema:"Page number"`
	PerPage int64  `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// ListOutput is the output for listing topics.
type ListOutput struct {
	toolutil.HintableOutput
	Topics     []TopicItem               `json:"topics"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List returns all project topics.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListTopicsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	topics, resp, err := client.GL().Topics.ListTopics(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_topics", err, http.StatusForbidden,
			"topic listing is public on most instances; search filter is case-insensitive substring; without_projects=true returns orphaned topics (admin-managed)")
	}
	items := make([]TopicItem, 0, len(topics))
	for _, t := range topics {
		items = append(items, topicToItem(t))
	}
	return ListOutput{
		Topics:     items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get.

// GetInput is the input for getting a topic.
type GetInput struct {
	TopicID int64 `json:"topic_id" jsonschema:"Topic ID,required"`
}

// GetOutput is the output for getting a topic.
type GetOutput struct {
	toolutil.HintableOutput
	Topic TopicItem `json:"topic"`
}

// Get retrieves a specific topic by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.TopicID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("get_topic", "topic_id")
	}
	topic, _, err := client.GL().Topics.GetTopic(input.TopicID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_topic", err, http.StatusNotFound,
			"verify topic id (numeric) with gitlab_topic_list; topic ids are instance-wide \u2014 names are unique per instance")
	}
	return GetOutput{Topic: topicToItem(topic)}, nil
}

// Create.

// CreateInput is the input for creating a topic.
type CreateInput struct {
	Name        string `json:"name" jsonschema:"Topic name (slug-like unique identifier),required"`
	Title       string `json:"title,omitempty" jsonschema:"Topic display title"`
	Description string `json:"description,omitempty" jsonschema:"Topic description"`
}

// CreateOutput is the output after creating a topic.
type CreateOutput struct {
	toolutil.HintableOutput
	Topic TopicItem `json:"topic"`
}

// Create creates a new project topic.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	opts := &gl.CreateTopicOptions{
		Name: new(input.Name),
	}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	topic, _, err := client.GL().Topics.CreateTopic(opts, gl.WithContext(ctx))
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithStatusHint("create_topic", err, http.StatusForbidden,
			"requires administrator access; name must be unique on the instance; title is the human-readable display name; avatar must be a valid file upload")
	}
	return CreateOutput{Topic: topicToItem(topic)}, nil
}

// Update.

// UpdateInput is the input for updating a topic.
type UpdateInput struct {
	TopicID     int64  `json:"topic_id" jsonschema:"Topic ID,required"`
	Name        string `json:"name,omitempty" jsonschema:"New topic name"`
	Title       string `json:"title,omitempty" jsonschema:"New topic title"`
	Description string `json:"description,omitempty" jsonschema:"New topic description"`
}

// UpdateOutput is the output after updating a topic.
type UpdateOutput struct {
	toolutil.HintableOutput
	Topic TopicItem `json:"topic"`
}

// Update modifies an existing topic.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	if input.TopicID <= 0 {
		return UpdateOutput{}, toolutil.ErrRequiredInt64("update_topic", "topic_id")
	}
	opts := &gl.UpdateTopicOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	topic, _, err := client.GL().Topics.UpdateTopic(input.TopicID, opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("update_topic", err, http.StatusForbidden,
			"requires administrator access; verify id with gitlab_topic_list; renaming may break existing project associations \u2014 prefer updating title and description")
	}
	return UpdateOutput{Topic: topicToItem(topic)}, nil
}

// Delete.

// DeleteInput is the input for deleting a topic.
type DeleteInput struct {
	TopicID int64 `json:"topic_id" jsonschema:"Topic ID,required"`
}

// Delete removes a project topic.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.TopicID <= 0 {
		return toolutil.ErrRequiredInt64("delete_topic", "topic_id")
	}
	_, err := client.GL().Topics.DeleteTopic(input.TopicID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_topic", err, http.StatusForbidden,
			"requires administrator access; deletion is irreversible and removes the topic from all associated projects; consider gitlab_topic_merge instead to consolidate duplicate topics")
	}
	return nil
}

// Markdown Formatters.
