package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestParseVerbs_Templates_PairsEachVerbWithItsArgument covers fmt's grammar
// as far as the audit reads it. Getting this wrong would pair a verb with the
// wrong expression, which is worse than not looking at all.
func TestParseVerbs_Templates_PairsEachVerbWithItsArgument(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string
	}{
		{name: "one verb", template: "| %s |", want: []string{"s@0"}},
		{name: "two verbs", template: "| %s | %d |", want: []string{"s@0", "d@1"}},
		{name: "a literal percent consumes nothing", template: "100%% of %s", want: []string{"s@0"}},
		{name: "flags and width", template: "|%-10.3s|", want: []string{"s@0"}},
		{name: "a star width takes an argument", template: "%*s", want: []string{"*@0", "s@1"}},
		{name: "a star precision takes one too", template: "%.*f", want: []string{"*@0", "f@1"}},
		{name: "explicit indices", template: "[%[1]s](%[1]s)", want: []string{"s@0", "s@0"}},
		{name: "an index resets the run", template: "%[2]s %s", want: []string{"s@1", "s@2"}},
		// fmt itself answers a malformed index with %!s(BADINDEX), so a
		// template carrying one is already broken. The walk stops at the
		// bracket rather than guessing, which leaves a verb no context judges.
		{name: "a malformed index stops at the bracket", template: "%[x]s", want: []string{"[@0"}},
		{name: "a zero index stops there too", template: "%[0]s", want: []string{"[@0"}},
		{name: "a trailing percent declares nothing", template: "value: %", want: nil},
		{name: "a truncated verb declares nothing", template: "value: %-", want: nil},
		{name: "a truncated precision declares nothing", template: "%.", want: nil},
		{name: "no verbs at all", template: "| name |", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, h := range parseVerbs(tc.template) {
				got = append(got, fmt.Sprintf("%c@%d", h.verb, h.arg))
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("parseVerbs(%q) = %v, want %v", tc.template, got, tc.want)
			}
		})
	}
}

// TestParseVerbs_Offsets_PointAtThePercentSign checks that a hole is located
// where its verb begins, which is what decides the construct around it.
func TestParseVerbs_Offsets_PointAtThePercentSign(t *testing.T) {
	template := "## %s\n| %s |"

	holes := parseVerbs(template)

	if len(holes) != 2 {
		t.Fatalf("parseVerbs found %d holes, want 2", len(holes))
	}
	if contextAt(template, holes[0].offset) != ctxHeading {
		t.Errorf("the first hole is not in the heading")
	}
	if contextAt(template, holes[1].offset) != ctxCell {
		t.Errorf("the second hole is not in the cell")
	}
}

// TestStringishVerbs_Set_JudgesOnlyWhatCanCarryText checks that the numeric
// verbs are skipped and %q is not, since Go's quoting escapes neither a pipe
// nor an angle bracket.
func TestStringishVerbs_Set_JudgesOnlyWhatCanCarryText(t *testing.T) {
	for _, verb := range []string{"s", "v", "q"} {
		t.Run(verb, func(t *testing.T) {
			if !stringishVerbs[verb[0]] {
				t.Errorf("%%%s is not judged, though it renders text", verb)
			}
		})
	}
	for _, verb := range []string{"d", "f", "t", "x", "*"} {
		t.Run(verb, func(t *testing.T) {
			if stringishVerbs[verb[0]] {
				t.Errorf("%%%s is judged, though it cannot emit a pipe, a newline or an angle bracket", verb)
			}
		})
	}
}
