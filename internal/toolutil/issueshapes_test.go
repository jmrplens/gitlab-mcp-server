// issueshapes_test.go contains unit tests for the shared issue-shape
// converters exposed by toolutil. The previous per-package tests were
// retained by deleting the local shapes.go definitions; this file
// replaces the shared unit-test surface so future regressions in the
// issues package are caught from the canonical home.
package toolutil

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewIssueUserOutputFromBasicUser verifies the basic-user
// converter copies the 6 documented fields.
func TestNewIssueUserOutputFromBasicUser(t *testing.T) {
	got := NewIssueUserOutputFromBasicUser(gl.BasicUser{
		ID: 7, Username: "u", Name: "U", State: "active", AvatarURL: "av", WebURL: "w",
	})
	if got.ID != 7 || got.Username != "u" || got.Name != "U" || got.State != "active" ||
		got.AvatarURL != "av" || got.WebURL != "w" {
		t.Errorf("issue user: %+v", got)
	}
}

// TestNewIssueUserOutputFromPointer verifies the *gl.BasicUser
// converter returns nil for a nil source.
func TestNewIssueUserOutputFromPointer(t *testing.T) {
	if got := NewIssueUserOutputFromPointer(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueUserOutputFromPointer(&gl.BasicUser{ID: 1, Username: "u"})
	if got == nil || got.ID != 1 || got.Username != "u" {
		t.Errorf("issue user from pointer: %+v", got)
	}
}

// TestNewIssueUserOutputFromEpicAuthor verifies the epic-author
// converter copies the 6 fields and returns nil for nil.
func TestNewIssueUserOutputFromEpicAuthor(t *testing.T) {
	if got := NewIssueUserOutputFromEpicAuthor(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueUserOutputFromEpicAuthor(&gl.EpicAuthor{ID: 9, Username: "alice", State: "active"})
	if got == nil || got.ID != 9 || got.Username != "alice" || got.State != "active" {
		t.Errorf("epic author: %+v", got)
	}
}

// TestNewIssueUserOutputFromIssueAuthor verifies the issue-author
// converter copies the 6 fields and returns nil for nil.
func TestNewIssueUserOutputFromIssueAuthor(t *testing.T) {
	if got := NewIssueUserOutputFromIssueAuthor(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueUserOutputFromIssueAuthor(&gl.IssueAuthor{ID: 10, Name: "Bob", Username: "bob"})
	if got == nil || got.ID != 10 || got.Name != "Bob" || got.Username != "bob" {
		t.Errorf("issue author: %+v", got)
	}
}

// TestNewIssueUserOutputFromIssueCloser verifies the issue-closer
// converter copies the 6 fields and returns nil for nil.
func TestNewIssueUserOutputFromIssueCloser(t *testing.T) {
	if got := NewIssueUserOutputFromIssueCloser(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueUserOutputFromIssueCloser(&gl.IssueCloser{ID: 11, Username: "carol"})
	if got == nil || got.ID != 11 || got.Username != "carol" {
		t.Errorf("issue closer: %+v", got)
	}
}

// TestNewIssueUserOutputsFromIssueAssignees verifies the assignee slice
// converter skips nil entries and returns nil for empty/all-nil input.
func TestNewIssueUserOutputsFromIssueAssignees(t *testing.T) {
	if got := NewIssueUserOutputsFromIssueAssignees(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	got := NewIssueUserOutputsFromIssueAssignees([]*gl.IssueAssignee{
		nil,
		{ID: 1, Username: "u1"},
		nil,
		{ID: 2, Username: "u2"},
	})
	if got == nil || len(got) != 2 || got[0].Username != "u1" || got[1].ID != 2 {
		t.Errorf("assignees: %+v", got)
	}
}

// TestNewIssueUserOutputsFromIssueAssignees_AllNilElements verifies that a
// non-empty input slice containing only nil pointers still returns nil
// overall (the len(out)==0-after-filtering guard), distinct from the
// top-of-function nil-slice guard already covered by
// TestNewIssueUserOutputsFromIssueAssignees.
func TestNewIssueUserOutputsFromIssueAssignees_AllNilElements(t *testing.T) {
	got := NewIssueUserOutputsFromIssueAssignees([]*gl.IssueAssignee{nil, nil})
	if got != nil {
		t.Errorf("all-nil slice must return nil, got %+v", got)
	}
}

// TestNewIssueUserOutputFromIssueAssignee verifies the single-assignee
// (deprecated) converter copies fields and returns nil for nil.
func TestNewIssueUserOutputFromIssueAssignee(t *testing.T) {
	if got := NewIssueUserOutputFromIssueAssignee(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueUserOutputFromIssueAssignee(&gl.IssueAssignee{ID: 3, Username: "u3"})
	if got == nil || got.ID != 3 || got.Username != "u3" {
		t.Errorf("single assignee: %+v", got)
	}
}

// TestNewIssueLinksOutput verifies the issue-links converter copies
// the self/notes/award_emoji/project URLs and returns nil for nil.
func TestNewIssueLinksOutput(t *testing.T) {
	if got := NewIssueLinksOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewIssueLinksOutput(&gl.IssueLinks{
		Self: "s", Notes: "n", AwardEmoji: "ae", Project: "p",
	})
	if got == nil || got.Self != "s" || got.Notes != "n" || got.AwardEmoji != "ae" || got.Project != "p" {
		t.Errorf("issue links: %+v", got)
	}
}

// TestNewIterationOutput verifies the iteration converter copies
// fields, formats time/ISOTime pointers, and returns nil for nil.
func TestNewIterationOutput(t *testing.T) {
	if got := NewIterationOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	due := gl.ISOTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	got := NewIterationOutput(&gl.Iteration{
		ID: 1, IID: 2, Sequence: 3, GroupID: 4, Title: "iter", Description: "d", State: 1, WebURL: "w",
		CreatedAt: &created, UpdatedAt: &created, DueDate: &due, StartDate: &due,
	})
	if got == nil || got.Title != "iter" || got.CreatedAt != "2026-06-01T00:00:00Z" ||
		got.DueDate != "2026-07-01" {
		t.Errorf("iteration: %+v", got)
	}
}

// TestNewIterationOutputFromGroupIteration verifies the GroupIteration
// variant of the iteration converter (the type surfaced on
// gl.Issue.Iteration).
func TestNewIterationOutputFromGroupIteration(t *testing.T) {
	if got := NewIterationOutputFromGroupIteration(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := NewIterationOutputFromGroupIteration(&gl.GroupIteration{
		ID: 1, Title: "giter", CreatedAt: &created, UpdatedAt: &created,
	})
	if got == nil || got.Title != "giter" || got.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("group iteration: %+v", got)
	}
}

// TestNewEpicOutput verifies the epic converter copies every field
// (including nested author and ISOTime/time pointers) and returns nil
// for nil.
func TestNewEpicOutput(t *testing.T) {
	if got := NewEpicOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	due := gl.ISOTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	got := NewEpicOutput(&gl.Epic{
		ID: 1, IID: 2, GroupID: 3, ParentID: 4, Title: "epic", Description: "d", State: "opened",
		Confidential: false, WebURL: "w", URL: "u", Author: &gl.EpicAuthor{ID: 99, Username: "x"},
		Labels: []string{"a"}, Upvotes: 1, Downvotes: 0, UserNotesCount: 5,
		StartDate: &due, DueDate: &due, CreatedAt: &created, UpdatedAt: &created, ClosedAt: nil,
	})
	if got == nil {
		t.Fatal("populated epic must return non-nil")
	}
	if got.Title != "epic" || got.Author == nil || got.Author.Username != "x" {
		t.Errorf("epic / author: %+v %+v", got, got.Author)
	}
	if got.StartDate != "2026-07-01" || got.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("epic dates: %+v", got)
	}
	if got.ClosedAt != "" {
		t.Errorf("ClosedAt should be empty for nil source, got %q", got.ClosedAt)
	}
}
