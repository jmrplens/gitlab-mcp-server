package labeldata

import (
	"reflect"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestOutputConverters_MapSharedFields verifies that both converters fill
// every published field of the shared output, each from the SDK field of the
// same meaning, plus the rendered description the capture read beside the SDK.
//
// Each field carries a value no other field carries, and the whole struct is
// compared against one literal, because the two converters are hand-written
// copies of one field list: a pair that crossed color with text_color, or
// dropped the archive flag the card now reads, agrees with itself on every
// field it does fill and so passes any assertion that only compares the two.
func TestOutputConverters_MapSharedFields(t *testing.T) {
	priority := gl.NewNullableWithValue(int64(3))
	extra := toolutil.LabelExtra{DescriptionHTML: "<p>Bug</p>"}
	want := Output{
		ID:                     11,
		Name:                   "bug",
		Color:                  "#d9534f",
		TextColor:              "#ffffff",
		Description:            "Bug",
		DescriptionHTML:        "<p>Bug</p>",
		OpenIssuesCount:        5,
		ClosedIssuesCount:      2,
		OpenMergeRequestsCount: 7,
		Priority:               3,
		PrioritySpecified:      true,
		IsProjectLabel:         true,
		Subscribed:             true,
		Archived:               true,
	}

	project := ProjectOutput(&gl.Label{ID: 11, Name: "bug", Color: "#d9534f", TextColor: "#ffffff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7, Priority: priority, IsProjectLabel: true, Subscribed: true, Archived: true}, extra)
	if !reflect.DeepEqual(project, want) {
		t.Errorf("ProjectOutput() = %+v, want %+v", project, want)
	}

	group := GroupOutput(&gl.GroupLabel{ID: 11, Name: "bug", Color: "#d9534f", TextColor: "#ffffff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7, Priority: priority, IsProjectLabel: true, Subscribed: true, Archived: true}, extra)
	if !reflect.DeepEqual(group, want) {
		t.Errorf("GroupOutput() = %+v, want %+v", group, want)
	}
}

// TestOutputConverters_NilInput verifies converters return zero-value output
// for nil API objects so callers can safely handle absent GitLab payloads.
func TestOutputConverters_NilInput(t *testing.T) {
	if got := ProjectOutput(nil, toolutil.LabelExtra{}); got.ID != 0 || got.Name != "" {
		t.Fatalf("ProjectOutput(nil) = %+v, want zero Output", got)
	}
	if got := GroupOutput(nil, toolutil.LabelExtra{}); got.ID != 0 || got.Name != "" {
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

// TestListOptions_NothingAsked_LeavesEveryOptionUnset verifies that a caller
// who asked for no page, no search and none of the scope flags leaves the
// options client-go serializes completely untouched.
//
// It matters because these builders write into the struct that becomes the
// query string, and every field they set is one GitLab is then asked to honor:
// a search written as the empty string, or a `with_counts=false` written where
// the caller never mentioned counts, replaces GitLab's own default with ours.
// The `if` around each assignment is the whole mechanism, and until this test
// existed no case exercised the side of it that writes nothing.
func TestListOptions_NothingAsked_LeavesEveryOptionUnset(t *testing.T) {
	project := NewProjectListOptions(0, 0, "", false, false)
	if project.Page != 0 || project.PerPage != 0 || project.Search != nil || project.WithCounts != nil || project.IncludeAncestorGroups != nil {
		t.Errorf("NewProjectListOptions(0, 0, \"\", false, false) = %+v, want every field unset", project)
	}

	group := NewGroupListOptions(0, 0, "", false, false, false, false)
	if group.Page != 0 || group.PerPage != 0 || group.Search != nil || group.WithCounts != nil ||
		group.IncludeAncestorGroups != nil || group.IncludeDescendantGroups != nil || group.OnlyGroupLabels != nil {
		t.Errorf("NewGroupListOptions(0, 0, \"\", false, false, false, false) = %+v, want every field unset", group)
	}
}

// TestListOptions_NegativePagination_IsNeverSent verifies that a negative page
// or page size is dropped rather than forwarded.
//
// That is what the `> 0` guards buy: the page fields are plain integers on the
// struct client-go serializes, so a negative one reaches GitLab as `page=-1`
// and is refused, turning a caller's arithmetic slip into a failed request
// instead of the first page.
func TestListOptions_NegativePagination_IsNeverSent(t *testing.T) {
	project := NewProjectListOptions(-1, -20, "", false, false)
	if project.Page != 0 || project.PerPage != 0 {
		t.Errorf("NewProjectListOptions(-1, -20, ...) pagination = (%d, %d), want (0, 0)", project.Page, project.PerPage)
	}

	group := NewGroupListOptions(-1, -20, "", false, false, false, false)
	if group.Page != 0 || group.PerPage != 0 {
		t.Errorf("NewGroupListOptions(-1, -20, ...) pagination = (%d, %d), want (0, 0)", group.Page, group.PerPage)
	}
}

// TestToMarkdown verifies shared output maps to the markdown formatter model
// without dropping label counts, priority, subscription state or the archive
// flag, which the view model carried nowhere until the card started showing it.
func TestToMarkdown(t *testing.T) {
	in := Output{ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, Priority: 3, PrioritySpecified: true, IsProjectLabel: true, Subscribed: true, Archived: true}

	got := ToMarkdown(in)

	want := toolutil.LabelMarkdown{
		ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug",
		OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1,
		Priority: 3, PrioritySpecified: true, IsProjectLabel: true, Subscribed: true, Archived: true,
	}
	if got != want {
		t.Fatalf("ToMarkdown() = %+v, want %+v", got, want)
	}
}

// TestMarkdownOptionsFor verifies the copy is chosen from the label's own
// scope: both label packages alias this one output type, so the formatter that
// wins the registry has to answer for a project label and a group one alike.
//
// The titles are asserted as the literals a reader sees rather than against
// the two variables the function returns. Compared against themselves the
// assertion holds however the two copies are defined, so it would pass just as
// happily on a tree where both scopes had been given the project wording,
// which is the only way this function can be wrong.
func TestMarkdownOptionsFor(t *testing.T) {
	if got := MarkdownOptionsFor(Output{IsProjectLabel: true}); got.DetailTitle != "Label" || got.ListTitle != "Labels" {
		t.Errorf("MarkdownOptionsFor(project) titles = (%q, %q), want (\"Label\", \"Labels\")", got.DetailTitle, got.ListTitle)
	}
	if got := MarkdownOptionsFor(Output{}); got.DetailTitle != "Group Label" || got.ListTitle != "Group Labels" {
		t.Errorf("MarkdownOptionsFor(group) titles = (%q, %q), want (\"Group Label\", \"Group Labels\")", got.DetailTitle, got.ListTitle)
	}
}

// TestFormatMarkdown_ProjectLabel_CarriesTheProjectCopy verifies the rendering
// both label packages register titles the card and names the follow-up actions
// from the project copy when GitLab says the label belongs to the project.
func TestFormatMarkdown_ProjectLabel_CarriesTheProjectCopy(t *testing.T) {
	got := FormatMarkdown(Output{ID: 3, Name: "bug", Color: "#d9534f", IsProjectLabel: true})

	if !strings.HasPrefix(got, "## Label: bug\n") {
		t.Errorf("FormatMarkdown(project label) heading = %q, want it to open \"## Label: bug\"", firstLine(got))
	}
	if !strings.Contains(got, "'label_delete'") {
		t.Errorf("FormatMarkdown(project label) = %q, want the project label actions in its hints", got)
	}
	if strings.Contains(got, "group-label") {
		t.Errorf("FormatMarkdown(project label) = %q, want no group-label action named", got)
	}
}

// TestFormatMarkdown_GroupLabel_CarriesTheGroupCopy verifies the same
// rendering answers for a group label with the group copy.
//
// This is the half that decides whether the shared formatter was worth having:
// a project's label list carries the group labels the project inherits, so a
// card that titled one of those "Label" and pointed at `label_delete` would
// name an action that cannot touch it, and that is exactly what this package
// serves whenever the group package's init happens to lose the registry race.
func TestFormatMarkdown_GroupLabel_CarriesTheGroupCopy(t *testing.T) {
	got := FormatMarkdown(Output{ID: 3, Name: "bug", Color: "#d9534f"})

	if !strings.HasPrefix(got, "## Group Label: bug\n") {
		t.Errorf("FormatMarkdown(group label) heading = %q, want it to open \"## Group Label: bug\"", firstLine(got))
	}
	if !strings.Contains(got, "group-label delete") {
		t.Errorf("FormatMarkdown(group label) = %q, want the group-label actions in its hints", got)
	}
	if strings.Contains(got, "'label_delete'") {
		t.Errorf("FormatMarkdown(group label) = %q, want no project label action named", got)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
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

	project := ProjectOutput(&gl.Label{ID: 7, Name: "needs-info", Priority: nullPriority}, toolutil.LabelExtra{})
	if project.Priority != 0 || project.PrioritySpecified {
		t.Fatalf("ProjectOutput priority = (%d, %t), want (0, false) for null nullable", project.Priority, project.PrioritySpecified)
	}

	group := GroupOutput(&gl.GroupLabel{ID: 7, Name: "needs-info", Priority: nullPriority}, toolutil.LabelExtra{})
	if group.Priority != 0 || group.PrioritySpecified {
		t.Fatalf("GroupOutput priority = (%d, %t), want (0, false) for null nullable", group.Priority, group.PrioritySpecified)
	}
}
