//go:build e2e

// snippets_meta_test.go tests extended snippet MCP tools against a live GitLab instance
// via the gitlab_snippet meta-tool. Covers personal snippets, project snippet CRUD,
// snippet discussions, snippet notes, and snippet award emoji lifecycle.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
)

// TestMeta_SnippetsPersonal exercises personal snippet actions not covered by snippets_test.go.
func TestMeta_SnippetsPersonal(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var snippetID int64

	t.Run("ListAll", func(t *testing.T) {
		out, err := callToolOn[snippets.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "list_all",
			"params": map[string]any{},
		})
		requireNoError(t, err, "list_all")
		t.Logf("All public snippets: %d", len(out.Snippets))
	})

	t.Run("Explore", func(t *testing.T) {
		out, err := callToolOn[snippets.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "explore",
			"params": map[string]any{},
		})
		requireNoError(t, err, "explore")
		t.Logf("Explored snippets: %d", len(out.Snippets))
	})

	// Create a snippet for further tests
	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "create",
			"params": map[string]any{
				"title":       "e2e-snippet-" + uniqueName(""),
				"file_name":   "test.txt",
				"content":     "hello world",
				"visibility":  "private",
				"description": "E2E test snippet",
			},
		})
		requireNoError(t, err, "create")
		requireTrue(t, out.ID > 0, "create: expected ID > 0")
		snippetID = out.ID
		t.Logf("Created personal snippet %d", snippetID)
	})
	defer func() {
		if snippetID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
				"action": "delete",
				"params": map[string]any{"snippet_id": snippetID},
			})
		}
	}()

	t.Run("FileContent", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.FileContentOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "file_content",
			"params": map[string]any{
				"snippet_id": snippetID,
				"ref":        "main",
				"file_name":  "test.txt",
			},
		})
		requireNoError(t, err, "file_content")
		requireTrue(t, out.Content != "", "file_content: empty content")
	})
}

// TestMeta_SnippetsProject exercises project-level snippet CRUD via gitlab_snippet.
func TestMeta_SnippetsProject(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	var snippetID int64

	t.Run("ProjectCreate", func(t *testing.T) {
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "proj-snippet-" + uniqueName(""),
				"file_name":  "data.txt",
				"content":    "project snippet content",
				"visibility": "private",
			},
		})
		requireNoError(t, err, "project_create")
		requireTrue(t, out.ID > 0, "project_create: expected ID > 0")
		snippetID = out.ID
		t.Logf("Created project snippet %d", snippetID)
	})
	defer func() {
		if snippetID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
				"action": "project_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"snippet_id": snippetID,
				},
			})
		}
	}()

	t.Run("ProjectList", func(t *testing.T) {
		out, err := callToolOn[snippets.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "project_list")
		requireTrue(t, len(out.Snippets) > 0, "project_list: expected at least 1")
	})

	t.Run("ProjectGet", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
			},
		})
		requireNoError(t, err, "project_get")
		requireTrue(t, out.ID == snippetID, "project_get: ID mismatch")
	})

	t.Run("ProjectContent", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.ContentOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_content",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
			},
		})
		requireNoError(t, err, "project_content")
		requireTrue(t, out.Content != "", "project_content: empty content")
	})

	t.Run("ProjectUpdate", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
				"title":      "updated-snippet",
			},
		})
		requireNoError(t, err, "project_update")
		requireTrue(t, out.Title == "updated-snippet", "project_update: title mismatch")
	})
}

