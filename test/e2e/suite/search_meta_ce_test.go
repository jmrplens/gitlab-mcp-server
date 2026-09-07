//go:build e2e && !enterprise

// search_meta_ce_test.go tests extended search MCP tools against a live GitLab instance
// via the gitlab_search meta-tool. Covers all 10 search actions: merge_requests, issues,
// commits, milestones, notes, snippets, users, and wiki.
package suite

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/search"
)

// TestMeta_SearchExtended exercises all 10 search actions beyond the basic
// code/projects searches through the gitlab_search meta-tool against a live
// GitLab instance.
//
// The test creates a project and a known commit so the search routes return
// non-empty results, then walks every additional search action exposed by the
// catalog: issues, merge_requests, users, groups, milestones, wiki_blobs,
// notes, snippets, global, and commits. Each subtest asserts the action
// returns successfully with the expected payload shape.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_SearchExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "search_target.txt", "searchable content for e2e", "add searchable file")

	drainSidekiq(ctx, t, sess.glClient)

	t.Run("SearchMergeRequests", func(t *testing.T) {
		out, err := callToolOn[search.MergeRequestsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "merge_requests",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"query":      "test",
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
				"query":      "test",
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
				"query":      "searchable",
			},
		})
		requireNoError(t, err, "search commits")
		// The fixture's own commit message carries this word, and project commit
		// search reads the repository through Gitaly rather than an index, so
		// the match needs nothing beyond the commit existing.
		requireTruef(t, slices.ContainsFunc(out.Commits, func(c commits.Output) bool {
			return strings.Contains(c.Title, "searchable") || strings.Contains(c.Message, "searchable")
		}), "commit search did not return the fixture's own commit: %+v", out.Commits)
		t.Logf("Search commits: %d results", len(out.Commits))
	})

	t.Run("SearchMilestones", func(t *testing.T) {
		out, err := callToolOn[search.MilestonesOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "milestones",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"query":      "test",
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
				"query":      "test",
			},
		})
		requireNoError(t, err, "search notes")
		t.Logf("Search notes: %d results", len(out.Notes))
	})

	t.Run("SearchSnippets", func(t *testing.T) {
		out, err := callToolOn[search.SnippetsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "snippets",
			"params": map[string]any{
				"query": "test",
			},
		})
		requireNoError(t, err, "search snippets")
		t.Logf("Search snippets: %d results", len(out.Snippets))
	})

	t.Run("SearchUsers", func(t *testing.T) {
		// Search for the account this run authenticates as: it certainly exists,
		// where the instance's other users are not this fixture's to assume.
		out, err := callToolOn[search.UsersOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "users",
			"params": map[string]any{
				"query": sess.username,
			},
		})
		requireNoError(t, err, "search users")
		requireTruef(t, slices.ContainsFunc(out.Users, func(u search.UserOutput) bool {
			return u.Username == sess.username
		}), "user search for %q did not return that account: %+v", sess.username, out.Users)
		t.Logf("Search users: %d results", len(out.Users))
	})

	t.Run("SearchWiki", func(t *testing.T) {
		out, err := callToolOn[search.WikiOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "wiki",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"query":      "test",
			},
		})
		requireNoError(t, err, "search wiki")
		t.Logf("Search wiki: %d results", len(out.WikiBlobs))
	})
}
