//go:build e2e

// b7_mr_events_test.go covers the three kinds of resource event a merge
// request records — a label added, a milestone set, the state changed — each
// as the listing and as the singular read of one event by its ID.
//
// The listing has to run first in every case, because the ID the singular read
// takes exists nowhere else: GitLab mints it when it records the event, and a
// caller learns it only from the listing. The scenario therefore causes each
// event through the update action, finds it in the listing, and reads that one
// event back.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// mergeRequestResourceType is what GitLab names as the resource a merge
// request's events belong to, and the resource_id beside it is the request's
// instance-wide ID rather than its project-scoped IID.
const mergeRequestResourceType = "MergeRequest"

// TestMergeRequestEvents_LabelMilestoneAndState_ListAndReadOne adds a label to
// a merge request of its own on every surface, sets a milestone on it and
// closes it, then for each of the three events reads the listing until the
// event it caused is there and reads that one event back by its ID, holding
// the singular answer to the same event the listing named and to the request
// it belongs to.
func TestMergeRequestEvents_LabelMilestoneAndState_ListAndReadOne(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrevents"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mrevents")
		params := f.params()
		label := fixture.NewLabel(e, project, "mrevent")
		milestone := fixture.NewMilestone(e, project, "mrevent")

		tagged := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate,
			withParams(params, map[string]any{"add_labels": []string{label.Name}}))
		if !slices.Contains(tagged.Labels, label.Name) {
			e.T.Fatalf("merge_request update answered labels %v, want %q among them", tagged.Labels, label.Name)
		}

		scheduled := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate,
			withParams(params, map[string]any{"milestone_id": milestone.ID}))
		if scheduled.Milestone == nil || scheduled.Milestone.ID != milestone.ID {
			e.T.Fatalf("merge_request update answered milestone %+v, want milestone %d", scheduled.Milestone, milestone.ID)
		}

		closed := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate,
			withParams(params, map[string]any{"state_event": "close"}))
		if closed.State != "closed" {
			e.T.Fatalf("merge_request update answered %+v, want request !%d closed", closed, f.mr.IID)
		}

		labelEvents := harness.Eventually(s, actionMergeRequestLabelEventList, params, stateEventInterval, stateEventWait,
			func(out resourceevents.ListLabelEventsOutput) bool {
				_, found := labelEventFor(out.Events, label.Name)
				return found
			})
		listedLabelEvent, _ := labelEventFor(labelEvents.Events, label.Name)
		gotLabelEvent := harness.Do[resourceevents.LabelEventOutput](s, actionMergeRequestLabelEventGet,
			withParams(params, map[string]any{"label_event_id": listedLabelEvent.ID}))
		if gotLabelEvent.ID != listedLabelEvent.ID || gotLabelEvent.Label == nil || gotLabelEvent.Label.Name != label.Name {
			e.T.Errorf("label event get answered %+v, want event %d naming the label %q", gotLabelEvent, listedLabelEvent.ID, label.Name)
		}
		assertEventBelongsTo(e, "label event", gotLabelEvent.ResourceType, gotLabelEvent.ResourceID, f.mr.ID)

		milestoneEvents := harness.Eventually(s, actionMergeRequestMilestoneEventList, params, stateEventInterval, stateEventWait,
			func(out resourceevents.ListMilestoneEventsOutput) bool {
				_, found := milestoneEventFor(out.Events, milestone.ID)
				return found
			})
		listedMilestoneEvent, _ := milestoneEventFor(milestoneEvents.Events, milestone.ID)
		gotMilestoneEvent := harness.Do[resourceevents.MilestoneEventOutput](s, actionMergeRequestMilestoneEventGet,
			withParams(params, map[string]any{"milestone_event_id": listedMilestoneEvent.ID}))
		if gotMilestoneEvent.ID != listedMilestoneEvent.ID || gotMilestoneEvent.Milestone == nil || gotMilestoneEvent.Milestone.ID != milestone.ID {
			e.T.Errorf("milestone event get answered %+v, want event %d naming milestone %d", gotMilestoneEvent, listedMilestoneEvent.ID, milestone.ID)
		}
		assertEventBelongsTo(e, "milestone event", gotMilestoneEvent.ResourceType, gotMilestoneEvent.ResourceID, f.mr.ID)

		stateEvents := harness.Eventually(s, actionMergeRequestStateEventList, params, stateEventInterval, stateEventWait,
			func(out resourceevents.ListStateEventsOutput) bool {
				_, found := stateEventFor(out.Events, "closed")
				return found
			})
		listedStateEvent, _ := stateEventFor(stateEvents.Events, "closed")
		gotStateEvent := harness.Do[resourceevents.StateEventOutput](s, actionMergeRequestStateEventGet,
			withParams(params, map[string]any{"state_event_id": listedStateEvent.ID}))
		if gotStateEvent.ID != listedStateEvent.ID || gotStateEvent.State != "closed" {
			e.T.Errorf("state event get answered %+v, want event %d recording the close", gotStateEvent, listedStateEvent.ID)
		}
		assertEventBelongsTo(e, "state event", gotStateEvent.ResourceType, gotStateEvent.ResourceID, f.mr.ID)
	})
}

// assertEventBelongsTo checks that a singular resource event names the merge
// request it was read from. The resource is identified by the request's
// instance-wide ID, not by the IID the call was addressed with, which is what
// makes this worth asserting: the two differ, and an event carrying the wrong
// one would still look plausible.
func assertEventBelongsTo(e *harness.Env, what, resourceType string, resourceID, wantID int64) {
	e.T.Helper()
	if resourceType != mergeRequestResourceType || resourceID != wantID {
		e.T.Errorf("the %s belongs to %s %d, want %s %d", what, resourceType, resourceID, mergeRequestResourceType, wantID)
	}
}

// labelEventFor returns the label event that names the given label, and
// whether the listing holds one.
func labelEventFor(events []resourceevents.LabelEventOutput, name string) (resourceevents.LabelEventOutput, bool) {
	for _, event := range events {
		if event.ID != 0 && event.Label != nil && event.Label.Name == name {
			return event, true
		}
	}
	return resourceevents.LabelEventOutput{}, false
}

// milestoneEventFor returns the milestone event that names the given
// milestone, and whether the listing holds one.
func milestoneEventFor(events []resourceevents.MilestoneEventOutput, milestoneID int64) (resourceevents.MilestoneEventOutput, bool) {
	for _, event := range events {
		if event.ID != 0 && event.Milestone != nil && event.Milestone.ID == milestoneID {
			return event, true
		}
	}
	return resourceevents.MilestoneEventOutput{}, false
}

// stateEventFor returns the state event recording the given state, and whether
// the listing holds one. It is the half of stateEventRecorded that a singular
// read needs: the ID, rather than only the fact that an event exists.
func stateEventFor(events []resourceevents.StateEventOutput, state string) (resourceevents.StateEventOutput, bool) {
	for _, event := range events {
		if event.ID != 0 && event.State == state {
			return event, true
		}
	}
	return resourceevents.StateEventOutput{}, false
}
