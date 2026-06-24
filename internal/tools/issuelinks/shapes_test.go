// shapes_test.go covers the additive 1:1 sub-object converters and the
// full-field relation/link converters for the issuelinks package.
package issuelinks

import (
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// isoTimePtr returns a *gl.ISOTime for the given date, for building fixtures.
func isoTimePtr(year int, month time.Month, day int) *gl.ISOTime {
	t := gl.ISOTime(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
	return &t
}

// timePtr returns a *time.Time for the given date, for building fixtures.
func timePtr(year int, month time.Month, day int) *time.Time {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &t
}

// TestFormatTimePtr verifies RFC 3339 rendering and the nil branch.
func TestFormatTimePtr(t *testing.T) {
	if got := formatTimePtr(nil); got != "" {
		t.Errorf("formatTimePtr(nil) = %q, want empty", got)
	}
	got := formatTimePtr(timePtr(2026, time.January, 2))
	if got != "2026-01-02T00:00:00Z" {
		t.Errorf("formatTimePtr = %q", got)
	}
}

// TestFormatISOTimePtr verifies YYYY-MM-DD rendering and the nil branch.
func TestFormatISOTimePtr(t *testing.T) {
	if got := formatISOTimePtr(nil); got != "" {
		t.Errorf("formatISOTimePtr(nil) = %q, want empty", got)
	}
	got := formatISOTimePtr(isoTimePtr(2026, time.March, 4))
	if got != "2026-03-04" {
		t.Errorf("formatISOTimePtr = %q", got)
	}
}

// TestAuthorOutput verifies the author converter for nil and populated input.
func TestAuthorOutput(t *testing.T) {
	if got := authorOutput(nil); got != nil {
		t.Errorf("authorOutput(nil) = %v, want nil", got)
	}
	got := authorOutput(&gl.IssueAuthor{ID: 7, State: "active", WebURL: "u", Name: "Ann", AvatarURL: "a", Username: "ann"})
	if got == nil || got.ID != 7 || got.Username != "ann" || got.State != "active" || got.WebURL != "u" || got.Name != "Ann" || got.AvatarURL != "a" {
		t.Errorf("authorOutput = %+v", got)
	}
}

// TestAssigneeOutput verifies the single-assignee converter branches.
func TestAssigneeOutput(t *testing.T) {
	if got := assigneeOutput(nil); got != nil {
		t.Errorf("assigneeOutput(nil) = %v, want nil", got)
	}
	got := assigneeOutput(&gl.IssueAssignee{ID: 3, Username: "bob"})
	if got == nil || got.ID != 3 || got.Username != "bob" {
		t.Errorf("assigneeOutput = %+v", got)
	}
}

// TestAssigneeOutputs verifies the slice converter for empty, nil-element, and
// populated inputs.
func TestAssigneeOutputs(t *testing.T) {
	if got := assigneeOutputs(nil); got != nil {
		t.Errorf("assigneeOutputs(nil) = %v, want nil", got)
	}
	in := []*gl.IssueAssignee{
		{ID: 1, Username: "a"},
		nil,
		{ID: 2, Username: "b"},
	}
	got := assigneeOutputs(in)
	if len(got) != 2 {
		t.Fatalf("assigneeOutputs len = %d, want 2", len(got))
	}
	if got[0].Username != "a" || got[1].Username != "b" {
		t.Errorf("assigneeOutputs = %+v", got)
	}
}

// TestReferencesOutput verifies the references converter branches.
func TestReferencesOutput(t *testing.T) {
	if got := referencesOutput(nil); got != nil {
		t.Errorf("referencesOutput(nil) = %v, want nil", got)
	}
	got := referencesOutput(&gl.IssueReferences{Short: "s", Relative: "r", Full: "f"})
	if got == nil || got.Short != "s" || got.Relative != "r" || got.Full != "f" {
		t.Errorf("referencesOutput = %+v", got)
	}
}

// TestMilestoneOutput verifies the milestone converter for nil and populated
// input, including the ISO/RFC date and Expired pointer fields.
func TestMilestoneOutput(t *testing.T) {
	if got := milestoneOutput(nil); got != nil {
		t.Errorf("milestoneOutput(nil) = %v, want nil", got)
	}
	expired := new(bool) // zero value: false
	got := milestoneOutput(&gl.Milestone{
		ID: 5, IID: 1, GroupID: 9, ProjectID: 42, Title: "M1", Description: "d",
		StartDate: isoTimePtr(2026, time.January, 1), DueDate: isoTimePtr(2026, time.February, 1),
		State: "active", WebURL: "w",
		UpdatedAt: timePtr(2026, time.January, 3), CreatedAt: timePtr(2026, time.January, 2),
		Expired: expired,
	})
	if got == nil || got.ID != 5 || got.Title != "M1" || got.StartDate != "2026-01-01" ||
		got.DueDate != "2026-02-01" || got.CreatedAt != "2026-01-02T00:00:00Z" ||
		got.UpdatedAt != "2026-01-03T00:00:00Z" || got.Expired == nil || *got.Expired {
		t.Errorf("milestoneOutput = %+v", got)
	}
}

// TestIssueRefOutput verifies the source/target issue converter for nil and a
// fully populated SDK issue.
func TestIssueRefOutput(t *testing.T) {
	if got := issueRefOutput(nil); got != nil {
		t.Errorf("issueRefOutput(nil) = %v, want nil", got)
	}
	got := issueRefOutput(&gl.Issue{
		ID: 50, IID: 10, ProjectID: 42, Title: "Source", Description: "desc",
		State: "opened", Confidential: true, Labels: gl.Labels{"bug", "urgent"},
		WebURL: "w", CreatedAt: timePtr(2026, time.January, 1),
		UpdatedAt: timePtr(2026, time.January, 2), ClosedAt: timePtr(2026, time.January, 3),
		DueDate: isoTimePtr(2026, time.February, 1), Weight: 4,
	})
	if got == nil || got.ID != 50 || got.IID != 10 || got.ProjectID != 42 ||
		got.Title != "Source" || got.Description != "desc" || got.State != "opened" ||
		!got.Confidential || len(got.Labels) != 2 || got.WebURL != "w" ||
		got.CreatedAt != "2026-01-01T00:00:00Z" || got.UpdatedAt != "2026-01-02T00:00:00Z" ||
		got.ClosedAt != "2026-01-03T00:00:00Z" || got.DueDate != "2026-02-01" || got.Weight != 4 {
		t.Errorf("issueRefOutput = %+v", got)
	}
}

// TestToRelationOutput_AllNestedObjects verifies the relation converter surfaces
// every 1:1 field including the full nested sub-objects and []string labels.
func TestToRelationOutput_AllNestedObjects(t *testing.T) {
	r := &gl.IssueRelation{
		ID: 100, IID: 8, State: "opened", Description: "rel desc", Confidential: true,
		Author:         &gl.IssueAuthor{ID: 1, Username: "ann"},
		Milestone:      &gl.Milestone{ID: 5, Title: "M1"},
		ProjectID:      42,
		Assignees:      []*gl.IssueAssignee{{ID: 2, Username: "bob"}},
		Assignee:       &gl.IssueAssignee{ID: 2, Username: "bob"},
		UpdatedAt:      timePtr(2026, time.January, 2),
		Title:          "Related",
		CreatedAt:      timePtr(2026, time.January, 1),
		Labels:         gl.Labels{"a", "b"},
		DueDate:        isoTimePtr(2026, time.February, 1),
		WebURL:         "w",
		References:     &gl.IssueReferences{Short: "s", Relative: "r", Full: "f"},
		Weight:         3,
		UserNotesCount: 7,
		IssueLinkID:    99,
		LinkType:       "relates_to",
		LinkCreatedAt:  timePtr(2026, time.January, 4),
		LinkUpdatedAt:  timePtr(2026, time.January, 5),
	}
	want := RelationOutput{
		ID: 100, IID: 8, State: "opened", Description: "rel desc", Confidential: true,
		Author:         &UserOutput{ID: 1, Username: "ann"},
		Milestone:      &MilestoneOutput{ID: 5, Title: "M1"},
		ProjectID:      42,
		Assignees:      []*UserOutput{{ID: 2, Username: "bob"}},
		Assignee:       &UserOutput{ID: 2, Username: "bob"},
		UpdatedAt:      "2026-01-02T00:00:00Z",
		Title:          "Related",
		CreatedAt:      "2026-01-01T00:00:00Z",
		Labels:         []string{"a", "b"},
		DueDate:        "2026-02-01",
		WebURL:         "w",
		References:     &ReferencesOutput{Short: "s", Relative: "r", Full: "f"},
		Weight:         3,
		UserNotesCount: 7,
		IssueLinkID:    99,
		LinkType:       "relates_to",
		LinkCreatedAt:  "2026-01-04T00:00:00Z",
		LinkUpdatedAt:  "2026-01-05T00:00:00Z",
	}
	if out := toRelationOutput(r); !reflect.DeepEqual(out, want) {
		t.Errorf("toRelationOutput = %+v, want %+v", out, want)
	}
}

// TestToRelationOutput_NilSubObjects verifies the relation converter handles a
// relation with all optional sub-objects and timestamps absent.
func TestToRelationOutput_NilSubObjects(t *testing.T) {
	out := toRelationOutput(&gl.IssueRelation{ID: 1, IID: 2, Title: "Bare"})
	if out.Author != nil || out.Milestone != nil || out.Assignee != nil ||
		out.Assignees != nil || out.References != nil ||
		out.CreatedAt != "" || out.UpdatedAt != "" || out.DueDate != "" ||
		out.LinkCreatedAt != "" || out.LinkUpdatedAt != "" {
		t.Errorf("toRelationOutput (bare) = %+v", out)
	}
}

// TestToOutput_FullAndNilIssues verifies the single-link converter surfaces the
// full source/target issue objects and tolerates missing endpoints.
func TestToOutput_FullAndNilIssues(t *testing.T) {
	full := toOutput(&gl.IssueLink{
		ID:          99,
		LinkType:    "relates_to",
		SourceIssue: &gl.Issue{ID: 50, IID: 10, ProjectID: 42, Title: "Source"},
		TargetIssue: &gl.Issue{ID: 80, IID: 20, ProjectID: 43, Title: "Target"},
	})
	if full.SourceIssue == nil || full.SourceIssue.Title != "Source" || full.SourceIssueIID != 10 ||
		full.TargetIssue == nil || full.TargetIssue.Title != "Target" || full.TargetProjectID != 43 {
		t.Errorf("toOutput (full) = %+v", full)
	}

	bare := toOutput(&gl.IssueLink{ID: 1, LinkType: "blocks"})
	if bare.SourceIssue != nil || bare.TargetIssue != nil ||
		bare.SourceIssueIID != 0 || bare.TargetIssueIID != 0 {
		t.Errorf("toOutput (bare) = %+v", bare)
	}
}

// TestIssueRefSuffix verifies the markdown helper for nil, empty-title, and
// populated issue references.
func TestIssueRefSuffix(t *testing.T) {
	if got := issueRefSuffix(nil); got != "" {
		t.Errorf("issueRefSuffix(nil) = %q, want empty", got)
	}
	if got := issueRefSuffix(&IssueRefOutput{Title: ""}); got != "" {
		t.Errorf("issueRefSuffix(empty title) = %q, want empty", got)
	}
	if got := issueRefSuffix(&IssueRefOutput{Title: "Hello"}); got != " — Hello" {
		t.Errorf("issueRefSuffix = %q", got)
	}
}

// TestFormatListMarkdown_WithAuthor verifies the list table renders the author
// column when the relation carries an author object.
func TestFormatListMarkdown_WithAuthor(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Relations: []RelationOutput{
		{ID: 1, IID: 2, Title: "T", State: "opened", LinkType: "relates_to", IssueLinkID: 9, Author: &UserOutput{Username: "ann"}},
	}})
	if !strings.Contains(md, "ann") {
		t.Errorf("FormatListMarkdown missing author: %s", md)
	}
}
