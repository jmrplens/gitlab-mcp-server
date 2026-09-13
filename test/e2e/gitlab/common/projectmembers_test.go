//go:build e2e

// projectmembers_test.go covers a project's direct members: the reads of a
// project of the test's own, whose creator is its one direct member, the
// life of a member added to it, and the read of an inherited membership,
// refused for a user who is a member of nothing and answered for one who
// holds the project's group.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// memberIDs lists the user ids of a member listing.
func memberIDs(listed []members.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, member := range listed {
		ids = append(ids, member.ID)
	}
	return ids
}

// TestProjectMembers_Lifecycle_AddEditAndRemove adds a disposable user to
// a personal project of each surface's own as a Developer, raises it to
// Maintainer, finds it in the listing, and removes it.
//
// Replaces: TestMeta_ProjectMemberLifecycle
func TestProjectMembers_Lifecycle_AddEditAndRemove(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("members"))
		user := fixture.NewUser(e, "member")
		params := map[string]any{"project_id": project.IDParam()}
		member := withParams(params, map[string]any{"user_id": user.ID})

		added := harness.Do[members.Output](s, actionProjectMemberAdd, withParams(member, map[string]any{"access_level": int64(gl.DeveloperPermissions)}))
		if added.ID != user.ID || added.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("member_add answered user %d at level %d, want %d as a Developer", added.ID, added.AccessLevel, user.ID)
		}
		edited := harness.Do[members.Output](s, actionProjectMemberEdit, withParams(member, map[string]any{"access_level": int64(gl.MaintainerPermissions)}))
		if edited.ID != user.ID || edited.AccessLevel != int(gl.MaintainerPermissions) {
			e.T.Errorf("member_edit answered user %d at level %d, want %d as a Maintainer", edited.ID, edited.AccessLevel, user.ID)
		}
		listed := harness.Do[members.ListOutput](s, actionProjectMembers, params)
		if !containsID(memberIDs(listed.Members), user.ID) {
			e.T.Errorf("the project lists the members %v, want user %d among them", memberIDs(listed.Members), user.ID)
		}

		harness.DoVoid(s, actionProjectMemberDelete, member)
		remaining := harness.Do[members.ListOutput](s, actionProjectMembers, params)
		if containsID(memberIDs(remaining.Members), user.ID) {
			e.T.Errorf("the project still lists user %d after the member was removed", user.ID)
		}
	})
}

// TestProjectMembers_Inherited_RefusedThenAnsweredThroughTheGroup asks for
// the inherited membership of a disposable user in a group project of each
// surface's own: refused while the user belongs to nothing, answered with
// the group's access level once the user holds the group.
//
// Replaces: TestMeta_ProjectMembersDeep
func TestProjectMembers_Inherited_RefusedThenAnsweredThroughTheGroup(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("inherited"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("inherited"), fixture.InGroup(group))
		user := fixture.NewUser(e, "inheritor")
		member := map[string]any{"project_id": project.IDParam(), "user_id": user.ID}

		refused := harness.Refused(s, actionProjectMemberInherited, member, harness.FailureNotFound)
		e.T.Logf("the inherited read of a user who is a member of nothing is refused: %s", firstLine(refused))

		fixture.AddGroupMember(e, group, user, gl.DeveloperPermissions)
		inherited := harness.Do[members.Output](s, actionProjectMemberInherited, member)
		if inherited.ID != user.ID || inherited.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("member_inherited answered user %d at level %d, want %d as the group's Developer", inherited.ID, inherited.AccessLevel, user.ID)
		}
	})
}

// TestProjectMembers_OwnProject_ListsAndGetsTheOwner lists the members of
// one project on every surface, finds the run user among them as its
// owner, and reads that membership back by user ID.
//
// Replaces: TestIndividual_Members, TestMeta_Members
func TestProjectMembers_OwnProject_ListsAndGetsTheOwner(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("members"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		id := project.IDParam()

		listed := harness.Do[members.ListOutput](s, actionProjectMembers, map[string]any{"project_id": id})
		if !containsID(memberUserIDs(listed.Members), rt.UserID) {
			e.T.Fatalf("the members of project %d do not hold its creator %d: %+v", project.ID, rt.UserID, listed.Members)
		}

		owner := harness.Do[members.Output](s, actionProjectMemberGet, map[string]any{"project_id": id, "user_id": rt.UserID})
		if owner.ID != rt.UserID || owner.Username != rt.Username {
			e.T.Errorf("member_get answered %d %q, want the run user %d %q", owner.ID, owner.Username, rt.UserID, rt.Username)
		}
		if owner.AccessLevel != int(gl.OwnerPermissions) {
			e.T.Errorf("the creator holds access level %d on project %d, want owner (%d)", owner.AccessLevel, project.ID, gl.OwnerPermissions)
		}
	})
}

// memberUserIDs collects the user IDs of listed members.
func memberUserIDs(listed []members.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, member := range listed {
		ids = append(ids, member.ID)
	}
	return ids
}
