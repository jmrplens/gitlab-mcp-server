//go:build e2e

// mrdraftnotes_test.go tests the MR draft note MCP tools against a live GitLab instance.
// Covers draft note create, list, get, update, and publish-all for both
// individual tools and the gitlab_mr_review meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
)

// TestIndividual_MRDraftNotes exercises the MR draft note lifecycle using individual tools:
// create → list → get → update → delete (2nd note) → publish all.
func TestIndividual_MRDraftNotes(t *testing.T) {
	t.Parallel()
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

	// Create a second draft note and delete it to exercise the destructive delete path.
	var deleteNoteID int64

	t.Run("CreateForDelete", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.individual, "gitlab_mr_draft_note_create", mrdraftnotes.CreateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			Note:      "E2E draft note to delete",
		})
		requireNoError(t, err, "create draft note for delete")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		deleteNoteID = out.ID
		t.Logf("Created draft note %d (for deletion)", deleteNoteID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, deleteNoteID > 0, "deleteNoteID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_draft_note_delete", mrdraftnotes.DeleteInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    deleteNoteID,
		})
		requireNoError(t, err, "delete draft note")
		t.Logf("Deleted draft note %d", deleteNoteID)
	})

	t.Run("PublishAll", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_draft_note_publish_all", mrdraftnotes.PublishAllInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "publish all draft notes")
	})
}

// TestMeta_MRDraftNotes exercises the same MR draft note lifecycle via the gitlab_mr_review meta-tool.
// Includes: create → list → get → update → delete (2nd note) → publish all.
func TestMeta_MRDraftNotes(t *testing.T) {
	t.Parallel()
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
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note":              "E2E draft note meta",
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
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "list draft notes meta")
		requireTrue(t, len(out.DraftNotes) >= 1, "expected >=1 draft note, got %d", len(out.DraftNotes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_get",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           noteID,
			},
		})
		requireNoError(t, err, "get draft note meta")
		requireTrue(t, out.Note == "E2E draft note meta", "expected note %q, got %q", "E2E draft note meta", out.Note)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_update",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           noteID,
				"note":              "E2E draft meta updated",
			},
		})
		requireNoError(t, err, "update draft note meta")
		requireTrue(t, out.Note == "E2E draft meta updated", "expected updated note, got %q", out.Note)
	})

	// Create a second draft note and delete it to exercise the destructive delete path.
	var deleteNoteID int64

	t.Run("CreateForDelete", func(t *testing.T) {
		out, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_create",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note":              "E2E draft note meta to delete",
			},
		})
		requireNoError(t, err, "create draft note meta for delete")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		deleteNoteID = out.ID
		t.Logf("Created draft note (meta) %d (for deletion)", deleteNoteID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, deleteNoteID > 0, "deleteNoteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_delete",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           deleteNoteID,
			},
		})
		requireNoError(t, err, "delete draft note meta")
		t.Logf("Deleted draft note (meta) %d", deleteNoteID)
	})

	t.Run("PublishAll", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "draft_note_publish_all",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "publish all draft notes meta")
	})
}
