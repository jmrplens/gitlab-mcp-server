//go:build e2e

// mergeable_mr_test.go drives the approval clearing against the stub: both
// records of the requirement, the three refusals that mean the surface is not
// licensed, and the one that is a real failure.

package fixture

import (
	"net/http"
	"testing"
)

// TestClearMergeRequestApprovals_BothRecords_AreCleared checks that the merge
// request's own count and each rule's are both sent to zero, since GitLab
// keeps the requirement in two places and either one refuses a merge.
func TestClearMergeRequestApprovals_BothRecords_AreCleared(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/approvals", stubOK(map[string]any{"iid": 2}))
	stub.answers(http.MethodGet, "/api/v4/projects/1/merge_requests/2/approval_rules", stubOK([]any{
		map[string]any{"id": 3, "name": "two eyes", "approvals_required": 2},
		map[string]any{"id": 4, "name": "nobody", "approvals_required": 0},
	}))
	stub.answers(http.MethodPut, "/api/v4/projects/1/merge_requests/2/approval_rules/3", stubOK(map[string]any{"id": 3}))

	if err := clearMergeRequestApprovals(t.Context(), client, 1, 2); err != nil {
		t.Fatalf("clearMergeRequestApprovals() error = %v, want nil", err)
	}

	requests := stub.recordedRequests()
	if len(requests) != 3 {
		t.Fatalf("clearMergeRequestApprovals() sent %d requests, want the count, the rules read and one rule edit", len(requests))
	}
	if requests[0].Body["approvals_required"] != float64(0) {
		t.Errorf("clearMergeRequestApprovals() sent approvals_required %v, want 0", requests[0].Body["approvals_required"])
	}
	if requests[2].Path != "/api/v4/projects/1/merge_requests/2/approval_rules/3" {
		t.Errorf("clearMergeRequestApprovals() edited %s, want only the rule that required approvals", requests[2].Path)
	}
}

// TestClearMergeRequestApprovals_SurfaceUnlicensed_IsNotAFailure checks the
// Free ending: the approvals API answers one of three refusals, there is
// nothing to clear, and that is the outcome the caller wanted.
func TestClearMergeRequestApprovals_SurfaceUnlicensed_IsNotAFailure(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "bad request", status: http.StatusBadRequest},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "not found", status: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/approvals",
				stubRefusal(tc.status, "refused"))
			stub.answers(http.MethodGet, "/api/v4/projects/1/merge_requests/2/approval_rules",
				stubRefusal(tc.status, "refused"))

			if err := clearMergeRequestApprovals(t.Context(), client, 1, 2); err != nil {
				t.Errorf("clearMergeRequestApprovals() error = %v, want the unlicensed surface tolerated", err)
			}
		})
	}
}

// TestClearMergeRequestApprovals_RulesUnreadable_IsReported checks that a
// refusal which is none of the three is a failure, since a merge request
// whose rules could not be read is not one a merge case can rely on.
func TestClearMergeRequestApprovals_RulesUnreadable_IsReported(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/approvals", stubOK(map[string]any{"iid": 2}))
	stub.answers(http.MethodGet, "/api/v4/projects/1/merge_requests/2/approval_rules",
		stubRefusal(http.StatusInternalServerError, "500 Internal Server Error"))

	if err := clearMergeRequestApprovals(t.Context(), client, 1, 2); err == nil {
		t.Error("clearMergeRequestApprovals() error = nil, want the unreadable rules reported")
	}
}

// TestApprovalsUnavailable_Statuses_NamesTheThreeAndNothingElse pins the
// classifier on its own, since it is what decides whether the fixture carries
// on or stops.
func TestApprovalsUnavailable_Statuses_NamesTheThreeAndNothingElse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "bad request", status: http.StatusBadRequest, want: true},
		{name: "forbidden", status: http.StatusForbidden, want: true},
		{name: "not found", status: http.StatusNotFound, want: true},
		{name: "conflict", status: http.StatusConflict},
		{name: "server error", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approvalsUnavailable(statusError(tc.status, "refused")); got != tc.want {
				t.Errorf("approvalsUnavailable(%d) = %t, want %t", tc.status, got, tc.want)
			}
		})
	}
}
