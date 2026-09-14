//go:build e2e

// b7_group_badges_test.go covers a group badge through its whole life on
// every surface, and the preview that renders a badge's URLs without storing
// one.
//
// The group half of the badge domain was the one sub-family nothing here
// reached: badges_test.go drives the project half, and the group half was
// left to the old suite, which drove it on the meta surface alone. A group
// badge is not a project badge under another id — GitLab stores it on the
// group, hands every project in the group a copy of it, and says so in the
// kind field — so the two halves are asserted apart.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupBadgeKind is what GitLab calls a badge stored on a group, and is what
// tells one apart from a project badge of the same shape.
const groupBadgeKind = "group"

// TestGroupBadges_Lifecycle_AddGetListEditPreviewDelete adds a badge to a
// group of each surface's own, reads it back by its id and in the listing,
// changes its link, previews another badge's rendering without storing it,
// deletes it and checks the listing lets it go.
//
// It runs on the dynamic, meta and individual surfaces. It asserts that the
// add answers the badge it was given with an id and the group kind, that the
// get and the listing find that id, that the edit answers the new link for
// the same id, that the preview renders back the URL it was handed and stores
// nothing, and that the delete reports success and empties the listing.
func TestGroupBadges_Lifecycle_AddGetListEditPreviewDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("badges"))
		params := map[string]any{"group_id": group.IDParam()}
		name := e.Name("badge")

		added := harness.Do[badges.AddGroupOutput](s, actionGroupBadgeAdd, withParams(params, map[string]any{
			"link_url": badgeLinkURL, "image_url": badgeImageURL, "name": name,
		}))
		if added.Badge.ID == 0 || added.Badge.LinkURL != badgeLinkURL || added.Badge.Name != name {
			e.T.Fatalf("badge_add answered %+v, want the badge %q linking to %s with an ID", added.Badge, name, badgeLinkURL)
		}
		if added.Badge.Kind != groupBadgeKind {
			e.T.Errorf("badge_add answered the kind %q, want %q for a badge stored on a group", added.Badge.Kind, groupBadgeKind)
		}
		badge := withParams(params, map[string]any{"badge_id": added.Badge.ID})

		got := harness.Do[badges.GetGroupOutput](s, actionGroupBadgeGet, badge)
		if got.Badge.ID != added.Badge.ID || got.Badge.ImageURL != badgeImageURL {
			e.T.Errorf("badge_get answered %+v, want badge %d with the image %s", got.Badge, added.Badge.ID, badgeImageURL)
		}
		listed := harness.Do[badges.ListGroupOutput](s, actionGroupBadgeList, params)
		if !containsID(badgeIDs(listed.Badges), added.Badge.ID) {
			e.T.Errorf("the group lists the badges %v, want badge %d among them", badgeIDs(listed.Badges), added.Badge.ID)
		}

		edited := harness.Do[badges.EditGroupOutput](s, actionGroupBadgeEdit, withParams(badge, map[string]any{"link_url": badgeUpdatedLinkURL}))
		if edited.Badge.ID != added.Badge.ID || edited.Badge.LinkURL != badgeUpdatedLinkURL {
			e.T.Errorf("badge_edit answered %+v, want badge %d linking to %s", edited.Badge, added.Badge.ID, badgeUpdatedLinkURL)
		}

		// The URL previewed carries no placeholder, so the rendering leaves it
		// alone, which is what makes the answer checkable.
		preview := harness.Do[badges.PreviewGroupOutput](s, actionGroupBadgePreview, withParams(params, map[string]any{
			"link_url": badgePreviewLinkURL, "image_url": badgeImageURL,
		}))
		if preview.Badge.RenderedLinkURL != badgePreviewLinkURL {
			e.T.Errorf("badge_preview rendered the link as %q, want %s unchanged", preview.Badge.RenderedLinkURL, badgePreviewLinkURL)
		}
		// A preview renders and stores nothing, so the group still holds the
		// one badge that was added.
		afterPreview := harness.Do[badges.ListGroupOutput](s, actionGroupBadgeList, params)
		if ids := badgeIDs(afterPreview.Badges); len(ids) != 1 || ids[0] != added.Badge.ID {
			e.T.Errorf("the group lists the badges %v after a preview, want only badge %d", ids, added.Badge.ID)
		}

		deleted := harness.Do[toolutil.DeleteOutput](s, actionGroupBadgeDelete, badge)
		if deleted.Status != voidStatusSuccess {
			e.T.Errorf("badge_delete answered %+v, want a %s status", deleted, voidStatusSuccess)
		}
		remaining := harness.Do[badges.ListGroupOutput](s, actionGroupBadgeList, params)
		if containsID(badgeIDs(remaining.Badges), added.Badge.ID) {
			e.T.Errorf("the group still lists badge %d after its delete", added.Badge.ID)
		}
	})
}
