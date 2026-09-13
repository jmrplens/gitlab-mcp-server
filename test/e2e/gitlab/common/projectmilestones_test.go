//go:build e2e

// projectmilestones_test.go covers the two listings scoped to a project
// milestone, the issues and the merge requests that carry it, beside the
// milestone listing itself. The old suite asked both listings of an empty
// milestone and asserted nothing about their answers; here an issue is
// given the milestone first, so the issue listing has something to hold
// and the merge request listing something to leave out.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// milestoneIIDs lists the iids of a milestone listing.
func milestoneIIDs(listed []milestones.Output) []int64 {
	iids := make([]int64, 0, len(listed))
	for _, milestone := range listed {
		iids = append(iids, milestone.IID)
	}
	return iids
}

// milestoneIssueIIDs lists the iids of a milestone's issue listing.
func milestoneIssueIIDs(issues []milestones.IssueItem) []int64 {
	iids := make([]int64, 0, len(issues))
	for _, issue := range issues {
		iids = append(iids, issue.IID)
	}
	return iids
}

// TestProjectMilestones_IssuesAndMergeRequests_ScopedToTheMilestone creates
// a milestone in a project of each surface's own, gives an issue to it,
// and reads the milestone listing, the issues of the milestone and the
// merge requests of the milestone.
//
// Replaces: TestMeta_ProjectMilestonesDeep
func TestProjectMilestones_IssuesAndMergeRequests_ScopedToTheMilestone(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("milestones"))
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("milestone")

		created := harness.Do[milestones.Output](s, actionProjectMilestoneCreate, withParams(params, map[string]any{"title": title}))
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("milestone_create answered %+v, want the milestone %q with an iid", created, title)
		}
		milestone := withParams(params, map[string]any{"milestone_iid": created.IID})

		listed := harness.Do[milestones.ListOutput](s, actionProjectMilestoneList, params)
		if !containsID(milestoneIIDs(listed.Milestones), created.IID) {
			e.T.Errorf("the project lists the milestones %v, want %d among them", milestoneIIDs(listed.Milestones), created.IID)
		}

		issue := fixture.NewIssue(e, project, e.Name("issue"))
		assignMilestone(e, project, issue, created.ID)
		issues := harness.Do[milestones.MilestoneIssuesOutput](s, actionProjectMilestoneIssues, milestone)
		if !containsID(milestoneIssueIIDs(issues.Issues), issue.IID) {
			e.T.Errorf("the milestone's issues %v do not hold issue %d", milestoneIssueIIDs(issues.Issues), issue.IID)
		}
		mergeRequests := harness.Do[milestones.MilestoneMergeRequestsOutput](s, actionProjectMilestoneMergeRequests, milestone)
		if len(mergeRequests.MergeRequests) != 0 {
			e.T.Errorf("the milestone lists %d merge request(s), and none was given it", len(mergeRequests.MergeRequests))
		}
	})
}

// assignMilestone gives an issue a milestone through client-go, which is
// fixture plumbing rather than the subject here.
func assignMilestone(e *harness.Env, project fixture.Project, issue fixture.Issue, milestoneID int64) {
	e.T.Helper()
	_, _, err := e.Client().GL().Issues.UpdateIssue(project.ID, issue.IID, &gl.UpdateIssueOptions{MilestoneID: new(milestoneID)}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("giving issue %d milestone %d through client-go: %v", issue.IID, milestoneID, err)
	}
}
