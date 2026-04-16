//go:build e2e

// mrdiscussions_test.go — E2E tests for MR discussions domain.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
)

func TestIndividual_MRDiscussions(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)
	mr := createMR(ctx, t, sess.individual, proj, branch, defaultBranch, "MR for discussions test")

	var discussionID string
	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.individual, "gitlab_mr_discussion_create", mrdiscussions.CreateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			Body:      "E2E discussion thread",
		})
		requireNoError(t, err, "create MR discussion")
		requireTrue(t, out.ID != "", "expected discussion ID, got empty")
		discussionID = out.ID
		requireTrue(t, len(out.Notes) >= 1, "expected >=1 note in discussion")
		noteID = out.Notes[0].ID
		t.Logf("Created discussion %s (note %d)", discussionID, noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.ListOutput](ctx, sess.individual, "gitlab_mr_discussion_list", mrdiscussions.ListInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "list MR discussions")
		requireTrue(t, len(out.Discussions) >= 1, "expected >=1 discussion, got %d", len(out.Discussions))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.individual, "gitlab_mr_discussion_get", mrdiscussions.GetInput{
			ProjectID:    proj.pidOf(),
			MRIID:        mr.IID,
			DiscussionID: discussionID,
		})
		requireNoError(t, err, "get MR discussion")
		requireTrue(t, out.ID == discussionID, "expected discussion %q, got %q", discussionID, out.ID)
	})

	t.Run("Reply", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.NoteOutput](ctx, sess.individual, "gitlab_mr_discussion_reply", mrdiscussions.ReplyInput{
			ProjectID:    proj.pidOf(),
			MRIID:        mr.IID,
			DiscussionID: discussionID,
			Body:         "E2E reply",
		})
		requireNoError(t, err, "reply to MR discussion")
		requireTrue(t, out.ID > 0, "expected reply note ID > 0, got %d", out.ID)
	})

	t.Run("Resolve", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.individual, "gitlab_mr_discussion_resolve", mrdiscussions.ResolveInput{
			ProjectID:    proj.pidOf(),
			MRIID:        mr.IID,
			DiscussionID: discussionID,
			Resolved:     true,
		})
		requireNoError(t, err, "resolve MR discussion")
		_ = out
	})

	t.Run("NoteDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_discussion_note_delete", mrdiscussions.DeleteNoteInput{
			ProjectID:    proj.pidOf(),
			MRIID:        mr.IID,
			DiscussionID: discussionID,
			NoteID:       noteID,
		})
		requireNoError(t, err, "delete MR discussion note")
	})
}

func TestMeta_MRDiscussions(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProjectMeta(ctx, t, sess.meta)
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "MR for discussions meta test")

	var discussionID string
	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
				"body":       "E2E discussion meta",
			},
		})
		requireNoError(t, err, "create MR discussion meta")
		requireTrue(t, out.ID != "", "expected discussion ID, got empty")
		discussionID = out.ID
		requireTrue(t, len(out.Notes) >= 1, "expected >=1 note")
		noteID = out.Notes[0].ID
		t.Logf("Created discussion (meta) %s", discussionID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.ListOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "list MR discussions meta")
		requireTrue(t, len(out.Discussions) >= 1, "expected >=1 discussion, got %d", len(out.Discussions))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"mr_iid":        mr.IID,
				"discussion_id": discussionID,
			},
		})
		requireNoError(t, err, "get MR discussion meta")
		requireTrue(t, out.ID == discussionID, "expected discussion %q, got %q", discussionID, out.ID)
	})

	t.Run("Reply", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.NoteOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_reply",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"mr_iid":        mr.IID,
				"discussion_id": discussionID,
				"body":          "E2E reply meta",
			},
		})
		requireNoError(t, err, "reply MR discussion meta")
		requireTrue(t, out.ID > 0, "expected reply note ID > 0")
	})

	t.Run("Resolve", func(t *testing.T) {
		out, err := callToolOn[mrdiscussions.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_resolve",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"mr_iid":        mr.IID,
				"discussion_id": discussionID,
				"resolved":      true,
			},
		})
		requireNoError(t, err, "resolve MR discussion meta")
		_ = out
	})

	t.Run("NoteDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "discussion_note_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"mr_iid":        mr.IID,
				"discussion_id": discussionID,
				"note_id":       noteID,
			},
		})
		requireNoError(t, err, "delete MR discussion note meta")
	})
}
