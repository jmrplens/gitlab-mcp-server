//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuediscussions"
)

func TestIndividual_IssueDiscussions(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	issue := createIssue(ctx, t, sess.individual, proj, "discussion-test")

	var (
		discussionID string
		noteID       int64
	)

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuediscussions.Output](ctx, sess.individual, "gitlab_create_issue_discussion", issuediscussions.CreateInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
			Body:      "E2E discussion body",
		})
		requireNoError(t, err, "create issue discussion")
		requireTrue(t, out.ID != "", "expected discussion ID")
		discussionID = out.ID
		requireTrue(t, len(out.Notes) > 0, "expected at least one note")
		noteID = out.Notes[0].ID
		t.Logf("Created discussion %s (note %d)", discussionID, noteID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuediscussions.ListOutput](ctx, sess.individual, "gitlab_list_issue_discussions", issuediscussions.ListInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
		})
		requireNoError(t, err, "list issue discussions")
		requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion")
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[issuediscussions.Output](ctx, sess.individual, "gitlab_get_issue_discussion", issuediscussions.GetInput{
			ProjectID:    proj.pidOf(),
			IssueIID:     issue.IID,
			DiscussionID: discussionID,
		})
		requireNoError(t, err, "get issue discussion")
		requireTrue(t, out.ID == discussionID, "expected discussion %s", discussionID)
	})

	t.Run("AddNote", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[issuediscussions.NoteOutput](ctx, sess.individual, "gitlab_add_issue_discussion_note", issuediscussions.AddNoteInput{
			ProjectID:    proj.pidOf(),
			IssueIID:     issue.IID,
			DiscussionID: discussionID,
			Body:         "E2E reply note",
		})
		requireNoError(t, err, "add discussion note")
		requireTrue(t, out.ID > 0, "expected note ID")
		t.Logf("Added note %d", out.ID)
	})

	t.Run("UpdateNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[issuediscussions.NoteOutput](ctx, sess.individual, "gitlab_update_issue_discussion_note", issuediscussions.UpdateNoteInput{
			ProjectID:    proj.pidOf(),
			IssueIID:     issue.IID,
			DiscussionID: discussionID,
			NoteID:       noteID,
			Body:         "E2E updated note body",
		})
		requireNoError(t, err, "update discussion note")
		requireTrue(t, out.Body == "E2E updated note body", "expected updated body")
	})

	t.Run("DeleteNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_issue_discussion_note", issuediscussions.DeleteNoteInput{
			ProjectID:    proj.pidOf(),
			IssueIID:     issue.IID,
			DiscussionID: discussionID,
			NoteID:       noteID,
		})
		requireNoError(t, err, "delete discussion note")
	})
}

func TestMeta_IssueDiscussions(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	issue := createIssueMeta(ctx, t, sess.meta, proj, "meta-discussion-test")

	var (
		discussionID string
		noteID       int64
	)

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuediscussions.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
				"body":       "E2E meta discussion body",
			},
		})
		requireNoError(t, err, "meta create discussion")
		requireTrue(t, out.ID != "", "expected discussion ID")
		discussionID = out.ID
		requireTrue(t, len(out.Notes) > 0, "expected at least one note")
		noteID = out.Notes[0].ID
		t.Logf("Created discussion %s via meta-tool", discussionID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuediscussions.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue.IID,
			},
		})
		requireNoError(t, err, "meta list discussions")
		requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion")
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[issuediscussions.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issue.IID,
				"discussion_id": discussionID,
			},
		})
		requireNoError(t, err, "meta get discussion")
		requireTrue(t, out.ID == discussionID, "expected discussion %s", discussionID)
	})

	t.Run("AddNote", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[issuediscussions.NoteOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_add_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issue.IID,
				"discussion_id": discussionID,
				"body":          "E2E meta reply",
			},
		})
		requireNoError(t, err, "meta add note")
		requireTrue(t, out.ID > 0, "expected note ID")
	})

	t.Run("UpdateNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[issuediscussions.NoteOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_update_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issue.IID,
				"discussion_id": discussionID,
				"note_id":       noteID,
				"body":          "E2E meta updated note",
			},
		})
		requireNoError(t, err, "meta update note")
		requireTrue(t, out.Body == "E2E meta updated note", "expected updated body")
	})

	t.Run("DeleteNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "discussion_delete_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issue.IID,
				"discussion_id": discussionID,
				"note_id":       noteID,
			},
		})
		requireNoError(t, err, "meta delete note")
	})
}
