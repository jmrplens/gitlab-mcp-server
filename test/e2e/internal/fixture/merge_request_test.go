// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// merge_request_test.go drives the merge request readiness wait, and the
// creation's retry of a branch GitLab's branch name cache does not list,
// against the stub.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// shortBranchListedPolls makes the branch name cache wait ask as fast as a
// test needs, and restores the interval afterwards.
func shortBranchListedPolls(t *testing.T) {
	t.Helper()
	saved := branchListedPollInterval
	branchListedPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { branchListedPollInterval = saved })
}

// shortBranchListedWait bounds the branch name cache wait, on either tier,
// for a test whose branch is never listed, polling as fast as
// shortBranchListedPolls does, and restores both afterwards.
func shortBranchListedWait(t *testing.T, wait time.Duration) {
	t.Helper()
	shortBranchListedPolls(t)
	saved, savedEnterprise := branchListedWait, enterpriseBranchListedWait
	branchListedWait, enterpriseBranchListedWait = wait, wait
	t.Cleanup(func() { branchListedWait, enterpriseBranchListedWait = saved, savedEnterprise })
}

// branchMissingRefusal is GitLab's answer to a merge request creation whose
// branch, named by its attribute, the branch name cache does not list.
func branchMissingRefusal(attributes ...string) scriptedAnswer {
	message := map[string][]string{}
	for _, attribute := range attributes {
		message[attribute] = []string{branchMissingMessage}
	}
	return scriptedAnswer{status: http.StatusBadRequest, body: map[string]any{"message": message}}
}

// refusalWithBody builds the error client-go returns for a refusal carrying
// body, as CheckResponse does.
func refusalWithBody(code int, body string) error {
	request := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "gitlab.example", Path: "/api/v4/projects/2/merge_requests"},
	}
	return &gl.ErrorResponse{
		StatusCode: code,
		Response:   &http.Response{StatusCode: code, Request: request},
		Body:       []byte(body),
		Message:    "refused",
	}
}

// TestUnlistedBranches_Refusals_NamesOnlyTheBranchesGitLabSaysAreMissing pins
// which failures of a merge request creation are read as a branch the cache
// does not list: a 400 whose validation errors say "does not exist" for the
// source or the target branch, and nothing else.
func TestUnlistedBranches_Refusals_NamesOnlyTheBranchesGitLabSaysAreMissing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{name: "no error"},
		{name: "source", err: refusalWithBody(http.StatusBadRequest, `{"message":{"source_branch":["does not exist"]}}`), want: []string{"feature"}},
		{name: "target", err: refusalWithBody(http.StatusBadRequest, `{"message":{"target_branch":["does not exist"]}}`), want: []string{"main"}},
		{
			name: "both, source first",
			err:  refusalWithBody(http.StatusBadRequest, `{"message":{"target_branch":["does not exist"],"source_branch":["does not exist"]}}`),
			want: []string{"feature", "main"},
		},
		{name: "another validation error", err: refusalWithBody(http.StatusBadRequest, `{"message":{"title":["can't be blank"]}}`)},
		{name: "another error of the source branch", err: refusalWithBody(http.StatusBadRequest, `{"message":{"source_branch":["is invalid"]}}`)},
		{name: "a message that is text", err: refusalWithBody(http.StatusBadRequest, `{"message":"400 Bad request"}`)},
		{name: "another status", err: refusalWithBody(http.StatusConflict, `{"message":{"source_branch":["does not exist"]}}`)},
		{name: "not a refusal", err: errors.New("source_branch: [does not exist]")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := unlistedBranches(testCase.err, "feature", "main"); !slices.Equal(got, testCase.want) {
				t.Errorf("unlistedBranches(%v) = %q, want %q", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestWaitForBranchListed_NotListedYet_PollsHEADUntilItIs checks that the 404
// the cache answers is waited through, and that the branch is asked about
// with HEAD, the read GitLab answers from the cache the creation validates
// against, never with GET, which GitLab answers from Gitaly.
func TestWaitForBranchListed_NotListedYet_PollsHEADUntilItIs(t *testing.T) {
	shortBranchListedPolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.branchMisses = 2 })

	if err := waitForBranchListed(t.Context(), client, 1, "feature/x", 5*time.Second); err != nil {
		t.Fatalf("waitForBranchListed() error = %v, want nil once the branch is listed", err)
	}
	if got, want := stub.recordedBranchReads(), []string{http.MethodHead, http.MethodHead, http.MethodHead}; !slices.Equal(got, want) {
		t.Errorf("branch reads = %q, want %q", got, want)
	}
}

// TestWaitForBranchListed_NeverListed_ReportsTheTimeoutWithTheLastState checks
// the failure names the answer last seen.
func TestWaitForBranchListed_NeverListed_ReportsTheTimeoutWithTheLastState(t *testing.T) {
	shortBranchListedPolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.branchMisses = 1 << 20 })

	err := waitForBranchListed(t.Context(), client, 1, "feature", 300*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForBranchListed() error = %v, want a poll timeout", err)
	}
	if !strings.Contains(err.Error(), `HEAD of branch "feature" in project 1: HTTP 404`) {
		t.Errorf("timeout error = %q, want it to carry the last answer", err)
	}
}

