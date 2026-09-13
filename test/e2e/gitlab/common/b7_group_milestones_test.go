//go:build e2e

// b7_group_milestones_test.go covers a group milestone's update and the two
// listings scoped to it: the issues that carry it and the merge requests that
// carry it.
//
// groupmilestones_test.go drives the create, list, get and delete. The update
// was never driven here, and the two listings were reached only by a read
// sweep asking for a milestone that is not there, which asserts the refusal
// and nothing about what a milestone holds. A group milestone can be given to
// an issue or a merge request of any project in the group, so one project
// under the group supplies both, and each listing is held to the object it
// should hold and to the one it should not.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupMilestoneIssueIIDs lists the iids of a group milestone's issue listing.
func groupMilestoneIssueIIDs(issues []groupmilestones.IssueItem) []int64 {
	iids := make([]int64, 0, len(issues))
	for _, issue := range issues {
		iids = append(iids, issue.IID)
	}
	return iids
}

// groupMilestoneMergeRequestIIDs lists the iids of a group milestone's merge
// request listing.
func groupMilestoneMergeRequestIIDs(mergeRequests []groupmilestones.MergeRequestItem) []int64 {
	iids := make([]int64, 0, len(mergeRequests))
	for _, mergeRequest := range mergeRequests {
		iids = append(iids, mergeRequest.IID)
	}
	return iids
}

// groupMilestoneTracker is a group milestone with one issue and one merge
// request given to it, both in one project under the group.
type groupMilestoneTracker struct {
	group        fixture.Group
	milestone    fixture.GroupMilestone
	issue        fixture.Issue
	mergeRequest fixture.MergeRequest
}

// TestGroupMilestones_Tracker_UpdateAndListWhatCarriesIt updates a group
// milestone's description and reads the issues and the merge requests given
// to it.
//
// It runs on the dynamic, meta and individual surfaces against one milestone
// built for all three, since a milestone carrying an issue and a merge
// request costs a project, a branch and a commit to build and each surface
// only reads it and rewrites its description. It asserts that the update
// answers the same milestone iid carrying the description that surface sent,
// that the issue listing holds the issue given the milestone, and that the
// merge request listing holds the merge request given it.
func TestGroupMilestones_Tracker_UpdateAndListWhatCarriesIt(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) groupMilestoneTracker {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("mstracker"))
		project := fixture.NewProject(e, fixture.InGroup(group), fixture.WithNamePrefix("mstracker"))
		milestone := fixture.NewGroupMilestone(e, group, "tracked")

		issue := fixture.NewIssue(e, project, e.Name("issue"))
		assignMilestone(e, project, issue, milestone.ID)
		opened := newMergeRequestIn(e, project, "mstracker")
		assignMergeRequestMilestone(e, project, opened.mr, milestone.ID)

		return groupMilestoneTracker{group: group, milestone: milestone, issue: issue, mergeRequest: opened.mr}
	}, func(e *harness.Env, surface harness.Surface, f groupMilestoneTracker) {
		s := e.On(surface)
		params := map[string]any{"group_id": f.group.IDParam(), "milestone_iid": f.milestone.IID}
		description := "tracked by the " + string(surface) + " surface"

		updated := harness.Do[groupmilestones.Output](s, actionGroupMilestoneUpdate, withParams(params, map[string]any{"description": description}))
		if updated.IID != f.milestone.IID || updated.Description != description {
			e.T.Errorf("group_milestone_update answered %+v, want milestone %d described as %q", updated, f.milestone.IID, description)
		}

		issues := harness.Do[groupmilestones.IssuesOutput](s, actionGroupMilestoneIssues, params)
		if !containsID(groupMilestoneIssueIIDs(issues.Issues), f.issue.IID) {
			e.T.Errorf("the milestone's issues %v do not hold issue %d", groupMilestoneIssueIIDs(issues.Issues), f.issue.IID)
		}

		mergeRequests := harness.Do[groupmilestones.MergeRequestsOutput](s, actionGroupMilestoneMergeRequests, params)
		if !containsID(groupMilestoneMergeRequestIIDs(mergeRequests.MergeRequests), f.mergeRequest.IID) {
			e.T.Errorf("the milestone's merge requests %v do not hold merge request %d",
				groupMilestoneMergeRequestIIDs(mergeRequests.MergeRequests), f.mergeRequest.IID)
		}
	})
}

// assignMergeRequestMilestone gives a merge request a milestone through
// client-go, which is fixture plumbing rather than the subject here. It is
// the merge request twin of assignMilestone.
func assignMergeRequestMilestone(e *harness.Env, project fixture.Project, mergeRequest fixture.MergeRequest, milestoneID int64) {
	e.T.Helper()
	_, _, err := e.Client().GL().MergeRequests.UpdateMergeRequest(project.ID, mergeRequest.IID,
		&gl.UpdateMergeRequestOptions{MilestoneID: new(milestoneID)}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("giving merge request %d milestone %d through client-go: %v", mergeRequest.IID, milestoneID, err)
	}
}
