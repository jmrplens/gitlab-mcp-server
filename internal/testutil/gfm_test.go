package testutil

import (
	"strings"
	"testing"
)

// TestScanGFM_Tables_ReadsTheBoundariesARendererApplies walks the table rules
// over the shapes the audit found in the tree and the shapes that look wrong
// and are not: a header lazily continuing a hint bullet, a footer absorbed
// as a row, a list item ending a card table, an orphan row, and the legal
// table directly under a heading, inside a quote, inside a fence, or beside
// pipes that are prose.
func TestScanGFM_Tables_ReadsTheBoundariesARendererApplies(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "a header lazily continuing the hint bullet above it",
			md:   "\n---\n" + GFMHintsHeading + "\n- keep links\n| A | B |\n| --- | --- |\n| 1 | 2 |\n",
			want: []string{"T1"},
		},
		{
			name: "a footer absorbed as one more row",
			md:   "| A | B |\n| --- | --- |\n| 1 | 2 |\nPage 1 of 1 | 2 items total | 20 per page",
			want: []string{"T3"},
		},
		{
			name: "a list item ending the table and the row after it orphaned",
			md:   "| Property | Value |\n|---|---|\n- **ID**: 5\n| Status | x |\n",
			want: []string{"T3", "T4"},
		},
		{
			name: "a row with no header or delimiter",
			md:   "## Card\n\n| Name | x |\n",
			want: []string{"T4"},
		},
		{
			name: "a delimiter that disagrees with the header",
			md:   "| A | B | C |\n| --- | --- |\n",
			want: []string{"T2"},
		},
		{
			name: "a row with one cell too many",
			md:   "| A | B |\n| --- | --- |\n| 1 | 2 | 3 |\n",
			want: []string{"T3b"},
		},
		{
			name: "a pipe inside a code span splits the row",
			md:   "| A | B |\n| --- | --- |\n| `x|y` | 2 |\n",
			want: []string{"T3b"},
		},
		{name: "a table directly under a heading", md: "## T\n| A |\n| --- |\n| 1 |\n"},
		{name: "a table followed by a blank line and the footer", md: "| A | B |\n| --- | --- |\n| 1 | 2 |\n\nPage 1 of 1 | 2 items total | 20 per page\n"},
		{name: "a row inside a quote", md: "> | a | b |\n"},
		{name: "a table inside a fence", md: "```\n| a | b |\n| --- | --- |\n| 1 | 2 |\nPage 1\n```\n"},
		{name: "pipes in a paragraph", md: "**Version**: 1 | **Edition**: ee\n"},
		{name: "pipes in a list item", md: "- **Passed**: 1 | **Failed**: 0\n"},
		{name: "an entity does not split", md: "| A | B |\n| --- | --- |\n| a &#124; b | 2 |\n"},
		{name: "cells composed in one hole", md: "| Setting | Value | Locked | Inherited From |\n| --- | --- | --- | --- |\n| Allow | yes | no | - |\n"},
		{name: "alignment delimiters", md: "| A | B |\n|---:|------|\n| 1 | 2 |\n"},
		{name: "a table ended by a heading", md: "| A |\n| --- |\n| 1 |\n## Next\n"},
		{name: "a lone pipe is no delimiter", md: "| A |\n|\n", want: []string{"T4", "T4"}},
		{name: "a second pipe line that is not a delimiter", md: "| A |\n| x |\n", want: []string{"T4", "T4"}},
		{name: "nothing at all", md: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := ScanGFM(tc.md)
			var rules []string
			for _, f := range doc.Findings {
				rules = append(rules, f.Rule)
			}
			if got := strings.Join(rules, " "); got != strings.Join(tc.want, " ") {
				t.Errorf("ScanGFM(%q) found %q, want %q:\n%+v", tc.md, got, tc.want, doc.Findings)
			}
		})
	}
}

