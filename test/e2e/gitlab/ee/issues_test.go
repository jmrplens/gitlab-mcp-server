//go:build e2e

// issues_test.go covers the licensed reads of the issue group: an issue's
// weight events, the iterations of a project and of its group, and an
// issue's iteration events.
//
// Every one of them stands on state GitLab records asynchronously, so the
// first read of each waits; and the iteration reads stand on a fixture
// GitLab offers no REST API to build, which the fixture library makes over
// GraphQL. These are the reads the old suite kept behind an enterprise
// guard in its Community issue file, where they never ran.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The resource event waits: GitLab records an event a moment after the
// change, and a loaded Docker instance takes longer.
const (
	resourceEventInterval = 2 * time.Second
	resourceEventWait     = 60 * time.Second
)

// The two weights the fixture issue is given, so at least two events are
// recorded whether or not GitLab records one for the first assignment.
const (
	firstWeight = int64(3)
	finalWeight = int64(5)
)

// weightFixture is a project with an issue whose weight was changed twice.
type weightFixture struct {
	project fixture.Project
	issue   fixture.Issue
}

// buildWeightFixture creates the project and the issue and changes the
// weight twice through client-go.
func buildWeightFixture(e *harness.Env) weightFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("weight"))
	issue := fixture.NewIssue(e, project, "weight events fixture")
	for _, weight := range []int64{firstWeight, finalWeight} {
		if _, _, err := e.Client().GL().Issues.UpdateIssue(project.ID, issue.IID, &gl.UpdateIssueOptions{Weight: new(weight)}, gl.WithContext(e.Ctx)); err != nil {
			e.T.Fatalf("setting the weight of issue #%d to %d: %v", issue.IID, weight, err)
		}
	}
	return weightFixture{project: project, issue: issue}
}

// TestIssueWeightEvents_TwoChanges_ListTheFinalWeight lists the weight
// events of the fixture issue on every surface, waiting for GitLab to
// record them, and checks that the final weight is among them.
//
// Replaces: TestMeta_IssueWeightEvents
func TestIssueWeightEvents_TwoChanges_ListTheFinalWeight(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildWeightFixture, func(e *harness.Env, surface harness.Surface, f weightFixture) {
		s := e.On(surface)

		// The events are written asynchronously and one at a time, so the
		// wait is for the final weight to appear, not for the first event:
		// a listing that holds only the first change is still on its way.
		events := harness.Eventually(s, actionIssueWeightEventList,
			map[string]any{"project_id": f.project.IDParam(), "issue_iid": f.issue.IID}, resourceEventInterval, resourceEventWait,
			func(out resourceevents.ListWeightEventsOutput) bool {
				for _, event := range out.Events {
					if event.Weight == finalWeight {
						return true
					}
				}
				return false
			})
		for _, event := range events.Events {
			if event.ID == 0 {
				e.T.Errorf("a weight event has no ID: %+v", event)
			}
		}
	})
}

// iterationFixture is a group with one iteration, a project in the group,
// and an issue of that project put into the iteration.
type iterationFixture struct {
	group     fixture.Group
	iteration fixture.Iteration
	project   fixture.Project
	issue     fixture.Issue
}

// buildIterationFixture creates all four.
func buildIterationFixture(e *harness.Env) iterationFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("iter"))
	iteration := fixture.NewIteration(e, group)
	project := fixture.NewProject(e, fixture.WithNamePrefix("iter"), fixture.InGroup(group))
	issue := fixture.NewIssue(e, project, "iteration fixture")
	fixture.AssignIssueIteration(e, project, issue, iteration)
	return iterationFixture{group: group, iteration: iteration, project: project, issue: issue}
}

// TestIterations_GroupAndProject_ListAndRecordTheIssueEvent lists the
// iterations of the fixture's project and of its group on every surface,
// finding the one the fixture created, then lists the issue's iteration
// events and reads the first back.
//
// Replaces: TestMeta_IssuesDeep, TestMeta_ProjectIterations
func TestIterations_GroupAndProject_ListAndRecordTheIssueEvent(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildIterationFixture, func(e *harness.Env, surface harness.Surface, f iterationFixture) {
		s := e.On(surface)

		ofProject := harness.Do[projectiterations.ListOutput](s, actionIterationListProject, map[string]any{"project_id": f.project.IDParam(), "state": "all"})
		if !iterationListed(ofProject.Iterations, f.iteration.IID) {
			e.T.Errorf("the project's iterations do not hold %s: %+v", f.iteration, ofProject.Iterations)
		}
		ofGroup := harness.Do[groupiterations.ListOutput](s, actionIterationListGroup, map[string]any{"group_id": f.group.IDParam(), "state": "all"})
		if !iterationListed(ofGroup.Iterations, f.iteration.IID) {
			e.T.Errorf("the group's iterations do not hold %s: %+v", f.iteration, ofGroup.Iterations)
		}

		params := map[string]any{"project_id": f.project.IDParam(), "issue_iid": f.issue.IID}
		events := harness.Eventually(s, actionIssueIterationEventList, params, resourceEventInterval, resourceEventWait,
			func(out resourceevents.ListIterationEventsOutput) bool { return len(out.Events) > 0 })
		got := harness.Do[resourceevents.IterationEventOutput](s, actionIssueIterationEventGet,
			withParams(params, map[string]any{"iteration_event_id": events.Events[0].ID}))
		if got.ID != events.Events[0].ID {
			e.T.Errorf("event_issue_iteration_get answered event %d, want the listed %d", got.ID, events.Events[0].ID)
		}
	})
}

// iterationListed reports whether an iteration listing holds one by iid.
// Both listings alias the one iteration shape, so both are read through it.
func iterationListed(iterations []iterationdata.Output, iid int64) bool {
	for _, iteration := range iterations {
		if iteration.IID == iid {
			return true
		}
	}
	return false
}
