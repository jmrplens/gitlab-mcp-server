// mrshapes_test.go contains unit tests for the shared MR-shape
// converters exposed by toolutil. The previous per-package tests were
// retained by deleting the local shapes.go definitions; this file
// replaces the shared unit-test surface so future regressions in any
// consumer (mergerequests, deploymentmergerequests, issues) are caught
// from the canonical home.
package toolutil

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewMRMilestoneOutputs verifies the MR-milestone converter skips
// nil entries, formats time/ISOTime pointers, and returns nil for an
// empty / all-nil input.
func TestNewMRMilestoneOutputs(t *testing.T) {
	if got := NewMRMilestoneOutputs(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	due := gl.ISOTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	expired := true
	got := NewMRMilestoneOutputs([]*gl.Milestone{
		nil,
		{
			ID: 1, IID: 2, GroupID: 3, ProjectID: 4, Title: "M", Description: "D", State: "active", WebURL: "w",
			StartDate: &due, DueDate: &due, CreatedAt: &created, UpdatedAt: &created, Expired: &expired,
		},
	})
	if got == nil || len(got) != 1 {
		t.Fatalf("want exactly one milestone (nil skipped), got %+v", got)
	}
	m := got[0]
	if m.ID != 1 || m.Title != "M" || m.GroupID != 3 || m.ProjectID != 4 {
		t.Errorf("scalar fields: %+v", m)
	}
	if m.StartDate != "2026-07-01" {
		t.Errorf("StartDate: %q", m.StartDate)
	}
	if m.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("CreatedAt: %q", m.CreatedAt)
	}
	if m.Expired == nil || !*m.Expired {
		t.Errorf("Expired should be &true, got %+v", m.Expired)
	}
}

// TestNewReferencesOutput verifies the references converter copies the
// short/relative/full triplet and returns nil for a nil source.
func TestNewReferencesOutput(t *testing.T) {
	if got := NewReferencesOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewReferencesOutput(&gl.IssueReferences{Short: "org/proj#1", Relative: "#1", Full: "https://full"})
	if got == nil || got.Short != "org/proj#1" || got.Relative != "#1" || got.Full != "https://full" {
		t.Errorf("refs: %+v", got)
	}
}

// TestNewLabelDetailsOutputs verifies the label-details list skips nil
// entries and copies every documented field.
func TestNewLabelDetailsOutputs(t *testing.T) {
	if got := NewLabelDetailsOutputs(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	got := NewLabelDetailsOutputs([]*gl.LabelDetails{
		nil,
		{ID: 1, Name: "bug", Color: "#FF0000", Description: "d", DescriptionHTML: "<p>d</p>", TextColor: "#FFFFFF"},
	})
	if got == nil || len(got) != 1 || got[0].Name != "bug" || got[0].TextColor != "#FFFFFF" {
		t.Errorf("label details: %+v", got)
	}
}

// TestNewTaskCompletionStatusOutput verifies the task-completion
// converter returns nil for nil and copies Count/CompletedCount.
func TestNewTaskCompletionStatusOutput(t *testing.T) {
	if got := NewTaskCompletionStatusOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewTaskCompletionStatusOutput(&gl.TasksCompletionStatus{Count: 10, CompletedCount: 7})
	if got == nil || got.Count != 10 || got.CompletedCount != 7 {
		t.Errorf("task status: %+v", got)
	}
}

// TestNewPipelineInfoOutput verifies the pipeline-info converter
// returns nil for nil and copies every field including formatted
// timestamps.
func TestNewPipelineInfoOutput(t *testing.T) {
	if got := NewPipelineInfoOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	got := NewPipelineInfoOutput(&gl.PipelineInfo{
		ID: 1, IID: 2, ProjectID: 3, Status: "success",
		Source: "push", Ref: "main", SHA: "abc", Name: "n", WebURL: "w",
		UpdatedAt: &ts, CreatedAt: &ts,
	})
	if got == nil || got.ID != 1 || got.Status != "success" || got.Ref != "main" ||
		got.UpdatedAt != "2026-06-01T12:00:00Z" || got.CreatedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("pipeline info: %+v", got)
	}
}

// TestNewMergeRequestUserOutput verifies the MR-user converter returns
// nil for nil and copies the CanMerge flag.
func TestNewMergeRequestUserOutput(t *testing.T) {
	if got := NewMergeRequestUserOutput(nil); got != nil {
		t.Errorf("nil source must return nil, got %+v", got)
	}
	got := NewMergeRequestUserOutput(&gl.MergeRequestUser{CanMerge: true})
	if got == nil || !got.CanMerge {
		t.Errorf("MR user: %+v", got)
	}
}
