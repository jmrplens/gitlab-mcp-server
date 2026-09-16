//go:build e2e

// custom_emoji.go builds a group's custom emoji, which GitLab offers over
// GraphQL and nowhere else.
//
// The image is served by the fixture service the Docker stack runs, because
// GitLab fetches the URL when the emoji is created and an address it cannot
// reach is refused. That is what makes NeedFixtureService the requirement of
// every case whose world this is.

package fixture

import (
	"context"
	"fmt"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// customEmojiImage is the path the fixture service serves the image at.
const customEmojiImage = "/emoji.png"

// customEmojiName is what a fixture emoji is called. GitLab accepts letters,
// digits and underscores, and holds the name unique within its group, which
// is why every fixture emoji lives in a group of its own rather than needing
// a generated name.
const customEmojiName = "e2e_eval_emoji"

// The documents the builder sends. They are written here rather than borrowed
// from the tool under test, so that a fixture stays a fixture: a builder
// sharing the tool's document would go green or red with it.
const (
	createCustomEmojiMutation = `
mutation($groupPath: ID!, $name: String!, $url: String!) {
  createCustomEmoji(input: { groupPath: $groupPath, name: $name, url: $url }) {
    customEmoji { id name url }
    errors
  }
}
`
	destroyCustomEmojiMutation = `
mutation($id: CustomEmojiID!) {
  destroyCustomEmoji(input: { id: $id }) {
    customEmoji { id }
    errors
  }
}
`
	listCustomEmojiQuery = `
query($groupPath: ID!) {
  group(fullPath: $groupPath) {
    customEmoji(first: 100) { nodes { id } }
  }
}
`
)

// CustomEmoji is a custom emoji a builder created in a group.
type CustomEmoji struct {
	// ID is the global identifier every emoji action takes, which is a
	// gid:// string rather than a number.
	ID string
	// Name is what it was created as.
	Name string
	// URL is the image GitLab fetched.
	URL string
}

// NewCustomEmoji creates a custom emoji in the group from the image the
// fixture service serves, and registers its removal.
func NewCustomEmoji(e *harness.Env, group Group) CustomEmoji {
	e.T.Helper()

	image := ServiceURL(e, customEmojiImage)
	emoji, err := retryTransient(e, "create custom emoji "+customEmojiName, createRetries, func() (CustomEmoji, error) {
		return createCustomEmoji(e.Ctx, e.Client(), group.Path, customEmojiName, image)
	})
	if err != nil {
		e.T.Fatalf("creating custom emoji %q in group %s: %v", customEmojiName, group.Path, err)
	}

	e.Defer("custom emoji "+emoji.ID, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteCustomEmoji(ctx, e.Client(), emoji.ID)
	})
	return emoji
}

// createCustomEmoji asks GitLab for the emoji.
func createCustomEmoji(ctx context.Context, client *gitlabclient.Client, groupPath, name, url string) (CustomEmoji, error) {
	var created struct {
		CreateCustomEmoji struct {
			CustomEmoji *CustomEmoji `json:"customEmoji"`
			Errors      []string     `json:"errors"`
		} `json:"createCustomEmoji"`
	}
	err := runGraphQL(ctx, client, createCustomEmojiMutation, map[string]any{
		"groupPath": groupPath, "name": name, "url": url,
	}, &created)
	if err != nil {
		return CustomEmoji{}, err
	}
	if refusals := created.CreateCustomEmoji.Errors; len(refusals) > 0 {
		return CustomEmoji{}, fmt.Errorf("GitLab refused the emoji: %s", strings.Join(refusals, "; "))
	}
	if created.CreateCustomEmoji.CustomEmoji == nil || created.CreateCustomEmoji.CustomEmoji.ID == "" {
		return CustomEmoji{}, fmt.Errorf("the emoji mutation answered no emoji for group %s", groupPath)
	}
	return *created.CreateCustomEmoji.CustomEmoji, nil
}

// deleteCustomEmoji removes the emoji and tolerates one a case deleted: a
// destroy of an emoji that is gone answers a document error rather than a
// status, so the refusal is read from the payload.
func deleteCustomEmoji(ctx context.Context, client *gitlabclient.Client, id string) error {
	var destroyed struct {
		DestroyCustomEmoji struct {
			Errors []string `json:"errors"`
		} `json:"destroyCustomEmoji"`
	}
	err := runGraphQL(ctx, client, destroyCustomEmojiMutation, map[string]any{"id": id}, &destroyed)
	if err != nil {
		if customEmojiGone(err.Error()) {
			return nil
		}
		return fmt.Errorf("deleting custom emoji %s: %w", id, err)
	}
	if refusals := destroyed.DestroyCustomEmoji.Errors; len(refusals) > 0 && !customEmojiGone(strings.Join(refusals, "; ")) {
		return fmt.Errorf("deleting custom emoji %s: %s", id, strings.Join(refusals, "; "))
	}
	return nil
}

// customEmojiGone reports whether a refusal is GitLab saying the emoji is not
// there, which is the ending a case that deleted it leaves behind.
func customEmojiGone(message string) bool {
	lowered := strings.ToLower(message)
	return strings.Contains(lowered, "not found") || strings.Contains(lowered, "does not exist")
}

// CustomEmojiExists reports whether the group still holds the emoji, which is
// what a case that deletes one is verified against.
func CustomEmojiExists(ctx context.Context, client *gitlabclient.Client, groupPath, id string) (bool, error) {
	var listed struct {
		Group *struct {
			CustomEmoji struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"customEmoji"`
		} `json:"group"`
	}
	if err := runGraphQL(ctx, client, listCustomEmojiQuery, map[string]any{"groupPath": groupPath}, &listed); err != nil {
		return false, fmt.Errorf("listing the custom emoji of group %s: %w", groupPath, err)
	}
	if listed.Group == nil {
		return false, nil
	}
	for _, node := range listed.Group.CustomEmoji.Nodes {
		if node.ID == id {
			return true, nil
		}
	}
	return false, nil
}
