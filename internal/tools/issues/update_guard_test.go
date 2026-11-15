// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// assignedIssueJSON is issue 10 with two assignees, which an update that
// empties the list would remove.
const assignedIssueJSON = `{"id":1,"iid":10,"title":"Test issue","state":"opened","assignees":[{"id":3,"username":"alice"},{"id":4,"username":"bob"}],"web_url":"https://gitlab.example.com/project/issues/10"}`

// removalGitLab answers the read of issue 10 with current, or with a 503 when
// current is empty, and every write with the minimal issue, counting both.
func removalGitLab(t *testing.T, current string, reads, writes *atomic.Int64) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathIssue10 {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			reads.Add(1)
			if current == "" {
				testutil.RespondJSON(w, http.StatusServiceUnavailable, `{"message":"503 Service Unavailable"}`)
				return
			}
			testutil.RespondJSON(w, http.StatusOK, current)
			return
		}
		writes.Add(1)
		testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
	})
}

// TestUpdate_Removals_Ask verifies that an update asks before it takes the
// issue out of its epic, as group.epic_issue_remove does, and before it
// removes every assignee, as issue.work_item_update does, and nowhere else.
// epic_id 0 is decided by the arguments, so it is refused before any request;
// an empty assignee list reads the issue first and asks only when somebody is
// assigned, or when the issue could not be read. A refused update writes
// nothing, and an update that removes nothing goes out unasked.
func TestUpdate_Removals_Ask(t *testing.T) {
	zero, epic := int64(0), int64(5)
	const tail = " Re-send the same call with confirm=true once the user has approved it"
	for _, tc := range []struct {
		name               string
		input              UpdateInput
		yolo, current      string
		wantReads, wantPut int64
		want               string
	}{
		{
			name: "epic_id 0", input: UpdateInput{EpicID: &zero},
			want: "issueUpdate: Take issue 10 in project `42` out of its epic?" +
				" The issue is no longer a child of the epic, as when it is removed from the epic." + tail,
		},
		{name: "another epic", input: UpdateInput{EpicID: &epic}, wantPut: 1},
		{
			name: "assignee_ids [] on an issue somebody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: assignedIssueJSON, wantReads: 1,
			want: "issueUpdate: Remove all 2 assignees from issue 10 in project `42`?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{
			name: "assignee_id 0 on an issue that could not be read", input: UpdateInput{AssigneeID: &zero},
			wantReads: 1,
			want: "issueUpdate: Remove every assignee (the current list could not be read) from issue 10 in project `42`?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{
			name: "assignee_ids [] on an issue nobody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: issueJSONMinimal, wantReads: 1, wantPut: 1,
		},
		{
			// GitLab reads the lone id 0 as nobody.
			name: "assignee_ids [0] on an issue somebody is assigned", input: UpdateInput{AssigneeIDs: []int64{0}},
			current: assignedIssueJSON, wantReads: 1,
			want: "issueUpdate: Remove all 2 assignees from issue 10 in project `42`?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{
			// An issue that takes a single assignee keeps the first id, 0.
			name: "assignee_ids [0, 9] on an issue somebody is assigned", input: UpdateInput{AssigneeIDs: []int64{0, 9}},
			current: assignedIssueJSON, wantReads: 1,
			want: "issueUpdate: Remove all 2 assignees from issue 10 in project `42`?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{name: "another assignee", input: UpdateInput{AssigneeIDs: []int64{4}}, wantPut: 1},
		{name: "a user first and 0 after it", input: UpdateInput{AssigneeIDs: []int64{9, 0}}, wantPut: 1},
		{
			name: "both removals in YOLO mode", input: UpdateInput{EpicID: &zero, AssigneeIDs: []int64{}},
			yolo: "true", current: assignedIssueJSON, wantPut: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", tc.yolo)
			t.Setenv("AUTOPILOT", "")
			var reads, writes atomic.Int64
			client := testutil.NewTestClient(t, removalGitLab(t, tc.current, &reads, &writes))
			tc.input.ProjectID, tc.input.IssueIID = testProjectID, 10
			_, err := Update(t.Context(), client, tc.input)
			if tc.want == "" && err != nil {
				t.Errorf("Update() error = %v, want the update to go out", err)
			}
			if tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tc.want)
			}
			if got := reads.Load(); got != tc.wantReads {
				t.Errorf("the issue was read %d time(s), want %d", got, tc.wantReads)
			}
			if got := writes.Load(); got != tc.wantPut {
				t.Errorf("the update was written %d time(s), want %d", got, tc.wantPut)
			}
		})
	}
}

// TestUpdate_AssigneesCleared_CanceledRead_ReturnsCanceled verifies that an
// update whose caller goes away while the guard reads the issue ends with the
// cancellation, which the dispatcher logs as one, rather than with the
// question nobody is there to answer, and sends no update.
func TestUpdate_AssigneesCleared_CanceledRead_ReturnsCanceled(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	var writes atomic.Int64
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		testutil.RespondJSON(w, http.StatusOK, assignedIssueJSON)
	})
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, IssueIID: 10, AssigneeIDs: []int64{}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Update() error = %v, want context.Canceled", err)
	}
	if got := writes.Load(); got != 0 {
		t.Errorf("the update was written %d time(s), want none", got)
	}
}
