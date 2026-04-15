//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
)

// TestMeta_SearchExtended exercises all 10 search actions beyond the basic code/projects.
func TestMeta_SearchExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "search_target.txt", "searchable content for e2e", "add searchable file")

	// Wait briefly for indexing
	time.Sleep(2 * time.Second)

	t.Run("SearchMergeRequests", func(t *testing.T) {
		out, err := callToolOn[search.MergeRequestsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "merge_requests",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "test",
			},
		})
		requireNoError(t, err, "search merge_requests")
		t.Logf("Search MRs: %d results", len(out.MergeRequests))
	})

	t.Run("SearchIssues", func(t *testing.T) {
		out, err := callToolOn[search.IssuesOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "issues",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "test",
			},
		})
		requireNoError(t, err, "search issues")
		t.Logf("Search issues: %d results", len(out.Issues))
	})

	t.Run("SearchCommits", func(t *testing.T) {
		out, err := callToolOn[search.CommitsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "commits",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "searchable",
			},
		})
		requireNoError(t, err, "search commits")
		t.Logf("Search commits: %d results", len(out.Commits))
	})

	t.Run("SearchMilestones", func(t *testing.T) {
		out, err := callToolOn[search.MilestonesOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "milestones",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "test",
			},
		})
		requireNoError(t, err, "search milestones")
		t.Logf("Search milestones: %d results", len(out.Milestones))
	})

	t.Run("SearchNotes", func(t *testing.T) {
		out, err := callToolOn[search.NotesOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "notes",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "test",
			},
		})
		requireNoError(t, err, "search notes")
		t.Logf("Search notes: %d results", len(out.Notes))
	})

	t.Run("SearchSnippets", func(t *testing.T) {
		out, err := callToolOn[search.SnippetsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "snippets",
			"params": map[string]any{
				"search": "test",
			},
		})
		requireNoError(t, err, "search snippets")
		t.Logf("Search snippets: %d results", len(out.Snippets))
	})

	t.Run("SearchUsers", func(t *testing.T) {
		out, err := callToolOn[search.UsersOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "users",
			"params": map[string]any{
				"search": "root",
			},
		})
		requireNoError(t, err, "search users")
		t.Logf("Search users: %d results", len(out.Users))
	})

	t.Run("SearchWiki", func(t *testing.T) {
		out, err := callToolOn[search.WikiOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "wiki",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"search":     "test",
			},
		})
		requireNoError(t, err, "search wiki")
		t.Logf("Search wiki: %d results", len(out.WikiBlobs))
	})
}
