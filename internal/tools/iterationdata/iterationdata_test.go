package iterationdata

import (
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatListMarkdown_Empty verifies FormatListMarkdown returns emptyText when no iterations.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := FormatListMarkdown("Iterations", "No iterations found.", nil, toolutil.PaginationOutput{})
	if out != "No iterations found.\n" {
		t.Errorf("FormatListMarkdown() = %q, want %q", out, "No iterations found.\n")
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown renders iteration rows.
func TestFormatListMarkdown_WithData(t *testing.T) {
	iterations := []Output{
		{ID: 1, IID: 10, Title: "Sprint 1", State: 1, WebURL: "https://gitlab.example.com/-/Iterations/1"},
		{ID: 2, IID: 11, Title: "Sprint 2", State: 2, WebURL: "https://gitlab.example.com/-/Iterations/2"},
	}
	pagination := toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1}
	out := FormatListMarkdown("Group Iterations", "No iterations.", iterations, pagination)
	if !strings.Contains(out, "Sprint 1") || !strings.Contains(out, "Sprint 2") {
		t.Errorf("FormatListMarkdown missing iteration data:\n%s", out)
	}
	if !strings.Contains(out, "[opened]") || !strings.Contains(out, "[upcoming]") {
		t.Errorf("FormatListMarkdown missing state links:\n%s", out)
	}
}

// TestFormatOutputMarkdown_WithAllFields verifies FormatOutputMarkdown renders all fields.
func TestFormatOutputMarkdown_WithAllFields(t *testing.T) {
	output := Output{
		ID:          5,
		IID:         20,
		Title:       "Sprint 5",
		State:       3,
		GroupID:     100,
		StartDate:   "2026-03-01T00:00:00Z",
		DueDate:     "2026-03-14T00:00:00Z",
		WebURL:      "https://gitlab.example.com/-/Iterations/5",
		Description: "A description with **markdown**",
	}
	out := FormatOutputMarkdown(output)
	if !strings.Contains(out, "Iteration #20") {
		t.Errorf("FormatOutputMarkdown missing IID header:\n%s", out)
	}
	if !strings.Contains(out, "State") || !strings.Contains(out, "current") {
		t.Errorf("FormatOutputMarkdown missing state:\n%s", out)
	}
	if !strings.Contains(out, "Group ID") || !strings.Contains(out, "100") {
		t.Errorf("FormatOutputMarkdown missing group ID:\n%s", out)
	}
}

// TestFormatOutputMarkdown_WithHint verifies FormatOutputMarkdown appends hints.
func TestFormatOutputMarkdown_WithHint(t *testing.T) {
	output := Output{ID: 1, IID: 1, Title: "Test"}
	out := FormatOutputMarkdown(output, "custom hint")
	if !strings.Contains(out, "custom hint") {
		t.Errorf("FormatOutputMarkdown missing hint:\n%s", out)
	}
}

// TestStateName_AllStates verifies StateName maps all GitLab iteration states.
func TestStateName_AllStates(t *testing.T) {
	tests := []struct {
		state    int64
		expected string
	}{
		{1, "opened"},
		{2, "upcoming"},
		{3, "current"},
		{4, "closed"},
		{0, "unknown(0)"},
		{99, "unknown(99)"},
	}
	for _, tt := range tests {
		got := StateName(tt.state)
		if got != tt.expected {
			t.Errorf("StateName(%d) = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

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
