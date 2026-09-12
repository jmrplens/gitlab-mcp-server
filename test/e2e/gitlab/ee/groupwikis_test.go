//go:build e2e

// groupwikis_test.go covers a group's wiki pages: create one, find it in
// the listing, read it, edit it and check the edit landed, delete it and
// check the listing lets it go.

package ee

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two bodies a wiki page is written with, so the edit is told apart from
// the create by what a read answers.
const (
	wikiInitialContent = "# E2E group wiki\n\nInitial content."
	wikiUpdatedContent = "# E2E group wiki\n\nUpdated content."
)

// wikiSlugs lists the slugs of a wiki listing.
func wikiSlugs(pages []groupwikis.Output) []string {
	slugs := make([]string, 0, len(pages))
	for _, page := range pages {
		slugs = append(slugs, page.Slug)
	}
	return slugs
}

// TestGroupWikis_Lifecycle_CreateListGetEditDelete walks one wiki page per
// surface through its whole life in a shared group.
//
// Replaces: TestMeta_GroupWikis, TestEE_MetaGroupEnterpriseOperations
func TestGroupWikis_Lifecycle_CreateListGetEditDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("wikis"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}

		created := harness.Do[groupwikis.Output](s, actionGroupWikiCreate, withParams(params, map[string]any{
			"title": e.Name("page"), "content": wikiInitialContent, "format": "markdown",
		}))
		if created.Slug == "" {
			e.T.Fatalf("wiki_create answered %+v, want a page with a slug", created)
		}
		page := withParams(params, map[string]any{"slug": created.Slug})

		listed := harness.Do[groupwikis.ListOutput](s, actionGroupWikiList, withParams(params, map[string]any{"with_content": true}))
		if !slices.Contains(wikiSlugs(listed.WikiPages), created.Slug) {
			e.T.Errorf("the group's wiki pages do not hold %q: %v", created.Slug, wikiSlugs(listed.WikiPages))
		}
		got := harness.Do[groupwikis.Output](s, actionGroupWikiGet, page)
		if got.Slug != created.Slug || got.Content != wikiInitialContent {
			e.T.Errorf("wiki_get answered %q with %q, want %q with the initial content", got.Slug, got.Content, created.Slug)
		}

		edited := harness.Do[groupwikis.Output](s, actionGroupWikiEdit, withParams(page, map[string]any{"content": wikiUpdatedContent}))
		if edited.Slug != created.Slug || edited.Content != wikiUpdatedContent {
			e.T.Errorf("wiki_edit answered %q with %q, want %q with the updated content", edited.Slug, edited.Content, created.Slug)
		}
		// The edit is checked through a read as well as through its own
		// answer: an edit endpoint that echoed the request and wrote
		// nothing would pass the first and fail this.
		reread := harness.Do[groupwikis.Output](s, actionGroupWikiGet, page)
		if reread.Content != wikiUpdatedContent {
			e.T.Errorf("the page reads %q after the edit, want the updated content", reread.Content)
		}

		harness.DoVoid(s, actionGroupWikiDelete, page)
		after := harness.Do[groupwikis.ListOutput](s, actionGroupWikiList, params)
		if slices.Contains(wikiSlugs(after.WikiPages), created.Slug) {
			e.T.Errorf("the group's wiki pages still hold %q after its delete", created.Slug)
		}
	})
}
