//go:build e2e

// groupmembers_test.go covers a group's direct members through their life:
// added as a Developer, raised to Maintainer, listed, and removed. The add
// alone is what groups_test.go covers, since the old suite drove it before
// its Enterprise scenarios; the rest of the life is here.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupMemberIDs lists the user ids of a group member listing.
func groupMemberIDs(listed []groups.MemberOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, member := range listed {
		ids = append(ids, member.ID)
	}
	return ids
}

// TestGroupMembers_Lifecycle_AddEditListAndRemove adds a disposable user to
// a group of each surface's own as a Developer, raises it to Maintainer,
// finds it in the listing, and removes it.
//
// Replaces: TestMeta_GroupMemberLifecycle
func TestGroupMembers_Lifecycle_AddEditListAndRemove(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("memberlife"))
		user := fixture.NewUser(e, "member")
		params := map[string]any{"group_id": group.IDParam()}
		member := withParams(params, map[string]any{"user_id": user.ID})

		added := harness.Do[groupmembers.Output](s, actionGroupMemberAdd, withParams(member, map[string]any{"access_level": int64(gl.DeveloperPermissions)}))
		if added.ID != user.ID || added.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("group_member_add answered user %d at level %d, want %d as a Developer", added.ID, added.AccessLevel, user.ID)
		}
		edited := harness.Do[groupmembers.Output](s, actionGroupMemberEdit, withParams(member, map[string]any{"access_level": int64(gl.MaintainerPermissions)}))
		if edited.ID != user.ID || edited.AccessLevel != int(gl.MaintainerPermissions) {
			e.T.Errorf("group_member_edit answered user %d at level %d, want %d as a Maintainer", edited.ID, edited.AccessLevel, user.ID)
		}
		listed := harness.Do[groups.MemberListOutput](s, actionGroupMembers, params)
		if !containsID(groupMemberIDs(listed.Members), user.ID) {
			e.T.Errorf("the group lists the members %v, want user %d among them", groupMemberIDs(listed.Members), user.ID)
		}

		removed := harness.Do[toolutil.DeleteOutput](s, actionGroupMemberRemove, member)
		if removed.Status != voidStatusSuccess {
			e.T.Errorf("group_member_remove answered %+v, want a %s status", removed, voidStatusSuccess)
		}
		remaining := harness.Do[groups.MemberListOutput](s, actionGroupMembers, params)
		if containsID(groupMemberIDs(remaining.Members), user.ID) {
			e.T.Errorf("the group still lists user %d after the member was removed", user.ID)
		}
	})
}
