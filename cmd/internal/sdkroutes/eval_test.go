package sdkroutes

import (
	"reflect"
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

// TestFirst_PrefersANonEmptySpelling verifies that a failure branch's empty
// return is passed over, and taken only when there is nothing else.
func TestFirst_PrefersANonEmptySpelling(t *testing.T) {
	if got := first([]string{"", "a", "b"}); got != "a" {
		t.Errorf("first() = %q, want %q", got, "a")
	}
	if got := first([]string{"", ""}); got != "" {
		t.Errorf("first() of only empty spellings = %q, want empty", got)
	}
}

// TestLimit_KeepsTheBoundAndDropsRepeats verifies the bound on spellings.
func TestLimit_KeepsTheBoundAndDropsRepeats(t *testing.T) {
	var values []string
	for i := range maxFolds + 4 {
		values = append(values, string(rune('a'+i)), string(rune('a'+i)))
	}
	kept := limit(values)
	if len(kept) != maxFolds || kept[0] != "a" || kept[maxFolds-1] != string(rune('a'+maxFolds-1)) {
		t.Errorf("limit() = %v, want the first %d distinct spellings", kept, maxFolds)
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
