// shapes_test.go covers the additive 1:1 sub-object converters and the
// full-field relation/link converters for the issuelinks package.
package issuelinks

import (
	"reflect"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	if got := issueRefOutput(nil, toolutil.IssueBasicExtra{}); got != nil {
		t.Errorf("issueRefOutput(nil) = %v, want nil", got)
	}
}

// TestIssueRefOutput_Full verifies the converter carries every key IssueBasic
// renders: every scalar, the dereferenced *string issue_type, every nested
// sub-object (author/assignees/assignee/closed_by/milestone/time_stats/
// task_completion_status), and the three keys the capture read. It also
// confirms a nil element in the assignees slice is skipped.
//
// The SDK issue carries every key IssueBasic does not render as well, and the
// comparison is against the whole struct, so none of them can reach the
// output: a converter that read one back would need a field to put it in.
func TestIssueRefOutput_Full(t *testing.T) {
	blocking := int64(3)
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
		Assignee:     &gl.IssueAssignee{ID: 2, Username: "bob"}, //nolint:staticcheck // deprecated SDK field: IssueBasic still renders the first assignee under this key
		ClosedBy:     &gl.IssueCloser{ID: 3, Username: "carol"},
		Milestone:    &gl.Milestone{ID: 5, Title: "M1"},
		References:   &gl.IssueReferences{Short: "s", Relative: "r", Full: "f"},
		LabelDetails: []*gl.LabelDetails{{ID: 11, Name: "bug", Color: "#f00"}},
		TimeStats: &gl.TimeStats{
			HumanTimeEstimate: "1h", HumanTotalTimeSpent: "30m",
			TimeEstimate: 3600, TotalTimeSpent: 1800,
		},
		TaskCompletionStatus: &gl.TasksCompletionStatus{Count: 4, CompletedCount: 2},
		Links:                &gl.IssueLinks{Self: "self", Notes: "notes", AwardEmoji: "ae", Project: "proj"},
		Iteration:            &gl.GroupIteration{ID: 30, Title: "Sprint 1"},
		Epic:                 &gl.Epic{ID: 60, IID: 6, Title: "Epic"},
	}, toolutil.IssueBasicExtra{BlockingIssuesCount: &blocking, StartDate: "2026-01-05", Type: "INCIDENT"})
	want := &IssueRefOutput{
		ID: 50, IID: 10, ProjectID: 42, Title: "Source",
		Description: "desc", State: "opened",
		Confidential: true, Labels: []string{"bug", "urgent"}, WebURL: "w",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
		ClosedAt: "2026-01-03T00:00:00Z", DueDate: "2026-02-01",
		Weight: 4, Upvotes: 5, Downvotes: 1, DiscussionLocked: true,
		UserNotesCount: 9, MergeRequestCount: 2, IssueType: "incident",
		Author:               &UserOutput{ID: 1, Username: "ann"},
		Assignees:            []*UserOutput{{ID: 2, Username: "bob"}},
		Assignee:             &UserOutput{ID: 2, Username: "bob"},
		ClosedBy:             &UserOutput{ID: 3, Username: "carol"},
		Milestone:            &MilestoneOutput{ID: 5, Title: "M1"},
		TimeStats:            &TimeStatsOutput{HumanTimeEstimate: "1h", HumanTotalTimeSpent: "30m", TimeEstimate: 3600, TotalTimeSpent: 1800},
		TaskCompletionStatus: &TaskCompletionStatusOutput{Count: 4, CompletedCount: 2},
		BlockingIssuesCount:  &blocking,
		StartDate:            "2026-01-05",
		Type:                 "INCIDENT",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("issueRefOutput =\n%+v\nwant\n%+v", got, want)
	}
}

// TestIssueRefOutput_NilSubObjects verifies a bare SDK issue yields nil for all
// optional nested objects, empty timestamps, and an empty issue_type (nil
// *string branch).
func TestIssueRefOutput_NilSubObjects(t *testing.T) {
	got := issueRefOutput(&gl.Issue{ID: 1, IID: 2, Title: "Bare"}, toolutil.IssueBasicExtra{})
	if got == nil {
		t.Fatal("issueRefOutput = nil, want non-nil")
	}
	want := &IssueRefOutput{ID: 1, IID: 2, Title: "Bare"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("issueRefOutput (bare) = %+v, want %+v", got, want)
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
	if out := toRelationOutput(r, relationExtra{}); !reflect.DeepEqual(out, want) {
		t.Errorf("toRelationOutput = %+v, want %+v", out, want)
	}
}

// TestToRelationOutput_OneFlagAtATime pins each boolean of a relation to the
// field it is read from, one case per flag with everything else off.
//
// A fixture carrying every flag true cannot tell them apart: two of them
// swapped render the same relation, so a relation could report an issue as
// imported because it has tasks and nothing would fail. Comparing the whole
// struct against one with only the field under test set distinguishes all of
// them, and one flag at a time is also what GitLab answers.
func TestToRelationOutput_OneFlagAtATime(t *testing.T) {
	cases := []struct {
		name     string
		relation gl.IssueRelation
		extra    relationExtra
		want     RelationOutput
	}{
		{"confidential", gl.IssueRelation{Confidential: true}, relationExtra{}, RelationOutput{Confidential: true}},
		{"discussion_locked", gl.IssueRelation{}, relationExtra{DiscussionLocked: true}, RelationOutput{DiscussionLocked: true}},
		{"has_tasks", gl.IssueRelation{}, relationExtra{HasTasks: true}, RelationOutput{HasTasks: true}},
		{"imported", gl.IssueRelation{}, relationExtra{Imported: true}, RelationOutput{Imported: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toRelationOutput(&tc.relation, tc.extra); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("toRelationOutput(%s only) = %+v, want %+v", tc.name, got, tc.want)
			}
		})
	}
}

