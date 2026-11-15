// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"errors"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// TestRemovesReference_IsTrueOnlyForASentZero pins the reading of an update
// field that names an epic, a parent or an assignee: 0 removes it, another id
// names one, and a field left out leaves it alone.
func TestRemovesReference_IsTrueOnlyForASentZero(t *testing.T) {
	zero, id := int64(0), int64(5)
	for _, tc := range []struct {
		name string
		id   *int64
		want bool
	}{
		{name: "left out"},
		{name: "zero", id: &zero, want: true},
		{name: "an id", id: &id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemovesReference(tc.id); got != tc.want {
				t.Errorf("RemovesReference() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClearsList_IsTrueOnlyForAnEmptyArray pins the reading of a list an
// update replaces: an explicit empty array removes every entry, and a list
// left out or holding entries does not.
func TestClearsList_IsTrueOnlyForAnEmptyArray(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []int64
		want bool
	}{
		{name: "left out"},
		{name: "empty", ids: []int64{}, want: true},
		{name: "an entry", ids: []int64{3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClearsList(tc.ids); got != tc.want {
				t.Errorf("ClearsList(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}

// TestClearsAssigneeIDs_IsTrueForTheListsGitLabEmpties pins the reading of
// the assignee_ids of a REST update: an empty array removes every assignee,
// and so does a list whose first id is 0, which GitLab reads as nobody alone
// and keeps alone on an item that takes a single assignee. A list left out,
// or one whose first id is a user, does not.
func TestClearsAssigneeIDs_IsTrueForTheListsGitLabEmpties(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []int64
		want bool
	}{
		{name: "left out"},
		{name: "empty", ids: []int64{}, want: true},
		{name: "zero alone", ids: []int64{0}, want: true},
		{name: "zero first", ids: []int64{0, 9}, want: true},
		{name: "a user", ids: []int64{3}},
		{name: "a user first", ids: []int64{9, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClearsAssigneeIDs(tc.ids); got != tc.want {
				t.Errorf("ClearsAssigneeIDs(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}

// parentQuestion is the question the parent removal tests ask, an epic's.
func parentQuestion() i18n.Message {
	return i18n.New(i18n.ConfirmEpicParentRemove, i18n.S("path", EscapeConsentValue("my-group")), i18n.N("iid", 7))
}

// TestConfirmParentRemoval_AsksOnlyWhenTheFieldRemoves verifies that a
// removal is refused on a client that cannot ask, with the question and the
// way to proceed, and that a call that does not remove, or that the operator
// or the caller confirmed, proceeds.
func TestConfirmParentRemoval_AsksOnlyWhenTheFieldRemoves(t *testing.T) {
	for _, tc := range []struct {
		name    string
		removes bool
		yolo    string
		want    string
	}{
		{name: "no removal"},
		{name: "a removal in YOLO mode", removes: true, yolo: "true"},
		{
			name: "a removal", removes: true,
			want: "epicUpdate: Take epic `my-group`&7 out of its parent epic?" +
				" The epic is no longer a child of its parent, as when it is removed from the parent." +
				" Re-send the same call with confirm=true once the user has approved it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", tc.yolo)
			t.Setenv("AUTOPILOT", "")
			err := ConfirmParentRemoval(t.Context(), "epicUpdate", tc.removes, "parent_id", parentQuestion())
			if tc.want == "" {
				if err != nil {
					t.Errorf("error = %v, want the call to proceed", err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tc.want)
			}
		})
	}
}

// assigneesQuestion renders the issue question around losses.
func assigneesQuestion(losses i18n.Message) i18n.Message {
	return i18n.New(i18n.ConfirmIssueAssigneesClear, i18n.L("losses", losses), i18n.N("iid", 7),
		i18n.M("target", ConfirmTargetProject("42")))
}

// TestConfirmAssigneesCleared_AsksWhenSomebodyWouldBeRemoved verifies each
// way the guard answers: a call that clears nothing and a confirmed call read
// nothing, a list that is already empty proceeds unasked, and a list with
// assignees, or one that could not be read, asks with what it would remove.
func TestConfirmAssigneesCleared_AsksWhenSomebodyWouldBeRemoved(t *testing.T) {
	const tail = " in project `42`? This replaces the list and cannot be undone from the response." +
		" Re-send the same call with confirm=true once the user has approved it"
	for _, tc := range []struct {
		name      string
		clears    bool
		yolo      string
		assigned  int
		readErr   error
		wantReads int
		want      string
	}{
		{name: "nothing cleared"},
		{name: "cleared in YOLO mode", clears: true, yolo: "true"},
		{name: "an empty list cleared", clears: true, wantReads: 1},
		{
			name: "two assignees removed", clears: true, assigned: 2, wantReads: 1,
			want: "issueUpdate: Remove all 2 assignees from issue 7" + tail,
		},
		{
			name: "a list that could not be read", clears: true, readErr: errors.New("503"), wantReads: 1,
			want: "issueUpdate: Remove every assignee (the current list could not be read) from issue 7" + tail,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", tc.yolo)
			t.Setenv("AUTOPILOT", "")
			reads := 0
			count := func(context.Context) (int, error) {
				reads++
				return tc.assigned, tc.readErr
			}
			err := ConfirmAssigneesCleared(t.Context(), "issueUpdate", tc.clears, assigneesQuestion, count)
			if reads != tc.wantReads {
				t.Errorf("the guard read the item %d time(s), want %d", reads, tc.wantReads)
			}
			if tc.want == "" {
				if err != nil {
					t.Errorf("error = %v, want the call to proceed", err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tc.want)
			}
		})
	}
}
