// filters_test.go contains unit tests for the conversion between the MCP filter
// shape and the SDK's WorkItemSavedViewFilters, covering every nested
// sub-filter, the pointer/empty-string distinction, and timestamp parsing.
package workitemsavedviews

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestFilters_ToSDK_NilReceiver verifies that an absent filter object converts
// to nil rather than to an empty filter, so an update leaves stored filters
// untouched.
func TestFilters_ToSDK_NilReceiver(t *testing.T) {
	var filters *Filters
	got, err := filters.toSDK()
	if err != nil {
		t.Fatalf("toSDK() unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("toSDK() = %+v, want nil", got)
	}
}

// TestFilters_ToSDK_EmptyOmitsEveryOptional verifies that a filter object with
// no values set produces no pointers at all: an empty string must mean "absent"
// rather than "match the empty value".
func TestFilters_ToSDK_EmptyOmitsEveryOptional(t *testing.T) {
	got, err := (&Filters{}).toSDK()
	if err != nil {
		t.Fatalf("toSDK() unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("toSDK() = nil, want an empty filter struct")
	}
	pointers := map[string]any{
		"assignee_wildcard_id":    got.AssigneeWildcardID,
		"author_username":         got.AuthorUsername,
		"crm_contact_id":          got.CRMContactID,
		"crm_organization_id":     got.CRMOrganizationID,
		"full_path":               got.FullPath,
		"health_status_filter":    got.HealthStatusFilter,
		"iid":                     got.IID,
		"iteration_wildcard_id":   got.IterationWildcardID,
		"milestone_wildcard_id":   got.MilestoneWildcardID,
		"my_reaction_emoji":       got.MyReactionEmoji,
		"release_tag_wildcard_id": got.ReleaseTagWildcardID,
		"search":                  got.Search,
		"state":                   got.State,
		"subscribed":              got.Subscribed,
		"weight":                  got.Weight,
		"weight_wildcard_id":      got.WeightWildcardID,
	}
	for name, value := range pointers {
		t.Run(name, func(t *testing.T) {
			if pointer, ok := value.(*string); !ok || pointer != nil {
				t.Errorf("%s = %v, want a nil pointer", name, value)
			}
		})
	}
	for name, value := range map[string]*bool{
		"confidential":                  got.Confidential,
		"exclude_group_work_items":      got.ExcludeGroupWorkItems,
		"exclude_projects":              got.ExcludeProjects,
		"include_descendant_work_items": got.IncludeDescendantWorkItems,
		"include_descendants":           got.IncludeDescendants,
	} {
		t.Run(name, func(t *testing.T) {
			if value != nil {
				t.Errorf("%s = %v, want nil", name, *value)
			}
		})
	}
	if got.Not != nil || got.Or != nil || got.HierarchyFilters != nil || got.Status != nil || got.CustomField != nil {
		t.Errorf("nested sub-filters = %+v, want all nil", got)
	}
}

// TestFilters_ToSDK_AllScalarsAndSlices verifies that every scalar and slice
// field reaches the SDK struct, which is what makes the MCP filter a 1:1 mirror
// of WorkItemSavedViewFilterInput rather than a curated subset.
//
// No two values in the fixture agree, which is what makes the claim a real one:
// the assignee and release-tag wildcards were both "ANY" here, so a converter
// that crossed the two assigned each other's value and every assertion still
// read the string it expected. The five flags cannot be told apart this way at
// all, and are driven one at a time in
// [TestFilters_ToSDK_EachBooleanReachesItsOwnField] instead; what this test
// holds about them is only that each arrives set.
func TestFilters_ToSDK_AllScalarsAndSlices(t *testing.T) {
	yes := true
	in := &Filters{
		AssigneeUsernames:          []string{"alice"},
		AssigneeWildcardID:         "ME",
		AuthorUsername:             "bob",
		Confidential:               &yes,
		CRMContactID:               "gid://gitlab/CustomerRelations::Contact/1",
		CRMOrganizationID:          "gid://gitlab/CustomerRelations::Organization/2",
		ExcludeGroupWorkItems:      &yes,
		ExcludeProjects:            &yes,
		FullPath:                   "g/p",
		HealthStatusFilter:         "atRisk",
		IID:                        "42",
		In:                         []string{"TITLE"},
		IncludeDescendantWorkItems: &yes,
		IncludeDescendants:         &yes,
		IterationCadenceID:         []string{"gid://gitlab/Iterations::Cadence/3"},
		IterationID:                []string{"gid://gitlab/Iteration/4"},
		IterationWildcardID:        "CURRENT",
		LabelName:                  []string{"bug"},
		MilestoneTitle:             []string{"v1"},
		MilestoneWildcardID:        "UPCOMING",
		MyReactionEmoji:            "thumbsup",
		ReleaseTag:                 []string{"v1.0.0"},
		ReleaseTagWildcardID:       "ANY",
		Search:                     "crash",
		State:                      "opened",
		Subscribed:                 "EXPLICITLY_SUBSCRIBED",
		Types:                      []string{"ISSUE"},
		Weight:                     "3",
		WeightWildcardID:           "NONE",
		WorkItemTypeIDs:            []string{"gid://gitlab/WorkItems::Type/5"},
	}

	got, err := in.toSDK()
	if err != nil {
		t.Fatalf("toSDK() unexpected error: %v", err)
	}
	strings := map[string]struct {
		got  *string
		want string
	}{
		"assignee_wildcard_id":    {got.AssigneeWildcardID, "ME"},
		"author_username":         {got.AuthorUsername, "bob"},
		"crm_contact_id":          {got.CRMContactID, "gid://gitlab/CustomerRelations::Contact/1"},
		"crm_organization_id":     {got.CRMOrganizationID, "gid://gitlab/CustomerRelations::Organization/2"},
		"full_path":               {got.FullPath, "g/p"},
		"health_status_filter":    {got.HealthStatusFilter, "atRisk"},
		"iid":                     {got.IID, "42"},
		"iteration_wildcard_id":   {got.IterationWildcardID, "CURRENT"},
		"milestone_wildcard_id":   {got.MilestoneWildcardID, "UPCOMING"},
		"my_reaction_emoji":       {got.MyReactionEmoji, "thumbsup"},
		"release_tag_wildcard_id": {got.ReleaseTagWildcardID, "ANY"},
		"search":                  {got.Search, "crash"},
		"state":                   {got.State, "opened"},
		"subscribed":              {got.Subscribed, "EXPLICITLY_SUBSCRIBED"},
		"weight":                  {got.Weight, "3"},
		"weight_wildcard_id":      {got.WeightWildcardID, "NONE"},
	}
	for name, tc := range strings {
		t.Run(name, func(t *testing.T) {
			if tc.got == nil || *tc.got != tc.want {
				t.Errorf("%s = %v, want %q", name, tc.got, tc.want)
			}
		})
	}
	slices := map[string]struct {
		got  []string
		want string
	}{
		"assignee_usernames":   {got.AssigneeUsernames, "alice"},
		"in":                   {got.In, "TITLE"},
		"iteration_cadence_id": {got.IterationCadenceID, "gid://gitlab/Iterations::Cadence/3"},
		"iteration_id":         {got.IterationID, "gid://gitlab/Iteration/4"},
		"label_name":           {got.LabelName, "bug"},
		"milestone_title":      {got.MilestoneTitle, "v1"},
		"release_tag":          {got.ReleaseTag, "v1.0.0"},
		"types":                {got.Types, "ISSUE"},
		"work_item_type_ids":   {got.WorkItemTypeIDs, "gid://gitlab/WorkItems::Type/5"},
	}
	for name, tc := range slices {
		t.Run(name, func(t *testing.T) {
			if len(tc.got) != 1 || tc.got[0] != tc.want {
				t.Errorf("%s = %v, want [%q]", name, tc.got, tc.want)
			}
		})
	}
	for name, value := range map[string]*bool{
		"confidential":                  got.Confidential,
		"exclude_group_work_items":      got.ExcludeGroupWorkItems,
		"exclude_projects":              got.ExcludeProjects,
		"include_descendant_work_items": got.IncludeDescendantWorkItems,
		"include_descendants":           got.IncludeDescendants,
	} {
		t.Run(name, func(t *testing.T) {
			if value == nil || !*value {
				t.Errorf("%s = %v, want true", name, value)
			}
		})
	}
}

// TestFilters_ToSDK_EachBooleanReachesItsOwnField drives the five optional
// flags one at a time, because no fixture setting them together can tell them
// apart: a block of booleans has two values to go round, and setting all five
// to true is exactly the arrangement in which a converter routing confidential
// into exclude_projects reads correct. Setting one and requiring the other four
// to stay absent distinguishes all five.
func TestFilters_ToSDK_EachBooleanReachesItsOwnField(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		in   *Filters
	}{
		{name: "confidential", in: &Filters{Confidential: &yes}},
		{name: "exclude_group_work_items", in: &Filters{ExcludeGroupWorkItems: &yes}},
		{name: "exclude_projects", in: &Filters{ExcludeProjects: &yes}},
		{name: "include_descendant_work_items", in: &Filters{IncludeDescendantWorkItems: &yes}},
		{name: "include_descendants", in: &Filters{IncludeDescendants: &yes}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.in.toSDK()
			if err != nil {
				t.Fatalf("toSDK() unexpected error: %v", err)
			}
			for name, value := range map[string]*bool{
				"confidential":                  got.Confidential,
				"exclude_group_work_items":      got.ExcludeGroupWorkItems,
				"exclude_projects":              got.ExcludeProjects,
				"include_descendant_work_items": got.IncludeDescendantWorkItems,
				"include_descendants":           got.IncludeDescendants,
			} {
				t.Run(name, func(t *testing.T) {
					switch {
					case name != testCase.name && value != nil:
						t.Errorf("%s = %v, want absent while only %s is set", name, *value, testCase.name)
					case name == testCase.name && (value == nil || !*value):
						t.Errorf("%s = %v, want true", name, value)
					}
				})
			}
		})
	}
}

