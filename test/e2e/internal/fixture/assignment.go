//go:build e2e

// assignment.go assigns an issue or a merge request to a user, or asks users
// to review a merge request, and waits until GitLab keeps them.
//
// GitLab keeps an assignee or a reviewer only once that user may read the
// object, and it answers the update 200 with the users it kept, so an
// assignment it dropped reads exactly like one that worked. A user made a
// member a moment ago is dropped that way: the membership is written at once,
// and the authorization it grants is recalculated afterwards. On a GitLab
// 19.4.1-ee under the suite's load, assigning a merge request right after
// AddProjectMember came back without the user, and the same update fifteen
// seconds later kept them. A scenario that needs a second participant, an
// assignee or a reviewer therefore goes through these helpers rather than the
// SDK, which would leave it asserting against an object that holds only its
// author.

package fixture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// errUsersNotKept is the failure an update answered without every user it
// named is retried on.
var errUsersNotKept = errors.New("GitLab answered the update without every user it was asked to keep")

// usersNotKeptReason is how a retry of that failure reads in the log.
const usersNotKeptReason = "user not kept yet"

// AssignMergeRequest assigns the merge request to user, repeating the update
// until GitLab's answer lists them. The assignment goes with the merge
// request, so nothing is registered.
func AssignMergeRequest(e *harness.Env, project Project, iid int64, user User) {
	e.T.Helper()
	if err := assignMergeRequest(e.Ctx, e.T, e.Client(), project.ID, iid, user.ID); err != nil {
		e.T.Fatalf("assigning merge request !%d to %s: %v", iid, user.Username, err)
	}
}

// assignMergeRequest is the half of [AssignMergeRequest] that takes a client.
func assignMergeRequest(ctx context.Context, tb testing.TB, client *gitlabclient.Client, projectID, iid, userID int64) error {
	tb.Helper()
	label := fmt.Sprintf("assign merge request !%d to user %d", iid, userID)
	want := []int64{userID}
	return keepUsers(ctx, tb, label, want, func() ([]int64, error) {
		mr, _, err := client.GL().MergeRequests.UpdateMergeRequest(projectID, iid,
			&gl.UpdateMergeRequestOptions{AssigneeIDs: &want}, gl.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		return basicUserIDs(mr.Assignees), nil
	})
}

// ReviewMergeRequest asks the given users to review the merge request,
// repeating the update until GitLab's answer lists every one of them. More
// than one reviewer is a licensed feature, and an instance without it keeps
// the first, which this reports once its wait runs out. The request goes with
// the merge request, so nothing is registered.
func ReviewMergeRequest(e *harness.Env, project Project, iid int64, userIDs ...int64) {
	e.T.Helper()
	if err := reviewMergeRequest(e.Ctx, e.T, e.Client(), project.ID, iid, userIDs); err != nil {
		e.T.Fatalf("asking %v to review merge request !%d: %v", userIDs, iid, err)
	}
}

// reviewMergeRequest is the half of [ReviewMergeRequest] that takes a client.
func reviewMergeRequest(ctx context.Context, tb testing.TB, client *gitlabclient.Client, projectID, iid int64, userIDs []int64) error {
	tb.Helper()
	label := fmt.Sprintf("ask users %v to review merge request !%d", userIDs, iid)
	return keepUsers(ctx, tb, label, userIDs, func() ([]int64, error) {
		mr, _, err := client.GL().MergeRequests.UpdateMergeRequest(projectID, iid,
			&gl.UpdateMergeRequestOptions{ReviewerIDs: &userIDs}, gl.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		return basicUserIDs(mr.Reviewers), nil
	})
}

// AssignIssue assigns the issue to user, repeating the update until GitLab's
// answer lists them. The assignment goes with the issue, so nothing is
// registered.
func AssignIssue(e *harness.Env, project Project, iid int64, user User) {
	e.T.Helper()
	if err := assignIssue(e.Ctx, e.T, e.Client(), project.ID, iid, user.ID); err != nil {
		e.T.Fatalf("assigning issue #%d to %s: %v", iid, user.Username, err)
	}
}

// assignIssue is the half of [AssignIssue] that takes a client.
func assignIssue(ctx context.Context, tb testing.TB, client *gitlabclient.Client, projectID, iid, userID int64) error {
	tb.Helper()
	label := fmt.Sprintf("assign issue #%d to user %d", iid, userID)
	want := []int64{userID}
	return keepUsers(ctx, tb, label, want, func() ([]int64, error) {
		issue, _, err := client.GL().Issues.UpdateIssue(projectID, iid,
			&gl.UpdateIssueOptions{AssigneeIDs: &want}, gl.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		kept := make([]int64, 0, len(issue.Assignees))
		for _, assignee := range issue.Assignees {
			kept = append(kept, assignee.ID)
		}
		return kept, nil
	})
}

// basicUserIDs is the IDs of the users a merge request names.
func basicUserIDs(users []*gl.BasicUser) []int64 {
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return ids
}

// keepUsers runs update until the users it reports GitLab kept include every
// one of want, with the budget and delays of the other fixture retries. A
// refusal is retried only where the other builders retry one, and a dropped
// user always is, since waiting is the whole of the cure.
func keepUsers(ctx context.Context, tb testing.TB, label string, want []int64, update func() ([]int64, error)) error {
	tb.Helper()
	_, err := harness.Retry(ctx, tb, label, assignRetries, retryBaseDelay, func(int) (struct{}, bool, string, error) {
		kept, err := update()
		if err != nil {
			return struct{}{}, IsRetryable(err), describeRetry(err), err
		}
		for _, id := range want {
			if !slices.Contains(kept, id) {
				return struct{}{}, true, usersNotKeptReason, errUsersNotKept
			}
		}
		return struct{}{}, false, "", nil
	})
	return err
}
