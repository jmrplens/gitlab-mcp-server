package labeldata

import (
	"encoding/json"
	"reflect"
	"strconv"
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
//
// The group label is handed a priority and a project flag and must publish
// neither: Entities::GroupLabel sends no such keys, so whatever the SDK struct
// holds there is not something GitLab said.
func TestOutputConverters_MapSharedFields(t *testing.T) {
	priority := gl.NewNullableWithValue(int64(3))
	extra := toolutil.LabelExtra{DescriptionHTML: "<p>Bug</p>"}
	shared := Output{
		ID:                     11,
		Name:                   "bug",
		Color:                  "#d9534f",
		TextColor:              "#ffffff",
		Description:            "Bug",
		DescriptionHTML:        "<p>Bug</p>",
		OpenIssuesCount:        5,
		ClosedIssuesCount:      2,
		OpenMergeRequestsCount: 7,
		Subscribed:             true,
		Archived:               true,
	}
	wantProject := shared
	wantProject.Priority = new(int64(3))
	wantProject.IsProjectLabel = new(true)

	project := ProjectOutput(&gl.Label{ID: 11, Name: "bug", Color: "#d9534f", TextColor: "#ffffff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7, Priority: priority, IsProjectLabel: true, Subscribed: true, Archived: true}, extra)
	if !reflect.DeepEqual(project, wantProject) {
		t.Errorf("ProjectOutput() = %+v, want %+v", project, wantProject)
	}

	group := GroupOutput(&gl.GroupLabel{ID: 11, Name: "bug", Color: "#d9534f", TextColor: "#ffffff", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7, Priority: priority, IsProjectLabel: true, Subscribed: true, Archived: true}, extra)
	if !reflect.DeepEqual(group, shared) {
		t.Errorf("GroupOutput() = %+v, want %+v", group, shared)
	}
}

// TestOutput_JSON_ProjectKeysOnlyOnAProjectLabel verifies what each scope
// publishes on the wire: a project label states whether it is the project's
// own even when it is not, and a group label states neither key, so no group
// label reads as an inherited project label with priority zero.
func TestOutput_JSON_ProjectKeysOnlyOnAProjectLabel(t *testing.T) {
	inherited, err := json.Marshal(ProjectOutput(&gl.Label{ID: 1, Priority: gl.NewNullNullable[int64]()}, toolutil.LabelExtra{}))
	if err != nil {
		t.Fatalf("json.Marshal(project label) error = %v", err)
	}
	if !strings.Contains(string(inherited), `"is_project_label":false`) {
		t.Errorf("project label JSON = %s, want is_project_label false stated", inherited)
	}
	if strings.Contains(string(inherited), `"priority"`) {
		t.Errorf("project label JSON = %s, want no priority for a null one", inherited)
	}

	group, err := json.Marshal(GroupOutput(&gl.GroupLabel{ID: 2}, toolutil.LabelExtra{}))
	if err != nil {
		t.Fatalf("json.Marshal(group label) error = %v", err)
	}
	for _, key := range []string{`"priority"`, `"is_project_label"`} {
		t.Run(key, func(t *testing.T) {
			if strings.Contains(string(group), key) {
				t.Errorf("group label JSON = %s, want no %s key", group, key)
			}
		})
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
	in := Output{ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, Priority: new(int64(3)), IsProjectLabel: new(true), Subscribed: true, Archived: true}

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

// TestToMarkdown_NoPriorityAndNoScope_LeavesBothUnset verifies a label that
// states neither a priority nor a scope, which is every group label, reaches
// the card model with no priority and as a group label, and that a project
// label stating it is not the project's own is read the same way.
func TestToMarkdown_NoPriorityAndNoScope_LeavesBothUnset(t *testing.T) {
	tests := []struct {
		name  string
		label Output
	}{
		{name: "group label", label: Output{ID: 2, Name: "infra"}},
		{name: "inherited project label", label: Output{ID: 3, Name: "infra", IsProjectLabel: new(false)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.label)
			if got.Priority != 0 || got.PrioritySpecified || got.IsProjectLabel {
				t.Errorf("ToMarkdown(%+v) = (priority %d, specified %t, project %t), want (0, false, false)", tt.label, got.Priority, got.PrioritySpecified, got.IsProjectLabel)
			}
		})
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
	if got := MarkdownOptionsFor(Output{IsProjectLabel: new(true)}); got.DetailTitle != "Label" || got.ListTitle != "Labels" {
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
	got := FormatMarkdown(Output{ID: 3, Name: "bug", Color: "#d9534f", IsProjectLabel: new(true)})

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
// GitLab's nullable priority field must collapse to no priority in the latter
// two cases, and keep zero in the second, so a consumer can tell "no
// priority" from "priority 0".
func TestPriorityFromNullable(t *testing.T) {
	tests := []struct {
		name     string
		nullable gl.Nullable[int64]
		want     *int64
	}{
		{name: "specified with positive value", nullable: gl.NewNullableWithValue(int64(5)), want: new(int64(5))},
		{name: "specified with zero value", nullable: gl.NewNullableWithValue(int64(0)), want: new(int64(0))},
		{name: "explicit null", nullable: gl.NewNullNullable[int64]()},
		{name: "unspecified (zero value)", nullable: gl.Nullable[int64]{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := priorityFromNullable(tt.nullable)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("priorityFromNullable() = %v, want %v", describePriority(got), describePriority(tt.want))
			}
		})
	}
}

// describePriority spells a priority for a failure message, which a pointer
// printed with %v would not.
func describePriority(p *int64) string {
	if p == nil {
		return "none"
	}
	return strconv.FormatInt(*p, 10)
}

// TestOutputConverters_PropagateNullPriority verifies the converters surface
// the explicit-null branch of priorityFromNullable so callers can tell a
// "cleared" priority from an unset one.
func TestOutputConverters_PropagateNullPriority(t *testing.T) {
	nullPriority := gl.NewNullNullable[int64]()

	project := ProjectOutput(&gl.Label{ID: 7, Name: "needs-info", Priority: nullPriority}, toolutil.LabelExtra{})
	if project.Priority != nil {
		t.Fatalf("ProjectOutput priority = %s, want none for a null one", describePriority(project.Priority))
	}

	group := GroupOutput(&gl.GroupLabel{ID: 7, Name: "needs-info", Priority: nullPriority}, toolutil.LabelExtra{})
	if group.Priority != nil {
		t.Fatalf("GroupOutput priority = %s, want none", describePriority(group.Priority))
	}
}
