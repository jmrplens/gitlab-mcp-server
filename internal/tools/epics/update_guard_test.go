// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package epics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// epicWithAssignees is the read of epic 1 answered with the given assignee
// nodes, the rest of the epic as getResponseJSON has it.
func epicWithAssignees(nodes string) string {
	return strings.Replace(getResponseJSON, `"assignees":{"assignees":{"nodes":[{"username":"bob"}]}}`,
		`"assignees":{"assignees":{"nodes":[`+nodes+`]}}`, 1)
}

// removalGitLab answers the workItemUpdate mutation with the updated epic and
// every query with current, or with a 503 when current is empty, counting the
// mutations as writes and the queries as reads.
func removalGitLab(t *testing.T, current string, reads, writes *atomic.Int64) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the request body: %v", err)
			return
		}
		if strings.Contains(string(body), "workItemUpdate") {
			writes.Add(1)
			testutil.RespondJSON(w, http.StatusOK, updateResponseJSON)
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

// TestUpdate_ParentRemoval_Asks verifies that an epic update asks before it
// takes the epic out of its parent epic, as group.epic_issue_remove does, and
// before it removes every assignee, as issue.work_item_update does, and
// nowhere else. parent_id 0 is decided by the arguments, so it is refused
// before any request; an empty assignee list reads the epic first and asks
// only when somebody is assigned, or when the epic could not be read. A
// refused update writes nothing, and one that removes nothing goes out
// unasked.
func TestUpdate_ParentRemoval_Asks(t *testing.T) {
	zero, parent := int64(0), int64(12)
	const tail = " Re-send the same call with confirm=true once the user has approved it"
	for _, tc := range []struct {
		name                 string
		input                UpdateInput
		yolo, current        string
		wantReads, wantWrite int64
		want                 string
	}{
		{
			name: "parent_id 0", input: UpdateInput{ParentID: &zero},
			want: "epicUpdate: Take epic `my-group`&1 out of its parent epic?" +
				" The epic is no longer a child of its parent, as when it is removed from the parent." + tail,
		},
		// An update that goes out reads once more on its own: client-go
		// looks the epic's global ID up before the mutation.
		{name: "another parent", input: UpdateInput{ParentID: &parent}, current: getResponseJSON, wantReads: 1, wantWrite: 1},
		{
			name: "assignee_ids [] on an epic somebody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: epicWithAssignees(`{"username":"bob"},{"username":"carol"}`), wantReads: 1,
			want: "epicUpdate: Remove all 2 assignees from epic `my-group`&1?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{
			name: "assignee_ids [] on an epic that could not be read", input: UpdateInput{AssigneeIDs: []int64{}},
			wantReads: 1,
			want: "epicUpdate: Remove every assignee (the current list could not be read) from epic `my-group`&1?" +
				" This replaces the list and cannot be undone from the response." + tail,
		},
		{
			name: "assignee_ids [] on an epic nobody is assigned", input: UpdateInput{AssigneeIDs: []int64{}},
			current: epicWithAssignees(""), wantReads: 2, wantWrite: 1,
		},
		{
			name: "both removals in YOLO mode", input: UpdateInput{ParentID: &zero, AssigneeIDs: []int64{}},
			yolo: "true", current: getResponseJSON, wantReads: 1, wantWrite: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", tc.yolo)
			t.Setenv("AUTOPILOT", "")
			var reads, writes atomic.Int64
			client := testutil.NewTestClient(t, removalGitLab(t, tc.current, &reads, &writes))
			tc.input.FullPath, tc.input.IID = testFullPath, 1
			_, err := Update(t.Context(), client, tc.input)
			if tc.want == "" && err != nil {
				t.Errorf("Update() error = %v, want the update to go out", err)
			}
			if tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tc.want)
			}
			if got := reads.Load(); got != tc.wantReads {
				t.Errorf("the epic was read %d time(s), want %d", got, tc.wantReads)
			}
			if got := writes.Load(); got != tc.wantWrite {
				t.Errorf("the update was written %d time(s), want %d", got, tc.wantWrite)
			}
		})
	}
}

// TestUpdate_AssigneesCleared_CanceledRead_ReturnsCanceled verifies that an
// update whose caller goes away while the guard reads the epic ends with the
// cancellation, which the dispatcher logs as one, rather than with the
// question nobody is there to answer. The guard's read is the only request
// that reaches the mock, so no update is sent.
func TestUpdate_AssigneesCleared_CanceledRead_ReturnsCanceled(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	var requests atomic.Int64
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusOK, getResponseJSON)
	})
	_, err := Update(ctx, client, UpdateInput{FullPath: testFullPath, IID: 1, AssigneeIDs: []int64{}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Update() error = %v, want context.Canceled", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("%d request(s) were answered, want the guard's read abandoned and nothing after it", got)
	}
}
