// constant_index_test.go covers the constant-index rule: the fixture text
// that makes two elements of a slice tell each other apart, the rule that
// reads a render for one element printed in every element's place, and the
// declaration table's own discipline.
package main

import (
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
	t.Cleanup(func() { constantIndexDeclarations = map[string]string{} })
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
