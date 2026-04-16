//go:build e2e

package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
)

// TestMeta_MRReviewChanges exercises changes_get, diff_versions_list, and
// diff_version_get via the gitlab_mr_review meta-tool.
func TestMeta_MRReviewChanges(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	commitFileMeta(ctx, t, sess.meta, proj, "main", "base.txt", "base", "base commit")

	// Create a branch and MR to get changes
	callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "branch_name": "feat-changes", "ref": "main"},
	})
	commitFileMeta(ctx, t, sess.meta, proj, "feat-changes", "new.txt", "new content", "add file")

	mrOut, err := callToolOn[struct {
		IID int64 `json:"mr_iid"`
	}](ctx, sess.meta, "gitlab_merge_request", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"source_branch": "feat-changes",
			"target_branch": "main",
			"title":         "MR for changes test",
		},
	})
	requireNoError(t, err, "create MR")
	requireTrue(t, mrOut.IID > 0, "MR IID should be > 0")
	mrIID := strconv.FormatInt(mrOut.IID, 10)

	t.Run("ChangesGet", func(t *testing.T) {
		out, err := callToolOn[mrchanges.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
			"action": "changes_get",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "changes_get")
		t.Logf("Changes: %d files, truncated: %d", len(out.Changes), len(out.TruncatedFiles))
	})

	t.Run("DiffVersionsList", func(t *testing.T) {
		drainSidekiq(ctx, t)
		var out mrchanges.DiffVersionsListOutput
		var err error
		deadline := time.Now().Add(120 * time.Second)
		delay := time.Second
		for time.Now().Before(deadline) {
			out, err = callToolOn[mrchanges.DiffVersionsListOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
				"action": "diff_versions_list",
				"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
			})
			if err == nil && len(out.DiffVersions) > 0 {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("context canceled waiting for diff versions: %v", ctx.Err())
			case <-time.After(delay):
			}
			if delay < 4*time.Second {
				delay *= 2
			}
		}
		requireNoError(t, err, "diff_versions_list")
		requireTrue(t, len(out.DiffVersions) > 0, "expected at least 1 diff version")
		t.Logf("Diff versions: %d", len(out.DiffVersions))

		t.Run("DiffVersionGet", func(t *testing.T) {
			versionID := strconv.FormatInt(out.DiffVersions[0].ID, 10)
			vOut, err := callToolOn[mrchanges.DiffVersionOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
				"action": "diff_version_get",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"mr_iid":     mrIID,
					"version_id": versionID,
				},
			})
			requireNoError(t, err, "diff_version_get")
			requireTrue(t, vOut.ID > 0, "expected diff version ID > 0")
			t.Logf("Diff version %d: state=%s, commits=%d", vOut.ID, vOut.State, len(vOut.Commits))
		})
	})
}

// TestMeta_MRReviewDiscussionNoteUpdate exercises the discussion_note_update
// action which updates body/resolved on an existing discussion note.
func TestMeta_MRReviewDiscussionNoteUpdate(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	commitFileMeta(ctx, t, sess.meta, proj, "main", "disc.txt", "v1", "initial")

	callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "branch_name": "feat-disc-note", "ref": "main"},
	})
	commitFileMeta(ctx, t, sess.meta, proj, "feat-disc-note", "disc-branch.txt", "v2", "add branch file")

	mrOut, err := callToolOn[struct {
		IID int64 `json:"mr_iid"`
	}](ctx, sess.meta, "gitlab_merge_request", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"source_branch": "feat-disc-note",
			"target_branch": "main",
			"title":         "MR for discussion note update",
		},
	})
	requireNoError(t, err, "create MR")
	mrIID := strconv.FormatInt(mrOut.IID, 10)

	// Create a discussion
	discOut, err := callToolOn[mrdiscussions.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
		"action": "discussion_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"mr_iid":     mrIID,
			"body":       "Original discussion note",
		},
	})
	requireNoError(t, err, "discussion_create")
	discID := discOut.ID

	// Get note ID from the discussion
	discDetail, err := callToolOn[mrdiscussions.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
		"action": "discussion_get",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"mr_iid":        mrIID,
			"discussion_id": discID,
		},
	})
	requireNoError(t, err, "discussion_get for note ID")
	requireTrue(t, len(discDetail.Notes) > 0, "expected at least one note in discussion")
	noteID := strconv.FormatInt(discDetail.Notes[0].ID, 10)

	// Update the note body
	updOut, err := callToolOn[mrdiscussions.NoteOutput](ctx, sess.meta, "gitlab_mr_review", map[string]any{
		"action": "discussion_note_update",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"mr_iid":        mrIID,
			"discussion_id": discID,
			"note_id":       noteID,
			"body":          "Updated discussion note body",
		},
	})
	requireNoError(t, err, "discussion_note_update")
	requireTrue(t, updOut.ID > 0, "expected note ID > 0 after update")
	t.Logf("Updated note %d in discussion %s", updOut.ID, discID)
}

// TestMeta_DraftNotePublish exercises draft_note_publish (individual publish)
// via the gitlab_mr_review meta-tool.
func TestMeta_DraftNotePublish(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	commitFileMeta(ctx, t, sess.meta, proj, "main", "draft.txt", "v1", "initial")

	callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "branch_name": "feat-draft-pub", "ref": "main"},
	})
	commitFileMeta(ctx, t, sess.meta, proj, "feat-draft-pub", "draft-v2.txt", "v2", "update")

	mrOut, err := callToolOn[struct {
		IID int64 `json:"mr_iid"`
	}](ctx, sess.meta, "gitlab_merge_request", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"source_branch": "feat-draft-pub",
			"target_branch": "main",
			"title":         "MR for draft publish",
		},
	})
	requireNoError(t, err, "create MR")
	mrIID := strconv.FormatInt(mrOut.IID, 10)

	// Create a draft note
	draftOut, err := callToolOn[mrdraftnotes.Output](ctx, sess.meta, "gitlab_mr_review", map[string]any{
		"action": "draft_note_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"mr_iid":     mrIID,
			"note":       "Draft to publish individually",
		},
	})
	requireNoError(t, err, "draft_note_create")
	draftID := strconv.FormatInt(draftOut.ID, 10)

	// Publish the single draft note
	err = callToolVoidOn(ctx, sess.meta, "gitlab_mr_review", map[string]any{
		"action": "draft_note_publish",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"mr_iid":     mrIID,
			"note_id":    draftID,
		},
	})
	requireNoError(t, err, "draft_note_publish")
	t.Logf("Published draft note %s", draftID)
}
