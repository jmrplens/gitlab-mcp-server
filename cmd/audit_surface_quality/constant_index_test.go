// constant_index_test.go covers the constant-index rule: the fixture text
// that makes two elements of a slice tell each other apart, the rule that
// reads a render for one element printed in every element's place, and the
// declaration table's own discipline.
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// constantIndexRow is one row of the list the rule's tests render.
type constantIndexRow struct {
	Name      string
	WebURL    string
	CreatedAt string
}

// constantIndexList is the output type the rule's tests fill: one field, a
// list of rows, which is the shape a list formatter reads.
type constantIndexList struct {
	Rows []constantIndexRow
}

// filledConstantIndexList returns the populated fixture the rule reads,
// filled through the audit's own text so the two rows differ.
func filledConstantIndexList(t *testing.T) (reflect.Value, constantIndexList) {
	t.Helper()
	value := testutil.FillFixture(reflect.TypeFor[constantIndexList](), testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: fixtureText})
	return value, value.Interface().(constantIndexList)
}

// textResult wraps rendered Markdown the way a formatter's result carries
// it, which is what the rule scans.
func textResult(md string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: md}}}
}

// TestFixtureText_SliceElements_AreTellableApart checks the property the
// whole rule rests on: the shared filler spells a sentinel from the field's
// name alone, so both rows of a slice of structs carry the same text, and
// the audit's own text spells the path instead so they differ. The two
// shapes a render depends on are kept: an address stays under the fixture
// origin and an instant stays the RFC 3339 sentinel.
func TestFixtureText_SliceElements_AreTellableApart(t *testing.T) {
	shared := testutil.FillFixture(reflect.TypeFor[constantIndexList](), testutil.FixtureOptions{State: testutil.FixtureMultiPage}).Interface().(constantIndexList)
	if shared.Rows[0].Name != shared.Rows[1].Name {
		t.Fatalf("the shared filler already tells the rows apart (%q, %q); the audit's own text is no longer needed",
			shared.Rows[0].Name, shared.Rows[1].Name)
	}

	_, list := filledConstantIndexList(t)
	if list.Rows[0].Name == list.Rows[1].Name {
		t.Errorf("both rows carry %q, so no render could tell one from the other", list.Rows[0].Name)
	}
	if !strings.HasPrefix(list.Rows[0].WebURL, testutil.FixtureURLBase) {
		t.Errorf("WebURL = %q, want an address under %q", list.Rows[0].WebURL, testutil.FixtureURLBase)
	}
	if list.Rows[0].WebURL == list.Rows[1].WebURL {
		t.Errorf("both rows link to %q, so a link column could not tell one row from the other", list.Rows[0].WebURL)
	}
	if list.Rows[0].CreatedAt != testutil.FixtureSentinelRFC3339 {
		t.Errorf("CreatedAt = %q, want the RFC 3339 sentinel so the timestamp rules still read it", list.Rows[0].CreatedAt)
	}
}

// TestSanitizePath_PathWithNoField_IsNamed checks the one path the walk can
// reach that names no field at all, which the sentinel still has to spell.
func TestSanitizePath_PathWithNoField_IsNamed(t *testing.T) {
	if got := sanitizePath(""); got != "Value" {
		t.Errorf("sanitizePath(\"\") = %q, want Value", got)
	}
	if got := sanitizePath(".Rows0.Web URL!"); got != "Rows0WebURL" {
		t.Errorf("sanitizePath() = %q, want the letters and digits of the path", got)
	}
}

// TestSanitizePath_CharactersAboveEachRange_AreDropped checks the upper end
// of each of the three character ranges, which a path made of punctuation
// below them never reaches. A sentinel is compared against a render as a
// substring, so a character the sanitizer let through could carry Markdown
// into the comparison and a byte that closes a table cell would make the rule
// read a render it never produced.
func TestSanitizePath_CharactersAboveEachRange_AreDropped(t *testing.T) {
	// '~' is above 'z', '[' is above 'Z', and ':' is above '9'.
	if got := sanitizePath("a~B[3:"); got != "aB3" {
		t.Errorf("sanitizePath() = %q, want only the letters and digits", got)
	}
	if got := sanitizePath("~[:"); got != "Value" {
		t.Errorf("sanitizePath() = %q, want Value when nothing survives", got)
	}
}

