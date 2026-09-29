//go:build e2e

// b7_issue_relations_test.go covers what an issue answers about the work
// around it: who is taking part in it, and the merge requests that close it
// or merely mention it. The old CE suite asserted all three and the rebuilt
// suite reached them only from the read sweep, which asserts nothing.
//
// The two merge request listings are polled rather than read once: GitLab
// records the reference between a merge request and the issue its
// description names in a background job, so the listing is right a moment
// after the merge request is opened rather than in the same breath.

package common

import (
	"fmt"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// relatedMRIIDs lists the iids of a related merge request listing.
func relatedMRIIDs(listed []issues.RelatedMROutput) []int64 {
	iids := make([]int64, 0, len(listed))
	for _, mr := range listed {
		iids = append(iids, mr.IID)
	}
	return iids
}

// findRelatedMR returns the entry of a related merge request listing with
// the given iid.
func findRelatedMR(listed []issues.RelatedMROutput, iid int64) (issues.RelatedMROutput, bool) {
	for _, mr := range listed {
		if mr.IID == iid {
			return mr, true
		}
	}
	return issues.RelatedMROutput{}, false
}

// assertRelatedMR waits until one of the two issue-to-merge-request
// listings holds the merge request this surface opened, and holds that
// entry to the merge request it is supposed to be.
func assertRelatedMR(e *harness.Env, s *harness.Session, action harness.ActionID, f issueFixture, opened mergerequests.Output) {
	e.T.Helper()

	listed := harness.Eventually(s, action, f.params(), stateEventInterval, stateEventWait,
		func(out issues.RelatedMRsOutput) bool {
			_, found := findRelatedMR(out.MergeRequests, opened.IID)
			return found
		})
	entry, found := findRelatedMR(listed.MergeRequests, opened.IID)
	if !found {
		e.T.Errorf("%s answered %v for issue #%d, want !%d among them",
			action, relatedMRIIDs(listed.MergeRequests), f.issue.IID, opened.IID)
		return
	}
	if entry.ProjectID != f.project.ID || entry.Title != opened.Title || entry.State != "opened" {
		e.T.Errorf("%s answered %+v for !%d, want the open merge request %q of project %d",
			action, entry, opened.IID, opened.Title, f.project.ID)
	}
}

// TestIssueParticipants_Lists_TheAuthorOfTheIssue reads the participants of
// one shared issue on every surface and finds the run's own user among them,
// which is what authoring an issue makes of you.
func TestIssueParticipants_Lists_TheAuthorOfTheIssue(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issueparticipants")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)
		me := e.Runtime()

		got := harness.Do[issues.ParticipantsOutput](s, actionIssueParticipants, f.params())
		if len(got.Participants) == 0 {
			e.T.Fatalf("participants answered nobody for issue #%d, want at least its author", f.issue.IID)
		}
		author := false
		for _, participant := range got.Participants {
			if participant.ID == me.UserID && participant.Username == me.Username {
				author = true
			}
			if participant.ID == 0 || participant.Username == "" {
				e.T.Errorf("participants answered %+v, want every participant named by id and username", participant)
			}
		}
		if !author {
			e.T.Errorf("the participants of issue #%d are %+v, want %s (%d) among them",
				f.issue.IID, got.Participants, me.Username, me.UserID)
		}
	})
}

// TestIssueParticipants_List_PagesOneParticipantAtATime gives one issue two
// participants, its author and a fixture user it is assigned to, and pages
// through them one at a time on every surface. GitLab pages the list although
// the route declares neither page nor per_page, and the action used to read
// the first twenty with no pagination block and no way to ask for more.
func TestIssueParticipants_List_PagesOneParticipantAtATime(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		f := newIssueFixture(e, "issueparticipantpage")
		assignee := fixture.NewUser(e, "issueparticipantpage")
		fixture.AddProjectMember(e, f.project, assignee, gl.DeveloperPermissions)
		fixture.AssignIssue(e, f.project, f.issue.IID, assignee)
		return f
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		assertPagesOneAtATime(e, e.On(surface), actionIssueParticipants, f.params(),
			func(out issues.ParticipantsOutput) ([]string, toolutil.PaginationOutput) {
				usernames := make([]string, 0, len(out.Participants))
				for _, participant := range out.Participants {
					usernames = append(usernames, participant.Username)
				}
				return usernames, out.Pagination
			})
	})
}

// TestIssueMergeRequests_ClosingAndRelated_HoldTheMergeRequestThatNamesIt
// opens a merge request per surface whose description closes one shared
// issue, and reads that merge request back out of both listings the issue
// answers: the ones that will close it on merge, and the wider set that
// mention it at all.
//
// The branch, the commit and the merge request are made through the server
// because the description is what ties the merge request to the issue and
// the fixture builder does not write one.
func TestIssueMergeRequests_ClosingAndRelated_HoldTheMergeRequestThatNamesIt(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuemrs")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)
		opened := openMergeRequestClosing(e, s, f, surface)

		assertRelatedMR(e, s, actionIssueMRsClosing, f, opened)
		assertRelatedMR(e, s, actionIssueMRsRelated, f, opened)
	})
}

// openMergeRequestClosing creates a branch of this surface's own, commits a
// file on it and opens a merge request whose description closes the fixture
// issue, holding each answer to what it asked for.
//
// A branch per surface, because a project admits one open merge request per
// source branch and all three surfaces open one against the same issue.
func openMergeRequestClosing(e *harness.Env, s *harness.Session, f issueFixture, surface harness.Surface) mergerequests.Output {
	e.T.Helper()

	params := map[string]any{"project_id": f.project.IDParam()}
	branch := "closes/" + e.Name(string(surface))

	created := harness.Do[branches.Output](s, actionBranchCreate,
		withParams(params, map[string]any{"branch_name": branch, "ref": f.project.DefaultBranch}))
	if created.Name != branch {
		e.T.Fatalf("branch create answered %+v, want the branch %q", created, branch)
	}

	message := "docs: add the file the merge request carries"
	commit := harness.Do[commits.Output](s, actionRepositoryCommitCreate, withParams(params, map[string]any{
		"branch": branch, "commit_message": message,
		"actions": []map[string]any{{
			"action": "create", "file_path": "docs/closes-" + string(surface) + ".md",
			"content": "# " + string(surface) + "\n",
		}},
	}))
	if commit.ID == "" || commit.Title != message {
		e.T.Fatalf("commit create answered %+v, want a commit carrying %q", commit, message)
	}

	opened := harness.Do[mergerequests.Output](s, actionMergeRequestCreate, withParams(params, map[string]any{
		"source_branch": branch, "target_branch": f.project.DefaultBranch,
		"title":       e.Name("closes"),
		"description": fmt.Sprintf("Closes #%d", f.issue.IID),
	}))
	if opened.IID == 0 || opened.SourceBranch != branch || opened.State != "opened" {
		e.T.Fatalf("merge request create answered %+v, want an opened merge request from %q", opened, branch)
	}
	return opened
}
