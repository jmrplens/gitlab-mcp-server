//go:build e2e && enterprise

// groupwikis_ee_test.go exercises the group wikis meta-tool against
// a live GitLab EE Ultimate instance. Group wikis are
// Premium/Ultimate-only. The test creates a real wiki page, lists,
// gets, updates, and deletes it via the per-test ledger.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite; tolerates EE-only feature flag activation timing.
package suite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupwikis"
)

// TestMeta_GroupWikis exercises the group wikis meta-tool lifecycle
// (create/list/get/update/delete) via the gitlab_group meta-tool.
func TestMeta_GroupWikis(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group wikis require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-wiki")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	var wikiSlug string

	t.Run("Meta/GroupWiki/Create", func(t *testing.T) {
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_create",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"title":    uniqueName("group-wiki"),
				"content":  "# E2E group wiki\n\nInitial content.",
				"format":   "markdown",
			},
		})
		requireNoError(t, err, "wiki_create")
		requireTruef(t, out.Slug != "", "expected wiki slug")
		wikiSlug = out.Slug
		t.Logf("Created group wiki page %s", wikiSlug)
	})

	t.Run("Meta/GroupWiki/List", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set (Create must run first)")
		out, err := callToolOn[groupwikis.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_list",
			"params": map[string]any{"group_id": grp.gidStr(), "with_content": true},
		})
		requireNoError(t, err, "wiki_list")
		found := false
		for _, p := range out.WikiPages {
			if p.Slug == wikiSlug {
				found = true
				break
			}
		}
		requireTruef(t, found, "created wiki slug %q not in list", wikiSlug)
		t.Logf("Group %s has %d wiki page(s) (created slug present)", groupName, len(out.WikiPages))
	})

	t.Run("Meta/GroupWiki/Get", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set (Create must run first)")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_get",
			"params": map[string]any{"group_id": grp.gidStr(), "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_get")
		requireTruef(t, out.Slug == wikiSlug, "wiki slug mismatch: got %q want %q", out.Slug, wikiSlug)
		t.Logf("Got group wiki page %s", out.Slug)
	})

	t.Run("Meta/GroupWiki/Update", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set (Create must run first)")
		const updatedContent = "# E2E group wiki\n\nUpdated content."
		_, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_edit",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"slug":     wikiSlug,
				"content":  updatedContent,
			},
		})
		requireNoError(t, err, "wiki_edit")
		// Verify the edit took effect by re-fetching the page and
		// asserting the content reflects the new value. Without this
		// check, an edit endpoint that silently no-ops would pass.
		got, getErr := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_get",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"slug":     wikiSlug,
			},
		})
		requireNoError(t, getErr, "wiki_get after edit")
		requireTruef(t, got.Slug == wikiSlug, "wiki slug after edit = %q, want %q", got.Slug, wikiSlug)
		requireTruef(t, strings.Contains(got.Content, "Updated content."), "wiki content after edit does not contain updated string (got: %q)", got.Content)
		t.Logf("Updated and verified group wiki page %s", wikiSlug)
	})

	t.Run("Meta/GroupWiki/Delete", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set (Create must run first)")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_delete",
			"params": map[string]any{"group_id": grp.gidStr(), "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_delete")
		t.Logf("Deleted group wiki page %s", wikiSlug)
	})
}