// TestScanGFM_Blocks_ReadsTheSeparationRules covers the three shapes a block
// runs into its neighbor: a rule under a paragraph, a footer continuing a
// bullet, and label lines with no bullet, beside the separated forms.
func TestScanGFM_Blocks_ReadsTheSeparationRules(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []string
	}{
		{name: "a rule under a paragraph is a setext heading", md: "Done\n---\n" + GFMHintsHeading + "\n- x\n", want: []string{"B1"}},
		{name: "an equals rule under a paragraph", md: "Done\n===\n", want: []string{"B1"}},
		{name: "a footer continuing the last bullet", md: "- a\n- b\nPage 1 of 2 | 3 items total | 20 per page\n", want: []string{"B2"}},
		{name: "bullet-less labels in one paragraph", md: "**Action:** x\n**Target:** y\n", want: []string{"B3"}},
		{name: "bullet-less labels with the colon outside", md: "**Action**: x\n**Target**: y\n", want: []string{"B3"}},
		{name: "a rule after a blank line", md: "Done\n\n---\n" + GFMHintsHeading + "\n- x\n"},
		{name: "a quoted continuation", md: "- a\n  > quoted continuation\n"},
		{name: "an indented continuation", md: "- a\n  more of a\n"},
		{name: "a nested item", md: "- a\n  - b\n"},
		{name: "labels as list rows", md: "- **Action**: x\n- **Target**: y\n"},
		{name: "one bold summary then a blank line", md: "**2 LDAP link(s)**\n\n| A |\n| --- |\n"},
		{name: "a heading after a bullet", md: "- a\n## Next\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := ScanGFM(tc.md)
			if got := strings.Join(doc.Rules(), " "); got != strings.Join(tc.want, " ") {
				t.Errorf("ScanGFM(%q) found %q, want %q:\n%+v", tc.md, got, tc.want, doc.Findings)
			}
		})
	}
}

// TestScanGFM_Hints_ReadsEverySectionAndItsBullets checks the guidance
// sections the scan reports: their position, their bullets, and the finding
// for a second one.
func TestScanGFM_Hints_ReadsEverySectionAndItsBullets(t *testing.T) {
	md := "## X\n\n- row\n\n---\n" + GFMHintsHeading + "\n- first\n- second\n\ntext\n\n" + GFMHintsHeading + "\n- third\n"

	doc := ScanGFM(md)

	if len(doc.Hints) != 2 {
		t.Fatalf("found %d guidance section(s), want 2: %+v", len(doc.Hints), doc.Hints)
	}
	if got := strings.Join(doc.Hints[0].Bullets, "|"); got != "first|second" {
		t.Errorf("first section bullets = %q, want first|second", got)
	}
	if got := strings.Join(doc.Hints[1].Bullets, "|"); got != "third" {
		t.Errorf("second section bullets = %q, want third", got)
	}
	if got := strings.Join(doc.Rules(), " "); got != "H1" {
		t.Errorf("rules = %q, want H1 for the second section", got)
	}
	if doc.Findings[0].Line != doc.Hints[1].Line+1 {
		t.Errorf("H1 reported at line %d, want the second heading's line %d", doc.Findings[0].Line, doc.Hints[1].Line+1)
	}
	if quoted := ScanGFM("> " + GFMHintsHeading + "\n> - x\n"); len(quoted.Hints) != 0 {
		t.Errorf("a quoted heading was read as a section: %+v", quoted.Hints)
	}
}

// TestScanGFM_Structure_CountsWhatAHostileValueMustNotChange checks the
// structure the hostile comparison holds constant: headings, top-level items,
// rows, cells and links, none of them counted inside a fence or a quote.
func TestScanGFM_Structure_CountsWhatAHostileValueMustNotChange(t *testing.T) {
	md := "## Title\n\n- **A**: [x](https://gitlab.example/A/7)\n  - nested\n- **B**: y\n\n" +
		"| C | D |\n| --- | --- |\n| 1 | [d](https://gitlab.example/D/7) |\n| 2 | 3 |\n\n" +
		"> ## quoted heading\n> - quoted item [q](https://attacker.invalid/q)\n\n" +
		"```\n## fenced heading\n| a | b |\n[f](https://attacker.invalid/f)\n```\n"

	doc := ScanGFM(md)

	if got := strings.Join(doc.Headings, "|"); got != "## Title" {
		t.Errorf("headings = %q, want the one outside the quote and the fence", got)
	}
	if got := strings.Join(doc.Items, "|"); got != "- **A**: [x](https://gitlab.example/A/7)|- **B**: y" {
		t.Errorf("items = %q, want the two top-level items", got)
	}
	if doc.Rows != 2 || doc.Cells != 4 {
		t.Errorf("rows, cells = %d, %d, want 2, 4", doc.Rows, doc.Cells)
	}
	if got := strings.Join(doc.Links, "|"); got != "https://gitlab.example/A/7|https://gitlab.example/D/7" {
		t.Errorf("links = %q, want the two outside the quote and the fence", got)
	}
	if got := strings.Join(doc.Content, "\n"); strings.Contains(got, "quoted") || strings.Contains(got, "fenced") || !strings.Contains(got, "## Title") {
		t.Errorf("content = %q, want the lines outside the quote and the fence", got)
	}
	if len(doc.Findings) != 0 {
		t.Errorf("a well-formed document produced findings: %+v", doc.Findings)
	}
}

