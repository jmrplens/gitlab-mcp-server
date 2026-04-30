//go:build e2e

// search_test.go tests the search MCP tools against a live GitLab instance.
// Covers code and project search via both individual tools and the gitlab_search meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
)

// TestIndividual_Search exercises code and project search via individual MCP tools.
// Creates a project with unique content, waits for Sidekiq indexing, then searches.
func TestIndividual_Search(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a project with some content to search for.
	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)
	commitFile(ctx, t, sess.individual, proj, defaultBranch, "searchable.txt", "unique-e2e-search-token-12345", "add searchable content")

	drainSidekiq(ctx, t, sess.glClient)

	t.Run("Code", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.individual, "gitlab_search_code", search.CodeInput{
			ProjectID: proj.pidOf(),
			Query:     "unique-e2e-search-token-12345",
		})
		requireNoError(t, err, "search code")
		requireTruef(t, len(out.Blobs) >= 1, "expected >=1 code result, got %d", len(out.Blobs))
	})

	t.Run("Projects", func(t *testing.T) {
		out, err := callToolOn[search.ProjectsOutput](ctx, sess.individual, "gitlab_search_projects", search.ProjectsInput{
			Query: "e2e",
		})
		requireNoError(t, err, "search projects")
		// At least one E2E project should be found.
		requireTruef(t, len(out.Projects) >= 1, "expected >=1 project result, got %d", len(out.Projects))
	})
}

// TestMeta_Search exercises code and project search via the gitlab_search meta-tool.
// Creates a project with unique content, waits for indexing, then searches.
func TestMeta_Search(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, "searchable-meta.txt", "unique-e2e-meta-search-67890", "add searchable meta content")

	drainSidekiq(ctx, t, sess.glClient)

	t.Run("Code", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "code",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"query":      "unique-e2e-meta-search-67890",
			},
		})
		requireNoError(t, err, "search code meta")
		requireTruef(t, len(out.Blobs) >= 1, "expected >=1 code result, got %d", len(out.Blobs))
	})

	t.Run("Projects", func(t *testing.T) {
		out, err := callToolOn[search.ProjectsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "projects",
			"params": map[string]any{
				"query": "e2e",
			},
		})
		requireNoError(t, err, "search projects meta")
		requireTruef(t, len(out.Projects) >= 1, "expected >=1 project result, got %d", len(out.Projects))
	})
}
