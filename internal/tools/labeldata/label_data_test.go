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
// holds there is not something GitLab said. Both are handed usage counts and
// must publish none, for the same reason: no route these two converters serve
// sends them (issue 1174).
func TestOutputConverters_MapSharedFields(t *testing.T) {
	priority := gl.NewNullableWithValue(int64(3))
	extra := toolutil.LabelExtra{DescriptionHTML: "<p>Bug</p>"}
	shared := Output{
		ID:              11,
		Name:            "bug",
		Color:           "#d9534f",
		TextColor:       "#ffffff",
		Description:     "Bug",
		DescriptionHTML: "<p>Bug</p>",
		Subscribed:      true,
		Archived:        true,
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
// for nil API objects so callers can safely handle absent GitLab payloads,
// the list converters included when their listing asked for counts.
func TestOutputConverters_NilInput(t *testing.T) {
	if got := ProjectOutput(nil, toolutil.LabelExtra{}); !reflect.DeepEqual(got, Output{}) {
		t.Errorf("ProjectOutput(nil) = %+v, want zero Output", got)
	}
	if got := GroupOutput(nil, toolutil.LabelExtra{}); !reflect.DeepEqual(got, Output{}) {
		t.Errorf("GroupOutput(nil) = %+v, want zero Output", got)
	}
	if got := ProjectListOutput(nil, toolutil.LabelExtra{}, true); !reflect.DeepEqual(got, Output{}) {
		t.Errorf("ProjectListOutput(nil, counts asked) = %+v, want zero Output", got)
	}
	if got := GroupListOutput(nil, toolutil.LabelExtra{}, true); !reflect.DeepEqual(got, Output{}) {
		t.Errorf("GroupListOutput(nil, counts asked) = %+v, want zero Output", got)
	}
}

// TestListOutputConverters_Counts_PublishedOnlyWhenTheListingAskedForThem
// verifies a listed label publishes its usage counts exactly when its listing
// asked GitLab for them with with_counts, each from the SDK field of the same
// meaning, and otherwise is the label its single-label converter publishes.
//
// GitLab sends the counts to a listing that asked and to no other answer, and
// the SDK decodes an absent count as 0, so a listing that did not ask used to
// publish 0, 0, 0 for every label, which reads as labels nothing uses (issue
// 1174). The counts handed here differ from one another so a converter that
// crossed two of them fails.
func TestListOutputConverters_Counts_PublishedOnlyWhenTheListingAskedForThem(t *testing.T) {
	extra := toolutil.LabelExtra{DescriptionHTML: "<p>Bug</p>"}
	project := &gl.Label{ID: 11, Name: "bug", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7, IsProjectLabel: true}
	group := &gl.GroupLabel{ID: 12, Name: "infra", OpenIssuesCount: 4, ClosedIssuesCount: 9, OpenMergeRequestsCount: 1}

	for _, tc := range []struct {
		name string
		got  Output
		want Output
	}{
		{name: "project label, counts asked", got: ProjectListOutput(project, extra, true), want: withUsageCounts(ProjectOutput(project, extra), 5, 2, 7)},
		{name: "project label, counts not asked", got: ProjectListOutput(project, extra, false), want: ProjectOutput(project, extra)},
		{name: "group label, counts asked", got: GroupListOutput(group, extra, true), want: withUsageCounts(GroupOutput(group, extra), 4, 9, 1)},
		{name: "group label, counts not asked", got: GroupListOutput(group, extra, false), want: GroupOutput(group, extra)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("converted label = %+v, want %+v", tc.got, tc.want)
			}
		})
	}
}