// nestedFilterFixture is the filter object the nested sub-filter tests convert.
// One fixture rather than one per test, because the point of each is that the
// same conversion carries every sub-object through at once.
func nestedFilterFixture() *Filters {
	yes := true
	return &Filters{
		CustomField: []CustomFieldFilter{{
			CustomFieldID:        "gid://gitlab/Issuables::CustomField/1",
			CustomFieldName:      "Team",
			SelectedOptionIDs:    []string{"gid://gitlab/Issuables::CustomFieldSelectOption/2"},
			SelectedOptionValues: []string{"Platform"},
		}},
		HierarchyFilters: &HierarchyFilter{
			ParentIDs:                  []string{"gid://gitlab/WorkItem/9"},
			IncludeDescendantWorkItems: &yes,
			ParentWildcardID:           "NONE",
		},
		Status: &StatusFilter{ID: "gid://gitlab/WorkItems::Statuses::Custom::Status/3", Name: "In progress"},
		Not: &NegatedFilters{
			AssigneeUsernames:   []string{"carol"},
			AuthorUsername:      []string{"dave"},
			CustomField:         []CustomFieldFilter{{CustomFieldName: "Team"}},
			HealthStatusFilter:  []string{"atRisk"},
			IterationID:         []string{"gid://gitlab/Iteration/4"},
			IterationWildcardID: "CURRENT",
			LabelName:           []string{"wontfix"},
			MilestoneTitle:      []string{"v0"},
			MilestoneWildcardID: "STARTED",
			MyReactionEmoji:     "thumbsdown",
			ParentIDs:           []string{"gid://gitlab/WorkItem/10"},
			ReleaseTag:          []string{"v0.9.0"},
			Types:               []string{"TASK"},
			Weight:              "1",
			WorkItemTypeIDs:     []string{"gid://gitlab/WorkItems::Type/6"},
		},
		Or: &UnionedFilters{
			AssigneeUsernames: []string{"erin"},
			AuthorUsernames:   []string{"frank"},
			CustomField:       []CustomFieldFilter{{CustomFieldID: "gid://gitlab/Issuables::CustomField/7"}},
			LabelNames:        []string{"urgent"},
		},
	}
}

