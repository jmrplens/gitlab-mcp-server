//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestIndividual_Releases exercises the release lifecycle using individual MCP tools.
func TestIndividual_Releases(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const tagName = "v1.0.0-releases-e2e"

	// Create tag first (releases require a tag).
	tagOut, tagErr := callToolOn[tags.Output](ctx, sess.individual, "gitlab_tag_create", tags.CreateInput{
		ProjectID: proj.pidOf(),
		TagName:   tagName,
		Ref:       defaultBranch,
		Message:   "Release E2E tag",
	})
	requireNoError(t, tagErr, "create tag for release")
	t.Logf("Created tag %s (target=%s)", tagOut.Name, tagOut.Target)

	var releaseLinkID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_create", releases.CreateInput{
			ProjectID:   proj.pidOf(),
			TagName:     tagName,
			Name:        "E2E Release " + tagName,
			Description: "Automated E2E test release.",
		})
		requireNoError(t, err, "create release")
		requireTrue(t, out.TagName == tagName, "expected release tag %q, got %q", tagName, out.TagName)
		t.Logf("Created release %s (%s)", out.Name, out.TagName)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_get", releases.GetInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
		})
		requireNoError(t, err, "get release")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Got release %s (created=%s)", out.Name, out.CreatedAt)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_update", releases.UpdateInput{
			ProjectID:   proj.pidOf(),
			TagName:     tagName,
			Description: "Updated E2E test release.",
		})
		requireNoError(t, err, "update release")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Updated release %s", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[releases.ListOutput](ctx, sess.individual, "gitlab_release_list", releases.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list releases")
		requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release, got %d", len(out.Releases))
		t.Logf("Listed %d releases", len(out.Releases))
	})

	t.Run("LinkCreate", func(t *testing.T) {
		out, err := callToolOn[releaselinks.Output](ctx, sess.individual, "gitlab_release_link_create", releaselinks.CreateInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
			Name:      "E2E Binary (Linux amd64)",
			URL:       "https://example.com/releases/" + tagName + "/binary-linux-amd64",
			LinkType:  "package",
		})
		requireNoError(t, err, "create release link")
		requireTrue(t, out.ID > 0, "release link ID should be positive")
		releaseLinkID = out.ID
		t.Logf("Created release link ID=%d (%s)", out.ID, out.Name)
	})

	t.Run("LinkList", func(t *testing.T) {
		out, err := callToolOn[releaselinks.ListOutput](ctx, sess.individual, "gitlab_release_link_list", releaselinks.ListInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
		})
		requireNoError(t, err, "list release links")
		requireTrue(t, len(out.Links) >= 1, "expected at least 1 release link, got %d", len(out.Links))

		found := false
		for _, l := range out.Links {
			if l.ID == releaseLinkID {
				found = true
				break
			}
		}
		requireTrue(t, found, "release link ID=%d not found in list", releaseLinkID)
		t.Logf("Listed %d release links", len(out.Links))
	})

	t.Run("LinkDelete", func(t *testing.T) {
		requireTrue(t, releaseLinkID > 0, "release link ID not set")
		out, err := callToolOn[releaselinks.Output](ctx, sess.individual, "gitlab_release_link_delete", releaselinks.DeleteInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
			LinkID:    releaseLinkID,
		})
		requireNoError(t, err, "delete release link")
		requireTrue(t, out.ID == releaseLinkID, "expected link ID %d, got %d", releaseLinkID, out.ID)
		t.Logf("Deleted release link ID=%d", out.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_delete", releases.DeleteInput{
			ProjectID: proj.pidOf(),
			TagName:   tagName,
		})
		requireNoError(t, err, "delete release")
		requireTrue(t, out.TagName == tagName, "expected deleted release tag %q, got %q", tagName, out.TagName)
		t.Logf("Deleted release %s", out.TagName)
	})
}

// TestMeta_Releases exercises the release lifecycle using the gitlab_release meta-tool.
func TestMeta_Releases(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)

	const tagName = "v1.0.0-releases-meta-e2e"

	// Create tag first.
	_, tagErr := callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"tag_name":   tagName,
			"ref":        defaultBranch,
			"message":    "Meta release E2E tag",
		},
	})
	requireNoError(t, tagErr, "create tag for release (meta)")

	var releaseLinkID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"tag_name":    tagName,
				"name":        "Meta E2E Release " + tagName,
				"description": "Release via meta-tool E2E.",
			},
		})
		requireNoError(t, err, "meta release create")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Created release %s", out.Name)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "meta release get")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Got release %s", out.Name)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"tag_name":    tagName,
				"description": "Updated meta-tool E2E release.",
			},
		})
		requireNoError(t, err, "meta release update")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Updated release %s", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[releases.ListOutput](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta release list")
		requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release")
		t.Logf("Listed %d releases", len(out.Releases))
	})

	t.Run("LinkCreate", func(t *testing.T) {
		out, err := callToolOn[releaselinks.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"name":       "Meta Binary (Linux arm64)",
				"url":        "https://example.com/releases/" + tagName + "/binary-linux-arm64",
				"link_type":  "package",
			},
		})
		requireNoError(t, err, "meta release link create")
		requireTrue(t, out.ID > 0, "link ID should be positive")
		releaseLinkID = out.ID
		t.Logf("Created release link ID=%d", out.ID)
	})

	t.Run("LinkList", func(t *testing.T) {
		out, err := callToolOn[releaselinks.ListOutput](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "meta release link list")
		requireTrue(t, len(out.Links) >= 1, "expected at least 1 link")
		t.Logf("Listed %d release links", len(out.Links))
	})

	t.Run("LinkDelete", func(t *testing.T) {
		requireTrue(t, releaseLinkID > 0, "release link ID not set")
		out, err := callToolOn[releaselinks.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"link_id":    releaseLinkID,
			},
		})
		requireNoError(t, err, "meta release link delete")
		requireTrue(t, out.ID == releaseLinkID, "expected link ID %d, got %d", releaseLinkID, out.ID)
		t.Logf("Deleted release link ID=%d", out.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		out, err := callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "meta release delete")
		requireTrue(t, out.TagName == tagName, "expected tag %q, got %q", tagName, out.TagName)
		t.Logf("Deleted release %s", out.TagName)
	})
}