// TestWithUsageCounts_SetsEachCountFromItsOwnArgument verifies the three
// counts land in their own fields, read as the values a reader sees.
func TestWithUsageCounts_SetsEachCountFromItsOwnArgument(t *testing.T) {
	got := withUsageCounts(Output{ID: 1}, 5, 2, 7)
	if got.ID != 1 || got.OpenIssuesCount == nil || *got.OpenIssuesCount != 5 || got.ClosedIssuesCount == nil || *got.ClosedIssuesCount != 2 || got.OpenMergeRequestsCount == nil || *got.OpenMergeRequestsCount != 7 {
		t.Errorf("withUsageCounts(Output{ID: 1}, 5, 2, 7) = %+v, want ID 1 and counts 5, 2, 7", got)
	}
}

// TestOutput_JSON_CountsOnlyWhenSent verifies what the wire carries: the
// three count keys, zeros included, on a label whose listing asked for them,
// and none of the three on any other label.
func TestOutput_JSON_CountsOnlyWhenSent(t *testing.T) {
	keys := []string{`"open_issues_count"`, `"closed_issues_count"`, `"open_merge_requests_count"`}

	counted, err := json.Marshal(ProjectListOutput(&gl.Label{ID: 1}, toolutil.LabelExtra{}, true))
	if err != nil {
		t.Fatalf("json.Marshal(counted label) error = %v", err)
	}
	uncounted, err := json.Marshal(ProjectListOutput(&gl.Label{ID: 1, OpenIssuesCount: 5}, toolutil.LabelExtra{}, false))
	if err != nil {
		t.Fatalf("json.Marshal(uncounted label) error = %v", err)
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			if !strings.Contains(string(counted), key+":0") {
				t.Errorf("counted label JSON = %s, want %s stated as 0", counted, key)
			}
			if strings.Contains(string(uncounted), key) {
				t.Errorf("uncounted label JSON = %s, want no %s key", uncounted, key)
			}
		})
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
	in := withUsageCounts(Output{ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug", Priority: new(int64(3)), IsProjectLabel: new(true), Subscribed: true, Archived: true}, 5, 2, 1)

	got := ToMarkdown(in)

	want := toolutil.LabelMarkdown{
		ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug",
		CountsSent: true, OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1,
		Priority: 3, PrioritySpecified: true, IsProjectLabel: true, Subscribed: true, Archived: true,
	}
	if got != want {
		t.Fatalf("ToMarkdown() = %+v, want %+v", got, want)
	}
}

// TestToMarkdown_Counts_SentOnlyWhenTheLabelCarriesAllThree verifies the card
// model marks the counts sent only for a label carrying all three, and
// otherwise carries no count at all.
//
// A label carries them together or not at all, set by withUsageCounts, so a
// label missing any one is a label GitLab sent none for, and reading the
// others as sent would render a zero for a count nobody sent. Each case drops
// a different one, so a check that read only one of the three, or joined them
// with "or", fails.
func TestToMarkdown_Counts_SentOnlyWhenTheLabelCarriesAllThree(t *testing.T) {
	counted := withUsageCounts(Output{ID: 1}, 5, 2, 7)
	withoutOpen, withoutClosed, withoutMergeRequests := counted, counted, counted
	withoutOpen.OpenIssuesCount = nil
	withoutClosed.ClosedIssuesCount = nil
	withoutMergeRequests.OpenMergeRequestsCount = nil

	for _, tc := range []struct {
		name  string
		label Output
		want  toolutil.LabelMarkdown
	}{
		{name: "all three", label: counted, want: toolutil.LabelMarkdown{ID: 1, CountsSent: true, OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 7}},
		{name: "none", label: Output{ID: 1}, want: toolutil.LabelMarkdown{ID: 1}},
		{name: "open issues missing", label: withoutOpen, want: toolutil.LabelMarkdown{ID: 1}},
		{name: "closed issues missing", label: withoutClosed, want: toolutil.LabelMarkdown{ID: 1}},
		{name: "open merge requests missing", label: withoutMergeRequests, want: toolutil.LabelMarkdown{ID: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToMarkdown(tc.label); got != tc.want {
				t.Errorf("ToMarkdown() = %+v, want %+v", got, tc.want)
			}
		})
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
