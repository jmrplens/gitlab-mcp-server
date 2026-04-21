// Package customemoji implements MCP tool handlers for GitLab Custom Emoji
// management using the GraphQL API. Custom emoji are group-level assets with
// custom images, distinct from award emoji (reactions on issues/MRs).
package customemoji

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Item represents a custom emoji in a GitLab group.
type Item struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	External  bool   `json:"external"`
	CreatedAt string `json:"created_at,omitempty"`
}

// GraphQL queries and mutations.

const queryListCustomEmoji = `
query($groupPath: ID!, $first: Int!, $after: String) {
  group(fullPath: $groupPath) {
    customEmoji(first: $first, after: $after) {
      nodes {
        id
        name
        url
        external
        createdAt
      }
      pageInfo {
        hasNextPage
        hasPreviousPage
        endCursor
        startCursor
      }
    }
  }
}
`

const mutationCreateCustomEmoji = `
mutation($groupPath: ID!, $name: String!, $url: String!) {
  createCustomEmoji(input: { groupPath: $groupPath, name: $name, url: $url }) {
    customEmoji {
      id
      name
      url
      external
      createdAt
    }
    errors
  }
}
`

const mutationDeleteCustomEmoji = `
mutation($id: CustomEmojiID!) {
  destroyCustomEmoji(input: { id: $id }) {
    customEmoji {
      id
      name
    }
    errors
  }
}
`

// GraphQL response structs.

type gqlCustomEmojiNode struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	URL       string  `json:"url"`
	External  bool    `json:"external"`
	CreatedAt *string `json:"createdAt"`
}

// nodeToItem converts a raw GraphQL custom emoji node into an [Item]
// output struct, dereferencing optional pointer fields.
func nodeToItem(n gqlCustomEmojiNode) Item {
	item := Item{
		ID:       n.ID,
		Name:     n.Name,
		URL:      n.URL,
		External: n.External,
	}
	if n.CreatedAt != nil {
		item.CreatedAt = *n.CreatedAt
	}
	return item
}

// gqlCustomEmojiConnection holds the paginated list of custom emoji nodes.
type gqlCustomEmojiConnection struct {
	Nodes    []gqlCustomEmojiNode        `json:"nodes"`
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
}

// gqlGroupCustomEmoji wraps the custom emoji connection inside a group.
type gqlGroupCustomEmoji struct {
	CustomEmoji gqlCustomEmojiConnection `json:"customEmoji"`
}

// gqlCreateCustomEmojiPayload is the response payload for creating a custom emoji.
type gqlCreateCustomEmojiPayload struct {
	CustomEmoji *gqlCustomEmojiNode `json:"customEmoji"`
	Errors      []string            `json:"errors"`
}

// gqlDestroyCustomEmojiPayload is the response payload for deleting a custom emoji.
type gqlDestroyCustomEmojiPayload struct {
	Errors []string `json:"errors"`
}

// List.

// ListInput is the input for listing custom emoji.
type ListInput struct {
	GroupPath string `json:"group_path" jsonschema:"required,Group full path (e.g. my-group)"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing custom emoji.
type ListOutput struct {
	toolutil.HintableOutput
	Emoji      []Item                           `json:"emoji"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves custom emoji for a group via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupPath == "" {
		return ListOutput{}, errors.New("list_custom_emoji: group_path is required")
	}

	vars := input.GraphQLPaginationInput.Variables()
	vars["groupPath"] = input.GroupPath

	var resp struct {
		Data struct {
			Group *gqlGroupCustomEmoji `json:"group"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListCustomEmoji,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_custom_emoji", err)
	}

	if resp.Data.Group == nil {
		return ListOutput{}, fmt.Errorf("list_custom_emoji: group %q not found", input.GroupPath)
	}

	items := make([]Item, 0, len(resp.Data.Group.CustomEmoji.Nodes))
	for _, n := range resp.Data.Group.CustomEmoji.Nodes {
		items = append(items, nodeToItem(n))
	}

	return ListOutput{
		Emoji:      items,
		Pagination: toolutil.PageInfoToOutput(resp.Data.Group.CustomEmoji.PageInfo),
	}, nil
}

// Create.

// CreateInput is the input for creating a custom emoji.
type CreateInput struct {
	GroupPath string `json:"group_path" jsonschema:"required,Group full path (e.g. my-group)"`
	Name      string `json:"name"       jsonschema:"required,Emoji name without colons (e.g. party_parrot)"`
	URL       string `json:"url"        jsonschema:"required,URL to the emoji image (PNG or GIF recommended)"`
}

// CreateOutput is the output for creating a custom emoji.
type CreateOutput struct {
	toolutil.HintableOutput
	Emoji Item `json:"emoji"`
}

// Create adds a new custom emoji to a group via the GitLab GraphQL API.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	if input.GroupPath == "" {
		return CreateOutput{}, errors.New("create_custom_emoji: group_path is required")
	}
	if input.Name == "" {
		return CreateOutput{}, errors.New("create_custom_emoji: name is required")
	}
	if input.URL == "" {
		return CreateOutput{}, errors.New("create_custom_emoji: url is required")
	}

	vars := map[string]any{
		"groupPath": input.GroupPath,
		"name":      input.Name,
		"url":       input.URL,
	}

	var resp struct {
		Data struct {
			CreateCustomEmoji gqlCreateCustomEmojiPayload `json:"createCustomEmoji"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationCreateCustomEmoji,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithMessage("create_custom_emoji", err)
	}

	if len(resp.Data.CreateCustomEmoji.Errors) > 0 {
		return CreateOutput{}, fmt.Errorf("create_custom_emoji: %s", resp.Data.CreateCustomEmoji.Errors[0])
	}

	if resp.Data.CreateCustomEmoji.CustomEmoji == nil {
		return CreateOutput{}, errors.New("create_custom_emoji: no emoji returned")
	}

	return CreateOutput{
		Emoji: nodeToItem(*resp.Data.CreateCustomEmoji.CustomEmoji),
	}, nil
}

// Delete.

// DeleteInput is the input for deleting a custom emoji.
type DeleteInput struct {
	ID string `json:"id" jsonschema:"required,Custom emoji GID (e.g. gid://gitlab/CustomEmoji/1)"`
}

// Delete removes a custom emoji via the GitLab GraphQL API.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID == "" {
		return errors.New("delete_custom_emoji: id is required")
	}

	vars := map[string]any{
		"id": input.ID,
	}

	var resp struct {
		Data struct {
			DestroyCustomEmoji gqlDestroyCustomEmojiPayload `json:"destroyCustomEmoji"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationDeleteCustomEmoji,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("delete_custom_emoji", err)
	}

	if len(resp.Data.DestroyCustomEmoji.Errors) > 0 {
		return fmt.Errorf("delete_custom_emoji: %s", resp.Data.DestroyCustomEmoji.Errors[0])
	}

	return nil
}
