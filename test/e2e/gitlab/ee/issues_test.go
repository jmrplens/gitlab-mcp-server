// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// issues_test.go covers the licensed reads of the issue group: an issue's
// weight events, the iterations of a project and of its group, and an
// issue's iteration events; and the one licensed write beside them, a work
// item update removing the iteration and the health status.
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
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/workitems"
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
					if event.Weight != nil && *event.Weight == finalWeight {
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

// TestIssue_Update_WeightZeroIsAWeight gives an issue of its own on every
// surface a weight of 3 through the update action, then sends a weight of 0
// as the only change and reads 0 back. The weight was a plain integer behind
// a "> 0" guard, so 0 sent an empty body, which GitLab refuses with "at least
// one parameter must be provided", and beside other fields it was dropped in
// silence (issue 1297).
func TestIssue_Update_WeightZeroIsAWeight(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("weightzero"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		issue := fixture.NewIssue(e, project, e.Name("weightzero"))
		params := map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID}

		set := harness.Do[issues.Output](s, actionIssueUpdate, withParams(params, map[string]any{"weight": 3}))
		if set.Weight != 3 {
			e.T.Fatalf("issue update answered weight %d, want 3 before the zero", set.Weight)
		}
		zeroed := harness.Do[issues.Output](s, actionIssueUpdate, withParams(params, map[string]any{"weight": 0}))
		read := harness.Do[issues.Output](s, actionIssueGet, params)
		if zeroed.Weight != 0 || read.Weight != 0 {
			e.T.Errorf("after a weight of 0 the update answered %d and the read %d, want 0", zeroed.Weight, read.Weight)
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

// TestWorkItems_Update_RemovesIterationAndHealthStatus puts the fixture's
// issue in the fixture's iteration and gives it a health status on every
// surface, then removes both in one work item update, 0 for the iteration and
// "" for the health status, and holds the answer to neither (issue 1303).
// The issue is put back in the iteration at the start of each surface,
// because the surface before it removed it and the fixture is built once.
//
// Before the fix 0 reached GitLab as gid://gitlab/Iteration/0, refused as
// "No object found", and "" sent nothing, so the health status stayed.
func TestWorkItems_Update_RemovesIterationAndHealthStatus(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildIterationFixture, func(e *harness.Env, surface harness.Surface, f iterationFixture) {
		s := e.On(surface)
		fixture.AssignIssueIteration(e, f.project, f.issue, f.iteration)
		item := map[string]any{"full_path": f.project.Path, "work_item_iid": f.issue.IID}

		rated := harness.Do[workitems.GetOutput](s, actionWorkItemUpdate, withParams(item, map[string]any{"health_status": "atRisk"}))
		if rated.WorkItem.HealthStatus != "atRisk" || rated.WorkItem.IterationID == 0 {
			e.T.Fatalf("work_item_update answered health %q and iteration %d, want atRisk in the fixture's %s",
				rated.WorkItem.HealthStatus, rated.WorkItem.IterationID, f.iteration)
		}

		cleared := harness.Do[workitems.GetOutput](s, actionWorkItemUpdate,
			withParams(item, map[string]any{"iteration_id": 0, "health_status": ""}))
		if cleared.WorkItem.IterationID != 0 {
			e.T.Errorf("work_item_update answered the issue still in iteration %d, want none", cleared.WorkItem.IterationID)
		}
		if cleared.WorkItem.HealthStatus != "" {
			e.T.Errorf("work_item_update answered the health status %q, want none", cleared.WorkItem.HealthStatus)
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