// TestWaitForBranchListed_AnswerInFlightAtTheDeadline_IsTheLastState checks
// that a HEAD still in flight when the wait runs out is answered rather than
// cut short by the wait's own deadline: GitLab answers it 404 after the wait
// has ended, and the timeout names that 404, not a request with no answer.
func TestWaitForBranchListed_AnswerInFlightAtTheDeadline_IsTheLastState(t *testing.T) {
	shortBranchListedPolls(t)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
			w.WriteHeader(http.StatusNotFound)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	client, err := gitlabclient.NewClientWithTokenRetries(slow.URL, stubToken, false, true)
	if err != nil {
		t.Fatalf("building a client for the slow GitLab: %v", err)
	}

	err = waitForBranchListed(t.Context(), client, 1, "feature", 50*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForBranchListed() error = %v, want a poll timeout", err)
	}
	if !strings.Contains(err.Error(), `HEAD of branch "feature" in project 1: HTTP 404`) {
		t.Errorf("timeout error = %q, want it to name the 404 answered after the wait ran out", err)
	}
}

// TestWaitForBranchListed_Refused_EndsTheWait checks that an answer no
// converging GitLab gives ends the wait at once, with that answer.
func TestWaitForBranchListed_Refused_EndsTheWait(t *testing.T) {
	shortBranchListedPolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.refusals = map[string]int{"/api/v4/projects/1/repository/branches/feature": http.StatusForbidden}
	})

	err := waitForBranchListed(t.Context(), client, 1, "feature", 5*time.Second)
	if err == nil || errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForBranchListed() error = %v, want the refusal itself", err)
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %q, want it to name the 403", err)
	}
}

// TestWaitForBranchListed_NoAnswer_IsWaitedThrough checks that a request no
// answer came back to is polled through like a 404, and named when the wait
// runs out.
func TestWaitForBranchListed_NoAnswer_IsWaitedThrough(t *testing.T) {
	shortBranchListedPolls(t)
	client, err := gitlabclient.NewClientWithTokenRetries(netfixture.DeadEndpoint(t), stubToken, false, true)
	if err != nil {
		t.Fatalf("building a client for the dead endpoint: %v", err)
	}

	err = waitForBranchListed(t.Context(), client, 1, "feature", 200*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForBranchListed() error = %v, want a poll timeout", err)
	}
	if !strings.Contains(err.Error(), `HEAD of branch "feature" in project 1: no answer`) {
		t.Errorf("timeout error = %q, want it to name the request that got no answer", err)
	}
}

