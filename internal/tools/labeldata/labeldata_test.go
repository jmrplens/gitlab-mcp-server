package labeldata

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestOutputConverters_MapSharedFields verifies project and group label
// converters preserve common fields including nullable priority metadata.
func TestOutputConverters_MapSharedFields(t *testing.T) {
	priority := gl.NewNullableWithValue(int64(3))
	project := ProjectOutput(&gl.Label{ID: 1, Name: "bug", Color: "#d9534f", TextColor: "#fff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, Priority: priority, IsProjectLabel: true, Subscribed: true})
	group := GroupOutput(&gl.GroupLabel{ID: 1, Name: "bug", Color: "#d9534f", TextColor: "#fff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, Priority: priority, IsProjectLabel: true, Subscribed: true})

	if project.ID != group.ID || project.Name != group.Name || project.Color != group.Color || project.OpenIssuesCount != group.OpenIssuesCount || project.OpenMergeRequestsCount != group.OpenMergeRequestsCount {
		t.Fatalf("ProjectOutput() = %+v, GroupOutput() = %+v, want equal shared fields", project, group)
	}
	if project.Priority != 3 || !project.PrioritySpecified {
		t.Fatalf("priority = (%d, %t), want (3, true)", project.Priority, project.PrioritySpecified)
	}
}

// TestOutputConverters_NilInput verifies converters return zero-value output
// for nil API objects so callers can safely handle absent GitLab payloads.
func TestOutputConverters_NilInput(t *testing.T) {
	if got := ProjectOutput(nil); got.ID != 0 || got.Name != "" {
		t.Fatalf("ProjectOutput(nil) = %+v, want zero Output", got)
	}
	if got := GroupOutput(nil); got.ID != 0 || got.Name != "" {
		t.Fatalf("GroupOutput(nil) = %+v, want zero Output", got)
	}
}

// TestListOptions_ApplyFilters verifies shared option builders set pagination
// and label-specific filters for project and group list requests.
func TestListOptions_ApplyFilters(t *testing.T) {
	project := NewProjectListOptions(2, 50, "bug", true, true)
	assertProjectListOptions(t, project)

	group := NewGroupListOptions(3, 25, "feature", true, true, true, true)
	assertGroupListOptions(t, group)
}

func assertProjectListOptions(t *testing.T, project *gl.ListLabelsOptions) {
	t.Helper()
	if project.Page != 2 || project.PerPage != 50 || project.Search == nil || *project.Search != "bug" || project.WithCounts == nil || !*project.WithCounts || project.IncludeAncestorGroups == nil || !*project.IncludeAncestorGroups {
		t.Fatalf("NewProjectListOptions() = %+v, want pagination and filters", project)
	}
}

func assertGroupListOptions(t *testing.T, group *gl.ListGroupLabelsOptions) {
	t.Helper()
	if group.Page != 3 || group.PerPage != 25 || group.Search == nil || *group.Search != "feature" || group.WithCounts == nil || !*group.WithCounts || group.IncludeAncestorGroups == nil || !*group.IncludeAncestorGroups || group.IncludeDescendantGroups == nil || !*group.IncludeDescendantGroups || group.OnlyGroupLabels == nil || !*group.OnlyGroupLabels {
		t.Fatalf("NewGroupListOptions() = %+v, want pagination and filters", group)
	}
}

// TestToMarkdown verifies shared output maps to the markdown formatter model
// without dropping label counts, priority, or subscription state.
func TestToMarkdown(t *testing.T) {
	got := ToMarkdown(Output{ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, Priority: 3, PrioritySpecified: true, IsProjectLabel: true, Subscribed: true})
	if got.ID != 1 || got.Name != "bug" || got.Priority != 3 || !got.PrioritySpecified || !got.IsProjectLabel || !got.Subscribed {
		t.Fatalf("ToMarkdown() = %+v, want all shared fields", got)
	}
}
