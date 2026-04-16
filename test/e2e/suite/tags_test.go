//go:build e2e

// tags_test.go tests the tag MCP tools against a live GitLab instance.
// Covers the full tag lifecycle: create → get → list → delete via both individual
// tools and the gitlab_tag meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestIndividual_Tags exercises the tag lifecycle using individual MCP tools.
func TestIndividual_Tags(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const tagName = "v0.1.0-tags-e2e"

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[tags.Output](ctx, sess.individual, "gitlab_tag_create", tags.CreateInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
			Ref:       defaultBranch,
			Message:   "E2E tag test",
		})
		requireNoError(t, err, "create tag")
		requireTrue(t, out.Name == tagName, "expected tag %q, got %q", tagName, out.Name)
		t.Logf("Created tag %s (target=%s)", out.Name, out.Target)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[tags.Output](ctx, sess.individual, "gitlab_tag_get", tags.GetInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
		})
		requireNoError(t, err, "get tag")
		requireTrue(t, out.Name == tagName, "expected tag %q, got %q", tagName, out.Name)
		requireTrue(t, out.Target != "", "tag target should not be empty")
		t.Logf("Got tag %s (target=%s)", out.Name, out.Target)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[tags.ListOutput](ctx, sess.individual, "gitlab_tag_list", tags.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list tags")
		requireTrue(t, len(out.Tags) >= 1, "expected at least 1 tag, got %d", len(out.Tags))

		found := false
		for _, tag := range out.Tags {
			if tag.Name == tagName {
				found = true
				break
			}
		}
		requireTrue(t, found, "tag %q not found in list", tagName)
		t.Logf("Listed %d tags", len(out.Tags))
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_tag_delete", tags.DeleteInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
		})
		requireNoError(t, err, "delete tag")
		t.Logf("Deleted tag %s", tagName)
	})
}

// TestMeta_Tags exercises the tag lifecycle using the gitlab_tag meta-tool.
func TestMeta_Tags(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)

	const tagName = "v0.1.0-tags-meta-e2e"

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"ref":        defaultBranch,
				"message":    "Meta tag test",
			},
		})
		requireNoError(t, err, "meta tag create")
		requireTrue(t, out.Name == tagName, "expected tag %q, got %q", tagName, out.Name)
		t.Logf("Created tag %s", out.Name)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "meta tag get")
		requireTrue(t, out.Name == tagName, "expected tag %q, got %q", tagName, out.Name)
		requireTrue(t, out.Target != "", "tag target should not be empty")
		t.Logf("Got tag %s (target=%s)", out.Name, out.Target)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[tags.ListOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta tag list")
		requireTrue(t, len(out.Tags) >= 1, "expected at least 1 tag")

		found := false
		for _, tag := range out.Tags {
			if tag.Name == tagName {
				found = true
				break
			}
		}
		requireTrue(t, found, "tag %q not found", tagName)
		t.Logf("Listed %d tags", len(out.Tags))
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "meta tag delete")
		t.Logf("Deleted tag %s", tagName)
	})
}
