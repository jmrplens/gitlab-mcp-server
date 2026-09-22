//go:build e2e

// b7_group_members_test.go covers the two single-member reads of a group, the
// direct one and the inherited one, on both the answer they give and the
// refusal they give.
//
// groupmembers_test.go drives the add, edit, listing and removal; these two
// reads were reached here only by a read sweep, which asserts nothing about
// the answer. The two are worth telling apart rather than driving together:
// the direct read sees only what the group itself grants, the inherited one
// sees what an ancestor grants as well, and the whole difference between them
// is what a subgroup answers about a member of its parent. Both refusals are
// asserted too, because the old CE suite covered these actions by their
// refusal alone and a refusal is a shape a successful call says nothing
// about.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupMemberReadsFixture is one membership seen from three places: the group
// that granted it, a subgroup that inherits it, and a group unrelated to
// either, which grants the user nothing.
type groupMemberReadsFixture struct {
	parent   fixture.Group
	child    fixture.Group
	stranger fixture.Group
	user     fixture.User
}

// TestGroupMembers_SingleReads_DirectAndInheritedAgreeAndRefuse adds a
// disposable user to a group as a Developer and reads that membership back
// four ways: directly on the group that granted it, inherited on a subgroup
// of it, directly on that subgroup, which does not grant it, and inherited on
// a group unrelated to both.
//
// It runs on the dynamic, meta and individual surfaces against one membership
// built for all three, since every call here is a read and none of them
// changes what the next one sees. It needs an admin credential because it
// creates a user. It asserts that the direct read answers the user at the
// level it was granted, that the inherited read answers the same from the
// subgroup, that the direct read of the subgroup is refused as not found and
// names the sibling tool that does see inherited members, and that the
// inherited read of the unrelated group is refused as not found and says the
// user is in neither it nor any ancestor of it.
func TestGroupMembers_SingleReads_DirectAndInheritedAgreeAndRefuse(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) groupMemberReadsFixture {
		parent := fixture.NewGroup(e, fixture.WithGroupNamePrefix("memberreads"))
		user := fixture.NewUser(e, "memberread")
		fixture.AddGroupMember(e, parent, user, gl.DeveloperPermissions)
		return groupMemberReadsFixture{
			parent:   parent,
			child:    fixture.NewSubgroup(e, parent, fixture.WithGroupNamePrefix("child")),
			stranger: fixture.NewGroup(e, fixture.WithGroupNamePrefix("stranger")),
			user:     user,
		}
	}, func(e *harness.Env, surface harness.Surface, f groupMemberReadsFixture) {
		s := e.On(surface)
		onParent := map[string]any{"group_id": f.parent.IDParam(), "user_id": f.user.ID}
		onChild := map[string]any{"group_id": f.child.IDParam(), "user_id": f.user.ID}

		direct := harness.Do[groupmembers.Output](s, actionGroupMemberGet, onParent)
		if direct.ID != f.user.ID || direct.Username != f.user.Username || direct.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("group_member_get answered %+v, want user %d (%s) as a Developer", direct, f.user.ID, f.user.Username)
		}

		inherited := harness.Do[groupmembers.Output](s, actionGroupMemberGetInherited, onChild)
		if inherited.ID != f.user.ID || inherited.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("group_member_get_inherited answered %+v for the subgroup, want user %d as a Developer inherited from its parent",
				inherited, f.user.ID)
		}

		// The subgroup grants nothing of its own, so the direct read of it
		// does not see the membership the inherited read just answered with.
		notDirect := harness.Refused(s, actionGroupMemberGet, onChild, harness.FailureNotFound)
		assertMentions(e, "the direct member read of a subgroup", notDirect, "group.group_member_get_inherited")

		// No group in the unrelated group's tree grants the user anything, so
		// even the inherited read has nothing to answer with.
		notInherited := harness.Refused(s, actionGroupMemberGetInherited,
			map[string]any{"group_id": f.stranger.IDParam(), "user_id": f.user.ID}, harness.FailureNotFound)
		assertMentions(e, "the inherited member read of an unrelated group", notInherited, "ancestor group", "group.members")
	})
}