// TestIssueRefOutput_OneFlagAtATime pins each boolean of the issue on a link
// to the SDK field it is read from, for the reason the relation converter has
// its own such test: the full fixture beside it sets both true, which is
// exactly the shape that hides a swap between them. The subscribed case holds
// the flag IssueBasic never renders to nothing: an SDK issue carrying it true
// converts to the zero value, since the output has no field to put it in.
func TestIssueRefOutput_OneFlagAtATime(t *testing.T) {
	cases := []struct {
		name  string
		issue gl.Issue
		want  IssueRefOutput
	}{
		{"confidential", gl.Issue{Confidential: true}, IssueRefOutput{Confidential: true}},
		{"discussion_locked", gl.Issue{DiscussionLocked: true}, IssueRefOutput{DiscussionLocked: true}},
		{"subscribed is not rendered", gl.Issue{Subscribed: true}, IssueRefOutput{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := issueRefOutput(&tc.issue, toolutil.IssueBasicExtra{})
			if got == nil || !reflect.DeepEqual(*got, tc.want) {
				t.Errorf("issueRefOutput(%s only) = %+v, want %+v", tc.name, got, tc.want)
			}
		})
	}
}

// TestToRelationOutput_NilSubObjects verifies the relation converter handles a
// relation with all optional sub-objects and timestamps absent.
func TestToRelationOutput_NilSubObjects(t *testing.T) {
	out := toRelationOutput(&gl.IssueRelation{ID: 1, IID: 2, Title: "Bare"}, relationExtra{})
	if out.Author != nil || out.Milestone != nil || out.Assignee != nil ||
		out.Assignees != nil || out.References != nil ||
		out.CreatedAt != "" || out.UpdatedAt != "" || out.DueDate != "" ||
		out.LinkCreatedAt != "" || out.LinkUpdatedAt != "" {
		t.Errorf("toRelationOutput (bare) = %+v", out)
	}
}

// TestToOutput_FullAndNilIssues verifies the single-link converter surfaces the
// source/target issue objects, pairs each with the captured keys of its own
// position rather than the other's, and tolerates missing endpoints.
func TestToOutput_FullAndNilIssues(t *testing.T) {
	full := toOutput(&gl.IssueLink{
		ID:          99,
		LinkType:    "relates_to",
		SourceIssue: &gl.Issue{ID: 50, IID: 10, ProjectID: 42, Title: "Source"},
		TargetIssue: &gl.Issue{ID: 80, IID: 20, ProjectID: 43, Title: "Target"},
	}, linkExtra{
		SourceIssue: toolutil.IssueBasicExtra{Type: "ISSUE", StartDate: "2026-01-01"},
		TargetIssue: toolutil.IssueBasicExtra{Type: "INCIDENT", StartDate: "2026-02-01"},
	})
	if full.SourceIssue == nil || full.SourceIssue.Title != "Source" || full.SourceIssue.IID != 10 ||
		full.SourceIssue.Type != "ISSUE" || full.SourceIssue.StartDate != "2026-01-01" ||
		full.TargetIssue == nil || full.TargetIssue.Title != "Target" || full.TargetIssue.ProjectID != 43 ||
		full.TargetIssue.Type != "INCIDENT" || full.TargetIssue.StartDate != "2026-02-01" {
		t.Errorf("toOutput (full) = %+v", full)
	}

	bare := toOutput(&gl.IssueLink{ID: 1, LinkType: "blocks"}, linkExtra{})
	if bare.SourceIssue != nil || bare.TargetIssue != nil {
		t.Errorf("toOutput (bare) = %+v", bare)
	}
}

// TestFormatListMarkdown_WithAuthor checks the whole rendering of a relation
// carrying an author: the handle reaches the author cell with its "@", and a
// relation the response sent no web address for is named rather than linked.
func TestFormatListMarkdown_WithAuthor(t *testing.T) {
	want := "## Issue Relations (1)\n\n" +
		"| ID | IID | Title | State | Link Type | Link ID | Author |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | 2 | T | 🟢 opened | relates_to | 9 | @ann |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'issue.link_create' to add a new link between issues\n"
	got := FormatListMarkdown(ListOutput{Relations: []RelationOutput{
		{ID: 1, IID: 2, Title: "T", State: "opened", LinkType: "relates_to", IssueLinkID: 9, Author: &UserOutput{Username: "ann"}},
	}})
	if got != want {
		t.Errorf("FormatListMarkdown(author)\n got %q\nwant %q", got, want)
	}
}
