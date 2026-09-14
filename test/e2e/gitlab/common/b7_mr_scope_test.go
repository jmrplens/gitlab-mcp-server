//go:build e2e

// b7_mr_scope_test.go covers the two merge request listings that are not
// scoped to one project: the instance-wide one and the group one.
//
// What makes the assertion worth making is that the fixture's own request has
// to be found in each answer. A listing at instance scope holds whatever else
// the instance has, so a length check would pass on somebody else's request;
// the scenario therefore narrows each listing by the request's own generated
// title and looks for the project and IID pair it created.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupMergeRequestFixture is a merge request in a project owned by a group,
// which is what the group listing needs: a project in the caller's own
// namespace belongs to no group and would be invisible to it.
type groupMergeRequestFixture struct {
	group fixture.Group
	mr    mergeRequestFixture
}

// buildGroupMergeRequestFixture creates the group, a project under it, and one
// merge request in that project. It is built once for all three surfaces,
// since every call here is a read and none of them consumes it.
func buildGroupMergeRequestFixture(e *harness.Env) groupMergeRequestFixture {
	e.T.Helper()

	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("mrscope"))
	project := fixture.NewProject(e, fixture.WithNamePrefix("mrscope"), fixture.InGroup(group))
	return groupMergeRequestFixture{group: group, mr: newMergeRequestIn(e, project, "mrscope")}
}

// TestMergeRequest_ListGlobalAndListGroup_FindTheRequest reads the
// instance-wide listing and the group listing on every surface, each narrowed
// to the fixture request's own title, and checks both answer with that
// request: the same project and the same IID the fixture opened.
func TestMergeRequest_ListGlobalAndListGroup_FindTheRequest(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildGroupMergeRequestFixture, func(e *harness.Env, surface harness.Surface, f groupMergeRequestFixture) {
		s := e.On(surface)
		projectID := f.mr.project.ID
		iid := f.mr.mr.IID

		global := harness.Do[mergerequests.ListOutput](s, actionMergeRequestListGlobal, map[string]any{
			"scope":  "created_by_me",
			"state":  "opened",
			"search": f.mr.mr.Title,
		})
		if !mergeRequestListed(global.MergeRequests, projectID, iid) {
			e.T.Errorf("the instance-wide listing does not hold !%d of project %d among its %d row(s): %v",
				iid, projectID, len(global.MergeRequests), mergeRequestIIDs(global.MergeRequests))
		}

		group := harness.Do[mergerequests.ListOutput](s, actionMergeRequestListGroup, map[string]any{
			"group_id": f.group.IDParam(),
			"state":    "opened",
			"search":   f.mr.mr.Title,
		})
		if !mergeRequestListed(group.MergeRequests, projectID, iid) {
			e.T.Errorf("the listing of group %d does not hold !%d of project %d among its %d row(s): %v",
				f.group.ID, iid, projectID, len(group.MergeRequests), mergeRequestIIDs(group.MergeRequests))
		}
	})
}

// mergeRequestListed reports whether a listing holds the request identified by
// its project and IID. Both halves are needed: an IID is project-scoped, so
// two requests in two projects share it routinely, and a listing that is not
// scoped to one project would otherwise be satisfied by a stranger's.
func mergeRequestListed(listed []mergerequests.Output, projectID, iid int64) bool {
	for _, mr := range listed {
		if mr.ProjectID == projectID && mr.IID == iid {
			return true
		}
	}
	return false
}
