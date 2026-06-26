// shapes_test.go covers the additive 1:1 sub-object converters and the
// full-field relation/link converters for the issuelinks package.
package issuelinks

import (
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	if got := toolutil.FormatTimePtr(nil); got != "" {
		t.Errorf("toolutil.FormatTimePtr(nil) = %q, want empty", got)
	}
	got := toolutil.FormatTimePtr(timePtr(2026, time.January, 2))
	if got != "2026-01-02T00:00:00Z" {
		t.Errorf("formatTimePtr = %q", got)
	}
}

// TestFormatISOTimePtr verifies YYYY-MM-DD rendering and the nil branch.
func TestFormatISOTimePtr(t *testing.T) {
	if got := toolutil.FormatISOTimePtr(nil); got != "" {
		t.Errorf("toolutil.FormatISOTimePtr(nil) = %q, want empty", got)
	}
	got := toolutil.FormatISOTimePtr(isoTimePtr(2026, time.March, 4))
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

// TestIssueRefOutput_Nil verifies the converter returns nil for a nil SDK issue.
func TestIssueRefOutput_Nil(t *testing.T) {
	if got := issueRefOutput(nil); got != nil {
		t.Errorf("issueRefOutput(nil) = %v, want nil", got)
	}
}

// TestIssueRefOutput_Full verifies the converter mirrors the full gitlab.Issue
// shape: every scalar, the dereferenced *string issue_type, and every nested
// sub-object (author/assignees/assignee/closed_by/milestone/references/epic/
// iteration/time_stats/task_completion_status/label_details/_links). It also
// confirms nil elements in the assignees and label_details slices are skipped.
func TestIssueRefOutput_Full(t *testing.T) {
	got := issueRefOutput(&gl.Issue{
		ID: 50, IID: 10, ExternalID: "ext-1", ProjectID: 42, Title: "Source",
		Description: "desc", State: "opened", HealthStatus: "on_track",
		Confidential: true, Labels: gl.Labels{"bug", "urgent"}, WebURL: "w",
		CreatedAt: timePtr(2026, time.January, 1), UpdatedAt: timePtr(2026, time.January, 2),
		ClosedAt: timePtr(2026, time.January, 3), DueDate: isoTimePtr(2026, time.February, 1),
		Weight: 4, MovedToID: 77, Upvotes: 5, Downvotes: 1, DiscussionLocked: true,
		Subscribed: true, UserNotesCount: 9, IssueLinkID: 99, MergeRequestCount: 2,
		EpicIssueID: 88, ServiceDeskReplyTo: "reply@example.com",
		IssueType:    new("incident"),
		Author:       &gl.IssueAuthor{ID: 1, Username: "ann"},
		Assignees:    []*gl.IssueAssignee{{ID: 2, Username: "bob"}, nil},
		Assignee:     &gl.IssueAssignee{ID: 2, Username: "bob"},
		ClosedBy:     &gl.IssueCloser{ID: 3, Username: "carol"},
		Milestone:    &gl.Milestone{ID: 5, Title: "M1"},
		References:   &gl.IssueReferences{Short: "s", Relative: "r", Full: "f"},
		LabelDetails: []*gl.LabelDetails{{ID: 11, Name: "bug", Color: "#f00"}, nil},
		TimeStats: &gl.TimeStats{
			HumanTimeEstimate: "1h", HumanTotalTimeSpent: "30m",
			TimeEstimate: 3600, TotalTimeSpent: 1800,
		},
		TaskCompletionStatus: &gl.TasksCompletionStatus{Count: 4, CompletedCount: 2},
		Links:                &gl.IssueLinks{Self: "self", Notes: "notes", AwardEmoji: "ae", Project: "proj"},
		Iteration: &gl.GroupIteration{
			ID: 30, IID: 3, Sequence: 1, GroupID: 9, Title: "Sprint 1", State: 2, WebURL: "iw",
			CreatedAt: timePtr(2026, time.January, 1), StartDate: isoTimePtr(2026, time.January, 1),
			DueDate: isoTimePtr(2026, time.January, 14),
		},
		Epic: &gl.Epic{
			ID: 60, IID: 6, GroupID: 9, Title: "Epic", State: "opened",
			Author: &gl.EpicAuthor{ID: 4, Username: "dan"}, Labels: []string{"x"},
			StartDate: isoTimePtr(2026, time.January, 1), DueDate: isoTimePtr(2026, time.March, 1),
		},
	})
	want := &IssueRefOutput{
		ID: 50, IID: 10, ExternalID: "ext-1", ProjectID: 42, Title: "Source",
		Description: "desc", State: "opened", HealthStatus: "on_track",
		Confidential: true, Labels: []string{"bug", "urgent"}, WebURL: "w",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
		ClosedAt: "2026-01-03T00:00:00Z", DueDate: "2026-02-01",
		Weight: 4, MovedToID: 77, Upvotes: 5, Downvotes: 1, DiscussionLocked: true,
		Subscribed: true, UserNotesCount: 9, IssueLinkID: 99, MergeRequestCount: 2,
		EpicIssueID: 88, ServiceDeskReplyTo: "reply@example.com", IssueType: "incident",
		Author:               &UserOutput{ID: 1, Username: "ann"},
		Assignees:            []*UserOutput{{ID: 2, Username: "bob"}},
		Assignee:             &UserOutput{ID: 2, Username: "bob"},
		ClosedBy:             &UserOutput{ID: 3, Username: "carol"},
		Milestone:            &MilestoneOutput{ID: 5, Title: "M1"},
		References:           &ReferencesOutput{Short: "s", Relative: "r", Full: "f"},
		LabelDetails:         []*LabelDetailsOutput{{ID: 11, Name: "bug", Color: "#f00"}},
		TimeStats:            &TimeStatsOutput{HumanTimeEstimate: "1h", HumanTotalTimeSpent: "30m", TimeEstimate: 3600, TotalTimeSpent: 1800},
		TaskCompletionStatus: &TaskCompletionStatusOutput{Count: 4, CompletedCount: 2},
		Links:                &LinksOutput{Self: "self", Notes: "notes", AwardEmoji: "ae", Project: "proj"},
		Iteration: &IterationOutput{
			ID: 30, IID: 3, Sequence: 1, GroupID: 9, Title: "Sprint 1", State: 2, WebURL: "iw",
			CreatedAt: "2026-01-01T00:00:00Z", StartDate: "2026-01-01", DueDate: "2026-01-14",
		},
		Epic: &EpicOutput{
			ID: 60, IID: 6, GroupID: 9, Title: "Epic", State: "opened",
			Author: &UserOutput{ID: 4, Username: "dan"}, Labels: []string{"x"},
			StartDate: "2026-01-01", DueDate: "2026-03-01",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("issueRefOutput =\n%+v\nwant\n%+v", got, want)
	}
}

// TestIssueRefOutput_NilSubObjects verifies a bare SDK issue yields nil for all
// optional nested objects, empty timestamps, and an empty issue_type (nil
// *string branch).
func TestIssueRefOutput_NilSubObjects(t *testing.T) {
	got := issueRefOutput(&gl.Issue{ID: 1, IID: 2, Title: "Bare"})
	if got == nil {
		t.Fatal("issueRefOutput = nil, want non-nil")
	}
	if got.Author != nil || got.Assignees != nil || got.Assignee != nil ||
		got.ClosedBy != nil || got.Milestone != nil || got.References != nil ||
		got.LabelDetails != nil || got.TimeStats != nil || got.TaskCompletionStatus != nil ||
		got.Links != nil || got.Iteration != nil || got.Epic != nil ||
		got.CreatedAt != "" || got.UpdatedAt != "" || got.ClosedAt != "" || got.DueDate != "" ||
		got.IssueType != "" {
		t.Errorf("issueRefOutput (bare) = %+v", got)
	}
}

// TestCloserOutput verifies the closer converter branches.
func TestCloserOutput(t *testing.T) {
	if got := closerOutput(nil); got != nil {
		t.Errorf("closerOutput(nil) = %v, want nil", got)
	}
	got := closerOutput(&gl.IssueCloser{ID: 3, State: "active", WebURL: "u", Name: "Carol", AvatarURL: "a", Username: "carol"})
	if got == nil || got.ID != 3 || got.Username != "carol" || got.State != "active" ||
		got.WebURL != "u" || got.Name != "Carol" || got.AvatarURL != "a" {
		t.Errorf("closerOutput = %+v", got)
	}
}

// TestEpicAuthorOutput verifies the epic-author converter branches.
func TestEpicAuthorOutput(t *testing.T) {
	if got := epicAuthorOutput(nil); got != nil {
		t.Errorf("epicAuthorOutput(nil) = %v, want nil", got)
	}
	got := epicAuthorOutput(&gl.EpicAuthor{ID: 4, State: "active", WebURL: "u", Name: "Dan", AvatarURL: "a", Username: "dan"})
	if got == nil || got.ID != 4 || got.Username != "dan" || got.State != "active" ||
		got.WebURL != "u" || got.Name != "Dan" || got.AvatarURL != "a" {
		t.Errorf("epicAuthorOutput = %+v", got)
	}
}

// TestLinksOutput verifies the _links converter branches.
func TestLinksOutput(t *testing.T) {
	if got := linksOutput(nil); got != nil {
		t.Errorf("linksOutput(nil) = %v, want nil", got)
	}
	got := linksOutput(&gl.IssueLinks{Self: "s", Notes: "n", AwardEmoji: "ae", Project: "p"})
	if got == nil || got.Self != "s" || got.Notes != "n" || got.AwardEmoji != "ae" || got.Project != "p" {
		t.Errorf("linksOutput = %+v", got)
	}
}

// TestTimeStatsOutput verifies the time-stats converter branches.
func TestTimeStatsOutput(t *testing.T) {
	if got := timeStatsOutput(nil); got != nil {
		t.Errorf("timeStatsOutput(nil) = %v, want nil", got)
	}
	got := timeStatsOutput(&gl.TimeStats{HumanTimeEstimate: "1h", HumanTotalTimeSpent: "30m", TimeEstimate: 3600, TotalTimeSpent: 1800})
	if got == nil || got.HumanTimeEstimate != "1h" || got.HumanTotalTimeSpent != "30m" ||
		got.TimeEstimate != 3600 || got.TotalTimeSpent != 1800 {
		t.Errorf("timeStatsOutput = %+v", got)
	}
}

// TestTaskCompletionStatusOutput verifies the task-completion converter branches.
func TestTaskCompletionStatusOutput(t *testing.T) {
	if got := taskCompletionStatusOutput(nil); got != nil {
		t.Errorf("taskCompletionStatusOutput(nil) = %v, want nil", got)
	}
	got := taskCompletionStatusOutput(&gl.TasksCompletionStatus{Count: 4, CompletedCount: 2})
	if got == nil || got.Count != 4 || got.CompletedCount != 2 {
		t.Errorf("taskCompletionStatusOutput = %+v", got)
	}
}

// TestLabelDetailsOutputs verifies the slice converter for empty, nil-element,
// and populated inputs.
func TestLabelDetailsOutputs(t *testing.T) {
	if got := labelDetailsOutputs(nil); got != nil {
		t.Errorf("labelDetailsOutputs(nil) = %v, want nil", got)
	}
	in := []*gl.LabelDetails{
		{ID: 1, Name: "bug", Color: "#f00", Description: "d", DescriptionHTML: "h", TextColor: "#000"},
		nil,
		{ID: 2, Name: "ux"},
	}
	got := labelDetailsOutputs(in)
	if len(got) != 2 {
		t.Fatalf("labelDetailsOutputs len = %d, want 2", len(got))
	}
	if got[0].Name != "bug" || got[0].Color != "#f00" || got[0].DescriptionHTML != "h" ||
		got[0].TextColor != "#000" || got[1].Name != "ux" {
		t.Errorf("labelDetailsOutputs = %+v", got)
	}
}

// TestIterationOutput verifies the iteration converter for nil and populated
// input, including the ISO/RFC date fields.
func TestIterationOutput(t *testing.T) {
	if got := iterationOutput(nil); got != nil {
		t.Errorf("iterationOutput(nil) = %v, want nil", got)
	}
	got := iterationOutput(&gl.GroupIteration{
		ID: 30, IID: 3, Sequence: 1, GroupID: 9, Title: "Sprint 1", Description: "d",
		State: 2, WebURL: "w", CreatedAt: timePtr(2026, time.January, 1),
		UpdatedAt: timePtr(2026, time.January, 2), StartDate: isoTimePtr(2026, time.January, 1),
		DueDate: isoTimePtr(2026, time.January, 14),
	})
	if got == nil || got.ID != 30 || got.IID != 3 || got.Sequence != 1 || got.GroupID != 9 ||
		got.Title != "Sprint 1" || got.Description != "d" || got.State != 2 || got.WebURL != "w" ||
		got.CreatedAt != "2026-01-01T00:00:00Z" || got.UpdatedAt != "2026-01-02T00:00:00Z" ||
		got.StartDate != "2026-01-01" || got.DueDate != "2026-01-14" {
		t.Errorf("iterationOutput = %+v", got)
	}
}

// TestEpicOutput verifies the epic converter for nil and a fully populated SDK
// epic, including the nested author and all date fields.
func TestEpicOutput(t *testing.T) {
	if got := epicOutput(nil); got != nil {
		t.Errorf("epicOutput(nil) = %v, want nil", got)
	}
	got := epicOutput(&gl.Epic{
		ID: 60, IID: 6, GroupID: 9, ParentID: 1, Title: "Epic", Description: "d",
		State: "opened", Confidential: true, WebURL: "w", URL: "u",
		Author: &gl.EpicAuthor{ID: 4, Username: "dan"}, Labels: []string{"x", "y"},
		Upvotes: 3, Downvotes: 1, UserNotesCount: 5,
		StartDate: isoTimePtr(2026, time.January, 1), StartDateIsFixed: true,
		StartDateFixed: isoTimePtr(2026, time.January, 2), StartDateFromMilestones: isoTimePtr(2026, time.January, 3),
		DueDate: isoTimePtr(2026, time.March, 1), DueDateIsFixed: true,
		DueDateFixed: isoTimePtr(2026, time.March, 2), DueDateFromMilestones: isoTimePtr(2026, time.March, 3),
		CreatedAt: timePtr(2026, time.January, 1), UpdatedAt: timePtr(2026, time.January, 2),
		ClosedAt: timePtr(2026, time.January, 3),
	})
	want := &EpicOutput{
		ID: 60, IID: 6, GroupID: 9, ParentID: 1, Title: "Epic", Description: "d",
		State: "opened", Confidential: true, WebURL: "w", URL: "u",
		Author: &UserOutput{ID: 4, Username: "dan"}, Labels: []string{"x", "y"},
		Upvotes: 3, Downvotes: 1, UserNotesCount: 5,
		StartDate: "2026-01-01", StartDateIsFixed: true, StartDateFixed: "2026-01-02",
		StartDateFromMilestones: "2026-01-03", DueDate: "2026-03-01", DueDateIsFixed: true,
		DueDateFixed: "2026-03-02", DueDateFromMilestones: "2026-03-03",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
		ClosedAt: "2026-01-03T00:00:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("epicOutput =\n%+v\nwant\n%+v", got, want)
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
	if full.SourceIssue == nil || full.SourceIssue.Title != "Source" || full.SourceIssue.IID != 10 ||
		full.TargetIssue == nil || full.TargetIssue.Title != "Target" || full.TargetIssue.ProjectID != 43 {
		t.Errorf("toOutput (full) = %+v", full)
	}

	bare := toOutput(&gl.IssueLink{ID: 1, LinkType: "blocks"})
	if bare.SourceIssue != nil || bare.TargetIssue != nil {
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
