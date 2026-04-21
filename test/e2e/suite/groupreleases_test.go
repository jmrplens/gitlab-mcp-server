//go:build e2e

package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupreleases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestMeta_GroupReleases exercises the release_list action for groups via the
// gitlab_group meta-tool. Group releases aggregate releases from projects
// within the group and are a Free-tier feature.
func TestMeta_GroupReleases(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test group.
	grpName := uniqueName("grp-rel")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupID := grpOut.ID
	groupIDStr := strconv.FormatInt(groupID, 10)
	t.Logf("Created group %d: %s", groupID, grpName)

	defer func() {
		if groupID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{"group_id": groupIDStr},
			})
		}
	}()

	// List releases on empty group — should be empty.
	t.Run("ReleaseList_Empty", func(t *testing.T) {
		out, err := callToolOn[groupreleases.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "release_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "release_list (empty)")
		requireTrue(t, len(out.Releases) == 0, "expected 0 releases, got %d", len(out.Releases))
		t.Logf("Releases: %d (expected 0)", len(out.Releases))
	})

	// Create a project inside the group, commit a file, create a tag & release.
	projName := uniqueName("rel-proj")
	projOut, setupErr := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   projName,
			"namespace_id":           groupID,
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         "main",
		},
	})
	requireNoError(t, setupErr, "create project in group")
	projIDStr := strconv.FormatInt(projOut.ID, 10)
	t.Logf("Created project %d in group %d", projOut.ID, groupID)

	// Wait for default branch to be available.
	waitForBranchOn(ctx, t, sess.meta, projOut.ID, "main")

	// Commit a file so there's content for the tag.
	commitFileMeta(ctx, t, sess.meta, ProjectFixture{ID: projOut.ID, Path: projOut.PathWithNamespace},
		"main", "release-test.txt", "release content", "init for group release test")

	// Create a tag.
	tagName := uniqueName("v-grp-rel")
	_, setupErr = callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": projIDStr,
			"tag_name":   tagName,
			"ref":        "main",
		},
	})
	requireNoError(t, setupErr, "create tag")

	// Create a release on that tag.
	_, setupErr = callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  projIDStr,
			"tag_name":    tagName,
			"name":        "Group Release Test",
			"description": "E2E test release for group releases",
		},
	})
	requireNoError(t, setupErr, "create release")

	// List releases on the group — should now contain at least one.
	t.Run("ReleaseList_WithRelease", func(t *testing.T) {
		out, err := callToolOn[groupreleases.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "release_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "release_list (with release)")
		requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release, got %d", len(out.Releases))
		t.Logf("Releases: %d (expected >=1)", len(out.Releases))

		found := false
		for _, r := range out.Releases {
			if r.TagName == tagName {
				found = true
				t.Logf("Found release: tag=%s name=%s", r.TagName, r.Name)
			}
		}
		requireTrue(t, found, "expected release with tag %s in group releases", tagName)
	})

	// List with simple=true.
	t.Run("ReleaseList_Simple", func(t *testing.T) {
		out, err := callToolOn[groupreleases.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "release_list",
			"params": map[string]any{
				"group_id": groupIDStr,
				"simple":   true,
			},
		})
		requireNoError(t, err, "release_list (simple)")
		requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release (simple), got %d", len(out.Releases))
		t.Logf("Simple releases: %d", len(out.Releases))
	})
}