// TestSanitizePath_EndsOfEachRange_AreKept checks both ends of the three
// ranges the sanitizer keeps. A range test that drops its last letter would
// spell a sentinel one character short and still name one position of the
// fixture, so nothing downstream could tell.
func TestSanitizePath_EndsOfEachRange_AreKept(t *testing.T) {
	if got := sanitizePath("az.AZ 09"); got != "azAZ09" {
		t.Errorf("sanitizePath() = %q, want both ends of every range kept", got)
	}
}

// TestConstantIndexViolations_RenderReadsTheRule checks the three renders the
// rule has to tell apart: every element rendered, the head rendered in every
// element's place, and a summary that renders the head once.
func TestConstantIndexViolations_RenderReadsTheRule(t *testing.T) {
	value, list := filledConstantIndexList(t)
	cases := []struct {
		name   string
		md     string
		report bool
	}{
		{
			name: "every element rendered",
			md:   "| " + list.Rows[0].Name + " |\n| " + list.Rows[1].Name + " |\n",
		},
		{
			name:   "the head rendered in every element's place",
			md:     "| " + list.Rows[0].Name + " |\n| " + list.Rows[0].Name + " |\n",
			report: true,
		},
		{
			name: "a summary that names the head once",
			md:   "2 rows, the first is " + list.Rows[0].Name + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := constantIndexViolations("pkg.Output", value, textResult(tc.md))
			if tc.report && len(got) != 1 {
				t.Fatalf("constantIndexViolations() = %+v, want one violation", got)
			}
			if !tc.report && len(got) != 0 {
				t.Fatalf("constantIndexViolations() = %+v, want no violation", got)
			}
			if tc.report {
				if got[0].category != constantIndexCategory {
					t.Errorf("category = %q, want %q", got[0].category, constantIndexCategory)
				}
				if !strings.Contains(got[0].detail, list.Rows[0].Name) || !strings.Contains(got[0].detail, list.Rows[1].Name) {
					t.Errorf("detail = %q, want both sentinels named", got[0].detail)
				}
			}
		})
	}
}

// TestConstantIndexViolations_Detail_NamesTheRepeatedElementFirst pins which
// of the two sentinels each half of the message is about. Both appear in it,
// so a message that named them the other way round would still carry both and
// would tell a reader the rendered element is the missing one.
func TestConstantIndexViolations_Detail_NamesTheRepeatedElementFirst(t *testing.T) {
	value, list := filledConstantIndexList(t)
	md := "| " + list.Rows[0].Name + " |\n| " + list.Rows[0].Name + " |\n"

	got := constantIndexViolations("pkg.Output", value, textResult(md))
	if len(got) != 1 {
		t.Fatalf("constantIndexViolations() = %+v, want one violation", got)
	}
	want := fmt.Sprintf("the populated render carries %q twice and %q never, which is one element of a list rendered for every element",
		list.Rows[0].Name, list.Rows[1].Name)
	if got[0].detail != want {
		t.Errorf("detail = %q, want %q", got[0].detail, want)
	}
	if got[0].tool != "pkg.Output" {
		t.Errorf("tool = %q, want the name the report prints for the formatter", got[0].tool)
	}
}

// TestCollectSlicePairs_Array_IsReadLikeASlice checks the kind a fixture
// filler never produces and a hand-written output type can: an array of two
// elements is paired and walked exactly as a slice is, and the pair's first
// entry is the first element.
func TestCollectSlicePairs_Array_IsReadLikeASlice(t *testing.T) {
	type row struct{ Name string }
	value := reflect.ValueOf(struct{ Rows [2]row }{Rows: [2]row{{Name: "firstRow"}, {Name: "secondRow"}}})

	var pairs [][2][]string
	collectSlicePairs(value, 0, &pairs)
	if len(pairs) != 1 {
		t.Fatalf("collectSlicePairs() found %d pair(s), want the array paired", len(pairs))
	}
	if got := pairs[0]; len(got[0]) != 1 || got[0][0] != "firstRow" || len(got[1]) != 1 || got[1][0] != "secondRow" {
		t.Errorf("pair = %v, want the first element's strings beside the second's", got)
	}

	var strs []string
	collectStrings(value, 0, &strs)
	if len(strs) != 2 || strs[0] != "firstRow" || strs[1] != "secondRow" {
		t.Errorf("collectStrings() = %v, want both elements of the array in order", strs)
	}
}

