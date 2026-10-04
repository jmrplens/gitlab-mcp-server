package sdkroutes

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
)

// TestSubstitute_FormatVerbs verifies how a format string is filled: a verb
// takes the next value, flags are passed over, a doubled percent is one, and a
// verb with no value left, or a lone percent at the end, takes an unknown
// piece.
func TestSubstitute_FormatVerbs(t *testing.T) {
	cases := []struct {
		name   string
		format string
		values []string
		want   string
	}{
		{name: "plain", format: "projects/%s/issues/%d", values: []string{"a", "b"}, want: "projects/a/issues/b"},
		{name: "flags", format: "%-5s.%03d", values: []string{"a", "b"}, want: "a.b"},
		{name: "plus flag, the first of the set", format: "%+d/x", values: []string{"a"}, want: "a/x"},
		{name: "escaped percent", format: "100%%/%s", values: []string{"a"}, want: "100%/a"},
		{name: "escaped percent with flags", format: "%5%/%s", values: []string{"a"}, want: "%/a"},
		{name: "values run out", format: "%s/%s", values: []string{"a"}, want: "a/" + unknown},
		{name: "lone percent at the end", format: "ends/%", want: "ends/" + unknown},
		{name: "no verbs", format: "projects", values: []string{"unused"}, want: "projects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := substitute(tc.format, tc.values); got != tc.want {
				t.Errorf("substitute(%q, %v) = %q, want %q", tc.format, tc.values, got, tc.want)
			}
		})
	}
}

// TestShape_ReducesAPathToItsRoute verifies the shape a formatted path is
// reduced to and whether it names anything static.
func TestShape_ReducesAPathToItsRoute(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		literal bool
	}{
		{name: "placeholder segment", raw: "projects/" + unknown + "/issues", want: "/projects/:/issues", literal: true},
		{name: "embedded unknown", raw: "projects/archive" + unknown, want: "/projects/archive", literal: true},
		{name: "dot separator", raw: "archive." + unknown, want: "/archive", literal: true},
		{name: "dash separator", raw: "v-" + unknown, want: "/v", literal: true},
		{name: "underscore separator", raw: "by_" + unknown, want: "/by", literal: true},
		{name: "query string", raw: "search?scope=" + unknown, want: "/search", literal: true},
		{name: "fragment", raw: "docs#" + unknown, want: "/docs", literal: true},
		{name: "query string alone", raw: "?scope=" + unknown, want: "/", literal: false},
		{name: "leading and trailing slashes", raw: "/projects/", want: "/projects", literal: true},
		{name: "only unknown pieces", raw: unknown + unknown, want: "/", literal: false},
		{name: "only a placeholder", raw: unknown, want: "/:", literal: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, literal := shape(tc.raw)
			if got != tc.want || literal != tc.literal {
				t.Errorf("shape(%q) = %q, %v; want %q, %v", tc.raw, got, literal, tc.want, tc.literal)
			}
		})
	}
}

// TestSpellings_PassesOverAnEmptySpelling verifies that a failure branch's
// empty return is dropped beside any other spelling, and kept, once, only
// when there is nothing else.
func TestSpellings_PassesOverAnEmptySpelling(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "empty beside others", values: []string{"", "a", "", "b"}, want: []string{"a", "b"}},
		{name: "only empty", values: []string{"", ""}, want: []string{""}},
		{name: "none empty", values: []string{"a"}, want: []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := spellings(tc.values); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("spellings(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// TestCombinations_TakesOneSpellingPerPieceUpToTheBound verifies the order of
// the combinations, the single empty combination of no pieces, and the bound,
// which a piece with more spellings than it allows reaches on its own, past
// which one combination of overflow pieces follows the ones kept, the last
// piece's included when the bound was reached at the first.
func TestCombinations_TakesOneSpellingPerPieceUpToTheBound(t *testing.T) {
	got := combinations([][]string{{"a", "b"}, {"1"}, {"x", "y"}})
	want := [][]string{{"a", "1", "x"}, {"a", "1", "y"}, {"b", "1", "x"}, {"b", "1", "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("combinations() = %q, want %q", got, want)
	}
	if none := combinations(nil); len(none) != 1 || len(none[0]) != 0 {
		t.Errorf("combinations(nil) = %q, want one empty combination", none)
	}
	wide := make([]string, maxFolds+3)
	for i := range wide {
		wide[i] = strconv.Itoa(i)
	}
	bounded := combinations([][]string{{"p"}, wide})
	if len(bounded) != maxFolds+1 || bounded[maxFolds-1][1] != strconv.Itoa(maxFolds-1) {
		t.Errorf("combinations() of a %d-way piece = %d combinations, want the first %d and one more", len(wide), len(bounded), maxFolds)
	}
	if last := bounded[len(bounded)-1]; !reflect.DeepEqual(last, []string{overflow, overflow}) {
		t.Errorf("combinations() past the bound ends %q, want an overflow piece for each piece", last)
	}
	early := combinations([][]string{wide, {"z"}})
	if len(early) != maxFolds+1 || !reflect.DeepEqual(early[maxFolds], []string{overflow, overflow}) {
		t.Errorf("combinations() bounded at the first piece = %d combinations ending %q, want %d and the overflow one", len(early), early[len(early)-1], maxFolds+1)
	}
}

// TestLimit_KeepsTheBoundAndDropsRepeats verifies the bound on spellings, the
// overflow piece after the ones kept when a distinct spelling was left out,
// and none when only repeats were.
func TestLimit_KeepsTheBoundAndDropsRepeats(t *testing.T) {
	var values []string
	for i := range maxFolds + 4 {
		values = append(values, string(rune('a'+i)), string(rune('a'+i)))
	}
	kept := limit(values)
	if len(kept) != maxFolds+1 || kept[0] != "a" || kept[maxFolds-1] != string(rune('a'+maxFolds-1)) || kept[maxFolds] != overflow {
		t.Errorf("limit() = %q, want the first %d distinct spellings and the overflow piece", kept, maxFolds)
	}
	var exact []string
	for i := range maxFolds {
		exact = append(exact, string(rune('a'+i)), string(rune('a'+i)))
	}
	if got := limit(exact); len(got) != maxFolds || slices.Contains(got, overflow) {
		t.Errorf("limit() of %d distinct spellings, each repeated = %q, want them all and no overflow piece", maxFolds, got)
	}
	if empty := limit(nil); empty != nil {
		t.Errorf("limit(nil) = %v, want nil", empty)
	}
}

// TestConcat_JoinsEverySpelling verifies the cross product of two pieces.
func TestConcat_JoinsEverySpelling(t *testing.T) {
	got := concat([]string{"a", "b"}, []string{"1", "2"})
	if want := []string{"a1", "a2", "b1", "b2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("concat() = %v, want %v", got, want)
	}
}

// TestVerbOf_DefaultsToGet verifies the verb of a method that names none.
func TestVerbOf_DefaultsToGet(t *testing.T) {
	if got := verbOf(""); got != "GET" {
		t.Errorf("verbOf(\"\") = %q, want GET", got)
	}
	if got := verbOf("POST"); got != "POST" {
		t.Errorf("verbOf(POST) = %q, want POST", got)
	}
}
