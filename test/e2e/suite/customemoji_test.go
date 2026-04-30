//go:build e2e

// customemoji_test.go tests the custom emoji MCP tools against a live
// GitLab instance using both individual tools and the gitlab_custom_emoji
// meta-tool. Exercises custom emoji create → list → delete lifecycle.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIndividual_CustomEmoji exercises custom emoji CRUD via individual tools:
// create → list → delete.
func TestIndividual_CustomEmoji(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	groupName := uniqueName("e2e-custom-emoji-ind")
	emojiURL := e2eFixtureServiceURL("/emoji.png")

	// Create a temporary group for custom emoji.
	groupOut, groupErr := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name: groupName,
		Path: groupName,
	})
	requireNoError(t, groupErr, "create group for custom emoji")
	t.Cleanup(func() {
		_ = callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupOut.Path),
		})
	})

	var emojiGID string

	t.Run("Individual/CustomEmoji/Create", func(t *testing.T) {
		out, err := callToolOn[customemoji.CreateOutput](ctx, sess.individual, "gitlab_create_custom_emoji", customemoji.CreateInput{
			GroupPath: groupOut.Path,
			Name:      "e2e_test_emoji",
			URL:       emojiURL,
		})
		requireNoError(t, err, "create custom emoji")
		requireTruef(t, out.Emoji.ID != "", "expected non-empty emoji GID")
		emojiGID = out.Emoji.ID
		t.Logf("Created custom emoji %s (%s)", out.Emoji.Name, out.Emoji.ID)
	})

	t.Run("Individual/CustomEmoji/List", func(t *testing.T) {
		out, err := callToolOn[customemoji.ListOutput](ctx, sess.individual, "gitlab_list_custom_emoji", customemoji.ListInput{
			GroupPath: groupOut.Path,
		})
		requireNoError(t, err, "list custom emoji")
		requireTruef(t, len(out.Emoji) >= 1, "expected at least 1 custom emoji, got %d", len(out.Emoji))
		t.Logf("Group %s has %d custom emoji", groupOut.Path, len(out.Emoji))
	})

	t.Run("Individual/CustomEmoji/Delete", func(t *testing.T) {
		requireTruef(t, emojiGID != "", "emojiGID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_custom_emoji", customemoji.DeleteInput{
			ID: emojiGID,
		})
		requireNoError(t, err, "delete custom emoji")
		t.Logf("Deleted custom emoji %s", emojiGID)
	})
}

// TestMeta_CustomEmoji exercises custom emoji CRUD via the gitlab_custom_emoji meta-tool:
// create → list → delete.
func TestMeta_CustomEmoji(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	groupName := uniqueName("e2e-custom-emoji-meta")
	emojiURL := e2eFixtureServiceURL("/emoji.png")

	groupOut, groupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name": groupName,
			"path": groupName,
		},
	})
	requireNoError(t, groupErr, "create group for custom emoji (meta)")
	t.Cleanup(func() {
		_ = callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupOut.Path),
		})
	})

	var emojiGID string

	t.Run("Meta/CustomEmoji/Create", func(t *testing.T) {
		out, err := callToolOn[customemoji.CreateOutput](ctx, sess.meta, "gitlab_custom_emoji", map[string]any{
			"action": "create",
			"params": map[string]any{
				"group_path": groupOut.Path,
				"name":       "e2e_test_emoji_meta",
				"url":        emojiURL,
			},
		})
		requireNoError(t, err, "meta custom emoji create")
		requireTruef(t, out.Emoji.ID != "", "expected non-empty emoji GID")
		emojiGID = out.Emoji.ID
		t.Logf("Created custom emoji (meta) %s (%s)", out.Emoji.Name, out.Emoji.ID)
	})

	t.Run("Meta/CustomEmoji/List", func(t *testing.T) {
		out, err := callToolOn[customemoji.ListOutput](ctx, sess.meta, "gitlab_custom_emoji", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_path": groupOut.Path,
			},
		})
		requireNoError(t, err, "meta custom emoji list")
		requireTruef(t, len(out.Emoji) >= 1, "expected at least 1 custom emoji, got %d", len(out.Emoji))
		t.Logf("Group %s has %d custom emoji (meta)", groupOut.Path, len(out.Emoji))
	})

	t.Run("Meta/CustomEmoji/Delete", func(t *testing.T) {
		requireTruef(t, emojiGID != "", "emojiGID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_custom_emoji", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"id": emojiGID,
			},
		})
		requireNoError(t, err, "meta custom emoji delete")
		t.Logf("Deleted custom emoji (meta) %s", emojiGID)
	})
}