// convertNestedFixture converts [nestedFilterFixture] and fails the test if the
// conversion errors.
func convertNestedFixture(t *testing.T) *gl.WorkItemSavedViewFilters {
	t.Helper()
	got, err := nestedFilterFixture().toSDK()
	if err != nil {
		t.Fatalf("toSDK() unexpected error: %v", err)
	}
	return got
}

// TestFilters_ToSDK_CustomFieldSubFilter verifies that a custom field filter
// carries both identifiers and both selected-option lists, each into the field
// it belongs in. Counting the entries is not enough: the option IDs and the
// option values are two lists of one string apiece, so a converter that swapped
// them satisfied every length this test used to check while asking GitLab to
// match an option by an identifier that names none.
func TestFilters_ToSDK_CustomFieldSubFilter(t *testing.T) {
	want := []gl.WorkItemCustomFieldFilter{{
		CustomFieldID:        new("gid://gitlab/Issuables::CustomField/1"),
		CustomFieldName:      new("Team"),
		SelectedOptionIDs:    []string{"gid://gitlab/Issuables::CustomFieldSelectOption/2"},
		SelectedOptionValues: []string{"Platform"},
	}}
	if got := convertNestedFixture(t).CustomField; !reflect.DeepEqual(got, want) {
		t.Errorf("CustomField = %s, want %s", pretty(got), pretty(want))
	}
}

