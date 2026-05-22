package iterationdata

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestOutputConverters_MapSharedFields verifies project and group iteration
// converters preserve all common fields and format date fields consistently.
func TestOutputConverters_MapSharedFields(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := gl.ISOTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))

	project := ProjectOutput(&gl.ProjectIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42", StartDate: &start, DueDate: &due, CreatedAt: &now, UpdatedAt: &now})
	group := GroupOutput(&gl.GroupIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42", StartDate: &start, DueDate: &due, CreatedAt: &now, UpdatedAt: &now})

	if project != group {
		t.Fatalf("ProjectOutput() = %+v, GroupOutput() = %+v, want equal shared fields", project, group)
	}
	if project.StartDate == "" || project.DueDate == "" || project.CreatedAt == "" || project.UpdatedAt == "" {
		t.Fatalf("converted dates = %+v, want all date fields populated", project)
	}
}

// TestOutputConverters_NilInput verifies converters return zero-value output
// for nil API objects, matching the previous package-local behavior.
func TestOutputConverters_NilInput(t *testing.T) {
	if got := ProjectOutput(nil); got != (Output{}) {
		t.Fatalf("ProjectOutput(nil) = %+v, want zero Output", got)
	}
	if got := GroupOutput(nil); got != (Output{}) {
		t.Fatalf("GroupOutput(nil) = %+v, want zero Output", got)
	}
}

// TestListOptions_ApplyFilters verifies shared option builders set pagination
// and optional filters for both project and group iteration list requests.
func TestListOptions_ApplyFilters(t *testing.T) {
	group := NewGroupListOptions(2, 50, "opened", "sprint", true)
	if group.Page != 2 || group.PerPage != 50 || group.State == nil || *group.State != "opened" || group.Search == nil || *group.Search != "sprint" || group.IncludeAncestors == nil || !*group.IncludeAncestors {
		t.Fatalf("NewGroupListOptions() = %+v, want pagination and filters", group)
	}

	project := NewProjectListOptions(3, 25, "current", "release", true)
	if project.Page != 3 || project.PerPage != 25 || project.State == nil || *project.State != "current" || project.Search == nil || *project.Search != "release" || project.IncludeAncestors == nil || !*project.IncludeAncestors {
		t.Fatalf("NewProjectListOptions() = %+v, want pagination and filters", project)
	}
}
