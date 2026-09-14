//go:build e2e

// groupmilestones_test.go covers a group milestone through its life on
// every surface: created, listed, read by its iid, and deleted.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupMilestoneIIDs lists the iids of a group milestone listing.
func groupMilestoneIIDs(listed []groupmilestones.Output) []int64 {
	iids := make([]int64, 0, len(listed))
	for _, milestone := range listed {
		iids = append(iids, milestone.IID)
	}
	return iids
}

// TestGroupMilestones_Lifecycle_CreateListGetDelete creates a milestone in
// a group of each surface's own, finds it in the listing, reads it by its
// iid, deletes it and checks the listing lets it go.
//
// Replaces: TestMeta_GroupMilestones
func TestGroupMilestones_Lifecycle_CreateListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("milestones"))
		params := map[string]any{"group_id": group.IDParam()}
		title := e.Name("milestone")

		created := harness.Do[groupmilestones.Output](s, actionGroupMilestoneCreate, withParams(params, map[string]any{"title": title}))
		if created.IID == 0 || created.Title != title || created.GroupID != group.ID {
			e.T.Fatalf("group_milestone_create answered %+v, want the milestone %q of group %d with an iid", created, title, group.ID)
		}
		milestone := withParams(params, map[string]any{"milestone_iid": created.IID})

		listed := harness.Do[groupmilestones.ListOutput](s, actionGroupMilestoneList, params)
		if !containsID(groupMilestoneIIDs(listed.Milestones), created.IID) {
			e.T.Errorf("the group lists the milestones %v, want %d among them", groupMilestoneIIDs(listed.Milestones), created.IID)
		}
		got := harness.Do[groupmilestones.Output](s, actionGroupMilestoneGet, milestone)
		if got.IID != created.IID || got.Title != title {
			e.T.Errorf("group_milestone_get answered milestone %d %q, want %d %q", got.IID, got.Title, created.IID, title)
		}

		harness.DoVoid(s, actionGroupMilestoneDelete, milestone)
		remaining := harness.Do[groupmilestones.ListOutput](s, actionGroupMilestoneList, params)
		if containsID(groupMilestoneIIDs(remaining.Milestones), created.IID) {
			e.T.Errorf("the group still lists milestone %d after its delete", created.IID)
		}
	})
}
