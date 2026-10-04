package finegrained

import (
	"slices"
	"testing"
)

// testTable is a hand-built table covering every shape a row can take: an
// allowed action, one denied per cause, one with static degraded positions,
// and permissions with and without a grantable display.
func testTable() *Table {
	return &Table{
		Version:        "19.4.1-ee",
		Bucket:         "19.4",
		Permissions:    []string{"approve_merge_request", "read_issue", "read_role_only"},
		Display:        []string{"Merge Request: Approve", "Issue: Read", ""},
		RefusalDisplay: []string{"Merge Request: Approve", "Issue: Read", "Role: Only"},
		Assignables: []Assignable{
			{Name: "approve_merge_request", Permissions: []uint16{0}, Boundaries: BoundaryProject, Grantable: true},
			{Name: "read_work_item", Permissions: []uint16{1}, Boundaries: BoundaryProject | BoundaryGroup, Grantable: true},
			{Name: "read_issue", Permissions: []uint16{1}, Boundaries: BoundaryProject, Deprecated: true},
		},
		Groups: []Group{
			{Perms: []uint16{0}, Any: BoundaryProject},
			{Perms: []uint16{1, 2}, Any: BoundaryProject | BoundaryGroup},
		},
		Operations: []Operation{
			{Name: "POST /projects/:id/merge_requests/:merge_request_iid/approve", Groups: []uint32{0}},
			{Name: "query project", Spine: []uint32{0}, OffSpine: []uint32{1}},
		},
		Elements: []Element{
			{Path: "project", Type: "Project", Groups: []uint32{1}, Effect: EffectNull},
			{Path: "project.issueLinks.nodes", Type: "VulnerabilityIssueLink", Undeclared: true, Effect: EffectRemoved},
		},
		Actions: []Requirement{
			{ID: "epic.list", Denied: &Denial{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull}},
			{ID: "merge_request.approve", Paths: [][]uint32{{0}}},
			{ID: "vulnerability.get", Paths: [][]uint32{{1}}, Degraded: []uint32{1}, GraphQL: true},
		},
	}
}

