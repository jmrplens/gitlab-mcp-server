//go:build e2e

// mergerequests_lifecycle_test.go covers the rest of a merge request's own
// lifecycle through the server: the read, the listing, the retitle, its
// commits and participants, the close and the state event it records, the
// delete. The create lives in mergerequests_test.go, which an earlier port
// wrote. Each surface opens a request of its own in one shared project,
// since the delete at the end consumes it.

package common

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The waits on what GitLab computes for a request after it exists: its
// commits and diff versions arrive once the diff is written, which the
// fixture's readiness wait usually covers and a loaded instance sometimes
// does not. The budget is the one the old suite used for the same reads.
const (
	mergeRequestReadInterval = 2 * time.Second
	mergeRequestReadWait     = 120 * time.Second
)

// mergeRequestFixture is a merge request in a project, with the branch and
// the one commit it carries over the default branch.
type mergeRequestFixture struct {
	project fixture.Project
	branch  fixture.Branch
	commit  fixture.Commit
	mr      fixture.MergeRequest
}

// newMergeRequestIn opens a mergeable request in an existing project: a
// branch off the default one, one file committed on it, the request from
// it, waited for until GitLab has finished preparing it. The prefix keeps
// the branches and files of the tests here apart.
func newMergeRequestIn(e *harness.Env, project fixture.Project, prefix string) mergeRequestFixture {
	e.T.Helper()

	branch := fixture.NewBranch(e, project, e.Name(prefix))
	commit := fixture.CommitFile(e, project, branch.Name, prefix+".txt", prefix+" fixture on "+branch.Name+"\n", "add the "+prefix+" fixture")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, e.Name(prefix))
	return mergeRequestFixture{project: project, branch: branch, commit: commit, mr: mr}
}

// newMergeRequestFixture creates a project and opens a mergeable request
// in it.
func newMergeRequestFixture(e *harness.Env, prefix string) mergeRequestFixture {
	e.T.Helper()
	return newMergeRequestIn(e, fixture.NewProject(e, fixture.WithNamePrefix(prefix)), prefix)
}

// params spells the two parameters every request-scoped action takes.
func (f mergeRequestFixture) params() map[string]any {
	return map[string]any{"project_id": f.project.IDParam(), "merge_request_iid": f.mr.IID}
}

// mergeRequestIIDs lists the iids of a request listing.
func mergeRequestIIDs(listed []mergerequests.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, mr := range listed {
		ids = append(ids, mr.IID)
	}
	return ids
}

// TestMergeRequest_Lifecycle_GetListUpdateCloseDelete opens a request of
// its own on every surface, reads it back, finds it among the open ones,
// retitles it, reads its commits and participants, closes it and reads the
// state event the close recorded, then deletes it and checks the read is
// refused.
//
// Replaces: TestIndividual_MergeRequests, TestMeta_MergeRequests, TestMeta_StateEvents
func TestMergeRequest_Lifecycle_GetListUpdateCloseDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrlife"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mrlife")
		params := f.params()

		got := harness.Do[mergerequests.Output](s, actionMergeRequestGet, params)
		if got.IID != f.mr.IID || got.Title != f.mr.Title || got.State != "opened" {
			e.T.Errorf("merge_request get answered %+v, want the opened request !%d titled %q", got, f.mr.IID, f.mr.Title)
		}

		listed := harness.Do[mergerequests.ListOutput](s, actionMergeRequestList, map[string]any{"project_id": project.IDParam(), "state": "opened"})
		if !containsID(mergeRequestIIDs(listed.MergeRequests), f.mr.IID) {
			e.T.Errorf("the open requests do not hold !%d: %v", f.mr.IID, mergeRequestIIDs(listed.MergeRequests))
		}

		retitled := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate, withParams(params, map[string]any{"title": "Updated " + f.mr.Title}))
		if retitled.IID != f.mr.IID || retitled.Title != "Updated "+f.mr.Title {
			e.T.Errorf("merge_request update answered %+v, want request !%d retitled", retitled, f.mr.IID)
		}

		commits := harness.Eventually(s, actionMergeRequestCommits, params, mergeRequestReadInterval, mergeRequestReadWait,
			func(out mergerequests.CommitsOutput) bool { return len(out.Commits) > 0 })
		if !commitListed(commits, f.commit.SHA) {
			e.T.Errorf("the request's commits do not hold the fixture commit %s: %+v", f.commit.ShortID, commits.Commits)
		}

		participants := harness.Do[mergerequests.ParticipantsOutput](s, actionMergeRequestParticipants, params)
		if !participantListed(participants, e.Runtime().UserID) {
			e.T.Errorf("the request's participants do not hold its author, user %d: %+v", e.Runtime().UserID, participants.Participants)
		}

		closed := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate, withParams(params, map[string]any{"state_event": "close"}))
		if closed.IID != f.mr.IID || closed.State != "closed" {
			e.T.Errorf("merge_request update answered %+v, want request !%d closed", closed, f.mr.IID)
		}
		events := harness.Eventually(s, actionMergeRequestStateEventList, params, stateEventInterval, stateEventWait,
			func(out resourceevents.ListStateEventsOutput) bool { return len(out.Events) > 0 })
		if !stateEventRecorded(events.Events, "closed") {
			e.T.Errorf("the request's state events do not record the close: %+v", events.Events)
		}

		harness.DoVoid(s, actionMergeRequestDelete, params)
		refused := harness.Refused(s, actionMergeRequestGet, params, harness.FailureNotFound)
		e.T.Logf("the read of the deleted request was refused: %s", firstLine(refused))
	})
}

// commitListed reports whether a request's commits hold the given SHA.
func commitListed(out mergerequests.CommitsOutput, sha string) bool {
	for _, commit := range out.Commits {
		if commit.ID == sha {
			return true
		}
	}
	return false
}

// participantListed reports whether a participant listing holds the user.
func participantListed(out mergerequests.ParticipantsOutput, userID int64) bool {
	for _, participant := range out.Participants {
		if participant.ID == userID {
			return true
		}
	}
	return false
}
