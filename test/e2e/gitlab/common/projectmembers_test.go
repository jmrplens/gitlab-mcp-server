//go:build e2e

// projectmembers_test.go covers the two member reads of the project tool
// on a project of the test's own: the listing, which holds its creator as
// the owner, and the read of that one direct member by user ID. The project
// is made in the run user's own namespace on purpose, since that is what
// makes the creator a direct member rather than one inherited from a group.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

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
