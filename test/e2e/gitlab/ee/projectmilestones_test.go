//go:build e2e

// projectmilestones_test.go covers the one licensed read of a project's
// milestones, the burndown chart events. GitLab draws a burndown only for a
// milestone with both a start date and a due date, and only from the issues
// put into it, so the fixture gives the milestone both dates and one issue.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// projectMilestoneLength is how long the fixture milestone runs from today,
// so its due date lies ahead and GitLab draws the chart up to today.
const projectMilestoneLength = 14 * 24 * time.Hour

// projectBurndownFixture is a project with a dated milestone and one issue
// assigned to it.
type projectBurndownFixture struct {
	project      fixture.Project
	milestoneIID int64
}

// buildProjectBurndownFixture creates the project, the milestone with its
// dates and the issue, and puts the issue into the milestone through
// client-go.
func buildProjectBurndownFixture(e *harness.Env) projectBurndownFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("projburndown"))
	start := time.Now().UTC()
	startDate, dueDate := gl.ISOTime(start), gl.ISOTime(start.Add(projectMilestoneLength))
	milestone, _, err := e.Client().GL().Milestones.CreateMilestone(project.ID, &gl.CreateMilestoneOptions{
		Title:     new(e.Name("sprint")),
		StartDate: &startDate,
		DueDate:   &dueDate,
	}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("creating a dated milestone in project %d: %v", project.ID, err)
	}
	issue := fixture.NewIssue(e, project, "burndown fixture")
	if _, _, err = e.Client().GL().Issues.UpdateIssue(project.ID, issue.IID, &gl.UpdateIssueOptions{MilestoneID: new(milestone.ID)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("putting issue #%d into milestone %d: %v", issue.IID, milestone.ID, err)
	}
	return projectBurndownFixture{project: project, milestoneIID: milestone.IID}
}

// TestProjectMilestoneBurndown_IssueAssigned_RecordsAnEvent reads the
// burndown chart events of the fixture milestone on every surface, waiting
// for GitLab to record the assignment, and checks the first event is the
// creation of the open issue the milestone holds.
func TestProjectMilestoneBurndown_IssueAssigned_RecordsAnEvent(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildProjectBurndownFixture, func(e *harness.Env, surface harness.Surface, f projectBurndownFixture) {
		s := e.On(surface)
		events := harness.Eventually(s, actionProjectMilestoneBurndown,
			map[string]any{"project_id": f.project.IDParam(), "milestone_iid": f.milestoneIID}, resourceEventInterval, resourceEventWait,
			func(out milestones.BurndownChartEventsOutput) bool { return len(out.Events) > 0 })
		if events.Events[0].Action != burndownCreated {
			e.T.Errorf("the first burndown event is %q, want the creation of the open issue: %+v", events.Events[0].Action, events.Events)
		}
	})
}