// TestCollectSlicePairs_ElementsCarryingDifferentCounts_AreNotPaired checks
// the guard that keeps the rule from comparing strings at positions that do
// not correspond: a nil element carries no strings, so the two lists are not
// the same length and reading them off as the same positions would be wrong.
func TestCollectSlicePairs_ElementsCarryingDifferentCounts_AreNotPaired(t *testing.T) {
	type row struct{ Name string }
	value := reflect.ValueOf(struct{ Rows []*row }{Rows: []*row{{Name: "firstRow"}, nil}})

	var pairs [][2][]string
	collectSlicePairs(value, 0, &pairs)
	if len(pairs) != 0 {
		t.Errorf("collectSlicePairs() = %v, want nothing paired when the elements carry different counts", pairs)
	}
}

// TestResultText_BlocksOtherThanText_AreLeftOut checks what the rule scans: a
// tool result may carry an image or an embedded resource beside its Markdown,
// and only the text a client reads is searched for a sentinel.
func TestResultText_BlocksOtherThanText_AreLeftOut(t *testing.T) {
	result := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{MIMEType: "image/png"},
		&mcp.TextContent{Text: "read me"},
		&mcp.EmbeddedResource{},
	}}
	if got := resultText(result); got != "read me\n" {
		t.Errorf("resultText() = %q, want only the text block", got)
	}
}

// TestConstantIndexViolations_NothingRendered_IsNotReported checks that a
// formatter the audit has no text from raises nothing, since the envelope
// section already counts a silent render.
func TestConstantIndexViolations_NothingRendered_IsNotReported(t *testing.T) {
	value, _ := filledConstantIndexList(t)
	if got := constantIndexViolations("pkg.Output", value, textResult("")); len(got) != 0 {
		t.Errorf("constantIndexViolations() = %+v, want no violation for an empty render", got)
	}
	if got := constantIndexViolations("pkg.Output", value, nil); len(got) != 0 {
		t.Errorf("constantIndexViolations() = %+v, want no violation for no result", got)
	}
}

// TestCollectSlicePairs_Nesting_ReachesTheInnerList checks that the walk
// pairs a list inside a list too, so a constant index in an inner loop is
// read as well as one in the outer.
func TestCollectSlicePairs_Nesting_ReachesTheInnerList(t *testing.T) {
	type inner struct{ Label string }
	type outer struct{ Groups []struct{ Items []inner } }

	value := testutil.FillFixture(reflect.TypeFor[outer](), testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: fixtureText})
	var pairs [][2][]string
	collectSlicePairs(value, 0, &pairs)

	if len(pairs) < 3 {
		t.Fatalf("collectSlicePairs() found %d pair(s), want the outer list and one inner list per element", len(pairs))
	}
	for _, pair := range pairs {
		if len(pair[0]) != len(pair[1]) {
			t.Errorf("a pair holds %d and %d string(s); the two elements have the same shape", len(pair[0]), len(pair[1]))
		}
		for i := range pair[0] {
			if pair[0][i] == pair[1][i] {
				t.Errorf("both elements carry %q at position %d", pair[0][i], i)
			}
		}
	}
}

// TestCollectSlicePairs_PastTheDepthBound_Stops checks the bound that keeps
// a type containing itself from walking forever. The filler stops before
// this depth, so nothing a fixture carries reaches it, and the bound is the
// second one: for a shape reached by another route.
func TestCollectSlicePairs_PastTheDepthBound_Stops(t *testing.T) {
	type row struct{ Name string }
	value := testutil.FillFixture(reflect.TypeFor[struct{ Rows []row }](), testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: fixtureText})

	var pairs [][2][]string
	collectSlicePairs(value, fixtureWalkDepth+1, &pairs)
	if len(pairs) != 0 {
		t.Errorf("collectSlicePairs() past the bound = %v, want nothing", pairs)
	}

	var strs []string
	collectStrings(value, fixtureWalkDepth+1, &strs)
	if len(strs) != 0 {
		t.Errorf("collectStrings() past the bound = %v, want nothing", strs)
	}
}

// rowChain is a shape the filler never builds: a list carried at a level of
// the test's choosing, so the walk's bound can be met exactly rather than
// only overshot.
type rowChain struct {
	Rows []constantIndexRow
	Next *rowChain
}

