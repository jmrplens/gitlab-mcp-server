// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package topics

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TopicItem represents a topic in output.
type TopicItem struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Title              string `json:"title"`
	Description        string `json:"description,omitempty"`
	TotalProjectsCount uint64 `json:"total_projects_count"`
	OrganizationID     int64  `json:"organization_id"`
	AvatarURL          string `json:"avatar_url,omitempty"`
}

// topicToItem converts the GitLab API response to the tool output format.
func topicToItem(t *gl.Topic) TopicItem {
	return TopicItem{
		ID:                 t.ID,
		Name:               t.Name,
		Title:              t.Title,
		Description:        t.Description,
		OrganizationID:     t.OrganizationID,
		TotalProjectsCount: t.TotalProjectsCount,
		AvatarURL:          t.AvatarURL,
	}
}

// List.

// ListInput is the input for listing topics.
type ListInput struct {
	toolutil.PaginationInput
	Search          string `json:"search,omitempty" jsonschema:"Filter topics by search query"`
	WithoutProjects bool   `json:"without_projects,omitempty" jsonschema:"Return only the topics no project is assigned to"`
	OrderBy         string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort            string `json:"sort,omitempty" jsonschema:"Sort order for keyset-paginated results: 'asc' or 'desc'"`
}

// ListOutput is the output for listing topics.
type ListOutput struct {
	toolutil.HintableOutput
	Topics     []TopicItem               `json:"topics"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List returns the instance's project topics, narrowed to those whose name
// matches search when one is given, and to the topics no project is assigned
// to when without_projects is set.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListTopicsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.WithoutProjects {
		opts.WithoutProjects = new(true)
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
			"verify topic id (numeric) with admin.topic_list; topic ids are instance-wide. Names are unique per instance")
	}
	return GetOutput{Topic: topicToItem(topic)}, nil
}

// Create.

// CreateInput is the input for creating a topic.
type CreateInput struct {
	Name        string `json:"name" jsonschema:"Topic name (slug-like unique identifier),required"`
	Title       string `json:"title,omitempty" jsonschema:"Topic display title,required"`
	Description string `json:"description,omitempty" jsonschema:"Topic description"`
	// Avatar is uploaded with the create, which GitLab takes only as a
	// multipart form.
	Avatar *AvatarInput `json:"avatar,omitempty" jsonschema:"Avatar image to upload with the topic, from a local file or base64 content"`
}

// AvatarInput is an image a topic create or update uploads as the topic's
// avatar, as [gl.TopicAvatar] carries it: the file name GitLab reads the
// image type from, and the content, from a local file or as base64.
type AvatarInput struct {
	Filename      string `json:"filename" jsonschema:"Avatar file name such as avatar.png, whose extension GitLab reads the image type from,required"`
	FilePath      string `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// openAvatar opens the image an avatar input names, nil for no avatar, with
// the function that releases it.
func openAvatar(op string, in *AvatarInput) (*gl.TopicAvatar, func(), error) {
	if in == nil {
		return nil, func() { /* no avatar: nothing to release */ }, nil
	}
	if in.Filename == "" {
		return nil, nil, fmt.Errorf("%s: avatar.filename is required with an avatar, since GitLab reads the image type from its extension", op)
	}
	reader, _, release, err := toolutil.OpenFileOrBase64Source(op+" avatar", in.FilePath, in.ContentBase64)
	if err != nil {
		return nil, nil, err
	}
	return &gl.TopicAvatar{Filename: in.Filename, Image: reader}, release, nil
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
	avatar, release, err := openAvatar("create_topic", input.Avatar)
	if err != nil {
		return CreateOutput{}, err
	}
	defer release()
	opts.Avatar = avatar
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
	TopicID     int64   `json:"topic_id" jsonschema:"Topic ID,required"`
	Name        string  `json:"name,omitempty" jsonschema:"New topic name"`
	Title       string  `json:"title,omitempty" jsonschema:"New topic title"`
	Description *string `json:"description,omitempty" jsonschema:"New topic description. Pass an empty string to remove it. Omit the field to leave it untouched"`
	// Avatar is uploaded with the change, which GitLab takes only as a
	// multipart form.
	Avatar *AvatarInput `json:"avatar,omitempty" jsonschema:"New avatar image to upload for the topic, from a local file or base64 content. Cannot be combined with remove_avatar"`
	// RemoveAvatar sends avatar as an empty value, which PUT /topics/:id
	// answers by removing the topic's avatar (lib/api/topics.rb).
	RemoveAvatar bool `json:"remove_avatar,omitempty" jsonschema:"Remove the topic's avatar. Cannot be combined with avatar"`
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
	// Presence decides, never emptiness: an empty description is the caller
	// removing it (issue 1306).
	opts.Description = input.Description
	if input.RemoveAvatar && input.Avatar != nil {
		return UpdateOutput{}, errors.New("update_topic: avatar and remove_avatar cannot be combined. Give avatar to replace the image or remove_avatar to remove it")
	}
	avatar, release, err := openAvatar("update_topic", input.Avatar)
	if err != nil {
		return UpdateOutput{}, err
	}
	defer release()
	opts.Avatar = avatar
	if input.RemoveAvatar {
		// An avatar with neither a file name nor an image is the one
		// client-go writes as "avatar": "" in the JSON body rather than
		// uploading.
		opts.Avatar = &gl.TopicAvatar{}
	}
	topic, _, err := client.GL().Topics.UpdateTopic(input.TopicID, opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("update_topic", err, http.StatusForbidden,
			"requires administrator access; verify id with admin.topic_list; renaming may break existing project associations. Prefer updating title and description")
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
			"requires administrator access; deletion is irreversible and removes the topic from all associated projects; consider merging topics first via the GitLab UI or Topics API (merge is not exposed here) instead to consolidate duplicate topics")
	}
	return nil
}

// Markdown Formatters.
