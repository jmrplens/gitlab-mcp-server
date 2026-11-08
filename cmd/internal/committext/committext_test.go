// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// committext_test.go holds the shared definitions to what GitHub reads as a
// request to skip workflows, and to the one character both gates refuse.
package committext

import "testing"

// TestEmDash_IsU2014 verifies the character itself, written here by its code
// point as the package writes it.
func TestEmDash_IsU2014(t *testing.T) {
	t.Parallel()

	if EmDash != string(rune(0x2014)) {
		t.Errorf("EmDash is %q, want U+2014", EmDash)
	}
}

// TestSkipCommand_MatchesTheFiveCommandsInAnySpelling verifies that the five
// bracketed commands GitHub documents are found anywhere in a line, in any
// case and with any spacing inside the brackets, and that a command named
// without its brackets is not.
func TestSkipCommand_MatchesTheFiveCommandsInAnySpelling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line string
		want bool
	}{
		{line: "[skip ci]", want: true},
		{line: "[ci skip]", want: true},
		{line: "[no ci]", want: true},
		{line: "[skip actions]", want: true},
		{line: "[actions skip]", want: true},
		{line: "the fixture commits with `[skip ci]`", want: true},
		{line: "commits with [Skip CI]", want: true},
		{line: "commits with [NO CI]", want: true},
		{line: "commits with [skip  ci]", want: true},
		{line: "commits with [actions\tskip]", want: true},
		{line: "commits with [ skip ci ]", want: true},
		{line: "the fixture commits with skip ci in square brackets"},
		{line: "[skipci]"},
		{line: "[skip ci"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()

			if got := SkipCommand.MatchString(tc.line); got != tc.want {
				t.Errorf("SkipCommand matches %q: %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

// TestSkipChecksTrailer_MatchesTheWholeWordTrueAtTheStartOfALine verifies
// the trailer rule: at the start of a line, indented or in backticks, any
// case and spacing, and the value the whole word true, followed by
// whitespace, a closing backtick or the end of the line. A longer word that
// starts with true is not the trailer, which the prefix match this replaces
// refused.
func TestSkipChecksTrailer_MatchesTheWholeWordTrueAtTheStartOfALine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		line string
		want bool
	}{
		{name: "no space", line: "skip-checks:true", want: true},
		{name: "one space", line: "skip-checks: true", want: true},
		{name: "any case", line: "Skip-Checks: TRUE", want: true},
		{name: "indented", line: "   skip-checks: true", want: true},
		{name: "in backticks", line: "`skip-checks: true`", want: true},
		{name: "indented in backticks", line: "  `skip-checks:true`", want: true},
		{name: "spaced around the colon", line: "skip-checks : true", want: true},
		{name: "followed by a space", line: "skip-checks: true and more", want: true},
		{name: "followed by a tab", line: "skip-checks: true\tnote", want: true},
		{name: "ending in a carriage return", line: "skip-checks: true\r", want: true},
		{name: "a longer word", line: "skip-checks: trueish"},
		{name: "a longer word in any case", line: "skip-checks: TRUEISH"},
		{name: "true and a full stop", line: "skip-checks: true."},
		{name: "false", line: "skip-checks: false"},
		{name: "named inside a sentence", line: "GitHub also reads a skip-checks: true trailer at the end."},
		{name: "no colon", line: "skip-checks true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := SkipChecksTrailer.MatchString(tc.line); got != tc.want {
				t.Errorf("SkipChecksTrailer matches %q: %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}
