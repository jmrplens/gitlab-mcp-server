//go:build e2e

// merge_request_test.go drives the merge request readiness wait against the
// stub.

package fixture

import (
	"context"
	"errors"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

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
