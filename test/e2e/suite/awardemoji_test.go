//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
)

func TestIndividual_AwardEmoji(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	issue := createIssue(ctx, t, sess.individual, proj, "emoji-test")

	var awardID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[awardemoji.Output](ctx, sess.individual, "gitlab_issue_emoji_create", awardemoji.CreateInput{
			ProjectID: proj.pidOf(),
			IID:       issue.IID,
			Name:      "thumbsup",
		})
		requireNoError(t, err, "create award emoji")
		requireTrue(t, out.ID > 0, "expected award ID")
		awardID = out.ID
		t.Logf("Created emoji %d (%s)", awardID, out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.individual, "gitlab_issue_emoji_list", awardemoji.ListInput{
			ProjectID: proj.pidOf(),
			IID:       issue.IID,
		})
		requireNoError(t, err, "list award emoji")
		requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 emoji")
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, awardID > 0, "awardID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.individual, "gitlab_issue_emoji_get", awardemoji.GetInput{
			ProjectID: proj.pidOf(),
			IID:       issue.IID,
			AwardID:   awardID,
		})
		requireNoError(t, err, "get award emoji")
		requireTrue(t, out.ID == awardID, "expected ID %d", awardID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, awardID > 0, "awardID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_issue_emoji_delete", awardemoji.DeleteInput{
			ProjectID: proj.pidOf(),
			IID:       issue.IID,
			AwardID:   awardID,
		})
		requireNoError(t, err, "delete award emoji")
	})
}

func TestMeta_AwardEmoji(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	issue := createIssueMeta(ctx, t, sess.meta, proj, "meta-emoji-test")

	var awardID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"iid":        issue.IID,
				"name":       "thumbsup",
			},
		})
		requireNoError(t, err, "meta create emoji")
		requireTrue(t, out.ID > 0, "expected award ID")
		awardID = out.ID
		t.Logf("Created emoji %d via meta-tool", awardID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"iid":        issue.IID,
			},
		})
		requireNoError(t, err, "meta list emoji")
		requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 emoji")
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, awardID > 0, "awardID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"iid":        issue.IID,
				"award_id":   awardID,
			},
		})
		requireNoError(t, err, "meta get emoji")
		requireTrue(t, out.ID == awardID, "expected ID %d", awardID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, awardID > 0, "awardID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"iid":        issue.IID,
				"award_id":   awardID,
			},
		})
		requireNoError(t, err, "meta delete emoji")
	})
}
