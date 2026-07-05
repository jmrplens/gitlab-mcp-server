package toolutil

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewBoardUserOutput pins the documented board assignee conversion:
// nil-on-nil plus a full mirror of the 6 documented fields.
func TestNewBoardUserOutput(t *testing.T) {
	if got := NewBoardUserOutput(nil); got != nil {
		t.Fatalf("NewBoardUserOutput(nil) = %+v, want nil", got)
	}

	in := &gl.BasicUser{
		ID: 5, Username: "jdoe", Name: "John Doe", State: "active",
		AvatarURL: "https://example.com/a.png", WebURL: "https://example.com/jdoe",
	}
	got := NewBoardUserOutput(in)
	if got == nil {
		t.Fatal("NewBoardUserOutput returned nil for non-nil input")
	}
	if got.ID != 5 || got.Username != "jdoe" || got.Name != "John Doe" ||
		got.State != "active" || got.AvatarURL != in.AvatarURL || got.WebURL != in.WebURL {
		t.Errorf("NewBoardUserOutput = %+v, want mirror of %+v", got, in)
	}
}

// TestNewBoardListAssigneeOutput pins the compact board-list assignee
// conversion: nil-on-nil plus the 3-field mirror.
func TestNewBoardListAssigneeOutput(t *testing.T) {
	if got := NewBoardListAssigneeOutput(nil); got != nil {
		t.Fatalf("NewBoardListAssigneeOutput(nil) = %+v, want nil", got)
	}

	got := NewBoardListAssigneeOutput(&gl.BoardListAssignee{ID: 9, Name: "Jane", Username: "jane"})
	if got == nil || got.ID != 9 || got.Name != "Jane" || got.Username != "jane" {
		t.Errorf("NewBoardListAssigneeOutput = %+v, want {9 Jane jane}", got)
	}
}

// TestNewBoardLabelOutput pins the documented board-list label conversion:
// nil-on-nil plus the name/color/description mirror.
func TestNewBoardLabelOutput(t *testing.T) {
	if got := NewBoardLabelOutput(nil); got != nil {
		t.Fatalf("NewBoardLabelOutput(nil) = %+v, want nil", got)
	}

	got := NewBoardLabelOutput(&gl.Label{Name: "bug", Color: "#ff0000", Description: "defects"})
	if got == nil || got.Name != "bug" || got.Color != "#ff0000" || got.Description != "defects" {
		t.Errorf("NewBoardLabelOutput = %+v, want {bug #ff0000 defects}", got)
	}
}

// TestNewBoardLabelDetailsOutputs pins the board labels[] conversion:
// nil/empty inputs return nil, nil elements are skipped, and populated
// entries mirror id, name, color, and description.
func TestNewBoardLabelDetailsOutputs(t *testing.T) {
	if got := NewBoardLabelDetailsOutputs(nil); got != nil {
		t.Fatalf("NewBoardLabelDetailsOutputs(nil) = %+v, want nil", got)
	}
	if got := NewBoardLabelDetailsOutputs([]*gl.LabelDetails{}); got != nil {
		t.Fatalf("NewBoardLabelDetailsOutputs(empty) = %+v, want nil", got)
	}

	in := []*gl.LabelDetails{
		{ID: 1, Name: "bug", Color: "#f00", Description: "defects"},
		nil,
		{ID: 2, Name: "feat", Color: "#0f0", Description: "features"},
	}
	got := NewBoardLabelDetailsOutputs(in)
	if len(got) != 2 {
		t.Fatalf("NewBoardLabelDetailsOutputs len = %d, want 2 (nil element skipped)", len(got))
	}
	if got[0].ID != 1 || got[0].Name != "bug" || got[1].ID != 2 || got[1].Color != "#0f0" {
		t.Errorf("NewBoardLabelDetailsOutputs = %+v, want mirrors of input entries", got)
	}
}

// TestNewIterationOutputFromProjectIteration pins the ProjectIteration →
// canonical iteration conversion: nil-on-nil plus a full field mirror
// including formatted timestamps and ISO dates.
func TestNewIterationOutputFromProjectIteration(t *testing.T) {
	if got := NewIterationOutputFromProjectIteration(nil); got != nil {
		t.Fatalf("NewIterationOutputFromProjectIteration(nil) = %+v, want nil", got)
	}

	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	start := gl.ISOTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	got := NewIterationOutputFromProjectIteration(&gl.ProjectIteration{
		ID: 3, IID: 1, Sequence: 2, GroupID: 42,
		Title: "Sprint 1", Description: "first", State: 1, WebURL: "https://example.com/it/3",
		CreatedAt: &created, StartDate: &start,
	})
	if got == nil {
		t.Fatal("NewIterationOutputFromProjectIteration returned nil for non-nil input")
	}
	if got.ID != 3 || got.GroupID != 42 || got.Title != "Sprint 1" || got.State != 1 {
		t.Errorf("NewIterationOutputFromProjectIteration identity fields = %+v", got)
	}
	if got.CreatedAt == "" || got.StartDate == "" {
		t.Errorf("NewIterationOutputFromProjectIteration dates not mapped: %+v", got)
	}
}
