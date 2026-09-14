//go:build e2e

// badges_test.go covers a project badge through its whole life on every
// surface, and the preview that renders a badge's URLs without storing
// one. The old suite drove the lifecycle on two surfaces and the preview
// on one.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The URLs a badge is created with, edited to, and previewed at. They are
// never fetched: GitLab stores them and renders their placeholders.
const (
	badgeLinkURL        = "https://example.com/badge"
	badgeUpdatedLinkURL = "https://example.com/badge-updated"
	badgeImageURL       = "https://example.com/badge.svg"
	badgePreviewLinkURL = "https://example.com/preview"
)

// badgeIDs lists the ids of a badge listing.
func badgeIDs(listed []badges.BadgeItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, badge := range listed {
		ids = append(ids, badge.ID)
	}
	return ids
}

// TestProjectBadges_Lifecycle_AddGetListEditPreviewDelete adds a badge to
// a project of each surface's own, reads it by id and in the listing,
// changes its link, previews another badge's rendering, deletes it and
// checks the listing lets it go.
//
// Replaces: TestIndividual_Badges, TestMeta_Badges, TestMeta_ProjectBadgesDeep
func TestProjectBadges_Lifecycle_AddGetListEditPreviewDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("badges"))
		params := map[string]any{"project_id": project.IDParam()}
		name := e.Name("badge")

		added := harness.Do[badges.AddProjectOutput](s, actionProjectBadgeAdd, withParams(params, map[string]any{
			"link_url": badgeLinkURL, "image_url": badgeImageURL, "name": name,
		}))
		if added.Badge.ID == 0 || added.Badge.LinkURL != badgeLinkURL || added.Badge.Name != name {
			e.T.Fatalf("badge_add answered %+v, want the badge %q linking to %s with an ID", added.Badge, name, badgeLinkURL)
		}
		badge := withParams(params, map[string]any{"badge_id": added.Badge.ID})

		got := harness.Do[badges.GetProjectOutput](s, actionProjectBadgeGet, badge)
		if got.Badge.ID != added.Badge.ID || got.Badge.ImageURL != badgeImageURL {
			e.T.Errorf("badge_get answered %+v, want badge %d with the image %s", got.Badge, added.Badge.ID, badgeImageURL)
		}
		listed := harness.Do[badges.ListProjectOutput](s, actionProjectBadgeList, params)
		if !containsID(badgeIDs(listed.Badges), added.Badge.ID) {
			e.T.Errorf("the project lists the badges %v, want badge %d among them", badgeIDs(listed.Badges), added.Badge.ID)
		}

		edited := harness.Do[badges.EditProjectOutput](s, actionProjectBadgeEdit, withParams(badge, map[string]any{"link_url": badgeUpdatedLinkURL}))
		if edited.Badge.ID != added.Badge.ID || edited.Badge.LinkURL != badgeUpdatedLinkURL {
			e.T.Errorf("badge_edit answered %+v, want badge %d linking to %s", edited.Badge, added.Badge.ID, badgeUpdatedLinkURL)
		}

		// The URL previewed carries no placeholder, so the rendering leaves
		// it alone, which is what makes the answer checkable.
		preview := harness.Do[badges.PreviewProjectOutput](s, actionProjectBadgePreview, withParams(params, map[string]any{
			"link_url": badgePreviewLinkURL, "image_url": badgeImageURL,
		}))
		if preview.Badge.RenderedLinkURL != badgePreviewLinkURL {
			e.T.Errorf("badge_preview rendered the link as %q, want %s unchanged", preview.Badge.RenderedLinkURL, badgePreviewLinkURL)
		}

		harness.DoVoid(s, actionProjectBadgeDelete, badge)
		remaining := harness.Do[badges.ListProjectOutput](s, actionProjectBadgeList, params)
		if containsID(badgeIDs(remaining.Badges), added.Badge.ID) {
			e.T.Errorf("the project still lists badge %d after its delete", added.Badge.ID)
		}
	})
}
