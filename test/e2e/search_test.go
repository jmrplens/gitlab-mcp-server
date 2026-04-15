//go:build e2e

// search_test.go — E2E tests for search domain.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
)

func TestIndividual_Search(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a project with some content to search for.
	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)
	commitFile(ctx, t, sess.individual, proj, defaultBranch, "searchable.txt", "unique-e2e-search-token-12345", "add searchable content")

	// Give Elasticsearch/basic search time to index.
	time.Sleep(2 * time.Second)

	t.Run("Code", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.individual, "gitlab_search_code", search.CodeInput{
			ProjectID: proj.pidOf(),
			Query:     "unique-e2e-search-token-12345",
		})
		requireNoError(t, err, "search code")
		requireTrue(t, len(out.Blobs) >= 1, "expected >=1 code result, got %d", len(out.Blobs))
	})

	t.Run("Projects", func(t *testing.T) {
		out, err := callToolOn[search.ProjectsOutput](ctx, sess.individual, "gitlab_search_projects", search.ProjectsInput{
			Query: "e2e",
		})
		requireNoError(t, err, "search projects")
		// At least one E2E project should be found.
		requireTrue(t, len(out.Projects) >= 1, "expected >=1 project result, got %d", len(out.Projects))
	})
}

func TestMeta_Search(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, "searchable-meta.txt", "unique-e2e-meta-search-67890", "add searchable meta content")

	time.Sleep(2 * time.Second)

	t.Run("Code", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "code",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"query":      "unique-e2e-meta-search-67890",
			},
		})
		requireNoError(t, err, "search code meta")
		requireTrue(t, len(out.Blobs) >= 1, "expected >=1 code result, got %d", len(out.Blobs))
	})

	t.Run("Projects", func(t *testing.T) {
		out, err := callToolOn[search.ProjectsOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "projects",
			"params": map[string]any{
				"query": "e2e",
			},
		})
		requireNoError(t, err, "search projects meta")
		requireTrue(t, len(out.Projects) >= 1, "expected >=1 project result, got %d", len(out.Projects))
	})
}