// TestMeta_SnippetDiscussions exercises snippet discussion lifecycle.
func TestMeta_SnippetDiscussions(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create a project snippet for discussion testing
	snipOut, snipErr := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
		"action": "project_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      "disc-snippet-" + uniqueName(""),
			"file_name":  "disc.txt",
			"content":    "discussion content",
			"visibility": "private",
		},
	})
	requireNoError(t, snipErr, "create snippet for discussions")
	snippetID := snipOut.ID
	snippetIDStr := strconv.FormatInt(snippetID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "snippet_id": snippetID},
		})
	}()

	var discID string
	var noteID int64

	t.Run("DiscussionCreate", func(t *testing.T) {
		out, err := callToolOn[snippetdiscussions.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetIDStr,
				"body":       "E2E discussion on snippet",
			},
		})
		requireNoError(t, err, "discussion_create")
		requireTrue(t, out.ID != "", "discussion_create: empty ID")
		discID = out.ID
		if len(out.Notes) > 0 {
			noteID = out.Notes[0].ID
		}
		t.Logf("Created discussion %s (note %d)", discID, noteID)
	})

	t.Run("DiscussionList", func(t *testing.T) {
		out, err := callToolOn[snippetdiscussions.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_list",
			"params": map[string]any{"project_id": proj.pidStr(), "snippet_id": snippetIDStr},
		})
		requireNoError(t, err, "discussion_list")
		requireTrue(t, len(out.Discussions) > 0, "discussion_list: expected at least 1")
	})

	t.Run("DiscussionGet", func(t *testing.T) {
		requireTrue(t, discID != "", "discID not set")
		out, err := callToolOn[snippetdiscussions.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"snippet_id":    snippetIDStr,
				"discussion_id": discID,
			},
		})
		requireNoError(t, err, "discussion_get")
		requireTrue(t, out.ID == discID, "discussion_get: ID mismatch")
	})

	t.Run("DiscussionAddNote", func(t *testing.T) {
		requireTrue(t, discID != "", "discID not set")
		out, err := callToolOn[snippetdiscussions.NoteOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_add_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"snippet_id":    snippetIDStr,
				"discussion_id": discID,
				"body":          "Reply to discussion",
			},
		})
		requireNoError(t, err, "discussion_add_note")
		requireTrue(t, out.ID > 0, "discussion_add_note: expected ID > 0")
		t.Logf("Added note %d to discussion %s", out.ID, discID)
	})

	t.Run("DiscussionUpdateNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[snippetdiscussions.NoteOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_update_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"snippet_id":    snippetIDStr,
				"discussion_id": discID,
				"note_id":       noteID,
				"body":          "Updated discussion note",
			},
		})
		requireNoError(t, err, "discussion_update_note")
		requireTrue(t, out.ID == noteID, "discussion_update_note: ID mismatch")
	})

	t.Run("DiscussionDeleteNote", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "discussion_delete_note",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"snippet_id":    snippetIDStr,
				"discussion_id": discID,
				"note_id":       noteID,
			},
		})
		requireNoError(t, err, "discussion_delete_note")
	})
}

// TestMeta_SnippetNotes exercises snippet note CRUD.
func TestMeta_SnippetNotes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	snipOut, snipErr := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
		"action": "project_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      "note-snippet-" + uniqueName(""),
			"file_name":  "note.txt",
			"content":    "note content",
			"visibility": "private",
		},
	})
	requireNoError(t, snipErr, "create snippet for notes")
	snippetID := snipOut.ID
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "snippet_id": snippetID},
		})
	}()

	var noteID int64

	t.Run("NoteCreate", func(t *testing.T) {
		out, err := callToolOn[snippetnotes.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
				"body":       "E2E snippet note",
			},
		})
		requireNoError(t, err, "note_create")
		requireTrue(t, out.ID > 0, "note_create: expected ID > 0")
		noteID = out.ID
		t.Logf("Created snippet note %d", noteID)
	})

	t.Run("NoteList", func(t *testing.T) {
		out, err := callToolOn[snippetnotes.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
			},
		})
		requireNoError(t, err, "note_list")
		requireTrue(t, len(out.Notes) > 0, "note_list: expected at least 1")
	})

	t.Run("NoteGet", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[snippetnotes.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "note_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "note_get")
		requireTrue(t, out.ID == noteID, "note_get: ID mismatch")
	})

	t.Run("NoteUpdate", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[snippetnotes.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "note_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
				"note_id":    noteID,
				"body":       "Updated snippet note",
			},
		})
		requireNoError(t, err, "note_update")
		requireTrue(t, out.ID == noteID, "note_update: ID mismatch")
	})

	t.Run("NoteDelete", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "note_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id": snippetID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "note_delete")
	})
}

