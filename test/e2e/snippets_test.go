//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
)

func TestIndividual_Snippets(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var snippetID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[snippets.Output](ctx, sess.individual, "gitlab_snippet_create", snippets.CreateInput{
			Title:       "E2E Test Snippet",
			FileName:    "test.txt",
			ContentBody: "Hello from E2E test",
			Visibility:  "private",
		})
		requireNoError(t, err, "create snippet")
		requireTrue(t, out.Title == "E2E Test Snippet", "expected title")
		snippetID = out.ID
		t.Logf("Created snippet %d: %s", out.ID, out.Title)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.individual, "gitlab_snippet_get", snippets.GetInput{
			SnippetID: snippetID,
		})
		requireNoError(t, err, "get snippet")
		requireTrue(t, out.ID == snippetID, "expected ID %d, got %d", snippetID, out.ID)
		t.Logf("Got snippet %d: %s", out.ID, out.Title)
	})

	t.Run("Content", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.ContentOutput](ctx, sess.individual, "gitlab_snippet_content", snippets.ContentInput{
			SnippetID: snippetID,
		})
		requireNoError(t, err, "get snippet content")
		requireTrue(t, out.Content != "", "expected non-empty content")
		t.Logf("Got snippet content (len=%d)", len(out.Content))
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[snippets.ListOutput](ctx, sess.individual, "gitlab_snippet_list", snippets.ListInput{})
		requireNoError(t, err, "list snippets")
		requireTrue(t, len(out.Snippets) >= 1, "expected at least 1 snippet, got %d", len(out.Snippets))
		t.Logf("Listed %d snippets", len(out.Snippets))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.individual, "gitlab_snippet_update", snippets.UpdateInput{
			SnippetID: snippetID,
			Title:     "E2E Updated Snippet",
		})
		requireNoError(t, err, "update snippet")
		requireTrue(t, out.Title == "E2E Updated Snippet", "expected updated title")
		t.Logf("Updated snippet %d: %s", out.ID, out.Title)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_snippet_delete", snippets.DeleteInput{
			SnippetID: snippetID,
		})
		requireNoError(t, err, "delete snippet")
		t.Log("Deleted snippet")
	})
}

func TestMeta_Snippets(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var snippetID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "create",
			"params": map[string]any{
				"title": "E2E Meta Snippet", "file_name": "meta.txt",
				"content": "Hello from meta E2E", "visibility": "private",
			},
		})
		requireNoError(t, err, "meta create snippet")
		requireTrue(t, out.Title == "E2E Meta Snippet", "expected title")
		snippetID = out.ID
		t.Logf("Created snippet %d via meta-tool", out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "get",
			"params": map[string]any{"snippet_id": snippetID},
		})
		requireNoError(t, err, "meta get snippet")
		requireTrue(t, out.ID == snippetID, "expected ID %d", snippetID)
		t.Logf("Got snippet %d via meta-tool", out.ID)
	})

	t.Run("Content", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.ContentOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "content",
			"params": map[string]any{"snippet_id": snippetID},
		})
		requireNoError(t, err, "meta get snippet content")
		requireTrue(t, out.Content != "", "expected non-empty content")
		t.Logf("Got snippet content via meta-tool (len=%d)", len(out.Content))
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[snippets.ListOutput](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta list snippets")
		requireTrue(t, len(out.Snippets) >= 1, "expected at least 1 snippet")
		t.Logf("Listed %d snippets via meta-tool", len(out.Snippets))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		out, err := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "update",
			"params": map[string]any{"snippet_id": snippetID, "title": "E2E Meta Updated"},
		})
		requireNoError(t, err, "meta update snippet")
		requireTrue(t, out.Title == "E2E Meta Updated", "expected updated title")
		t.Logf("Updated snippet %d via meta-tool", out.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, snippetID > 0, "snippetID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "delete",
			"params": map[string]any{"snippet_id": snippetID},
		})
		requireNoError(t, err, "meta delete snippet")
		t.Log("Deleted snippet via meta-tool")
	})
}
