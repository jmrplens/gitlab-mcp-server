// shapes_test.go validates the client-go sub-object mirrors (group, label
// details, board list label/assignee/iteration/milestone scopes) and the
// time-formatting helpers, covering both the fully-populated and nil branches
// of every converter per the 1:1 audit policy.
package groupepicboards

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestConvertersNil verifies that every sub-object converter returns nil (or a
// zero value) when given a nil input pointer.
func TestConvertersNil(t *testing.T) {
	if got := groupRefOutput(nil); got != nil {
		t.Errorf("groupRefOutput(nil) = %+v, want nil", got)
	}
	if got := labelDetailsOutput(nil); got != nil {
		t.Errorf("labelDetailsOutput(nil) = %+v, want nil", got)
	}
	if got := labelDetailsOutputs(nil); got != nil {
		t.Errorf("labelDetailsOutputs(nil) = %+v, want nil", got)
	}
	if got := labelOutput(nil); got != nil {
		t.Errorf("labelOutput(nil) = %+v, want nil", got)
	}
	if got := milestoneOutput(nil); got != nil {
		t.Errorf("milestoneOutput(nil) = %+v, want nil", got)
	}
	if got := boardListAssigneeOutput(nil); got != nil {
		t.Errorf("boardListAssigneeOutput(nil) = %+v, want nil", got)
	}
	if got := iterationOutput(nil); got != nil {
		t.Errorf("iterationOutput(nil) = %+v, want nil", got)
	}
	if got := convertBoardList(nil); got != (BoardListOutput{}) {
		t.Errorf("convertBoardList(nil) = %+v, want zero", got)
	}
	if got := toOutput(nil); got.ID != 0 || got.Name != "" {
		t.Errorf("toOutput(nil) = %+v, want zero", got)
	}
}

// TestTimeHelpers verifies the RFC3339 and ISO date helpers across nil and
// populated inputs.
func TestTimeHelpers(t *testing.T) {
	if got := formatTimePtr(nil); got != "" {
		t.Errorf("formatTimePtr(nil) = %q, want empty", got)
	}
	if got := formatISOTimePtr(nil); got != "" {
		t.Errorf("formatISOTimePtr(nil) = %q, want empty", got)
	}
	tm := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := formatTimePtr(&tm); got != "2024-01-02T03:04:05Z" {
		t.Errorf("formatTimePtr = %q", got)
	}
	iso := gl.ISOTime(tm)
	if got := formatISOTimePtr(&iso); got != "2024-01-02" {
		t.Errorf("formatISOTimePtr = %q", got)
	}
}

// TestToOutputFullyPopulated verifies that toOutput surfaces every nested
// sub-object (group, label details, and the per-list assignee, label,
// iteration, and milestone scopes with timestamps) of a fully-populated
// gl.GroupEpicBoard.
func TestToOutputFullyPopulated(t *testing.T) {
	tm := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	iso := gl.ISOTime(tm)
	priority := int64(3)

	board := &gl.GroupEpicBoard{
		ID:   1,
		Name: "Board",
		Group: &gl.Group{
			ID: 7, Name: "G", Path: "g", FullPath: "ns/g", FullName: "NS / G",
			Description: "d", Visibility: gl.PublicVisibility, WebURL: "https://x",
			AvatarURL: "https://a", ParentID: 2, RequestAccessEnabled: true,
			LFSEnabled: true, CreatedAt: &tm,
		},
		Labels: []*gl.LabelDetails{
			{ID: 10, Name: "Priority", Color: "#f00", Description: "p", DescriptionHTML: "<p>p</p>", TextColor: "#fff"},
			nil,
		},
		Lists: []*gl.BoardList{
			{
				ID:        100,
				Assignee:  &gl.BoardListAssignee{ID: 5, Name: "Dev", Username: "dev"},
				Iteration: &gl.ProjectIteration{ID: 9, IID: 1, Sequence: 1, GroupID: 7, Title: "I", Description: "id", State: 1, WebURL: "https://i", CreatedAt: &tm, UpdatedAt: &tm, StartDate: &iso, DueDate: &iso},
				Label: &gl.Label{
					ID: 10, Name: "Priority", Color: "#f00", TextColor: "#fff", Description: "p",
					OpenIssuesCount: 2, ClosedIssuesCount: 1, OpenMergeRequestsCount: 3,
					Subscribed: true, Priority: gl.NewNullableWithValue(priority), IsProjectLabel: true, Archived: true,
				},
				MaxIssueCount:  4,
				MaxIssueWeight: 8,
				Milestone:      &gl.Milestone{ID: 20, IID: 2, GroupID: 7, ProjectID: 0, Title: "M", Description: "md", State: "active", WebURL: "https://m", StartDate: &iso, DueDate: &iso, CreatedAt: &tm, UpdatedAt: &tm, Expired: new(false)},
				Position:       0,
			},
			nil,
		},
	}

	out := toOutput(board)
	assertFullGroup(t, out)
	assertFullLabels(t, out)
	assertFullList(t, out)
}

func assertFullGroup(t *testing.T, out Output) {
	t.Helper()
	if out.Group == nil {
		t.Fatal("Group nil")
	}
	if out.Group.FullPath != "ns/g" || out.Group.Visibility != "public" || !out.Group.RequestAccessEnabled || out.Group.CreatedAt == "" {
		t.Errorf("Group = %+v", out.Group)
	}
}

func assertFullLabels(t *testing.T, out Output) {
	t.Helper()
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1 (nil filtered)", len(out.Labels))
	}
	l := out.Labels[0]
	if l.Color != "#f00" || l.DescriptionHTML != "<p>p</p>" || l.TextColor != "#fff" {
		t.Errorf("LabelDetails = %+v", l)
	}
}

