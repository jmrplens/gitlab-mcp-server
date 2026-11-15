// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mergerequests

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// assigneesGitLab answers the read of merge request 1 with current, or with a
// 503 when current is empty, and every write with a merge request, counting
// both.
func assigneesGitLab(current string, reads, writes *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMR1 {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			writes.Add(1)
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
			return
		}
		reads.Add(1)
		if current == "" {
			testutil.RespondJSON(w, http.StatusServiceUnavailable, `{"message":"503 Service Unavailable"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, current)
	})
}

// TestUpdate_ClearingAssignees_Asks verifies that an update asks before it
// removes every assignee of the merge request, as issue.work_item_update asks
// before an empty assignee list, and nowhere else. The merge request is read
// first, so a list that is already empty goes out unasked, a list that could
// not be read asks, and a refused update writes nothing. An empty reviewer
// list asks nothing.
func TestUpdate_ClearingAssignees_Asks(t *testing.T) {
	zero := int64(0)
	const (
		assigned = `{"id":100,"iid":1,"title":"Test MR","state":"opened","assignees":[{"id":3,"username":"alice"},{"id":4,"username":"bob"}]}`
		tail     = " This replaces the list and cannot be undone from the response." +
			" Re-send the same call with confirm=true once the user has approved it"
	)
	for _, tc := range []struct {
		name               string
		input              UpdateInput
		yolo, current      string
		wantReads, wantPut int64
		want               string
	}{
		{
			name: "assignee_ids [] on a merge request somebody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: assigned, wantReads: 1,
			want: "mrUpdate: Remove all 2 assignees from merge request 1 in project `42`?" + tail,
		},
		{
			name: "assignee_id 0 on a merge request that could not be read", input: UpdateInput{AssigneeID: &zero},
			wantReads: 1,
			want:      "mrUpdate: Remove every assignee (the current list could not be read) from merge request 1 in project `42`?" + tail,
		},
		{
			name: "assignee_ids [] on a merge request nobody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: mrJSONCoverage, wantReads: 1, wantPut: 1,
		},
		{name: "another assignee", input: UpdateInput{AssigneeIDs: []int64{5}}, wantPut: 1},
		{name: "reviewer_ids []", input: UpdateInput{ReviewerIDs: []int64{}}, wantPut: 1},
		{name: "assignee_ids [] in YOLO mode", input: UpdateInput{AssigneeIDs: []int64{}}, yolo: "true", current: assigned, wantPut: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", tc.yolo)
			t.Setenv("AUTOPILOT", "")
			var reads, writes atomic.Int64
			client := testutil.NewTestClient(t, assigneesGitLab(tc.current, &reads, &writes))
			tc.input.ProjectID, tc.input.MRIID = testProjectID, 1
			_, err := Update(t.Context(), client, tc.input)
			if tc.want == "" && err != nil {
				t.Errorf("Update() error = %v, want the update to go out", err)
			}
			if tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tc.want)
			}
			if got := reads.Load(); got != tc.wantReads {
				t.Errorf("the merge request was read %d time(s), want %d", got, tc.wantReads)
			}
			if got := writes.Load(); got != tc.wantPut {
				t.Errorf("the update was written %d time(s), want %d", got, tc.wantPut)
			}
		})
	}
}