// pretty renders a converted filter with its pointers dereferenced, so a
// failure names the values that differ instead of their addresses.
func pretty(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(encoded)
}

// TestFilters_ToSDK_HierarchyAndStatusSubFilters verifies the two small nested
// objects, which share a shape: a couple of scalars each, all optional. Each is
// compared whole, because a status addressed by the wrong one of its two
// identifiers matches nothing and looks, field by field, exactly like one
// addressed correctly.
func TestFilters_ToSDK_HierarchyAndStatusSubFilters(t *testing.T) {
	got := convertNestedFixture(t)

	t.Run("hierarchy_filters", func(t *testing.T) {
		want := &gl.WorkItemHierarchyFilter{
			ParentIDs:                  []string{"gid://gitlab/WorkItem/9"},
			IncludeDescendantWorkItems: new(true),
			ParentWildcardID:           new("NONE"),
		}
		if !reflect.DeepEqual(got.HierarchyFilters, want) {
			t.Errorf("HierarchyFilters = %s, want %s", pretty(got.HierarchyFilters), pretty(want))
		}
	})

	t.Run("status", func(t *testing.T) {
		want := &gl.WorkItemStatusFilter{
			ID:   new("gid://gitlab/WorkItems::Statuses::Custom::Status/3"),
			Name: new("In progress"),
		}
		if !reflect.DeepEqual(got.Status, want) {
			t.Errorf("Status = %s, want %s", pretty(got.Status), pretty(want))
		}
	})
}

// TestFilters_ToSDK_NegatedSubFilter verifies that every field of the "not"
// sub-filter reaches the SDK struct, and reaches the field it belongs in.
//
// It compares the struct whole rather than counting entries, which is what it
// did before: fifteen fields carrying one value apiece are all the same length,
// so labels could arrive as milestone titles and a reaction as a weight without
// a single count moving. What a caller sees then is a view excluding work items
// nobody asked to exclude.
func TestFilters_ToSDK_NegatedSubFilter(t *testing.T) {
	want := &gl.WorkItemSavedViewNegatedFilters{
		AssigneeUsernames:   []string{"carol"},
		AuthorUsername:      []string{"dave"},
		CustomField:         []gl.WorkItemCustomFieldFilter{{CustomFieldName: new("Team")}},
		HealthStatusFilter:  []string{"atRisk"},
		IterationID:         []string{"gid://gitlab/Iteration/4"},
		IterationWildcardID: new("CURRENT"),
		LabelName:           []string{"wontfix"},
		MilestoneTitle:      []string{"v0"},
		MilestoneWildcardID: new("STARTED"),
		MyReactionEmoji:     new("thumbsdown"),
		ParentIDs:           []string{"gid://gitlab/WorkItem/10"},
		ReleaseTag:          []string{"v0.9.0"},
		Types:               []string{"TASK"},
		Weight:              new("1"),
		WorkItemTypeIDs:     []string{"gid://gitlab/WorkItems::Type/6"},
	}
	if got := convertNestedFixture(t).Not; !reflect.DeepEqual(got, want) {
		t.Errorf("Not = %s, want %s", pretty(got), pretty(want))
	}
}