// chainWithRowsAt returns a pointer to a chain whose two rows sit at the
// given level, every level above carrying only its link. Reached through the
// pointer at the top, level k's Rows field is at depth 2k+2 of the walk.
func chainWithRowsAt(level int) reflect.Value {
	head := &rowChain{}
	node := head
	for range level {
		node.Next = &rowChain{}
		node = node.Next
	}
	node.Rows = []constantIndexRow{{Name: "firstRow"}, {Name: "secondRow"}}
	return reflect.ValueOf(head)
}

// nameChain is the same shape for the string walk: a name carried at a
// chosen level, at depth 2k+2 through the pointer at the top.
type nameChain struct {
	Name string
	Next *nameChain
}

// chainWithNameAt returns a pointer to a chain whose one name sits at the
// given level.
func chainWithNameAt(level int) reflect.Value {
	head := &nameChain{}
	node := head
	for range level {
		node.Next = &nameChain{}
		node = node.Next
	}
	node.Name = "deepName"
	return reflect.ValueOf(head)
}

// wrapInSlices wraps v in n slices of one element each, so v is reached at
// depth n through elements alone.
func wrapInSlices(v reflect.Value, n int) reflect.Value {
	for range n {
		wrapper := reflect.MakeSlice(reflect.SliceOf(v.Type()), 1, 1)
		wrapper.Index(0).Set(v)
		v = wrapper
	}
	return v
}

// TestCollectSlicePairs_DepthBound_IsMetAtTheBoundAndNotBeyond checks that
// the bound is counted at every level of the walk, a pointer, a field and an
// element alike: a list at exactly fixtureWalkDepth is paired and one past
// it is not, whichever kinds the path to it is made of. A level that stopped
// counting would walk a self-referential value until the stack ran out, and
// nothing a fixture carries is deep enough to notice.
func TestCollectSlicePairs_DepthBound_IsMetAtTheBoundAndNotBeyond(t *testing.T) {
	rows := reflect.ValueOf([]constantIndexRow{{Name: "firstRow"}, {Name: "secondRow"}})
	cases := []struct {
		name   string
		value  reflect.Value
		paired bool
	}{
		{name: "pointers and fields to the bound", value: chainWithRowsAt(3), paired: true},
		{name: "pointers and fields past the bound", value: chainWithRowsAt(4)},
		{name: "elements to the bound", value: wrapInSlices(rows, fixtureWalkDepth), paired: true},
		{name: "elements past the bound", value: wrapInSlices(rows, fixtureWalkDepth+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pairs [][2][]string
			collectSlicePairs(tc.value, 0, &pairs)
			if !tc.paired {
				if len(pairs) != 0 {
					t.Errorf("collectSlicePairs() = %v, want nothing past the bound", pairs)
				}
				return
			}
			want := [][2][]string{{{"firstRow"}, {"secondRow"}}}
			if !reflect.DeepEqual(pairs, want) {
				t.Errorf("collectSlicePairs() = %v, want %v", pairs, want)
			}
		})
	}
}

// TestCollectStrings_DepthBound_IsMetAtTheBoundAndNotBeyond is the same check
// over the string walk, which counts its own depth from the element it is
// handed: a string at exactly the bound is read and one past it is not.
func TestCollectStrings_DepthBound_IsMetAtTheBoundAndNotBeyond(t *testing.T) {
	deep := reflect.ValueOf([]string{"deepName"})
	cases := []struct {
		name  string
		value reflect.Value
		read  bool
	}{
		{name: "pointers and fields to the bound", value: chainWithNameAt(3), read: true},
		{name: "pointers and fields past the bound", value: chainWithNameAt(4)},
		// The string is one level under the innermost slice.
		{name: "elements to the bound", value: wrapInSlices(deep, fixtureWalkDepth-1), read: true},
		{name: "elements past the bound", value: wrapInSlices(deep, fixtureWalkDepth)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var strs []string
			collectStrings(tc.value, 0, &strs)
			if !tc.read {
				if len(strs) != 0 {
					t.Errorf("collectStrings() = %v, want nothing past the bound", strs)
				}
				return
			}
			if len(strs) != 1 || strs[0] != "deepName" {
				t.Errorf("collectStrings() = %v, want the one string at the bound", strs)
			}
		})
	}
}