func assertFullList(t *testing.T, out Output) {
	t.Helper()
	if len(out.Lists) != 1 {
		t.Fatalf("len(Lists) = %d, want 1 (nil filtered)", len(out.Lists))
	}
	bl := out.Lists[0]
	if bl.Assignee == nil || bl.Assignee.Username != "dev" {
		t.Errorf("Assignee = %+v", bl.Assignee)
	}
	if bl.Iteration == nil || bl.Iteration.StartDate != "2024-06-01" {
		t.Errorf("Iteration = %+v", bl.Iteration)
	}
	if bl.Label == nil || bl.Label.Priority == nil || *bl.Label.Priority != 3 || !bl.Label.Archived {
		t.Errorf("Label = %+v", bl.Label)
	}
	if bl.Milestone == nil || bl.Milestone.Expired == nil || bl.Milestone.DueDate != "2024-06-01" {
		t.Errorf("Milestone = %+v", bl.Milestone)
	}
	if bl.MaxIssueCount != 4 || bl.MaxIssueWeight != 8 {
		t.Errorf("Max counts = %d/%d", bl.MaxIssueCount, bl.MaxIssueWeight)
	}
}

// TestLabelOutputNoPriority verifies labelOutput leaves Priority nil when the
// source label has no priority set.
func TestLabelOutputNoPriority(t *testing.T) {
	got := labelOutput(&gl.Label{ID: 1, Name: "x"})
	if got.Priority != nil {
		t.Errorf("Priority = %v, want nil", got.Priority)
	}
}

// TestMarkdownHelpersNil verifies the markdown label helpers tolerate nil and
// empty inputs.
func TestMarkdownHelpersNil(t *testing.T) {
	if got := labelNames(nil); got != nil {
		t.Errorf("labelNames(nil) = %v, want nil", got)
	}
	if got := labelNames([]*LabelDetailsOutput{nil, {Name: "a"}}); len(got) != 1 || got[0] != "a" {
		t.Errorf("labelNames filtered = %v", got)
	}
	if got := listLabelName(BoardListOutput{}); got != "" {
		t.Errorf("listLabelName(no label) = %q, want empty", got)
	}
	if got := listLabelName(BoardListOutput{Label: &LabelOutput{Name: "L"}}); got != "L" {
		t.Errorf("listLabelName = %q, want L", got)
	}
}
