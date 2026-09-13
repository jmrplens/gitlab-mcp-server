//go:build e2e

// groupmilestones_test.go covers the one licensed read of a group's
// milestones, the burndown chart events, which the old suite kept behind
// an enterprise guard in its Community group file, where it never ran. The
// events come from an issue put into the milestone, since a milestone
// nothing was ever assigned to has no chart to draw.

package ee

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// burndownFixture is a group with a milestone and one issue assigned to
// it, in a project of the group.
type burndownFixture struct {
	group     fixture.Group
	milestone fixture.GroupMilestone
}

// buildBurndownFixture creates the group, the project, the milestone and
// the issue, and puts the issue into the milestone through client-go.
func buildBurndownFixture(e *harness.Env) burndownFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("burndown"))
	project := fixture.NewProject(e, fixture.WithNamePrefix("burndown"), fixture.InGroup(group))
	milestone := fixture.NewGroupMilestone(e, group, "sprint")
	issue := fixture.NewIssue(e, project, "burndown fixture")
	if _, _, err := e.Client().GL().Issues.UpdateIssue(project.ID, issue.IID, &gl.UpdateIssueOptions{MilestoneID: new(milestone.ID)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("putting issue #%d into milestone %d: %v", issue.IID, milestone.ID, err)
	}
	return burndownFixture{group: group, milestone: milestone}
}

// burndownCreated is the action a burndown chart records for an issue that
// entered the milestone open: the chart is drawn from the issues' own
// creation, closing and reopening, and an open issue contributes its
// creation.
const burndownCreated = "created"

// TestGroupMilestoneBurndown_IssueAssigned_RecordsAnEvent reads the
// burndown chart events of the fixture milestone on every surface, waiting
// for GitLab to record the assignment, and checks the first event is the
// creation of the open issue the milestone holds.
//
// Replaces: TestMeta_GroupDeep
func TestGroupMilestoneBurndown_IssueAssigned_RecordsAnEvent(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildBurndownFixture, func(e *harness.Env, surface harness.Surface, f burndownFixture) {
		s := e.On(surface)
		events := harness.Eventually(s, actionGroupMilestoneBurndown,
			map[string]any{"group_id": f.group.IDParam(), "milestone_iid": f.milestone.IID}, resourceEventInterval, resourceEventWait,
			func(out groupmilestones.BurndownChartEventsOutput) bool { return len(out.Events) > 0 })
		if events.Events[0].Action != burndownCreated {
			e.T.Errorf("the first burndown event is %q, want the creation of the open issue: %+v", events.Events[0].Action, events.Events)
		}
	})
}