// TestCollectSlicePairs_ElementsCarryingNoStrings_AreNotPaired checks that a
// list whose elements carry nothing to compare records no pair: an empty
// pair would be walked by the rule and could never report anything, and a
// list of numbers is what most output types carry beside their rows.
func TestCollectSlicePairs_ElementsCarryingNoStrings_AreNotPaired(t *testing.T) {
	value := reflect.ValueOf(struct{ Counts []int }{Counts: []int{1, 2}})

	var pairs [][2][]string
	collectSlicePairs(value, 0, &pairs)
	if len(pairs) != 0 {
		t.Errorf("collectSlicePairs() = %v, want no pair for elements carrying no strings", pairs)
	}
}

// TestCollectStrings_Map_IsLeftOut checks the one kind the walk declines: a
// map's entries arrive in no order, so two elements' strings could not be
// read off as the same positions.
func TestCollectStrings_Map_IsLeftOut(t *testing.T) {
	type withMap struct {
		Name   string
		Labels map[string]string
	}
	value := testutil.FillFixture(reflect.TypeFor[withMap](), testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: fixtureText})
	var got []string
	collectStrings(value, 0, &got)

	if len(got) != 1 {
		t.Fatalf("collectStrings() = %v, want only the string field", got)
	}
}

// TestApplyConstantIndexDeclarations_Discipline checks what a declaration
// does and what it has to be: it answers a finding, and one that answers
// nothing, or gives no reason, is itself reported.
func TestApplyConstantIndexDeclarations_Discipline(t *testing.T) {
	// Restore what the tree declares rather than an empty map: a cleanup that
	// wrote its own empty one would drop a declaration added later for every
	// test that runs after this.
	committed := constantIndexDeclarations
	t.Cleanup(func() { constantIndexDeclarations = committed })
	constantIndexDeclarations = map[string]string{
		"pkg.Answered": "renders the head of the list on purpose",
		"pkg.Stale":    "nothing reports this any more",
		"pkg.Silent":   "",
	}

	got := applyConstantIndexDeclarations([]violation{
		{"pkg.Answered", constantIndexCategory, "reported"},
		{"pkg.Other", constantIndexCategory, "reported"},
	})

	if len(got) != 3 {
		t.Fatalf("applyConstantIndexDeclarations() = %+v, want the undeclared finding and the two bad declarations", got)
	}
	if got[0].tool != "pkg.Other" {
		t.Errorf("first violation = %q, want the undeclared finding kept", got[0].tool)
	}
	if got[1].tool != "pkg.Silent" || !strings.Contains(got[1].detail, "no reason") {
		t.Errorf("second violation = %+v, want the reasonless declaration reported", got[1])
	}
	if got[2].tool != "pkg.Stale" || !strings.Contains(got[2].detail, "stale") {
		t.Errorf("third violation = %+v, want the stale declaration reported", got[2])
	}
}

// TestConstantIndex_ServedFormatters_RenderEveryElement is the rule over the
// real registry: every formatter the server registers renders each element
// of a populated list.
//
// The types this package's own tests register are left out by name. The
// Markdown registry is global and has no way to unregister, so a formatter
// registered to prove a rule fires stays in the walk for the rest of the
// process; nothing the server registers is declared in package main.
func TestConstantIndex_ServedFormatters_RenderEveryElement(t *testing.T) {
	_, constant := auditResultEnvelopes()

	for _, v := range applyConstantIndexDeclarations(constant) {
		if strings.HasPrefix(v.tool, "main.") {
			continue
		}
		t.Errorf("%s [%s]: %s", v.tool, v.category, v.detail)
	}
}

// TestAuditResultEnvelopes_ConstantIndex_IsReadOffThePopulatedRender checks
// the wiring: a formatter registered here that prints its first row twice is
// reported by the same walk that drives the envelope section, and the
// envelope section itself stays a report.
func TestAuditResultEnvelopes_ConstantIndex_IsReadOffThePopulatedRender(t *testing.T) {
	type constantIndexed struct{ Rows []constantIndexRow }
	toolutil.RegisterMarkdown(func(v constantIndexed) string {
		var b strings.Builder
		b.WriteString("## Rows (2)\n\n| Name |\n| --- |\n")
		for range v.Rows {
			b.WriteString("| " + v.Rows[0].Name + " |\n")
		}
		return b.String()
	})

	_, constant := auditResultEnvelopes()

	var found bool
	for _, v := range constant {
		if strings.Contains(v.tool, "constantIndexed") {
			found = true
		}
	}
	if !found {
		t.Errorf("the constant-index list = %+v, want the formatter registered here reported", constant)
	}
}