// TestMeta_SnippetEmoji exercises snippet award emoji actions.
func TestMeta_SnippetEmoji(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	snipOut, snipErr := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
		"action": "project_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      "emoji-snippet-" + uniqueName(""),
			"file_name":  "emoji.txt",
			"content":    "emoji content",
			"visibility": "private",
		},
	})
	requireNoError(t, snipErr, "create snippet for emoji")
	// The emoji endpoints use IID, which for project snippets should match
	snippetIID := snipOut.ID
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "project_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "snippet_id": snippetIID},
		})
	}()

	var emojiID int64

	t.Run("EmojiSnippetCreate", func(t *testing.T) {
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"name":       "thumbsup",
			},
		})
		requireNoError(t, err, "emoji_snippet_create")
		requireTrue(t, out.ID > 0, "emoji_snippet_create: expected ID > 0")
		emojiID = out.ID
		t.Logf("Created snippet emoji %d", emojiID)
	})

	t.Run("EmojiSnippetList", func(t *testing.T) {
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
			},
		})
		requireNoError(t, err, "emoji_snippet_list")
		requireTrue(t, len(out.AwardEmoji) > 0, "emoji_snippet_list: expected at least 1")
	})

	t.Run("EmojiSnippetGet", func(t *testing.T) {
		requireTrue(t, emojiID > 0, "emojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"award_id":   emojiID,
			},
		})
		requireNoError(t, err, "emoji_snippet_get")
		requireTrue(t, out.ID == emojiID, "emoji_snippet_get: ID mismatch")
	})

	t.Run("EmojiSnippetDelete", func(t *testing.T) {
		requireTrue(t, emojiID > 0, "emojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"award_id":   emojiID,
			},
		})
		requireNoError(t, err, "emoji_snippet_delete")
	})

	// Create a note for emoji-on-note tests
	noteOut, noteErr := callToolOn[snippetnotes.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
		"action": "note_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"snippet_id": snippetIID,
			"body":       "emoji note target",
		},
	})
	requireNoError(t, noteErr, "create note for emoji")
	noteID := noteOut.ID

	var noteEmojiID int64

	t.Run("EmojiSnippetNoteCreate", func(t *testing.T) {
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"note_id":    noteID,
				"name":       "heart",
			},
		})
		requireNoError(t, err, "emoji_snippet_note_create")
		requireTrue(t, out.ID > 0, "emoji_snippet_note_create: expected ID > 0")
		noteEmojiID = out.ID
		t.Logf("Created snippet note emoji %d", noteEmojiID)
	})

	t.Run("EmojiSnippetNoteList", func(t *testing.T) {
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "emoji_snippet_note_list")
		requireTrue(t, len(out.AwardEmoji) > 0, "emoji_snippet_note_list: expected at least 1")
	})

	t.Run("EmojiSnippetNoteGet", func(t *testing.T) {
		requireTrue(t, noteEmojiID > 0, "noteEmojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_note_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"note_id":    noteID,
				"award_id":   noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_snippet_note_get")
		requireTrue(t, out.ID == noteEmojiID, "emoji_snippet_note_get: ID mismatch")
	})

	t.Run("EmojiSnippetNoteDelete", func(t *testing.T) {
		requireTrue(t, noteEmojiID > 0, "noteEmojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "emoji_snippet_note_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"snippet_id":        snippetIID,
				"note_id":    noteID,
				"award_id":   noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_snippet_note_delete")
	})
}
