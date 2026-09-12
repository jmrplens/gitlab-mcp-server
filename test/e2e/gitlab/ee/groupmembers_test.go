//go:build e2e

// groupmembers_test.go covers the two licensed listings of a group's
// people: the billable members, with the memberships of one of them and
// the removal of one, and the users an identity provider provisioned,
// which on a stack with no provider is nobody.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

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
