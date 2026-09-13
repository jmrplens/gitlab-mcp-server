//go:build e2e

// b7_issue_events_test.go covers the resource events an issue records, both
// halves of each: the listing, and the single event read by the id the
// listing gave. The old CE suite asserted the label and milestone listings
// and all three singular reads; the rebuilt suite reached the two listings
// from a sweep and the singular reads from nothing.
//
// Every event needs a change to record: a label added, a milestone set and
// the issue closed, each made through the server and asserted on the way, so
// the listings below are read against changes this test knows it made.
//
// The listings are polled rather than read once for the reason
// issues_lifecycle_test.go gives about state events: GitLab records the event
// with the change, and a loaded instance answers the listing a moment later.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueEventsFixture is a project with the label and the milestone the
// changes below move onto an issue. All three surfaces share them; the issue
// each change is made to is the surface's own.
type issueEventsFixture struct {
	project   fixture.Project
	label     fixture.Label
	milestone fixture.Milestone
}

// TestIssueResourceEvents_LabelMilestoneAndState_ListThenReadOne adds a
// label to an issue of its own on every surface, moves it onto a milestone
// and closes it, then reads each of the three event listings and the single
// event the listing named.
func TestIssueResourceEvents_LabelMilestoneAndState_ListThenReadOne(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueEventsFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("issueevents"))
		return issueEventsFixture{
			project:   project,
			label:     fixture.NewLabel(e, project, "eventlabel"),
			milestone: fixture.NewMilestone(e, project, "eventmilestone"),
		}
	}, func(e *harness.Env, surface harness.Surface, f issueEventsFixture) {
		s := e.On(surface)
		issue := fixture.NewIssue(e, f.project, "resource event fixture for the "+string(surface)+" surface")
		params := map[string]any{"project_id": f.project.IDParam(), "issue_iid": issue.IID}

		makeIssueEvents(e, s, f, params)
		assertIssueLabelEvent(e, s, f, params)
		assertIssueMilestoneEvent(e, s, f, params)
		assertIssueStateEvent(e, s, params)
	})
}

// makeIssueEvents makes the three changes the listings below read back: the
// label, the milestone and the close, each asserted on the answer that made
// it so a listing that comes back empty is not blamed on a change that never
// happened.
func makeIssueEvents(e *harness.Env, s *harness.Session, f issueEventsFixture, params map[string]any) {
	e.T.Helper()

	labeled := harness.Do[issues.Output](s, actionIssueUpdate,
		withParams(params, map[string]any{"add_labels": []string{f.label.Name}}))
	if !slices.Contains(labeled.Labels, f.label.Name) {
		e.T.Fatalf("the update answered labels %v, want %q among them", labeled.Labels, f.label.Name)
	}

	milestoned := harness.Do[issues.Output](s, actionIssueUpdate,
		withParams(params, map[string]any{"milestone_id": f.milestone.ID}))
	if milestoned.Milestone == nil || milestoned.Milestone.ID != f.milestone.ID {
		e.T.Fatalf("the update answered milestone %+v, want milestone %d", milestoned.Milestone, f.milestone.ID)
	}

	closed := harness.Do[issues.Output](s, actionIssueUpdate,
		withParams(params, map[string]any{"state_event": "close"}))
	if closed.State != "closed" {
		e.T.Fatalf("the update answered state %q, want the issue closed", closed.State)
	}
}

// assertIssueLabelEvent reads the label events of the issue and then the one
// the listing named, holding the single read to the label that was added.
func assertIssueLabelEvent(e *harness.Env, s *harness.Session, f issueEventsFixture, params map[string]any) {
	e.T.Helper()

	listed := harness.Eventually(s, actionIssueLabelEventList, params, stateEventInterval, stateEventWait,
		func(out resourceevents.ListLabelEventsOutput) bool {
			return len(out.Events) > 0 && out.Events[0].ID != 0
		})
	first := listed.Events[0]
	if first.Label == nil || first.Label.Name != f.label.Name || first.Action != "add" {
		e.T.Errorf("the label events hold %+v first, want the %q label added", first, f.label.Name)
	}

	got := harness.Do[resourceevents.LabelEventOutput](s, actionIssueLabelEventGet,
		withParams(params, map[string]any{"label_event_id": first.ID}))
	if got.ID != first.ID || got.Label == nil || got.Label.Name != f.label.Name {
		e.T.Errorf("label_event_get answered %+v, want event %d carrying the %q label", got, first.ID, f.label.Name)
	}
}

// assertIssueMilestoneEvent reads the milestone events of the issue and then
// the one the listing named, holding the single read to the milestone the
// issue was moved onto.
func assertIssueMilestoneEvent(e *harness.Env, s *harness.Session, f issueEventsFixture, params map[string]any) {
	e.T.Helper()

	listed := harness.Eventually(s, actionIssueMilestoneEventList, params, stateEventInterval, stateEventWait,
		func(out resourceevents.ListMilestoneEventsOutput) bool {
			return len(out.Events) > 0 && out.Events[0].ID != 0
		})
	first := listed.Events[0]
	if first.Milestone == nil || first.Milestone.ID != f.milestone.ID || first.Action != "add" {
		e.T.Errorf("the milestone events hold %+v first, want milestone %d added", first, f.milestone.ID)
	}

	got := harness.Do[resourceevents.MilestoneEventOutput](s, actionIssueMilestoneEventGet,
		withParams(params, map[string]any{"milestone_event_id": first.ID}))
	if got.ID != first.ID || got.Milestone == nil || got.Milestone.ID != f.milestone.ID {
		e.T.Errorf("milestone_event_get answered %+v, want event %d carrying milestone %d", got, first.ID, f.milestone.ID)
	}
}

// assertIssueStateEvent reads the state events of the issue and then the one
// the listing named, holding the single read to the close that recorded it.
func assertIssueStateEvent(e *harness.Env, s *harness.Session, params map[string]any) {
	e.T.Helper()

	listed := harness.Eventually(s, actionIssueStateEventList, params, stateEventInterval, stateEventWait,
		func(out resourceevents.ListStateEventsOutput) bool {
			return stateEventRecorded(out.Events, "closed")
		})
	var closed resourceevents.StateEventOutput
	for _, event := range listed.Events {
		if event.State == "closed" && event.ID != 0 {
			closed = event
			break
		}
	}

	got := harness.Do[resourceevents.StateEventOutput](s, actionIssueStateEventGet,
		withParams(params, map[string]any{"state_event_id": closed.ID}))
	if got.ID != closed.ID || got.State != "closed" {
		e.T.Errorf("state_event_get answered %+v, want event %d recording the close", got, closed.ID)
	}
}
