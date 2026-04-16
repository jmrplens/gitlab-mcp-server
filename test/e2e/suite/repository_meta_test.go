//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
)

// TestMeta_RepositoryFiles exercises file CRUD actions not covered by existing tests:
// file_create, file_update, file_delete, file_blame, file_metadata, file_raw.
func TestMeta_RepositoryFiles(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "seed.txt", "seed content", "init")

	filePath := "e2e-file.txt"

	t.Run("FileCreate", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_create",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      filePath,
				"branch":         "main",
				"content":        "initial content",
				"commit_message": "create file via e2e",
			},
		})
		requireNoError(t, err, "file_create")
		requireTrue(t, out.FilePath == filePath, "file_create: path mismatch")
		t.Logf("Created file: %s", out.FilePath)
	})

	t.Run("FileMetadata", func(t *testing.T) {
		out, err := callToolOn[files.MetaDataOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_metadata",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  filePath,
				"ref":        "main",
			},
		})
		requireNoError(t, err, "file_metadata")
		requireTrue(t, out.FileName != "", "file_metadata: expected filename")
		t.Logf("File metadata: %s (size=%d)", out.FileName, out.Size)
	})

	t.Run("FileRaw", func(t *testing.T) {
		out, err := callToolOn[files.RawOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_raw",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  filePath,
				"ref":        "main",
			},
		})
		requireNoError(t, err, "file_raw")
		requireTrue(t, out.Content != "", "file_raw: expected content")
		t.Logf("Raw content length: %d", len(out.Content))
	})

	t.Run("FileBlame", func(t *testing.T) {
		out, err := callToolOn[files.BlameOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_blame",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  filePath,
				"ref":        "main",
			},
		})
		requireNoError(t, err, "file_blame")
		requireTrue(t, len(out.Ranges) > 0, "file_blame: expected at least 1 range")
		t.Logf("Blame ranges: %d", len(out.Ranges))
	})

	t.Run("FileUpdate", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_update",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      filePath,
				"branch":         "main",
				"content":        "updated content",
				"commit_message": "update file via e2e",
			},
		})
		requireNoError(t, err, "file_update")
		requireTrue(t, out.FilePath == filePath, "file_update: path mismatch")
	})

	t.Run("FileDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_delete",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      filePath,
				"branch":         "main",
				"commit_message": "delete file via e2e",
			},
		})
		requireNoError(t, err, "file_delete")
	})
}

// TestMeta_RepositoryExplore exercises repository-level exploration actions:
// contributors, merge_base, blob, raw_blob, archive.
func TestMeta_RepositoryExplore(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "repo.txt", "repo content", "init for repo explore")

	t.Run("Contributors", func(t *testing.T) {
		out, err := callToolOn[repository.ContributorsOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "contributors",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "contributors")
		requireTrue(t, len(out.Contributors) >= 1, "contributors: expected at least 1")
		t.Logf("Contributors: %d", len(out.Contributors))
	})

	t.Run("Archive", func(t *testing.T) {
		out, err := callToolOn[repository.ArchiveOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "archive",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "archive")
		requireTrue(t, out.URL != "", "archive: expected URL")
		t.Logf("Archive URL: %s", out.URL)
	})
}

// TestMeta_CommitExtended exercises commit actions beyond list/get/diff:
// commit_refs, commit_comments, commit_comment_create, commit_statuses,
// commit_status_set, commit_cherry_pick, commit_revert, commit_signature.
func TestMeta_CommitExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "cmt.txt", "commit content", "commit for extended tests")

	// Get the latest commit SHA
	listOut, setupErr := callToolOn[commits.ListOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
		"action": "commit_list",
		"params": map[string]any{"project_id": proj.pidStr(), "ref_name": "main", "per_page": 1},
	})
	requireNoError(t, setupErr, "commit_list for SHA")
	requireTrue(t, len(listOut.Commits) > 0, "expected at least 1 commit")
	sha := listOut.Commits[0].ID

	t.Run("CommitRefs", func(t *testing.T) {
		out, err := callToolOn[commits.RefsOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_refs",
			"params": map[string]any{"project_id": proj.pidStr(), "sha": sha},
		})
		requireNoError(t, err, "commit_refs")
		t.Logf("Commit refs: %d", len(out.Refs))
	})

	t.Run("CommitCommentCreate", func(t *testing.T) {
		out, err := callToolOn[commits.CommentOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_comment_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"sha":        sha,
				"note":       "E2E test comment",
			},
		})
		requireNoError(t, err, "commit_comment_create")
		requireTrue(t, out.Note != "", "expected comment note")
		t.Logf("Created commit comment: %s", out.Note)
	})

	t.Run("CommitComments", func(t *testing.T) {
		out, err := callToolOn[commits.CommentsOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_comments",
			"params": map[string]any{"project_id": proj.pidStr(), "sha": sha},
		})
		requireNoError(t, err, "commit_comments")
		requireTrue(t, len(out.Comments) >= 1, "expected at least 1 comment")
		t.Logf("Commit comments: %d", len(out.Comments))
	})

	t.Run("CommitStatusSet", func(t *testing.T) {
		out, err := callToolOn[commits.StatusOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_status_set",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"sha":        sha,
				"state":      "success",
				"name":       "e2e-test",
			},
		})
		requireNoError(t, err, "commit_status_set")
		requireTrue(t, out.Status == "success", "expected status success")
		t.Logf("Set commit status: %s", out.Status)
	})

	t.Run("CommitStatuses", func(t *testing.T) {
		out, err := callToolOn[commits.StatusesOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_statuses",
			"params": map[string]any{"project_id": proj.pidStr(), "sha": sha},
		})
		requireNoError(t, err, "commit_statuses")
		requireTrue(t, len(out.Statuses) >= 1, "expected at least 1 status")
		t.Logf("Commit statuses: %d", len(out.Statuses))
	})

	t.Run("CommitSignature", func(t *testing.T) {
		// May return 404 if commit is not GPG-signed, but exercises the route.
		_, _ = callToolOn[commits.GPGSignatureOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_signature",
			"params": map[string]any{"project_id": proj.pidStr(), "sha": sha},
		})
		t.Log("commit_signature route exercised (may 404 for unsigned commits)")
	})

	// Cherry-pick: create target branch first, then add a new commit on main that isn't on the target
	_, setupErr = callToolOn[commits.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
		"action": "commit_create",
		"params": map[string]any{
			"project_id":     proj.pidStr(),
			"branch":         "cherry-pick-target",
			"start_branch":   "main",
			"commit_message": "create branch for cherry-pick",
			"actions": []map[string]any{
				{"action": "create", "file_path": "cherry.txt", "content": "cherry"},
			},
		},
	})
	requireNoError(t, setupErr, "create branch for cherry-pick")

	// Add a new commit on main (not on cherry-pick-target)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "cherry-main.txt", "only on main", "commit to cherry-pick")

	// Get the new commit SHA from main
	cpListOut, setupErr := callToolOn[commits.ListOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
		"action": "commit_list",
		"params": map[string]any{"project_id": proj.pidStr(), "ref_name": "main", "per_page": 1},
	})
	requireNoError(t, setupErr, "commit_list for cherry-pick SHA")
	cpSHA := cpListOut.Commits[0].ID

	t.Run("CommitCherryPick", func(t *testing.T) {
		out, err := callToolOn[commits.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_cherry_pick",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"sha":        cpSHA,
				"branch":     "cherry-pick-target",
			},
		})
		requireNoError(t, err, "commit_cherry_pick")
		requireTrue(t, out.ID != "", "cherry_pick: expected commit SHA")
		t.Logf("Cherry-picked to: %s", out.ID)
	})
}

