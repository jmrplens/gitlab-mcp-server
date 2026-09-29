//go:build e2e

// assignment_test.go drives the assignment helpers' pure halves against the
// stub: an update GitLab answers without a user it named is repeated until it
// keeps them, one it never keeps ends with that failure named, and a refusal
// is not retried at all. Each case runs for a merge request's assignee, its
// reviewers and an issue's assignee, since the three differ only in the
// endpoint, the field they send and the list they read back.

package fixture

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// assignTarget is one of the three kinds of update an assignment helper
// makes, and the answer field GitLab lists the kept users in.
type assignTarget struct {
	name   string
	path   string
	sent   string
	kept   string
	assign func(ctx context.Context, tb testing.TB, client *gitlabclient.Client, projectID, iid, userID int64) error
}

// assignTargets are the three updates, at project 1 and IID 2.
var assignTargets = []assignTarget{
	{
		name: "merge request assignee", path: "/api/v4/projects/1/merge_requests/2",
		sent: "assignee_ids", kept: "assignees", assign: assignMergeRequest,
	},
	{
		name: "merge request reviewer", path: "/api/v4/projects/1/merge_requests/2",
		sent: "reviewer_ids", kept: "reviewers",
		assign: func(ctx context.Context, tb testing.TB, client *gitlabclient.Client, projectID, iid, userID int64) error {
			tb.Helper()
			return reviewMergeRequest(ctx, tb, client, projectID, iid, []int64{userID})
		},
	},
	{
		name: "issue assignee", path: "/api/v4/projects/1/issues/2",
		sent: "assignee_ids", kept: "assignees", assign: assignIssue,
	},
}

// keptAnswer is an update GitLab answered keeping the given user IDs in the
// named list.
func keptAnswer(list string, ids ...int64) scriptedAnswer {
	users := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		users = append(users, map[string]any{"id": id, "username": fmt.Sprintf("user%d", id)})
	}
	return stubOK(map[string]any{"iid": 2, list: users})
}

// TestKeepUsers_DroppedAtFirst_RepeatsUntilKept checks that an update
// answered without the user, which is how GitLab answers for a member whose
// access it has not recalculated yet, is sent again, and that the helper
// returns once an answer lists them. Both updates must name the user.
func TestKeepUsers_DroppedAtFirst_RepeatsUntilKept(t *testing.T) {
	for _, target := range assignTargets {
		t.Run(target.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPut, target.path, keptAnswer(target.kept, 1), keptAnswer(target.kept, 7, 1))

			if err := target.assign(t.Context(), t, client, 1, 2, 7); err != nil {
				t.Fatalf("updating the %s error = %v, want nil", target.name, err)
			}

			requests := stub.recordedRequests()
			if len(requests) != 2 {
				t.Fatalf("updating the %s sent %d requests, want 2", target.name, len(requests))
			}
			for i, request := range requests {
				if got := fmt.Sprint(request.Body[target.sent]); got != "[7]" {
					t.Errorf("request %d %s = %s, want [7]", i+1, target.sent, got)
				}
			}
		})
	}
}

// TestKeepUsers_NeverKept_EndsWithTheDroppedUser checks that a user GitLab
// keeps dropping ends the helper when its context does, and that the error
// names the drop rather than only the deadline, since the deadline alone
// would send a reader to the network.
func TestKeepUsers_NeverKept_EndsWithTheDroppedUser(t *testing.T) {
	for _, target := range assignTargets {
		t.Run(target.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPut, target.path, keptAnswer(target.kept, 1))
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()

			err := target.assign(ctx, t, client, 1, 2, 7)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("updating the %s error = %v, want the deadline", target.name, err)
			}
			if !strings.Contains(err.Error(), errUsersNotKept.Error()) {
				t.Errorf("updating the %s error = %v, want it to name the dropped user", target.name, err)
			}
		})
	}
}

// TestKeepUsers_Refused_StopsWithoutRetrying checks that a refusal no
// waiting cures is reported after the one request that drew it.
func TestKeepUsers_Refused_StopsWithoutRetrying(t *testing.T) {
	for _, target := range assignTargets {
		t.Run(target.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPut, target.path, stubRefusal(http.StatusForbidden, "403 Forbidden"))

			err := target.assign(t.Context(), t, client, 1, 2, 7)
			if err == nil || errors.Is(err, errUsersNotKept) {
				t.Fatalf("updating the %s error = %v, want the refusal", target.name, err)
			}
			if n := len(stub.recordedRequests()); n != 1 {
				t.Errorf("updating the %s sent %d requests, want 1", target.name, n)
			}
		})
	}
}

// TestReviewMergeRequest_SomeReviewersKept_WaitsForEveryOne checks that an
// answer keeping one reviewer of two is a drop too, since a licensed scenario
// asserting two reviewers would otherwise run against one.
func TestReviewMergeRequest_SomeReviewersKept_WaitsForEveryOne(t *testing.T) {
	stub, client := newStubGitLab(t)
	path := "/api/v4/projects/1/merge_requests/2"
	stub.answers(http.MethodPut, path, keptAnswer("reviewers", 1), keptAnswer("reviewers", 1, 7))

	if err := reviewMergeRequest(t.Context(), t, client, 1, 2, []int64{1, 7}); err != nil {
		t.Fatalf("reviewMergeRequest() error = %v, want nil", err)
	}
	if n := len(stub.recordedRequests()); n != 2 {
		t.Errorf("reviewMergeRequest() sent %d requests, want 2", n)
	}
}
