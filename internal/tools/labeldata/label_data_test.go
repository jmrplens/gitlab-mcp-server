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

// TestPriorityFromNullable covers the three nullability states handled by
// the converter: specified with a value, explicitly null, and unspecified.
// GitLab's nullable priority field must collapse to (0, false) in the
// latter two cases so downstream consumers can distinguish "no priority"
// from "priority 0".
func TestPriorityFromNullable(t *testing.T) {
	tests := []struct {
		name          string
		nullable      gl.Nullable[int64]
		wantPriority  int64
		wantSpecified bool
	}{
		{
			name:          "specified with positive value",
			nullable:      gl.NewNullableWithValue(int64(5)),
			wantPriority:  5,
			wantSpecified: true,
		},
		{
			name:          "specified with zero value",
			nullable:      gl.NewNullableWithValue(int64(0)),
			wantPriority:  0,
			wantSpecified: true,
		},
		{
			name:          "explicit null",
			nullable:      gl.NewNullNullable[int64](),
			wantPriority:  0,
			wantSpecified: false,
		},
		{
			name:          "unspecified (zero value)",
			nullable:      gl.Nullable[int64]{},
			wantPriority:  0,
			wantSpecified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPriority, gotSpecified := priorityFromNullable(tt.nullable)
			if gotPriority != tt.wantPriority {
				t.Errorf("priority = %d, want %d", gotPriority, tt.wantPriority)
			}
			if gotSpecified != tt.wantSpecified {
				t.Errorf("specified = %t, want %t", gotSpecified, tt.wantSpecified)
			}
		})
	}
}

// TestOutputConverters_PropagateNullPriority verifies the converters surface
// the explicit-null branch of priorityFromNullable so callers can tell a
// "cleared" priority from an unset one.
func TestOutputConverters_PropagateNullPriority(t *testing.T) {
	nullPriority := gl.NewNullNullable[int64]()

	project := ProjectOutput(&gl.Label{ID: 7, Name: "needs-info", Priority: nullPriority})
	if project.Priority != 0 || project.PrioritySpecified {
		t.Fatalf("ProjectOutput priority = (%d, %t), want (0, false) for null nullable", project.Priority, project.PrioritySpecified)
	}

	group := GroupOutput(&gl.GroupLabel{ID: 7, Name: "needs-info", Priority: nullPriority})
	if group.Priority != 0 || group.PrioritySpecified {
		t.Fatalf("GroupOutput priority = (%d, %t), want (0, false) for null nullable", group.Priority, group.PrioritySpecified)
	}
}
