//go:build e2e

// groupmembers_test.go covers the two licensed listings of a group's
// people: the billable members, with the memberships of one of them and
// the removal of one, and the users an identity provider provisioned,
// which on a stack with no provider is nobody. It also covers the one
// licensed part of a group share, the custom role it grants.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The wait for a new membership to reach the billable listing, which
// GitLab serves from a count it updates a moment after the add.
const (
	billableInterval = 2 * time.Second
	billableWait     = 60 * time.Second
)

// The wait for a removal to leave the billable listing. GitLab documents
// the removal as asynchronous and completing in a few minutes: the API
// schedules the deletion and a worker performs it, so the wait is sized to
// the documentation and made once for every surface's member rather than
// once per surface.
const (
	billableRemovalInterval = 5 * time.Second
	billableRemovalWait     = 5 * time.Minute
)

// billableFixture is a group with one Developer per surface in it, so each
// surface has a member of its own to remove and the one wait for the
// removals covers all three.
type billableFixture struct {
	group fixture.Group
	users map[harness.Surface]fixture.User
}

// buildBillableFixture creates the group and adds the users.
func buildBillableFixture(e *harness.Env) billableFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("billable"))
	users := map[harness.Surface]fixture.User{}
	for _, surface := range harness.AllSurfaces() {
		user := fixture.NewUser(e, "billable-"+string(surface))
		fixture.AddGroupMember(e, group, user, gl.DeveloperPermissions)
		users[surface] = user
	}
	return billableFixture{group: group, users: users}
}

// containsAnyUsername reports whether a billable listing holds any of the
// users.
func containsAnyUsername(members []groupmembers.BillableMemberOutput, users map[harness.Surface]fixture.User) bool {
	for _, user := range users {
		if containsUsername(members, user.Username) {
			return true
		}
	}
	return false
}

// billableUsernames lists the usernames of a billable member listing.
func billableUsernames(members []groupmembers.BillableMemberOutput) []string {
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.Username)
	}
	return names
}

// containsUsername reports whether a billable listing holds a member.
func containsUsername(members []groupmembers.BillableMemberOutput, username string) bool {
	for _, member := range members {
		if member.Username == username {
			return true
		}
	}
	return false
}

// TestGroupBillableMembers_DeveloperAdded_ListedWithMembershipsAndRemoved
// gives each surface a Developer of its own in a shared group, waits for
// the billable listing to show them beside the owner, lists their
// memberships and asks for their removal; then waits, once for all three,
// for the listing to let them go, which GitLab does in the background. The
// owner cannot be the one removed: GitLab keeps the last owner, which is
// why the scenario needs the other users.
//
// Replaces: TestMeta_GroupBillableMembers
func TestGroupBillableMembers_DeveloperAdded_ListedWithMembershipsAndRemoved(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	// Built on the parent's Env rather than through SurfacesWith, because the
	// wait for the removals comes after the surfaces and needs the users.
	f := buildBillableFixture(e)
	params := map[string]any{"group_id": f.group.IDParam()}
	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		user := f.users[surface]

		before := harness.Do[groupmembers.BillableMembersOutput](s, actionGroupBillableMembersList, params)
		if !containsUsername(before.Members, e.Runtime().Username) {
			e.T.Errorf("the group's billable members do not hold its owner %q: %v", e.Runtime().Username, billableUsernames(before.Members))
		}
		listed := harness.Eventually(s, actionGroupBillableMembersList, withParams(params, map[string]any{"search": user.Username}),
			billableInterval, billableWait,
			func(out groupmembers.BillableMembersOutput) bool { return containsUsername(out.Members, user.Username) })
		if !listed.Members[0].Removable {
			e.T.Errorf("the billable member %q is listed as not removable, and a Developer is", user.Username)
		}

		memberships := harness.Do[groupmembers.BillableMembershipsOutput](s, actionGroupBillableMemberMembershipsList, withParams(params, map[string]any{"user_id": user.ID}))
		if len(memberships.Memberships) == 0 {
			e.T.Errorf("the billable member %q has no memberships listed, and was just added to the group", user.Username)
		}

		harness.DoVoid(s, actionGroupBillableMemberRemove, withParams(params, map[string]any{"user_id": user.ID}))
	})

	// The removal is acknowledged before it is done: the API schedules the
	// deletion and a worker performs it, in a few minutes by GitLab's own
	// account. The three removals are waited for together, and the owner is
	// what must remain.
	s := e.On(harness.SurfaceDynamic)
	fixture.DrainSidekiq(e.Ctx, e.Client())
	after := harness.Eventually(s, actionGroupBillableMembersList, params, billableRemovalInterval, billableRemovalWait,
		func(out groupmembers.BillableMembersOutput) bool { return !containsAnyUsername(out.Members, f.users) })
	if !containsUsername(after.Members, e.Runtime().Username) {
		e.T.Errorf("the group's billable members no longer hold its owner %q after the removals: %v", e.Runtime().Username, billableUsernames(after.Members))
	}
}