// TestScanGFM_Links_ReadsOnlyABracketedLabel checks what counts as a link: a
// label and a destination, so a value carrying "](url)" opens nothing on its
// own and does open a link to its own destination once it lands inside a
// label that was not escaped.
func TestScanGFM_Links_ReadsOnlyABracketedLabel(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{name: "a link", md: "| [Title7](https://gitlab.example/T/7) |\n", want: "https://gitlab.example/T/7"},
		{name: "a bare closing half is text", md: "| x](http://attacker.invalid/y) |\n"},
		{name: "a value closing the label it sits in", md: "| [x](http://attacker.invalid/y)](https://gitlab.example/T/7) |\n", want: "http://attacker.invalid/y"},
		{name: "an escaped bracket keeps the label", md: "| [x&#93;(http://attacker.invalid/y)](https://gitlab.example/T/7) |\n", want: "https://gitlab.example/T/7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ScanGFM(tc.md).Links, "|"); got != tc.want {
				t.Errorf("links = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGFMCells_Lines_SplitsTheWayGFMDoes pins the cell split: the outer
// pipes dropped, a backslash-escaped pipe kept, a code-span pipe split, the
// entity left alone.
func TestGFMCells_Lines_SplitsTheWayGFMDoes(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{name: "a row", line: "| a | b |", want: []string{"a", "b"}},
		{name: "no outer pipes", line: "a | b", want: []string{"a", "b"}},
		{name: "an escaped pipe", line: `| a \| b | c |`, want: []string{`a \| b`, "c"}},
		{name: "a code-span pipe splits", line: "| `x|y` | 2 |", want: []string{"`x", "y`", "2"}},
		{name: "the entity does not split", line: "| a &#124; b | 2 |", want: []string{"a &#124; b", "2"}},
		{name: "an empty cell", line: "| a |  | c |", want: []string{"a", "", "c"}},
		{name: "a delimiter", line: "|---:|------|", want: []string{"---:", "------"}},
		{name: "a lone pipe", line: "|", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GFMCells(tc.line); strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Errorf("GFMCells(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestFenceRun_Lines_ReadsAFenceMarker checks which lines open or close a
// fence, the same rule the static gate applies.
func TestFenceRun_Lines_ReadsAFenceMarker(t *testing.T) {
	cases := []struct {
		name string
		line string
		want int
	}{
		{name: "three backticks", line: "```", want: 3},
		{name: "with an info string", line: "```json", want: 3},
		{name: "five", line: "`````", want: 5},
		{name: "indented three", line: "   ```", want: 3},
		{name: "indented four is code", line: "    ```", want: 0},
		{name: "a code span", line: "``x``", want: 0},
		{name: "not at the start", line: "x ```", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run, ok := fenceRun(tc.line)
			if (tc.want == 0) == ok || run != tc.want {
				t.Errorf("fenceRun(%q) = %d, %v, want %d", tc.line, run, ok, tc.want)
			}
		})
	}
}

// TestScanGFM_FencedContent_IsNotStructure checks that a closing fence
// shorter than the opening one keeps the block open, so everything inside it
// stays content.
func TestScanGFM_FencedContent_IsNotStructure(t *testing.T) {
	doc := ScanGFM("`````\n```\n## inside\n| a |\n`````\n## outside\n")

	if got := strings.Join(doc.Headings, "|"); got != "## outside" {
		t.Errorf("headings = %q, want only the one after the block closed", got)
	}
	if len(doc.Findings) != 0 {
		t.Errorf("fenced content produced findings: %+v", doc.Findings)
	}
}