// TestFilters_ToSDK_UnionedSubFilter verifies that every field of the "or"
// sub-filter reaches the SDK struct, and reaches the field it belongs in: its
// four fields are all lists of one string, so only the values tell them apart.
func TestFilters_ToSDK_UnionedSubFilter(t *testing.T) {
	want := &gl.WorkItemSavedViewUnionedFilters{
		AssigneeUsernames: []string{"erin"},
		AuthorUsernames:   []string{"frank"},
		CustomField:       []gl.WorkItemCustomFieldFilter{{CustomFieldID: new("gid://gitlab/Issuables::CustomField/7")}},
		LabelNames:        []string{"urgent"},
	}
	if got := convertNestedFixture(t).Or; !reflect.DeepEqual(got, want) {
		t.Errorf("Or = %s, want %s", pretty(got), pretty(want))
	}
}

// TestFilters_ToSDK_Timestamps verifies that each of the eight time filters is
// parsed, and that the three accepted layouts all resolve to the same instant.
func TestFilters_ToSDK_Timestamps(t *testing.T) {
	in := &Filters{
		ClosedAfter:   "2025-01-01T00:00:00Z",
		ClosedBefore:  "2025-01-02T00:00:00Z",
		CreatedAfter:  "2025-01-03T00:00:00Z",
		CreatedBefore: "2025-01-04T00:00:00Z",
		DueAfter:      "2025-01-05T00:00:00Z",
		DueBefore:     "2025-01-06T00:00:00Z",
		UpdatedAfter:  "2025-01-07T00:00:00Z",
		UpdatedBefore: "2025-01-08T00:00:00Z",
	}
	got, err := in.toSDK()
	if err != nil {
		t.Fatalf("toSDK() unexpected error: %v", err)
	}
	days := map[string]struct {
		got  *time.Time
		want int
	}{
		"closed_after":   {got.ClosedAfter, 1},
		"closed_before":  {got.ClosedBefore, 2},
		"created_after":  {got.CreatedAfter, 3},
		"created_before": {got.CreatedBefore, 4},
		"due_after":      {got.DueAfter, 5},
		"due_before":     {got.DueBefore, 6},
		"updated_after":  {got.UpdatedAfter, 7},
		"updated_before": {got.UpdatedBefore, 8},
	}
	for name, tc := range days {
		t.Run(name, func(t *testing.T) {
			if tc.got == nil || tc.got.Day() != tc.want {
				t.Errorf("%s = %v, want day %d", name, tc.got, tc.want)
			}
		})
	}
}

// TestParseTimestamp verifies the layouts accepted for a filter timestamp, that
// an empty value is not an error, and that a malformed one names its field.
func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantNil bool
		wantErr bool
	}{
		{name: "empty is optional", value: "", wantNil: true},
		{name: "rfc3339", value: "2025-06-01T12:30:00Z"},
		{name: "local date and time", value: "2025-06-01T12:30:00"},
		{name: "date only", value: "2025-06-01"},
		{name: "prose is rejected", value: "next tuesday", wantErr: true},
		{name: "reversed date is rejected", value: "01-06-2025", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTimestamp("filters.due_before", tc.value)
			switch {
			case tc.wantErr:
				if err == nil {
					t.Fatalf("parseTimestamp(%q) error = nil, want one", tc.value)
				}
				if !strings.Contains(err.Error(), "filters.due_before") {
					t.Errorf("parseTimestamp(%q) error = %q, want it to name the field", tc.value, err)
				}
			case err != nil:
				t.Fatalf("parseTimestamp(%q) unexpected error: %v", tc.value, err)
			case tc.wantNil && got != nil:
				t.Errorf("parseTimestamp(%q) = %v, want nil", tc.value, got)
			case !tc.wantNil && got == nil:
				t.Errorf("parseTimestamp(%q) = nil, want a time", tc.value)
			}
		})
	}
}

