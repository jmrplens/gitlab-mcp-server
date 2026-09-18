package testutil

import (
	"strconv"
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
		// The other three blocks that end a table where a heading does, and
		// the two that let one open after them. Each is a line the renderer
		// treats as a block of its own, so a row written beside it is not a
		// row, and the scan has to agree with that in both directions.
		{name: "a table ended by a rule", md: "| A |\n| --- |\n| 1 |\n---\n"},
		{name: "a table ended by a fence", md: "| A |\n| --- |\n| 1 |\n```\nx\n```\n"},
		{name: "a table ended by a quote", md: "| A |\n| --- |\n| 1 |\n> q\n"},
		{name: "a table opened under a rule", md: "---\n| A |\n| --- |\n| 1 |\n"},
		{name: "a table opened after a fence", md: "```\nx\n```\n| A |\n| --- |\n| 1 |\n"},
		{name: "a pipe line outside a table that closes no row", md: "| A | B\n"},
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
		// Three label lines are two run-ons and one rule: what a reader is
		// handed names each rule the document hit, once, rather than one entry
		// per line that hit it.
		{name: "three bullet-less labels name the rule once", md: "**A:** x\n**B:** y\n**C:** z\n", want: []string{"B3"}},
		// A label line under ordinary prose is not a run-on of labels: the
		// rule reads both lines, so the one above has to be a label too or
		// every sentence followed by a bold field would be reported.
		{name: "a label line under a plain one", md: "a sentence\n**A:** x\n"},
		{name: "a rule after a blank line", md: "Done\n\n---\n" + GFMHintsHeading + "\n- x\n"},
		{name: "a quoted continuation", md: "- a\n  > quoted continuation\n"},
		{name: "an indented continuation", md: "- a\n  more of a\n"},
		{name: "a nested item", md: "- a\n  - b\n"},
		{name: "labels as list rows", md: "- **Action**: x\n- **Target**: y\n"},
		// Two ordinary prose lines are one paragraph and that is what they are
		// for, so the run-on rule has to read the labels as well as the kinds:
		// reporting every second text line would condemn every wrapped
		// sentence this server writes.
		{name: "two plain paragraph lines", md: "first line\nsecond line\n"},
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
		// A code span takes precedence over a link, so a bracketed address
		// inside one is text: the value a formatter moved into a span on
		// purpose, which the model used to report as a destination anyway.
		{name: "a link inside a code span is text", md: "| `[x](http://attacker.invalid/y)` |\n"},
		{name: "a code span in a cell beside a real link", md: "| `[x](http://attacker.invalid/y)` | [T](https://gitlab.example/T/7) |\n", want: "https://gitlab.example/T/7"},
		{name: "a code span in a paragraph", md: "See `[x](http://attacker.invalid/y)` here.\n"},
		{name: "an unclosed backtick opens no span", md: "| `[x](http://attacker.invalid/y) |\n", want: "http://attacker.invalid/y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ScanGFM(tc.md).Links, "|"); got != tc.want {
				t.Errorf("links = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestScanGFM_Links_ReadsAnEscapedBracketAsLabelText pins the clause that
// decides whether the links this server builds are its own: CommonMark's
// backslash escapes, where an escaped character "is treated as a regular
// character and does not have its usual Markdown meaning", and its link rule,
// where brackets are allowed in link text when they are backslash-escaped or
// matched. Every link in the tree passes its label through
// [toolutil.EscapeMdLinkLabel], which escapes the bracket, so a label class
// that accepts a backslash without honoring it reads the value's own "](url)"
// as the end of the label and reports an attacker destination for a link that
// points where the server pointed it. The cases are the four the boundary
// turns on, each naming the clause that settles it.
func TestScanGFM_Links_ReadsAnEscapedBracketAsLabelText(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{
			// Backslash escapes: the "]" is a regular character, so the label
			// runs on to the bracket the formatter wrote and the destination
			// is the fixture's own. This is what MdTitleLink produces for a
			// hostile title and what the rule used to report as an attack.
			name: "an escaped bracket inside the label keeps the fixture destination",
			md:   `| [x\](http://attacker.invalid/y)](https://gitlab.example/T/7) |` + "\n",
			want: "https://gitlab.example/T/7",
		},
		{
			// The same line with nothing escaping the bracket: the label ends
			// at the value's own "]" and the link points at the value's
			// destination. This is the hand-written fmt.Sprintf("[%s](%s)",
			// EscapeMdTableCell(title), url) the rule exists to catch, and it
			// must keep being caught.
			name: "the same line unescaped still opens the attacker destination",
			md:   "| [x](http://attacker.invalid/y)](https://gitlab.example/T/7) |\n",
			want: "http://attacker.invalid/y",
		},
		{
			// An ordinary link has no backslash at all, so neither half of
			// the label rule applies and the destination is read as before.
			name: "an ordinary link is unchanged",
			md:   "| [Title7](https://gitlab.example/T/7) |\n",
			want: "https://gitlab.example/T/7",
		},
		{
			// Backslash escapes again, one level up: the backslash escapes a
			// backslash, so the "]" after it is unescaped and really does
			// close the label. Accepting "a backslash and whatever follows"
			// as one label character is what gets this right; skipping every
			// character after a backslash would swallow the real terminator.
			name: "an escaped backslash leaves the bracket closing the label",
			md:   `| [a\\](http://attacker.invalid/y) |` + "\n",
			want: "http://attacker.invalid/y",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ScanGFM(tc.md).Links, "|"); got != tc.want {
				t.Errorf("links = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestScanGFM_Bare_ElidesTheCodeSpansAndKeepsTheQuotes checks the view the
// raw-tag rule reads: a code span's contents are literal and HTML-escaped by
// CommonMark's code-spans clause, so a tag inside one is text and must be
// elided; backticks that never close are literal themselves and hide nothing;
// a row is tokenized per cell, because GFM splits on unescaped pipes before
// inline parsing and a span cannot cross a cell; a tag written before a span
// opens is outside it; and a quote line is carried, since the quote contains
// structure and not tags.
func TestScanGFM_Bare_ElidesTheCodeSpansAndKeepsTheQuotes(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{name: "a tag inside a span is elided", md: "- **A**: `<a href=\"http://x\">`\n", want: "- **A**: " + gfmCodeSpanPlaceholder},
		{name: "a bare tag stays", md: "- **A**: <a href=\"http://x\">\n", want: "- **A**: <a href=\"http://x\">"},
		{name: "an unclosed backtick elides nothing", md: "- **A**: `<a href=\"http://x\">\n", want: "- **A**: `<a href=\"http://x\">"},
		{name: "a longer run does not close a shorter one", md: "- ``a` <b>\n", want: "- ``a` <b>"},
		{name: "a double-backtick span closes", md: "- ``<a>`` <b>\n", want: "- " + gfmCodeSpanPlaceholder + " <b>"},
		{name: "a span cannot cross a cell", md: "| `a | <b>` |\n| --- | --- |\n", want: "| `a | <b>` |"},
		{name: "a span inside one cell is elided", md: "| `<a>` | x |\n| --- | --- |\n", want: "| " + gfmCodeSpanPlaceholder + " | x |"},
		// An escaped pipe is content of its cell, so the row is tokenized
		// around it and the span beside it still closes. Splitting on it would
		// cut the cell in two and leave a span open across the halves.
		{name: "an escaped pipe does not split a cell", md: "| `<a>` \\| y | z |\n| --- | --- |\n", want: "| " + gfmCodeSpanPlaceholder + " \\| y | z |"},
		{name: "a tag before a span stays", md: "- <a> `x`\n", want: "- <a> " + gfmCodeSpanPlaceholder},
		{name: "a quote line is carried", md: "> <a href=\"http://x\">\n", want: "> <a href=\"http://x\">"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := ScanGFM(tc.md)
			if len(doc.Bare) == 0 {
				t.Fatalf("ScanGFM(%q) read no bare line", tc.md)
			}
			if got := doc.Bare[0].Bare; got != tc.want {
				t.Errorf("bare = %q, want %q", got, tc.want)
			}
			if doc.Bare[0].Line != 1 || doc.Bare[0].Text != strings.Split(tc.md, "\n")[0] {
				t.Errorf("bare line = %d, %q, want 1 and the line as written", doc.Bare[0].Line, doc.Bare[0].Text)
			}
		})
	}
}

// TestScanGFM_Bare_SkipsAFenceAndCarriesWhatContentDrops checks the two ways
// the bare view differs from Content: a fenced line is left out of both,
// because a fence is already containment, and a quote line is in the bare
// view and not in Content, which is the decision the raw-tag rule rests on.
func TestScanGFM_Bare_SkipsAFenceAndCarriesWhatContentDrops(t *testing.T) {
	doc := ScanGFM("## T\n\n> quoted <a>\n\n```\nfenced <a>\n```\n")

	var bare []string
	for _, line := range doc.Bare {
		bare = append(bare, line.Bare)
	}
	if got := strings.Join(bare, "|"); strings.Contains(got, "fenced") || !strings.Contains(got, "quoted <a>") {
		t.Errorf("bare = %q, want the quote line and nothing from the fence", got)
	}
	if got := strings.Join(doc.Content, "|"); strings.Contains(got, "quoted") {
		t.Errorf("content = %q, want the quote line left out, as it always was", got)
	}
}

// TestGFMCells_Lines_SplitsTheWayGFMDoes pins the cell split: the outer
// pipes dropped, a backslash-escaped pipe kept, a code-span pipe split, the
// entity left alone.
//
// The cell count is asserted beside the contents, because joining cannot tell
// no cells from one empty cell, and that is exactly the difference the two
// outer-pipe trims turn on: each drops an empty cell only when the line really
// opens or closes with a pipe, so a line with neither must come back whole.
func TestGFMCells_Lines_SplitsTheWayGFMDoes(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{name: "a row", line: "| a | b |", want: []string{"a", "b"}},
		{name: "no outer pipes", line: "a | b", want: []string{"a", "b"}},
		{name: "a line with no pipe at all", line: "abc", want: []string{"abc"}},
		{name: "an empty line is one empty cell", line: "", want: []string{""}},
		{name: "an escaped pipe", line: `| a \| b | c |`, want: []string{`a \| b`, "c"}},
		{name: "a code-span pipe splits", line: "| `x|y` | 2 |", want: []string{"`x", "y`", "2"}},
		{name: "the entity does not split", line: "| a &#124; b | 2 |", want: []string{"a &#124; b", "2"}},
		{name: "an empty cell", line: "| a |  | c |", want: []string{"a", "", "c"}},
		{name: "a delimiter", line: "|---:|------|", want: []string{"---:", "------"}},
		{name: "a lone pipe", line: "|", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GFMCells(tc.line)
			if len(got) != len(tc.want) || strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Errorf("GFMCells(%q) = %#v, want %#v", tc.line, got, tc.want)
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

// TestScanGFM_ADocumentEndingOnItsLastLine_IsReadWithoutRunningOff covers the
// documents whose last line carries structure, which is every render that does
// not end in a newline.
//
// Both walks that read a line beside the one they are on stop one line early
// on purpose: the table walk looks at the line after a pipe line to see
// whether it is a delimiter, and the guidance walk reads the bullets under a
// heading. A render ending on a pipe row or on a bullet is what puts the last
// line in front of each of them, and nothing else does: every other document
// here ends with a newline, so the split leaves a blank line for them to stop
// on and the bound is never reached.
func TestScanGFM_ADocumentEndingOnItsLastLine_IsReadWithoutRunningOff(t *testing.T) {
	t.Run("a pipe row with nothing after it", func(t *testing.T) {
		doc := ScanGFM("| A |")

		if got := strings.Join(doc.Rules(), " "); got != "T4" {
			t.Errorf("rules = %q, want T4 for a row no table owns", got)
		}
		if len(doc.Tables) != 0 {
			t.Errorf("tables = %+v, want none: a header needs a delimiter under it", doc.Tables)
		}
	})

	t.Run("a table whose last row ends the document", func(t *testing.T) {
		doc := ScanGFM("| A |\n| --- |\n| 1 |")

		if len(doc.Tables) != 1 || doc.Rows != 1 {
			t.Errorf("tables = %+v, rows = %d, want one table with one row", doc.Tables, doc.Rows)
		}
		if len(doc.Findings) != 0 {
			t.Errorf("a well-formed table ending the document produced findings: %+v", doc.Findings)
		}
	})

	t.Run("a guidance bullet with nothing after it", func(t *testing.T) {
		doc := ScanGFM(GFMHintsHeading + "\n- one")

		if len(doc.Hints) != 1 {
			t.Fatalf("found %d guidance section(s), want 1: %+v", len(doc.Hints), doc.Hints)
		}
		if got := strings.Join(doc.Hints[0].Bullets, "|"); got != "one" {
			t.Errorf("bullets = %q, want one", got)
		}
	})
}

// TestScanGFM_TwoTablesSeparatedByOneBlankLine_AreBothRead pins where the
// table walk resumes after a table, and where each table's delimiter sits.
//
// A card followed by a list is two tables with one blank line between them,
// which is the tightest spacing the formatters write. The walk resumes on the
// line the body stopped at rather than past it, so that blank line is still
// examined; resuming one line later would step over the second header and
// report all three of its lines as rows no table owns. The delimiter index is
// asserted beside it because nothing else reads that field, and a table whose
// delimiter does not sit under its header describes no table at all.
func TestScanGFM_TwoTablesSeparatedByOneBlankLine_AreBothRead(t *testing.T) {
	doc := ScanGFM("| A |\n| --- |\n| 1 |\n\n| B |\n| --- |\n| 2 |\n")

	if len(doc.Tables) != 2 {
		t.Fatalf("found %d table(s), want 2: %+v (findings %+v)", len(doc.Tables), doc.Tables, doc.Findings)
	}
	for i, table := range doc.Tables {
		t.Run("table "+strconv.Itoa(i+1), func(t *testing.T) {
			if table.Delimiter != table.Header+1 {
				t.Errorf("delimiter at line %d, want the line under the header at %d", table.Delimiter, table.Header)
			}
			if len(table.Rows) != 1 {
				t.Errorf("rows = %v, want one", table.Rows)
			}
		})
	}
	if len(doc.Findings) != 0 {
		t.Errorf("two well-formed tables produced findings: %+v", doc.Findings)
	}
}

// TestScanGFM_ADelimiterUnderAParagraph_OpensNoTable checks the half of the
// table condition that reads the header itself.
//
// GFM opens a table on a pipe line followed by a delimiter row, and a
// paragraph line is not a pipe line however well-formed the row under it is.
// Reading only the second line would turn any prose line above a delimiter
// into a header and report the cell counts of a table the client never sees.
func TestScanGFM_ADelimiterUnderAParagraph_OpensNoTable(t *testing.T) {
	doc := ScanGFM("Done\n| --- | --- |\n")

	if len(doc.Tables) != 0 {
		t.Errorf("tables = %+v, want none: the line above the delimiter is prose", doc.Tables)
	}
	if got := strings.Join(doc.Rules(), " "); got != "T4" {
		t.Errorf("rules = %q, want T4 for the delimiter row rendering as text", got)
	}
}

// TestScanGFM_ThreeSpacesOfIndentation_StillOpensTheBlock checks the
// indentation CommonMark allows in front of a block, at the last column that
// is still indentation rather than code.
//
// Three spaces open a quote or a row and four make an indented code block, so
// the boundary decides two different things about the same line: a row one
// space further in stops being part of a table, and a quote one space further
// in stops being a quote and becomes content the structure rules read. Both
// are silent failures — the scan reports nothing either way — which is why
// each is asserted on what the scan built rather than on what it found.
func TestScanGFM_ThreeSpacesOfIndentation_StillOpensTheBlock(t *testing.T) {
	t.Run("a table indented to the last allowed column", func(t *testing.T) {
		doc := ScanGFM("   | A |\n   | --- |\n   | 1 |\n")

		if len(doc.Tables) != 1 || doc.Rows != 1 {
			t.Errorf("tables = %+v, rows = %d, want one table with one row", doc.Tables, doc.Rows)
		}
		if len(doc.Findings) != 0 {
			t.Errorf("an indented table produced findings: %+v", doc.Findings)
		}
	})

	t.Run("a table indented one column too far", func(t *testing.T) {
		doc := ScanGFM("    | A |\n    | --- |\n    | 1 |\n")

		if len(doc.Tables) != 0 {
			t.Errorf("tables = %+v, want none: four spaces open a code block", doc.Tables)
		}
	})

	t.Run("a quote indented one column too far", func(t *testing.T) {
		doc := ScanGFM("    > quoted [q](https://gitlab.example/q)\n")

		if len(doc.Links) != 1 {
			t.Errorf("links = %v, want the one on a line that is content rather than a quote", doc.Links)
		}
	})

	t.Run("a quote indented to the last allowed column", func(t *testing.T) {
		doc := ScanGFM("   > quoted [q](https://attacker.invalid/q)\n")

		if len(doc.Links) != 0 {
			t.Errorf("links = %v, want none: a quoted destination is not the server's", doc.Links)
		}
		if got := strings.Join(doc.Content, "|"); strings.Contains(got, "quoted") {
			t.Errorf("content = %q, want the quote line left out of it", got)
		}
	})
}

// TestScanGFM_Links_EveryLinkOnALineIsRead checks that a line carrying more
// than one link reports all of them.
//
// A list row that names an object and its author carries two destinations, and
// the hostile comparison holds the destinations constant: reading only the
// first would let a value change every link after it on its own line without
// the comparison noticing.
func TestScanGFM_Links_EveryLinkOnALineIsRead(t *testing.T) {
	doc := ScanGFM("- [a](https://gitlab.example/a) by [b](https://gitlab.example/b)\n")

	if got := strings.Join(doc.Links, "|"); got != "https://gitlab.example/a|https://gitlab.example/b" {
		t.Errorf("links = %q, want both destinations on the line", got)
	}
}
