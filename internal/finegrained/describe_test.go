package finegrained

import (
	"reflect"
	"testing"
)

// describeTable is a table whose rows exercise every way Describe words one:
// two operations whose groups word the same, a request that opts out of the
// grant, a GraphQL operation with spine and off-spine elements, and a
// permission no assignable words.
func describeTable() *Table {
	return &Table{
		Version:     "19.4.1-ee",
		Permissions: []string{"read_a", "read_b", "write_c", "read_d"},
		// read_b has no words and read_d none either, the slice stopping
		// short of it, so both fall back to their raw names.
		Display: []string{"A: Read", "", "C: Write"},
		Groups: []Group{
			{Perms: []uint16{0}, Any: BoundaryProject},
			{Perms: []uint16{1}, Any: BoundaryGroup},
			{Perms: []uint16{0, 2, 0}, Any: BoundaryProject},
			{Perms: []uint16{0}, Any: BoundaryProject},
			{Perms: []uint16{3}, Any: BoundaryUser | BoundaryInstance},
		},
		Operations: []Operation{
			{Name: "GET /a", Groups: []uint32{0}},
			{Name: "GET /a/other", Groups: []uint32{3}},
			{Name: "GET /skipped", Skip: true},
			{Name: "query thing", Groups: []uint32{1}, Spine: []uint32{0}, OffSpine: []uint32{1, 1}},
			{Name: "GET /twice", Groups: []uint32{0, 3, 4}},
			{Name: "mutation skipped", Groups: []uint32{0}, Skip: true, Spine: []uint32{1}},
		},
		Elements: []Element{
			{Path: "thing", Groups: []uint32{2}},
			{Path: "thing.part", Groups: []uint32{1}},
			{Path: "thing.never", Undeclared: true},
		},
		Actions: []Requirement{
			{ID: "a.denied", Denied: &Denial{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull}},
			{ID: "a.graphql", Paths: [][]uint32{{3, 0}}, Degraded: []uint32{2}},
			{ID: "a.skipped", Paths: [][]uint32{{5}}},
			{ID: "a.some", Paths: [][]uint32{{0}}, DeniedWays: []Denial{
				{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull},
			}},
			{ID: "a.twice", Paths: [][]uint32{{4}}},
			{ID: "a.ways", Paths: [][]uint32{{0}, {1}, {2}}},
		},
	}
}

// TestTable_Describe_NothingToDescribe_IsNil verifies a nil table and a nil
// row describe nothing, so a detail with no row carries no block.
func TestTable_Describe_NothingToDescribe_IsNil(t *testing.T) {
	var none *Table
	if got := none.Describe(&Requirement{}); got != nil {
		t.Errorf("Describe on a nil table = %+v, want nil", got)
	}
	if got := describeTable().Describe(nil); got != nil {
		t.Errorf("Describe(nil) = %+v, want nil", got)
	}
}

// TestTable_Describe_WordsEachRow verifies each kind of row is worded the way
// the detail and the reference page print it: a denial with its cause, element
// and effect, the ways of running an action in GitLab's words, the parts of
// the answer served empty, a request the grant does not judge, and two paths
// that need the same thing written once.
func TestTable_Describe_WordsEachRow(t *testing.T) {
	table := describeTable()
	cases := []struct {
		id   string
		want *Description
	}{
		{
			id: "a.denied",
			want: &Description{GitLabVersion: "19.4.1", Denied: &Denial{
				Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull,
			}},
		},
		{
			// The second path words the same as the first, so the action
			// has two ways, not three; the skipped request needs nothing the
			// grant decides.
			id: "a.ways",
			want: &Description{GitLabVersion: "19.4.1", AnyOf: []Way{
				{Needs: []Need{{Permissions: []string{"A: Read"}, At: []string{"project"}}}},
				{NotJudged: true},
			}},
		},
		{
			// The operation's group, its spine element's and the REST
			// request's, in that order, each permission once; the off-spine
			// element named twice is listed once.
			id: "a.graphql",
			want: &Description{
				GitLabVersion: "19.4.1",
				AnyOf: []Way{{Needs: []Need{
					{Permissions: []string{"read_b"}, At: []string{"group"}},
					{Permissions: []string{"A: Read", "C: Write"}, At: []string{"project"}},
					{Permissions: []string{"A: Read"}, At: []string{"project"}},
				}}},
				AlwaysEmpty: []string{"thing { never }"},
				EmptyWithout: []Position{{
					Selection: "thing { part }", Needs: []Need{{Permissions: []string{"read_b"}, At: []string{"group"}}},
				}},
			},
		},
		{
			// A mutation that opts out of the check leaves its own group to
			// GitLab, while the object its answer is made of is still checked.
			id: "a.skipped",
			want: &Description{GitLabVersion: "19.4.1", AnyOf: []Way{{
				Needs:     []Need{{Permissions: []string{"read_b"}, At: []string{"group"}}},
				NotJudged: true,
			}}},
		},
		{
			// A way no token passes is named beside the way that runs.
			id: "a.some",
			want: &Description{
				GitLabVersion: "19.4.1",
				AnyOf:         []Way{{Needs: []Need{{Permissions: []string{"A: Read"}, At: []string{"project"}}}}},
				DeniedWays:    []Denial{{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull}},
			},
		},
		{
			// Two groups wording the same are one need; a permission past
			// the end of the words reads as its raw name.
			id: "a.twice",
			want: &Description{GitLabVersion: "19.4.1", AnyOf: []Way{{Needs: []Need{
				{Permissions: []string{"A: Read"}, At: []string{"project"}},
				{Permissions: []string{"read_d"}, At: []string{"user", "instance"}},
			}}}},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			got := table.Describe(table.Requirement(testCase.id))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("Describe(%s) =\n%+v\nwant\n%+v", testCase.id, got, testCase.want)
			}
		})
	}
}