// shareGrant is what a group share records for the group it is made with:
// the access level its members gain and the custom role it grants, read off
// the row of either package that publishes one.
type shareGrant struct {
	level int64
	role  int64
}

// memberShareGrant finds the share with a group among the rows a members'
// share answers with.
func memberShareGrant(links []groupmembers.SharedWithGroupOutput, groupID int64) (shareGrant, bool) {
	for _, link := range links {
		if link.GroupID == groupID {
			return shareGrant{level: link.GroupAccessLevel, role: link.MemberRoleID}, true
		}
	}
	return shareGrant{}, false
}

// detailShareGrant finds the share with a group among the rows a group's
// detail lists.
func detailShareGrant(links []groups.SharedWithGroupOutput, groupID int64) (shareGrant, bool) {
	for _, link := range links {
		if link.GroupID == groupID {
			return shareGrant{level: link.GroupAccessLevel, role: link.MemberRoleID}, true
		}
	}
	return shareGrant{}, false
}

// TestGroupMemberShare_CustomRole_RecordedOnTheShare shares a host group of
// each surface's own with a guest group of its own through
// group.group_member_share, at Guest and with an instance custom role based
// on Guest, and checks that the share with the guest carries that level and
// that role twice: in the answer, and in the host's detail read back
// afterwards. The answer alone could only echo what was sent; the detail is
// what GitLab kept. The share is then revoked through the members' unshare.
//
// GitLab drops member_role_id without a word where custom roles are not
// licensed, so on an Ultimate instance a share that comes back without the
// role is the defect this scenario exists to catch: until issue 1027 the
// action offered no member_role_id at all.
func TestGroupMemberShare_CustomRole_RecordedOnTheShare(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	// fixture.NewMemberRole builds the role on Guest, the level the share
	// has to name, since GitLab refuses a custom role whose base access
	// level is not the share's.
	role := fixture.NewMemberRole(e)
	want := shareGrant{level: memberRoleBaseAccessLevel, role: role.ID}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		host := fixture.NewGroup(e, fixture.WithGroupNamePrefix("share-host"))
		guest := fixture.NewGroup(e, fixture.WithGroupNamePrefix("share-guest"))
		params := map[string]any{"group_id": host.IDParam()}

		shared := harness.Do[groupmembers.ShareOutput](s, actionGroupMemberShare, withParams(params, map[string]any{
			"share_group_id": guest.ID, "group_access": memberRoleBaseAccessLevel, "member_role_id": role.ID,
		}))
		if shared.ID != host.ID {
			e.T.Errorf("group_member_share answered group %d, want the host %d", shared.ID, host.ID)
		}
		if got, found := memberShareGrant(shared.SharedWithGroups, guest.ID); !found || got != want {
			e.T.Errorf("the share's answer records the share with group %d as %+v (found %t), want %+v", guest.ID, got, found, want)
		}

		detail := harness.Do[groups.DetailOutput](s, actionGroupGet, params)
		if got, found := detailShareGrant(detail.SharedWithGroups, guest.ID); !found || got != want {
			e.T.Errorf("the host's detail records the share with group %d as %+v (found %t), want %+v", guest.ID, got, found, want)
		}

		harness.DoVoid(s, actionGroupMemberUnshare, withParams(params, map[string]any{"share_group_id": guest.ID}))
		if after := harness.Do[groups.DetailOutput](s, actionGroupGet, params); len(after.SharedWithGroups) != 0 {
			e.T.Errorf("the host is still shared with %+v after the members' unshare", after.SharedWithGroups)
		}
	})
}

// TestGroupProvisionedUsers_NoProvider_ListsNobody lists the provisioned
// users of a fresh group on every surface, ordered the way the action
// offers, and checks the answer names nobody.
//
// Replaces: TestMeta_GroupProvisionedUsers
func TestGroupProvisionedUsers_NoProvider_ListsNobody(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("provisioned"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		listed := harness.Do[groups.ProvisionedUsersListOutput](s, actionGroupListProvisionedUsers, map[string]any{
			"group_id": group.IDParam(), "order_by": "id", "sort": "asc",
		})
		if len(listed.Users) != 0 {
			e.T.Errorf("a group with no identity provider lists %d provisioned user(s): %+v", len(listed.Users), listed.Users)
		}
	})
}
