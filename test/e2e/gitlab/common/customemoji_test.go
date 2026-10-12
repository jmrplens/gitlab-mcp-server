//go:build e2e

// customemoji_test.go covers a group's custom emoji through its life on
// every surface: created from an image the fixture service serves, listed,
// and deleted. The old suite drove it on two surfaces with a group each;
// each surface gets a group of its own here too, since an emoji name is
// unique within its group.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// customEmojiName is the name every group of this file gets an emoji
// under: letters, digits and underscores, which is all GitLab accepts.
const customEmojiName = "e2e_emoji"

// customEmojiIDs lists the global ids of an emoji listing.
func customEmojiIDs(listed []customemoji.Item) []string {
	ids := make([]string, 0, len(listed))
	for _, emoji := range listed {
		ids = append(ids, emoji.ID)
	}
	return ids
}

// TestCustomEmoji_Lifecycle_CreateListDelete creates an emoji in a group of
// each surface's own, finds it in the listing, deletes it and checks the
// listing lets it go.
//
// Replaces: TestIndividual_CustomEmoji, TestMeta_CustomEmoji
func TestCustomEmoji_Lifecycle_CreateListDelete(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedFixtureService))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("emoji"))
		params := map[string]any{"group_path": group.Path}
		image := fixture.ServiceURL(e, "/emoji.png")

		created := harness.Do[customemoji.CreateOutput](s, actionCustomEmojiCreate, withParams(params, map[string]any{"name": customEmojiName, "url": image}))
		if created.Emoji.ID == "" || created.Emoji.Name != customEmojiName || created.Emoji.URL != image {
			e.T.Fatalf("create answered %+v, want the emoji %q at %s with a global id", created.Emoji, customEmojiName, image)
		}

		listed := harness.Do[customemoji.ListOutput](s, actionCustomEmojiList, params)
		if !containsKey(customEmojiIDs(listed.Emoji), created.Emoji.ID) {
			e.T.Errorf("the group lists the emoji %v, want %s among them", customEmojiIDs(listed.Emoji), created.Emoji.ID)
		}

		harness.DoVoid(s, actionCustomEmojiDelete, map[string]any{"id": created.Emoji.ID})
		remaining := harness.Do[customemoji.ListOutput](s, actionCustomEmojiList, params)
		if containsKey(customEmojiIDs(remaining.Emoji), created.Emoji.ID) {
			e.T.Errorf("the group still lists the emoji %s after its delete", created.Emoji.ID)
		}
	})
}

// TestCustomEmoji_Subgroup_ListsTheAncestorsEmojiWhenAskedTo creates an emoji
// in a parent group and lists an empty subgroup of it twice: as GitLab
// defaults, which leaves the parent's emoji out and has to say so and name
// include_ancestor_groups, and with include_ancestor_groups, which lists it
// (issue 1342). The empty answer used to read as "this group has no custom
// emoji", and a model following it created a duplicate the parent already had.
func TestCustomEmoji_Subgroup_ListsTheAncestorsEmojiWhenAskedTo(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedFixtureService))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		parent := fixture.NewGroup(e, fixture.WithGroupNamePrefix("emoji-parent"))
		subgroup := fixture.NewSubgroup(e, parent, fixture.WithGroupNamePrefix("emoji-sub"))
		image := fixture.ServiceURL(e, "/emoji.png")

		created := harness.Do[customemoji.CreateOutput](s, actionCustomEmojiCreate, map[string]any{"group_path": parent.Path, "name": customEmojiName, "url": image})
		if created.Emoji.ID == "" {
			e.T.Fatalf("create answered %+v, want an emoji with a global id", created.Emoji)
		}

		own, said := harness.DoAnswer[customemoji.ListOutput](s, actionCustomEmojiList, map[string]any{"group_path": subgroup.Path})
		if containsKey(customEmojiIDs(own.Emoji), created.Emoji.ID) {
			e.T.Errorf("the subgroup lists the parent's emoji %s without include_ancestor_groups, want its own only", created.Emoji.ID)
		}
		assertMentions(e, "the subgroup listed without its ancestors", said, "include_ancestor_groups=true")

		inherited := harness.Do[customemoji.ListOutput](s, actionCustomEmojiList, map[string]any{"group_path": subgroup.Path, "include_ancestor_groups": true})
		if !containsKey(customEmojiIDs(inherited.Emoji), created.Emoji.ID) {
			e.T.Errorf("the subgroup lists %v with include_ancestor_groups, want the parent's emoji %s among them", customEmojiIDs(inherited.Emoji), created.Emoji.ID)
		}

		harness.DoVoid(s, actionCustomEmojiDelete, map[string]any{"id": created.Emoji.ID})
	})
}