// TestAwaitBranchesListed_Outcomes_ReportsWhetherEveryBranchWasListed drives
// the wait an Env makes: every branch listed is true, and a branch never
// listed is false without failing the test, which is left to the creation's
// own refusal.
func TestAwaitBranchesListed_Outcomes_ReportsWhetherEveryBranchWasListed(t *testing.T) {
	cases := []struct {
		name   string
		misses int
		want   bool
	}{
		{name: "listed after a miss", misses: 1, want: true},
		{name: "never listed", misses: 1 << 20},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			shortBranchListedWait(t, 200*time.Millisecond)
			stub, e := detachedStub(t)
			stub.configure(func() { stub.branchMisses = testCase.misses })

			if got := awaitBranchesListed(e, Project{ID: 2}, []string{"feature", "main"}); got != testCase.want {
				t.Errorf("awaitBranchesListed() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestNewMergeRequest_BranchNotListedYet_RetriesOnceTheCacheListsIt drives
// the builder whole through the refusal the rehearsal of 2026-10-31 failed
// on: the first creation is refused for a source branch the cache does not
// list, the builder waits until HEAD finds it, and the second creation is the
// merge request it returns.
func TestNewMergeRequest_BranchNotListedYet_RetriesOnceTheCacheListsIt(t *testing.T) {
	shortBranchListedPolls(t)
	stub, e := detachedStub(t)
	stub.configure(func() {
		stub.branchMisses = 1
		stub.mergeStatuses = []string{"mergeable"}
	})
	stub.answers(http.MethodPost, "/api/v4/projects/2/merge_requests",
		branchMissingRefusal("source_branch"),
		stubCreated(map[string]any{"iid": 3, "id": 30, "source_branch": "feature", "target_branch": "main", "title": "t"}),
	)

	mr := NewMergeRequest(e, Project{ID: 2}, "feature", "main", "t")

	if mr.IID != 3 || mr.Status != "mergeable" {
		t.Errorf("NewMergeRequest() = %+v, want !3 settled as mergeable", mr)
	}
	creations := 0
	for _, request := range stub.recordedRequests() {
		if request.Method == http.MethodPost && request.Path == "/api/v4/projects/2/merge_requests" {
			creations++
		}
	}
	if creations != 2 {
		t.Errorf("NewMergeRequest() sent %d creations, want the refused one and the one after the branch was listed", creations)
	}
	if got, want := stub.recordedBranchReads(), []string{http.MethodHead, http.MethodHead}; !slices.Equal(got, want) {
		t.Errorf("branch reads = %q, want %q: one miss and the HEAD that found it", got, want)
	}
}

// TestNewMergeRequest_ServerError_IsRetriedWithoutAskingForTheBranch checks
// that a failure the cache has nothing to do with is still retried the way
// every builder retries it, and sends no branch read.
func TestNewMergeRequest_ServerError_IsRetriedWithoutAskingForTheBranch(t *testing.T) {
	stub, e := detachedStub(t)
	stub.configure(func() { stub.mergeStatuses = []string{"mergeable"} })
	stub.answers(http.MethodPost, "/api/v4/projects/2/merge_requests",
		stubRefusal(http.StatusBadGateway, "502 Bad Gateway"),
		stubCreated(map[string]any{"iid": 4, "id": 40, "source_branch": "feature", "target_branch": "main", "title": "t"}),
	)

	if mr := NewMergeRequest(e, Project{ID: 2}, "feature", "main", "t"); mr.IID != 4 {
		t.Errorf("NewMergeRequest() = %+v, want !4 from the second attempt", mr)
	}
	if got := stub.recordedBranchReads(); len(got) != 0 {
		t.Errorf("branch reads = %q, want none for a server error", got)
	}
}

// TestWaitForMergeRequestReady_Transitional_WaitsForASettledStatus checks
// that the three states GitLab reports while it computes the merge ref are
// waited through, and the status it settles on is what comes back.
func TestWaitForMergeRequestReady_Transitional_WaitsForASettledStatus(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.mergeStatuses = []string{"preparing", "checking", "unchecked", "mergeable"} })

	status, err := waitForMergeRequestReady(context.Background(), client, 1, 2, 5*time.Second)
	if err != nil {
		t.Fatalf("waitForMergeRequestReady() error = %v, want nil", err)
	}
	if status != "mergeable" {
		t.Errorf("status = %q, want mergeable", status)
	}
}

// TestWaitForMergeRequestReady_NeverSettles_ReportsTheLastStatus checks the
// timeout hands back what was last seen, since the caller logs it and goes
// on.
func TestWaitForMergeRequestReady_NeverSettles_ReportsTheLastStatus(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.mergeStatuses = []string{"checking"} })

	status, err := waitForMergeRequestReady(context.Background(), client, 1, 2, 300*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForMergeRequestReady() error = %v, want a poll timeout", err)
	}
	if status != "checking" {
		t.Errorf("status = %q, want the last one seen", status)
	}
}

// TestWaitForMergeRequestReady_NotFound_EndsTheWait checks that an answer
// the wait cannot poll through, a 404 for a merge request that is not there,
// ends it with that error rather than with a timeout.
func TestWaitForMergeRequestReady_NotFound_EndsTheWait(t *testing.T) {
	_, client := newStubGitLab(t)

	_, err := waitForMergeRequestReady(context.Background(), client, 1, 404, 5*time.Second)
	if err == nil || errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForMergeRequestReady() error = %v, want the 404 itself", err)
	}
}

// TestTransitionalMergeStatus_Values_NamesTheThreeAndEmpty pins the set.
func TestTransitionalMergeStatus_Values_NamesTheThreeAndEmpty(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "preparing", want: true},
		{status: "checking", want: true},
		{status: "unchecked", want: true},
		{status: "", want: true},
		{status: "mergeable", want: false},
		{status: "conflict", want: false},
	}
	for _, testCase := range cases {
		t.Run("status "+testCase.status, func(t *testing.T) {
			if got := transitionalMergeStatus(testCase.status); got != testCase.want {
				t.Errorf("transitionalMergeStatus(%q) = %t, want %t", testCase.status, got, testCase.want)
			}
		})
	}
}

// TestMergeRequestOf_Fields_ReadsWhatATestNeeds checks the projection.
func TestMergeRequestOf_Fields_ReadsWhatATestNeeds(t *testing.T) {
	got := mergeRequestOf(&gl.MergeRequest{IID: 3, ID: 30, SourceBranch: "feature", TargetBranch: "main", Title: "t", DetailedMergeStatus: "mergeable"})
	want := MergeRequest{IID: 3, ID: 30, SourceBranch: "feature", TargetBranch: "main", Title: "t", Status: "mergeable"}
	if got != want {
		t.Errorf("mergeRequestOf() = %+v, want %+v", got, want)
	}
}