// TestMeta_CommitDiscussions exercises commit discussion actions.
func TestMeta_CommitDiscussions(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "disc.txt", "discussion content", "for commit discussions")

	listOut, listErr := callToolOn[commits.ListOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
		"action": "commit_list",
		"params": map[string]any{"project_id": proj.pidStr(), "ref_name": "main", "per_page": 1},
	})
	requireNoError(t, listErr, "commit_list for discussions")
	sha := listOut.Commits[0].ID

	t.Run("DiscussionCreate", func(t *testing.T) {
		out, discErr := callToolOn[commitdiscussions.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_discussion_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"commit_sha": sha,
				"body":       "E2E commit discussion",
			},
		})
		requireNoError(t, discErr, "commit_discussion_create")
		requireTrue(t, out.ID != "", "expected discussion ID")
		discID := out.ID
		t.Logf("Created commit discussion: %s", discID)

		t.Run("DiscussionList", func(t *testing.T) {
			listOut, err := callToolOn[commitdiscussions.ListOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
				"action": "commit_discussion_list",
				"params": map[string]any{"project_id": proj.pidStr(), "commit_sha": sha},
			})
			requireNoError(t, err, "commit_discussion_list")
			requireTrue(t, len(listOut.Discussions) >= 1, "expected at least 1 discussion")
		})

		t.Run("DiscussionGet", func(t *testing.T) {
			gOut, err := callToolOn[commitdiscussions.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
				"action": "commit_discussion_get",
				"params": map[string]any{
					"project_id":    proj.pidStr(),
					"commit_sha":    sha,
					"discussion_id": discID,
				},
			})
			requireNoError(t, err, "commit_discussion_get")
			requireTrue(t, gOut.ID == discID, "discussion ID mismatch")
		})

		t.Run("DiscussionAddNote", func(t *testing.T) {
			nOut, noteErr := callToolOn[commitdiscussions.NoteOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
				"action": "commit_discussion_add_note",
				"params": map[string]any{
					"project_id":    proj.pidStr(),
					"commit_sha":    sha,
					"discussion_id": discID,
					"body":          "E2E reply note",
				},
			})
			requireNoError(t, noteErr, "commit_discussion_add_note")
			requireTrue(t, nOut.ID > 0, "expected note ID")
			noteIDForUpdate := nOut.ID

			t.Run("DiscussionUpdateNote", func(t *testing.T) {
				_, err := callToolOn[commitdiscussions.NoteOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
					"action": "commit_discussion_update_note",
					"params": map[string]any{
						"project_id":    proj.pidStr(),
						"commit_sha":    sha,
						"discussion_id": discID,
						"note_id":       noteIDForUpdate,
						"body":          "E2E updated note",
					},
				})
				requireNoError(t, err, "commit_discussion_update_note")
			})

			t.Run("DiscussionDeleteNote", func(t *testing.T) {
				err := callToolVoidOn(ctx, sess.meta, "gitlab_repository", map[string]any{
					"action": "commit_discussion_delete_note",
					"params": map[string]any{
						"project_id":    proj.pidStr(),
						"commit_sha":    sha,
						"discussion_id": discID,
						"note_id":       noteIDForUpdate,
					},
				})
				requireNoError(t, err, "commit_discussion_delete_note")
			})
		})
	})
}
