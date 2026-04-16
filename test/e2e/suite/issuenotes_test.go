//go:build e2e

// issuenotes_test.go — E2E tests for issue notes domain.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
)

func TestIndividual_IssueNotes(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	issue := createIssue(ctx, t, sess.individual, proj, "Issue for notes test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.individual, "gitlab_issue_note_create", issuenotes.CreateInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
			Body:      "E2E issue note",
		})
		requireNoError(t, err, "create issue note")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created issue note %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuenotes.ListOutput](ctx, sess.individual, "gitlab_issue_note_list", issuenotes.ListInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
		})
		requireNoError(t, err, "list issue notes")
		requireTrue(t, len(out.Notes) >= 1, "expected >=1 note, got %d", len(out.Notes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.individual, "gitlab_issue_note_get", issuenotes.GetInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
			NoteID:    noteID,
		})
		requireNoError(t, err, "get issue note")
		requireTrue(t, out.Body == "E2E issue note", "expected body %q, got %q", "E2E issue note", out.Body)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.individual, "gitlab_issue_note_update", issuenotes.UpdateInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
			NoteID:    noteID,
			Body:      "E2E issue note updated",
		})
		requireNoError(t, err, "update issue note")
		requireTrue(t, out.Body == "E2E issue note updated", "expected updated body, got %q", out.Body)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_issue_note_delete", issuenotes.DeleteInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
			NoteID:    noteID,
		})
		requireNoError(t, err, "delete issue note")
	})
}

func TestMeta_IssueNotes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	issue := createIssueMeta(ctx, t, sess.meta, proj, "Issue for notes meta test")

	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
				"body":       "E2E issue note meta",
			},
		})
		requireNoError(t, err, "create issue note meta")
		requireTrue(t, out.ID > 0, "expected note ID > 0, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created issue note (meta) %d", noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuenotes.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
			},
		})
		requireNoError(t, err, "list issue notes meta")
		requireTrue(t, len(out.Notes) >= 1, "expected >=1 note, got %d", len(out.Notes))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "get issue note meta")
		requireTrue(t, out.Body == "E2E issue note meta", "expected body %q, got %q", "E2E issue note meta", out.Body)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[issuenotes.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
				"note_id":    noteID,
				"body":       "E2E issue note meta updated",
			},
		})
		requireNoError(t, err, "update issue note meta")
		requireTrue(t, out.Body == "E2E issue note meta updated", "expected updated body, got %q", out.Body)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "delete issue note meta")
	})
}