// TestSubFilters_NilReceivers verifies that every nested converter tolerates a
// nil receiver, which is what lets the parent convert without a check per field.
func TestSubFilters_NilReceivers(t *testing.T) {
	var (
		negated   *NegatedFilters
		unioned   *UnionedFilters
		hierarchy *HierarchyFilter
		status    *StatusFilter
	)
	t.Run("not", func(t *testing.T) {
		if got := negated.toSDK(); got != nil {
			t.Errorf("toSDK() = %+v, want nil", got)
		}
	})
	t.Run("or", func(t *testing.T) {
		if got := unioned.toSDK(); got != nil {
			t.Errorf("toSDK() = %+v, want nil", got)
		}
	})
	t.Run("hierarchy", func(t *testing.T) {
		if got := hierarchy.toSDK(); got != nil {
			t.Errorf("toSDK() = %+v, want nil", got)
		}
	})
	t.Run("status", func(t *testing.T) {
		if got := status.toSDK(); got != nil {
			t.Errorf("toSDK() = %+v, want nil", got)
		}
	})
	t.Run("custom fields", func(t *testing.T) {
		if got := customFieldsToSDK(nil); got != nil {
			t.Errorf("customFieldsToSDK(nil) = %+v, want nil", got)
		}
	})
}

// TestFilters_TierTags_MatchTheWorkItemListFilters pins the tier of every
// gated filter, in all three filter objects, to the tier
// internal/tools/workitems publishes for the filter of the same name.
//
// The two domains publish the same condition names through different tools,
// and until this change one of them advertised every one of them as Free. The
// table is written down rather than read from the sibling package because
// ADR-0004 forbids a domain sub-package from importing another; the names are
// the join, so a tag dropped or downgraded on either side shows up as a
// mismatch here or in that package's own tier assertions.
func TestFilters_TierTags_MatchTheWorkItemListFilters(t *testing.T) {
	cases := []struct {
		object reflect.Type
		field  string
		want   string
	}{
		{reflect.TypeFor[Filters](), "CustomField", "premium"},
		{reflect.TypeFor[Filters](), "HealthStatusFilter", "ultimate"},
		{reflect.TypeFor[Filters](), "IterationCadenceID", "premium"},
		{reflect.TypeFor[Filters](), "IterationID", "premium"},
		{reflect.TypeFor[Filters](), "IterationWildcardID", "premium"},
		{reflect.TypeFor[Filters](), "Status", "premium"},
		{reflect.TypeFor[Filters](), "Weight", "premium"},
		{reflect.TypeFor[Filters](), "WeightWildcardID", "premium"},
		{reflect.TypeFor[NegatedFilters](), "CustomField", "premium"},
		{reflect.TypeFor[NegatedFilters](), "HealthStatusFilter", "ultimate"},
		{reflect.TypeFor[NegatedFilters](), "IterationID", "premium"},
		{reflect.TypeFor[NegatedFilters](), "IterationWildcardID", "premium"},
		{reflect.TypeFor[NegatedFilters](), "Weight", "premium"},
		{reflect.TypeFor[UnionedFilters](), "CustomField", "premium"},
	}
	for _, testCase := range cases {
		t.Run(testCase.object.Name()+"."+testCase.field, func(t *testing.T) {
			field, ok := testCase.object.FieldByName(testCase.field)
			if !ok {
				t.Fatalf("%s has no field %s", testCase.object.Name(), testCase.field)
			}
			if got := field.Tag.Get("tier"); got != testCase.want {
				t.Errorf("tier tag = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestOptionalString verifies that only a non-empty string produces a pointer.
func TestOptionalString(t *testing.T) {
	if got := optionalString(""); got != nil {
		t.Errorf("optionalString(\"\") = %v, want nil", got)
	}
	if got := optionalString("value"); got == nil || *got != "value" {
		t.Errorf("optionalString(\"value\") = %v, want a pointer to it", got)
	}
}
