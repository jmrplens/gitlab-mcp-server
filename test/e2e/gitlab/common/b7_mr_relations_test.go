//go:build e2e

// b7_mr_relations_test.go covers the two reads that answer with what a merge
// request points at rather than with the request itself: the issues its merge
// would close, and the reviewers assigned to it.
//
// The closing reference is written into the description through the server and
// then waited for, because GitLab reads the reference out of the description
// in a background job and answers the closes-issues endpoint with nothing
// until it has. The budget is the one the related-issues scenario beside this
// one uses for the same wait.

package common

import (
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMergeRequestRelations_ClosedIssuesAndReviewers points a merge request of
// its own on every surface at an issue of its own, through a closing reference
// in its description, and waits until the closes-issues read answers with that
// issue; then it assigns the caller as a reviewer and checks the reviewers read
// agrees with what the update recorded.
//
// The reviewer half reads the update's own answer before asserting on the
// listing, because an instance that declined to keep the reviewer would
// otherwise be indistinguishable from one that lost it between the two calls:
// the scenario says which of the two happened rather than failing on the
// listing alone.
func TestMergeRequestRelations_ClosedIssuesAndReviewers(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrrelations"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mrrelations")
		params := f.params()
		issue := fixture.NewIssue(e, project, "closed by the merge request")

		description := fmt.Sprintf("Closes #%d", issue.IID)
		described := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate,
			withParams(params, map[string]any{"description": description}))
		if described.IID != f.mr.IID || described.Description != description {
			e.T.Fatalf("merge_request update answered %+v, want request !%d describing %q", described, f.mr.IID, description)
		}

		closes := harness.Eventually(s, actionMergeRequestIssuesClosed, params, relatedIssuesInterval, relatedIssuesWait,
			func(out mergerequests.IssuesClosedOutput) bool { return closedIssueListed(out.Issues, issue.IID) })
		e.T.Logf("merging the request would close %d issue(s), #%d among them", len(closes.Issues), issue.IID)

		reviewer := e.Runtime().UserID
		reviewed := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate,
			withParams(params, map[string]any{"reviewer_ids": []int64{reviewer}}))
		listed := harness.Do[mergerequests.ReviewersOutput](s, actionMergeRequestReviewers, params)

		// Held unconditionally on purpose. A branch that accepted an empty
		// answer would credit both actions on a run where the update silently
		// dropped the reviewer, which is the defect this is here to catch;
		// Community Edition lets the author review their own request and the
		// caller owns the project, so the reviewer is expected to stick.
		if !containsID(basicUserIDs(reviewed.Reviewers), reviewer) {
			e.T.Errorf("the update answered the reviewers %v, want user %d among them", basicUserIDs(reviewed.Reviewers), reviewer)
		}
		if !containsID(reviewerIDs(listed.Reviewers), reviewer) {
			e.T.Errorf("the request's reviewers do not hold user %d: %v", reviewer, reviewerIDs(listed.Reviewers))
		}
	})
}

// closedIssueListed reports whether a closes-issues answer holds the issue.
func closedIssueListed(listed []issues.BasicOutput, iid int64) bool {
	for _, issue := range listed {
		if issue.IID == iid {
			return true
		}
	}
	return false
}

// reviewerIDs lists the user IDs of a reviewers listing.
// basicUserIDs reads the identifiers out of the user objects a merge request
// carries, which is a different type from the one the reviewers listing
// answers with: the request embeds the shared basic-user shape and the listing
// has a reviewer shape of its own.
func basicUserIDs(users []*toolutil.BasicUserOutput) []int64 {
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		if user != nil {
			ids = append(ids, user.ID)
		}
	}
	return ids
}

func reviewerIDs(reviewers []mergerequests.ReviewerOutput) []int64 {
	ids := make([]int64, 0, len(reviewers))
	for _, reviewer := range reviewers {
		ids = append(ids, reviewer.ID)
	}
	return ids
}