// TestParseBoundary_KnownAndUnknownNames_ReadAsGitLabSpellsThem verifies the
// four boundary names are read exactly as GitLab spells them, and that any
// other spelling, a capital included, is refused rather than guessed.
func TestParseBoundary_KnownAndUnknownNames_ReadAsGitLabSpellsThem(t *testing.T) {
	cases := []struct {
		name string
		want Boundary
		ok   bool
	}{
		{name: "project", want: BoundaryProject, ok: true},
		{name: "group", want: BoundaryGroup, ok: true},
		{name: "user", want: BoundaryUser, ok: true},
		{name: "instance", want: BoundaryInstance, ok: true},
		{name: "Project", want: 0, ok: false},
		{name: "namespace", want: 0, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseBoundary(tc.name)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ParseBoundary(%q) = %v, %v; want %v, %v", tc.name, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestBoundary_NamesAndString_ReadInBitOrder verifies a set of boundaries is
// named project, group, user, instance in that order whatever order it was
// built in, so the same set always reads the same.
func TestBoundary_NamesAndString_ReadInBitOrder(t *testing.T) {
	cases := []struct {
		name  string
		set   Boundary
		names []string
		text  string
	}{
		{name: "empty", set: 0, names: nil, text: ""},
		{name: "project", set: BoundaryProject, names: []string{"project"}, text: "project"},
		{name: "project or group", set: BoundaryGroup | BoundaryProject, names: []string{"project", "group"}, text: "project or group"},
		{name: "all", set: AllBoundaries, names: []string{"project", "group", "user", "instance"}, text: "project or group or user or instance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.set.Names(); !slices.Equal(got, tc.names) {
				t.Errorf("Names() = %v, want %v", got, tc.names)
			}
			if got := tc.set.String(); got != tc.text {
				t.Errorf("String() = %q, want %q", got, tc.text)
			}
		})
	}
}

// TestCause_GraphQL_TellsTheGraphQLCausesApart verifies the four causes a
// GraphQL position or mutation gives are told apart from the REST causes and
// from a grant that lacks a permission.
func TestCause_GraphQL_TellsTheGraphQLCausesApart(t *testing.T) {
	cases := []struct {
		cause Cause
		want  bool
	}{
		{CauseMutationUndeclared, true},
		{CauseTypeUndeclared, true},
		{CausePayloadUndeclared, true},
		{CauseBoundaryUnresolvable, true},
		{CauseRESTTodo, false},
		{CauseRESTUndeclared, false},
		{CauseNotGranted, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.cause), func(t *testing.T) {
			if got := tc.cause.GraphQL(); got != tc.want {
				t.Errorf("GraphQL() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTable_Requirement_FindsARowByBinarySearch verifies every row is found by
// its ID and that an ID before the first row, between two rows or after the
// last finds none.
func TestTable_Requirement_FindsARowByBinarySearch(t *testing.T) {
	table := testTable()
	cases := []struct {
		id    string
		found bool
	}{
		{id: "epic.list", found: true},
		{id: "merge_request.approve", found: true},
		{id: "vulnerability.get", found: true},
		{id: "aaa.first", found: false},
		{id: "issue.list", found: false},
		{id: "zzz.last", found: false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			row := table.Requirement(tc.id)
			if (row != nil) != tc.found {
				t.Fatalf("Requirement(%q) = %v, want found %v", tc.id, row, tc.found)
			}
			if row != nil && row.ID != tc.id {
				t.Errorf("Requirement(%q).ID = %q", tc.id, row.ID)
			}
		})
	}
}

// TestTable_Requirement_PointsIntoTheTable verifies a row found is the
// table's own and not a copy, so every catalog that reads it shares one.
func TestTable_Requirement_PointsIntoTheTable(t *testing.T) {
	table := testTable()
	if table.Requirement("merge_request.approve") != &table.Actions[1] {
		t.Error("the row returned is a copy, so two catalogs would not share one requirement")
	}
}

// TestTable_NilTable_AnswersNothing verifies a nil table finds no row and no
// assignable permission, so a server built without one decides nothing.
func TestTable_NilTable_AnswersNothing(t *testing.T) {
	var table *Table
	if table.Requirement("issue.list") != nil {
		t.Error("a nil table returned a row")
	}
	if table.Assignable("read_issue") != nil {
		t.Error("a nil table returned an assignable")
	}
}

// TestTable_AssignableAndKnows_HoldAGrantToTheRecordedVocabulary verifies a
// permission name is looked up in the vocabulary the table was recorded with,
// a deprecated name kept as such, and that a grant is known only when every
// name it holds is one the table records.
func TestTable_AssignableAndKnows_HoldAGrantToTheRecordedVocabulary(t *testing.T) {
	table := testTable()
	if got := table.Assignable("read_issue"); got == nil || !got.Deprecated {
		t.Errorf("Assignable(read_issue) = %v, want the deprecated name kept", got)
	}
	if table.Assignable("create_everything") != nil {
		t.Error("an undefined name was found")
	}
	cases := []struct {
		name  string
		names []string
		want  bool
	}{
		{name: "none", names: nil, want: true},
		{name: "current and deprecated", names: []string{"read_work_item", "read_issue"}, want: true},
		{name: "one renamed", names: []string{"read_work_item", "read_work_items"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := table.Knows(tc.names); got != tc.want {
				t.Errorf("Knows(%v) = %v, want %v", tc.names, got, tc.want)
			}
		})
	}
}

// TestTable_DisplayVersion_DropsTheEditionSuffix verifies the release a page
// names is the version without its edition suffix, and a version with none is
// left as it is.
func TestTable_DisplayVersion_DropsTheEditionSuffix(t *testing.T) {
	cases := []struct{ version, want string }{
		{"19.4.1-ee", "19.4.1"},
		{"19.4.1", "19.4.1"},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			if got := (&Table{Version: tc.version}).DisplayVersion(); got != tc.want {
				t.Errorf("DisplayVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBucket_ReadsMajorAndMinor verifies a version is bucketed by its major
// and minor numbers, a suffix ignored, and that a version missing either
// number, or carrying one that is not a number, has no bucket.
func TestBucket_ReadsMajorAndMinor(t *testing.T) {
	cases := []struct{ version, want string }{
		{"19.4.1", "19.4"},
		{"19.4.1-ee", "19.4"},
		{"19.5.0-pre", "19.5"},
		{"19.40", "19.40"},
		{"17.9.2", "17.9"},
		{"19", ""},
		{"x.4.1", ""},
		{"-19.4", ""},
		{"19.x", ""},
		{".4", ""},
		{"19.", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			if got := Bucket(tc.version); got != tc.want {
				t.Errorf("Bucket(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}