// TestTable_Describe_DeniedIsACopy verifies the description does not hand a
// reader a pointer into the table, which every session shares: neither the
// denial of a row nothing reaches nor the denied ways of one something does.
func TestTable_Describe_DeniedIsACopy(t *testing.T) {
	table := describeTable()
	row := table.Requirement("a.denied")
	got := table.Describe(row)
	got.Denied.Element = "changed"
	if row.Denied.Element != "Namespace" {
		t.Errorf("writing the description changed the table's denial to %q", row.Denied.Element)
	}
	some := table.Requirement("a.some")
	table.Describe(some).DeniedWays[0].Element = "changed"
	if some.DeniedWays[0].Element != "Namespace" {
		t.Errorf("writing the description changed the table's denied way to %q", some.DeniedWays[0].Element)
	}
}

// TestSelection_WritesAPositionAsTheSelectionThatReachesIt verifies a dotted
// path is written as the GraphQL selection reaching it, which no reader takes
// for an action ID, and a root field as itself.
func TestSelection_WritesAPositionAsTheSelectionThatReachesIt(t *testing.T) {
	cases := map[string]string{
		"vulnerability":                            "vulnerability",
		"vulnerability.findingTokenStatus":         "vulnerability { findingTokenStatus }",
		"project.vulnerabilities.nodes.issueLinks": "project { vulnerabilities { nodes { issueLinks } } }",
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := Selection(path); got != want {
				t.Errorf("Selection(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// TestSameWay_DiffersOnEitherHalf verifies two ways are the same only when
// their needs and whether the grant judges them both agree.
func TestSameWay_DiffersOnEitherHalf(t *testing.T) {
	need := Need{Permissions: []string{"A: Read"}, At: []string{"project"}}
	cases := []struct {
		name string
		a, b Way
		want bool
	}{
		{name: "same", a: Way{Needs: []Need{need}}, b: Way{Needs: []Need{need}}, want: true},
		{name: "judged differently", a: Way{Needs: []Need{need}}, b: Way{Needs: []Need{need}, NotJudged: true}},
		{name: "needs differ", a: Way{Needs: []Need{need}}, b: Way{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := sameWay(testCase.a, testCase.b); got != testCase.want {
				t.Errorf("sameWay() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestSameNeed_DiffersOnEitherHalf verifies two needs are the same only when
// their permissions and their boundaries both agree.
func TestSameNeed_DiffersOnEitherHalf(t *testing.T) {
	need := Need{Permissions: []string{"A: Read"}, At: []string{"project"}}
	cases := []struct {
		name  string
		other Need
		want  bool
	}{
		{name: "same", other: Need{Permissions: []string{"A: Read"}, At: []string{"project"}}, want: true},
		{name: "other permissions", other: Need{Permissions: []string{"B: Read"}, At: []string{"project"}}},
		{name: "other boundary", other: Need{Permissions: []string{"A: Read"}, At: []string{"group"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := sameNeed(need, testCase.other); got != testCase.want {
				t.Errorf("sameNeed() = %t, want %t", got, testCase.want)
			}
		})
	}
}
