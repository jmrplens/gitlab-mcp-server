//go:build e2e

// mrdraftnotes_test.go — E2E tests for MR draft notes domain.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
)

func TestIndividual_MRDraftNotes(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)
	mr := createMR(ctx, t, sess.individual, proj, branch, defaultBranch, "MR for draft notes test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.individual, "gitlab_mr_draft_note_create", mrdraftnotes.CreateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			Note:      "E2E draft note",
		})
		requireNoError(t, err, "create draft note")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created draft note %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.ListOutput](ctx, sess.individual, "gitlab_mr_draft_note_list", mrdraftnotes.ListInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "list draft notes")
		requireTrue(t, len(out.DraftNotes) >= 1, "expected >=1 draft note, got %d", len(out.DraftNotes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.individual, "gitlab_mr_draft_note_get", mrdraftnotes.GetInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    noteID,
		})
		requireNoError(t, err, "get draft note")
		requireTrue(t, out.Note == "E2E draft note", "expected note %q, got %q", "E2E draft note", out.Note)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.individual, "gitlab_mr_draft_note_update", mrdraftnotes.UpdateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    noteID,
			Note:      "E2E draft note updated",
		})
		requireNoError(t, err, "update draft note")
		requireTrue(t, out.Note == "E2E draft note updated", "expected updated note, got %q", out.Note)
	})

	t.Run("PublishAll", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_draft_note_publish_all", mrdraftnotes.PublishAllInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "publish all draft notes")
	})
}

func TestMeta_MRDraftNotes(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProjectMeta(ctx, t, sess.meta)
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "MR for draft notes meta test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
				"note":       "E2E draft note meta",
			},
		})
		requireNoError(t, err, "create draft note meta")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created draft note (meta) %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.ListOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "list draft notes meta")
		requireTrue(t, len(out.DraftNotes) >= 1, "expected >=1 draft note, got %d", len(out.DraftNotes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "get draft note meta")
		requireTrue(t, out.Note == "E2E draft note meta", "expected note %q, got %q", "E2E draft note meta", out.Note)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
				"note_id":    noteID,
				"note":       "E2E draft meta updated",
			},
		})
		requireNoError(t, err, "update draft note meta")
		requireTrue(t, out.Note == "E2E draft meta updated", "expected updated note, got %q", out.Note)
	})

	t.Run("PublishAll", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_publish_all",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mr.IID,
			},
		})
		requireNoError(t, err, "publish all draft notes meta")
	})
}
