//go:build e2e

// mrnotes_test.go tests the MR note MCP tools against a live GitLab instance.
// Covers note create, list, get, update, and delete for both individual tools
// and the gitlab_mr_review meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
)

// TestIndividual_MRNotes exercises the MR note lifecycle using individual tools:
// create → list → get → update → delete.
func TestIndividual_MRNotes(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)
	mr := createMR(ctx, t, sess.individual, proj, branch, defaultBranch, "MR for notes test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.individual, "gitlab_mr_note_create", mrnotes.CreateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			Body:      "E2E note on MR",
		})
		requireNoError(t, err, "create MR note")
		requireTruef(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created MR note %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrnotes.ListOutput](ctx, sess.individual, "gitlab_mr_notes_list", mrnotes.ListInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, err, "list MR notes")
		requireTruef(t, len(out.Notes) >= 1, "expected >=1 note, got %d", len(out.Notes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.individual, "gitlab_mr_note_get", mrnotes.GetInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    noteID,
		})
		requireNoError(t, err, "get MR note")
		requireTruef(t, out.Body == "E2E note on MR", "expected body %q, got %q", "E2E note on MR", out.Body)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.individual, "gitlab_mr_note_update", mrnotes.UpdateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    noteID,
			Body:      "E2E note updated",
		})
		requireNoError(t, err, "update MR note")
		requireTruef(t, out.Body == "E2E note updated", "expected updated body, got %q", out.Body)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_note_delete", mrnotes.DeleteInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
			NoteID:    noteID,
		})
		requireNoError(t, err, "delete MR note")
	})
}

// TestMeta_MRNotes exercises the same MR note lifecycle via the gitlab_mr_review meta-tool.
func TestMeta_MRNotes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj, branch := setupMRProjectMeta(ctx, t, sess.meta)
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "MR for notes meta test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"body":              "E2E note meta",
			},
		})
		requireNoError(t, err, "create MR note meta")
		requireTruef(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created MR note (meta) %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mrnotes.ListOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "note_list",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "list MR notes meta")
		requireTruef(t, len(out.Notes) >= 1, "expected >=1 note, got %d", len(out.Notes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "note_get",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           noteID,
			},
		})
		requireNoError(t, err, "get MR note meta")
		requireTruef(t, out.Body == "E2E note meta", "expected body %q, got %q", "E2E note meta", out.Body)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mrnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "note_update",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           noteID,
				"body":              "E2E note meta updated",
			},
		})
		requireNoError(t, err, "update MR note meta")
		requireTruef(t, out.Body == "E2E note meta updated", "expected updated body, got %q", out.Body)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "note_delete",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"note_id":           noteID,
			},
		})
		requireNoError(t, err, "delete MR note meta")
	})
}
