//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIndividual_CustomEmoji exercises custom emoji listing via the individual tool.
func TestIndividual_CustomEmoji(t *testing.T) {
	ctx := context.Background()

	// Create a temporary group for custom emoji listing.
	groupOut, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name: "e2e-custom-emoji-ind",
		Path: "e2e-custom-emoji-ind",
	})
	requireNoError(t, err, "create group for custom emoji")
	t.Cleanup(func() {
		_ = callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupOut.Path),
		})
	})

	t.Run("Individual/CustomEmoji/List", func(t *testing.T) {
		out, err := callToolOn[customemoji.ListOutput](ctx, sess.individual, "gitlab_list_custom_emoji", customemoji.ListInput{
			GroupPath: groupOut.Path,
		})
		requireNoError(t, err, "list custom emoji")
		t.Logf("Group %s has %d custom emoji", groupOut.Path, len(out.Emoji))
	})
}

// TestMeta_CustomEmoji exercises custom emoji listing via the gitlab_custom_emoji meta-tool.
func TestMeta_CustomEmoji(t *testing.T) {
	ctx := context.Background()

	groupOut, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name": "e2e-custom-emoji-meta",
			"path": "e2e-custom-emoji-meta",
		},
	})
	requireNoError(t, err, "create group for custom emoji (meta)")
	t.Cleanup(func() {
		_ = callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupOut.Path),
		})
	})

	t.Run("Meta/CustomEmoji/List", func(t *testing.T) {
		out, err := callToolOn[customemoji.ListOutput](ctx, sess.meta, "gitlab_custom_emoji", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_path": groupOut.Path,
			},
		})
		requireNoError(t, err, "meta custom emoji list")
		t.Logf("Group %s has %d custom emoji (meta)", groupOut.Path, len(out.Emoji))
	})
}
