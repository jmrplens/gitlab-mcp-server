// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
		// Inside a fence two label lines are code, not a paragraph, and the
		// kind of the line above is what tells the rule so.
		{name: "labels inside a fence", md: "```\n**Action**: x\n**Target**: y\n```\n"},
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
// label that was not escaped; and an autolink, whose address is read whole
// when it begins before any code span does, backticks included, and is text
// when a span opened before it.
//
// The precedence cases come in pairs, one for each direction: the form that
// begins first wins, whether it is the span, an autolink, an email
// autolink, raw HTML, a backslash escape or a link whose "]" came first. The
// cases where the form begins first were red against the walk that knew
// only spans and URI autolinks, and each of them hid or invented a
// destination; micromark, patched to the specification in the three places
// where it departs from it, reads every paragraph case as written here.
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
		// An autolink is a link a client follows exactly as it follows an
		// inline one, so its address is read in line order beside them.
		{name: "an autolink", md: "| <https://gitlab.example/T/7> |\n", want: "https://gitlab.example/T/7"},
		{name: "an autolink after a link", md: "- **A**: [T](https://gitlab.example/A/7) and <https://gitlab.example/B/7>\n", want: "https://gitlab.example/A/7|https://gitlab.example/B/7"},
		{name: "an autolink before a link", md: "- **A**: <https://gitlab.example/B/7> and [T](https://gitlab.example/A/7)\n", want: "https://gitlab.example/B/7|https://gitlab.example/A/7"},
		{name: "a scheme of two letters", md: "<ab:x>\n", want: "ab:x"},
		{name: "a scheme of one letter is no autolink", md: "<a:x>\n"},
		{name: "a scheme of thirty-two characters", md: "<" + strings.Repeat("s", 32) + ":x>\n", want: strings.Repeat("s", 32) + ":x"},
		{name: "a scheme of thirty-three characters is no autolink", md: "<" + strings.Repeat("s", 33) + ":x>\n"},
		{name: "a space ends no autolink", md: "<https://gitlab.example/a b>\n"},
		{name: "a raw tag is no autolink", md: `<a href="https://gitlab.example/T/7">x</a>` + "\n"},
		{name: "an escaped angle bracket is text", md: "| &lt;https://gitlab.example/T/7> |\n"},
		{name: "an autolink inside a code span is text", md: "| `<https://gitlab.example/T/7>` |\n"},
		// An autolink and a code span have the same precedence, and the one
		// that begins first wins: a backtick inside an autolink is part of
		// its address, and a span opened before the "<" makes what follows
		// literal up to its closing run, wherever that run sits.
		{name: "a backtick inside an autolink opens no span", md: "<https://gitlab.example/a`b`c>\n", want: "https://gitlab.example/a`b`c"},
		{name: "a backtick inside an autolink in a cell opens no span", md: "| <https://gitlab.example/a`b`c> |\n", want: "https://gitlab.example/a`b`c"},
		{name: "an autolink that begins where a span ends", md: "`x`<https://gitlab.example/a`b`c>\n", want: "https://gitlab.example/a`b`c"},
		{name: "a span after an autolink that holds a backtick", md: "<https://gitlab.example/a`b> `[x](http://attacker.invalid/y)`\n", want: "https://gitlab.example/a`b"},
		{name: "a span holding the opening of an autolink", md: "`a <https://gitlab.example/x`y>\n"},
		// The space inside the span ends the autolink the "<" tries to open,
		// so that "<" is text and the span is the first construct; the bare
		// line must not join the angle brackets around the placeholder into
		// an address.
		{name: "a span inside what would be an autolink leaves no address", md: "<https://gitlab.example/a`b c`>\n"},
		{name: "a lookalike with a one-letter scheme leaves the span", md: "<a:x`[y](http://attacker.invalid/y)`>\n"},
		// An email autolink is a link to mailto:, and its local part may
		// hold a backtick, which then opens nothing.
		{name: "an email autolink", md: "<user7@gitlab.example>\n", want: "mailto:user7@gitlab.example"},
		{name: "a backtick in an email autolink opens no span", md: "<q`r@w.ex> [x](http://attacker.invalid/y) `\n", want: "mailto:q`r@w.ex|http://attacker.invalid/y"},
		{name: "a span opened before an email autolink holds its start", md: "`a <q`r@w.ex> [x](http://attacker.invalid/y)\n", want: "http://attacker.invalid/y"},
		{name: "an exclamation mark in an email address", md: "<q!r@w.ex>\n", want: "mailto:q!r@w.ex"},
		{name: "an email domain label ends in a letter or a digit", md: "<q@w-.ex>\n"},
		// Raw HTML has the span's precedence as well, so a bracket inside a
		// tag's attribute neither closes a label nor opens one.
		{name: "a bracket inside a tag's attribute closes no label", md: `[x<a title="](http://attacker.invalid/y)">y</a>` + "\n"},
		{name: "a tag inside a label", md: "[x <b>y</b>](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a backtick inside a tag's attribute opens no span", md: "<b title=\"`\">[x](http://attacker.invalid/y) `\n", want: "http://attacker.invalid/y"},
		{name: "a span opened before a tag holds its start", md: "`<b title=\"`\">[x](http://attacker.invalid/y)\n", want: "http://attacker.invalid/y"},
		{name: "an autolink holds a label's bracket", md: "[x<http://h/](http://attacker.invalid/y)>\n", want: "http://h/](http://attacker.invalid/y)"},
		// A backslash before ASCII punctuation makes it literal: an escaped
		// backtick opens no span, an escaped "<" opens no autolink and an
		// escaped bracket opens no label; inside a span it is a backslash.
		{name: "an escaped angle bracket opens no autolink", md: "\\<https://h/a`b> `[x](http://attacker.invalid/y)`\n", want: "http://attacker.invalid/y"},
		{name: "an escaped backtick opens no span", md: "\\`[x](http://attacker.invalid/y)`\n", want: "http://attacker.invalid/y"},
		{name: "a run after an escaped backtick opens one", md: "\\``[x](http://attacker.invalid/y)`\n"},
		{name: "a backslash inside a span does not keep it from closing", md: "`a\\`[x](http://attacker.invalid/y)\n", want: "http://attacker.invalid/y"},
		{name: "an escaped bracket opens no label", md: "\\[x](http://attacker.invalid/y)\n"},
		{name: "an escaped backslash leaves the bracket a label", md: "\\\\[x](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a backslash before a letter is a backslash", md: "\\a[x](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a backslash ending the line", md: "[x](https://gitlab.example/T/7)\\\n", want: "https://gitlab.example/T/7"},
		// A link whose "]" comes first reads its resource as written, so a
		// backtick in the destination opens nothing; a span opened before
		// the label holds the label instead.
		{name: "a backtick in a destination opens no span", md: "[a](https://gitlab.example/a`b) `[x](http://attacker.invalid/y)`\n", want: "https://gitlab.example/a`b"},
		{name: "a span opened before a label holds the destination", md: "`[a](https://gitlab.example/a`b) [x](http://attacker.invalid/y)\n", want: "http://attacker.invalid/y"},
		// The bracket rule: the next "]" closes the latest "[" still open,
		// links do not nest, and an image's description is alt text.
		{name: "brackets inside a label are balanced", md: "[a [b] c](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a link inside a label leaves the outer brackets text", md: "[a [b](https://gitlab.example/B/7) c](http://attacker.invalid/y)\n", want: "https://gitlab.example/B/7"},
		{name: "a closing bracket with no label", md: "x] [y](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a label that forms nothing", md: "[a] [b](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "an image's source", md: "![x](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a link inside an image's description is alt text", md: "![a [b](http://attacker.invalid/y) c](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "an autolink inside an image's description is alt text", md: "![<http://attacker.invalid/y>](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "an image inside an image's description is alt text", md: "![![a](http://attacker.invalid/y)](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "a link before an image", md: "[a](https://gitlab.example/A/7) ![b](https://gitlab.example/B/7)\n", want: "https://gitlab.example/A/7|https://gitlab.example/B/7"},
		{name: "a link holding an image", md: "[![a](https://gitlab.example/I/7)](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7|https://gitlab.example/I/7"},
		{name: "a link holding an autolink", md: "[<https://gitlab.example/A/7>](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7|https://gitlab.example/A/7"},
		// The resource: a title, a destination between angle brackets, an
		// empty one, the whitespace around it, and its parentheses.
		{name: "a title in double quotes", md: `[x](https://gitlab.example/T/7 "t")` + "\n", want: "https://gitlab.example/T/7"},
		{name: "a title in single quotes", md: "[x](https://gitlab.example/T/7 't')\n", want: "https://gitlab.example/T/7"},
		{name: "a title in parentheses", md: "[x](https://gitlab.example/T/7 (t))\n", want: "https://gitlab.example/T/7"},
		{name: "a parenthesis inside a title in parentheses", md: "[x](https://gitlab.example/T/7 (t(u)))\n"},
		{name: "an escaped parenthesis inside a title in parentheses", md: "[x](https://gitlab.example/T/7 (t\\(u))\n", want: "https://gitlab.example/T/7"},
		// With no whitespace before the title there is no link, and what the
		// resource held is read again as text, where the address is an
		// autolink.
		{name: "a title needs whitespace before it", md: `[x](<https://gitlab.example/T/7>"t")` + "\n", want: "https://gitlab.example/T/7"},
		{name: "a destination between angle brackets", md: "[x](<https://gitlab.example/T 7>)\n", want: "https://gitlab.example/T 7"},
		{name: "an unclosed angle bracket", md: "[x](<https://gitlab.example/T/7)\n"},
		{name: "an empty destination", md: "[x]() [y](https://gitlab.example/T/7)\n", want: "|https://gitlab.example/T/7"},
		{name: "whitespace around the destination", md: "[x]( https://gitlab.example/T/7\t)\n", want: "https://gitlab.example/T/7"},
		{name: "balanced parentheses in a destination", md: "[x](https://gitlab.example/T(7))\n", want: "https://gitlab.example/T(7)"},
		{name: "an unbalanced parenthesis in a destination", md: "[x](https://gitlab.example/T(7)\n"},
		{name: "a parenthesis still open where whitespace ends the destination", md: "[x](https://gitlab.example/T(7 )\n"},
		{name: "an escaped parenthesis in a destination", md: "[x](https://gitlab.example/T\\)7)\n", want: "https://gitlab.example/T\\)7"},
		{name: "a space inside a bare destination", md: "[x](https://gitlab.example/T 7)\n"},
		{name: "a control character inside a destination", md: "[x](https://gitlab.example/T\x7f7)\n"},
		// U+0000 is replaced with U+FFFD before anything is parsed, and U+FFFD
		// is no control character: a bare destination or an autolink holding a
		// U+0000 is still one, where any other control character in either
		// leaves no link.
		{name: "U+0000 inside a destination", md: "[x](https://gitlab.example/T\x007)\n", want: "https://gitlab.example/T\x007"},
		{name: "a control character below the space inside a destination", md: "[x](https://gitlab.example/T\x017)\n"},
		{name: "U+0000 inside an autolink", md: "<https://gitlab.example/T\x007>\n", want: "https://gitlab.example/T\x007"},
		{name: "a control character inside an autolink", md: "<https://gitlab.example/T\x017>\n"},
		{name: "a delete character inside an autolink", md: "<https://gitlab.example/T\x7f7>\n"},
		{name: "a destination running to the end of the line", md: "[x](https://gitlab.example/T/7\n"},
		{name: "a label ending the line", md: "[x](\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ScanGFM(tc.md).Links, "|"); got != tc.want {
				t.Errorf("links = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestScanGFM_RawHTML_ListsWhatARendererPassesThrough checks the raw HTML the
// raw-tag rule reads: each of the six forms CommonMark passes through, what an
// unquoted attribute value may and may not hold, the forms that look like one
// and are text, and the three places a tag is text
// although it is spelled as one, a code span, an escaped "<" and an image's
// description. A tag that begins before a code span keeps the backtick in its
// attribute, which used to pair with a later one and hide every tag between
// them; a span that begins first holds the start of the tag instead.
func TestScanGFM_RawHTML_ListsWhatARendererPassesThrough(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []string
	}{
		{name: "an open tag", md: `<a href="http://attacker.invalid">x` + "\n", want: []string{`<a href="http://attacker.invalid">`}},
		{name: "a closing tag", md: "x</a >\n", want: []string{"</a >"}},
		{name: "a self-closing tag with every kind of attribute", md: "<b a b=c d = 'e' f=\"g\" h:i-j.k_l=/m/n />\n", want: []string{"<b a b=c d = 'e' f=\"g\" h:i-j.k_l=/m/n />"}},
		{name: "a comment", md: "<!-- a -- b -->\n", want: []string{"<!-- a -- b -->"}},
		{name: "the two shortest comments", md: "<!--> <!--->\n", want: []string{"<!-->", "<!--->"}},
		{name: "a processing instruction", md: "<?php x ?>\n", want: []string{"<?php x ?>"}},
		{name: "a declaration", md: "<!DOCTYPE html>\n", want: []string{"<!DOCTYPE html>"}},
		{name: "a CDATA section", md: "<![CDATA[ a ]] b ]]>\n", want: []string{"<![CDATA[ a ]] b ]]>"}},
		{name: "a tag of a table cell", md: "| <b> | </b> |\n| --- | --- |\n", want: []string{"<b>", "</b>"}},
		{name: "a tag on a quote line", md: "> <a href=\"http://x\">\n", want: []string{`<a href="http://x">`}},
		{name: "a tag inside a label", md: "[<b>x</b>](https://gitlab.example/T/7)\n", want: []string{"<b>", "</b>"}},
		{name: "a tag before an image and one after it", md: "<b> ![a](https://gitlab.example/T/7) </b>\n", want: []string{"<b>", "</b>"}},
		{name: "a tag that begins before a span keeps its backtick", md: "<b title=\"`\">x <a href=\"http://attacker.invalid\">y</a> `\n", want: []string{"<b title=\"`\">", `<a href="http://attacker.invalid">`, "</a>"}},
		{name: "a span that begins before a tag holds its start", md: "`<b title=\"`\">x <a href=\"http://attacker.invalid\">y</a>\n", want: []string{`<a href="http://attacker.invalid">`, "</a>"}},
		{name: "a backtick inside a comment opens no span", md: "<!-- ` --> <a href=\"http://x\"> `\n", want: []string{"<!-- ` -->", `<a href="http://x">`}},
		{name: "a tag inside a span is text", md: "`<a href=\"http://x\">`\n"},
		{name: "an escaped angle bracket is text", md: "\\<a href=\"http://x\">\n"},
		{name: "a tag inside an image's description is alt text", md: "![<a href=\"http://x\">y</a>](https://gitlab.example/T/7)\n"},
		{name: "an autolink is no tag", md: "<https://gitlab.example/T/7>\n"},
		{name: "an unclosed tag", md: "<a href=\"http://x\"\n"},
		{name: "an attribute with no value after its equals sign", md: "<a b=>\n"},
		{name: "an unclosed quoted value", md: "<a b='c>\n"},
		{name: "a space after the angle bracket", md: "< a>\n"},
		{name: "a tag name that opens with a digit", md: "<1a>\n"},
		{name: "attributes with no whitespace between them", md: "<a b=\"c\"d>\n"},
		// An unquoted attribute value holds anything CommonMark does not
		// exclude from it, and it excludes a space, a tab, a line ending, ", ',
		// =, <, > and the backtick: any other control character is part of the
		// value, and so is U+0000, which the specification replaces with
		// U+FFFD.
		{name: "a control character inside an unquoted value", md: "<a b=c\x01d>\n", want: []string{"<a b=c\x01d>"}},
		{name: "a vertical tab inside an unquoted value", md: "<a b=c\x0bd>\n", want: []string{"<a b=c\x0bd>"}},
		{name: "a form feed inside an unquoted value", md: "<a b=c\x0cd>\n", want: []string{"<a b=c\x0cd>"}},
		{name: "U+0000 inside an unquoted value", md: "<a b=c\x00d>\n", want: []string{"<a b=c\x00d>"}},
		{name: "a space ends an unquoted value", md: "<a b=c 1>\n"},
		{name: "a tab ends an unquoted value", md: "<a b=c\t1>\n"},
		{name: "a carriage return ends an unquoted value", md: "<a b=c\r1>\n"},
		{name: "a double quote inside an unquoted value", md: "<a b=c\"d>\n"},
		{name: "a single quote inside an unquoted value", md: "<a b=c'd>\n"},
		{name: "an equals sign inside an unquoted value", md: "<a b=c=d>\n"},
		{name: "a backtick inside an unquoted value", md: "<a b=c`d>\n"},
		{name: "an angle bracket inside an unquoted value opens the next tag", md: "<a b=c<d>\n", want: []string{"<d>"}},
		{name: "an unclosed comment", md: "<!-- a\n"},
		{name: "an empty declaration", md: "<!>\n"},
		{name: "an unclosed CDATA section", md: "<![CDATA[ a ]>\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, line := range ScanGFM(tc.md).Bare {
				got = append(got, line.RawHTML...)
			}
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") || len(got) != len(tc.want) {
				t.Errorf("raw HTML = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestScanGFM_SelfLinks_RecordsALabelThatIsItsDestination checks the record
// the address rule of issue 1396 reads: an inline link whose label is its own
// destination is recorded, wherever the line model reads links, and nothing
// else is, an autolink, a link with a real label, and a label that is only
// close to its destination included.
//
// The label and the destination are compared as CommonMark renders their
// characters, so a backslash escape or a character reference on either side
// is the character it stands for: [https://h/a\_b](https://h/a_b) renders as
// its own address and used to go unrecorded. micromark 4.0.3, patched to the
// specification as [TestScanGFM_Links_ReadsOnlyABracketedLabel] says, renders
// every case here as its expectation says, except the two the doc comment of
// [GFMDocument.SelfLinks] names as deliberate: a label that emphasis would
// format, and a numeric reference to a control character or a noncharacter,
// which micromark replaces with U+FFFD and the specification keeps.
func TestScanGFM_SelfLinks_RecordsALabelThatIsItsDestination(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{name: "a link labeled with its destination", md: "- **URL**: [https://gitlab.example/T/7](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "one in a table cell", md: "| A |\n| --- |\n| [https://gitlab.example/T/7](https://gitlab.example/T/7) |\n", want: "https://gitlab.example/T/7"},
		{name: "two on one line", md: "[https://a.example/1](https://a.example/1) [https://b.example/2](https://b.example/2)\n", want: "https://a.example/1|https://b.example/2"},
		{name: "the autolink is not one", md: "- **URL**: <https://gitlab.example/T/7>\n"},
		{name: "a real label is not one", md: "| [!7](https://gitlab.example/T/7) |\n"},
		{name: "a label close to its destination is not one", md: "[https://gitlab.example/T/7/](https://gitlab.example/T/7)\n"},
		{name: "one inside a code span is text", md: "`[https://gitlab.example/T/7](https://gitlab.example/T/7)`\n"},
		{name: "one inside a quote is GitLab's", md: "> [https://gitlab.example/T/7](https://gitlab.example/T/7)\n"},
		{name: "one inside a fence is content", md: "```\n[https://gitlab.example/T/7](https://gitlab.example/T/7)\n```\n"},
		// The label is what the bracket rule read, so a title, a destination
		// between angle brackets and the text before the label change
		// nothing, and an image, an empty link and a label holding a span
		// are not one.
		{name: "one with a title", md: `[https://gitlab.example/T/7](https://gitlab.example/T/7 "t")` + "\n", want: "https://gitlab.example/T/7"},
		{name: "one with its destination between angle brackets", md: "[https://gitlab.example/T/7](<https://gitlab.example/T/7>)\n", want: "https://gitlab.example/T/7"},
		{name: "one after a span", md: "`x` [https://gitlab.example/T/7](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "one inside a label left open", md: "[a [https://gitlab.example/T/7](https://gitlab.example/T/7)\n", want: "https://gitlab.example/T/7"},
		{name: "an image is not one", md: "![https://gitlab.example/T/7](https://gitlab.example/T/7)\n"},
		{name: "one inside an image's description is alt text", md: "![[https://gitlab.example/T/7](https://gitlab.example/T/7)](https://gitlab.example/I/7)\n"},
		{name: "an empty link is not one", md: "[]()\n"},
		{name: "a label holding a span is not one", md: "[`https://gitlab.example/T/7`](https://gitlab.example/T/7)\n"},
		// A backslash escape is the punctuation character it makes literal,
		// in the label and in the destination, whichever form the destination
		// takes; an escaped backslash is a backslash, and so is one before a
		// letter. Each is recorded as its destination is written.
		{name: "an escaped underscore in the label", md: "[https://gitlab.example/a\\_b](https://gitlab.example/a_b)\n", want: "https://gitlab.example/a_b"},
		{name: "an escaped underscore in the destination", md: "[https://gitlab.example/a_b](https://gitlab.example/a\\_b)\n", want: "https://gitlab.example/a\\_b"},
		{name: "an escaped underscore on both sides", md: "[https://gitlab.example/a\\_b](https://gitlab.example/a\\_b)\n", want: "https://gitlab.example/a\\_b"},
		{name: "an escaped underscore beside a destination between angle brackets", md: "[https://gitlab.example/a\\_b](<https://gitlab.example/a_b>)\n", want: "https://gitlab.example/a_b"},
		{name: "an escaped underscore inside a destination between angle brackets", md: "[https://gitlab.example/a_b](<https://gitlab.example/a\\_b>)\n", want: "https://gitlab.example/a\\_b"},
		{name: "an escaped backslash leaves the underscore after it", md: "[https://gitlab.example/\\\\_](https://gitlab.example/\\_)\n"},
		{name: "a backslash before a letter is a backslash", md: "[https://gitlab.example/\\a](https://gitlab.example/\\a)\n", want: "https://gitlab.example/\\a"},
		// A character reference is the character it stands for, on either
		// side: a name HTML5 defines, a decimal number and a hexadecimal one.
		// A name HTML5 does not define, a reference without its semicolon and
		// one whose "&" is escaped are text as written.
		{name: "an entity in the label", md: "[https://gitlab.example/?a=1&amp;b=2](https://gitlab.example/?a=1&b=2)\n", want: "https://gitlab.example/?a=1&b=2"},
		{name: "an entity in the destination", md: "[https://gitlab.example/?a=1&b=2](https://gitlab.example/?a=1&amp;b=2)\n", want: "https://gitlab.example/?a=1&amp;b=2"},
		{name: "a decimal reference", md: "[https://gitlab.example/&#95;](https://gitlab.example/_)\n", want: "https://gitlab.example/_"},
		{name: "a hexadecimal reference", md: "[https://gitlab.example/&#X5f;](https://gitlab.example/_)\n", want: "https://gitlab.example/_"},
		{name: "an entity of two code points", md: "[https://gitlab.example/&ngE;](https://gitlab.example/≧̸)\n", want: "https://gitlab.example/≧̸"},
		{name: "a name HTML5 does not define is text", md: "[https://gitlab.example/&ampx;](https://gitlab.example/&x;)\n"},
		{name: "a reference without its semicolon is text", md: "[https://gitlab.example/&amp](https://gitlab.example/&)\n"},
		{name: "an escaped ampersand opens no reference", md: "[https://gitlab.example/\\&amp;](https://gitlab.example/&amp;)\n"},
		// U+0000 is U+FFFD by reference and by byte, and a reference to a C1
		// code point is that code point, as the specification reads it, where
		// micromark writes U+FFFD for it, as it does for the other controls
		// and the noncharacters.
		{name: "a reference to U+0000", md: "[https://gitlab.example/&#0;](https://gitlab.example/&#xFFFD;)\n", want: "https://gitlab.example/&#xFFFD;"},
		{name: "a U+0000 byte in the label", md: "[https://gitlab.example/\x00](https://gitlab.example/&#0;)\n", want: "https://gitlab.example/&#0;"},
		{name: "a U+0000 byte in the destination", md: "[https://gitlab.example/�](https://gitlab.example/\x00)\n", want: "https://gitlab.example/\x00"},
		{name: "a reference to U+0000 against the byte", md: "[https://gitlab.example/&#0;](https://gitlab.example/\x00)\n", want: "https://gitlab.example/\x00"},
		{name: "a reference to a C1 code point", md: "[https://gitlab.example/&#x80;](https://gitlab.example/\u0080)\n", want: "https://gitlab.example/\u0080"},
		// What the label shows is compared whole: a space it adds is not the
		// destination's, nor is the long form an address an autolink cannot
		// hold keeps, whose destination is percent-encoded where its label
		// shows the character.
		{name: "a label with a space after the address", md: "[https://gitlab.example/T/7 ](https://gitlab.example/T/7)\n"},
		{name: "the long form of an address holding a pipe", md: "[https://gitlab.example/a&#124;b](https://gitlab.example/a%7Cb)\n"},
		// A link is one only when an autolink could hold its address, as
		// toolutil.MdAutolink writes it: an http or https address, its scheme
		// in either case, with something after the scheme and no space,
		// control character, "<", ">" or pipe.
		{name: "an address with a space, which no autolink holds", md: "[https://gitlab.example/a b](<https://gitlab.example/a b>)\n"},
		{name: "an address with a pipe outside a table", md: "[https://gitlab.example/a|b](https://gitlab.example/a|b)\n"},
		{name: "an address with a DEL between angle brackets", md: "[https://gitlab.example/T\x7f7](<https://gitlab.example/T\x7f7>)\n"},
		{name: "an address with an escaped less-than sign", md: "[https://gitlab.example/a\\<b](https://gitlab.example/a\\<b)\n"},
		{name: "an address with an escaped greater-than sign", md: "[https://gitlab.example/a\\>b](https://gitlab.example/a\\>b)\n"},
		{name: "an address that is not http or https", md: "[ftp://gitlab.example/T](ftp://gitlab.example/T)\n"},
		{name: "a scheme with nothing after it", md: "[https://](https://)\n"},
		{name: "an HTTPS scheme in capitals", md: "[HTTPS://gitlab.example/T](HTTPS://gitlab.example/T)\n", want: "HTTPS://gitlab.example/T"},
		// A label holding a construct that renders as markup is not one, even
		// where its characters are the destination's; emphasis is not modeled,
		// so a label it would format is still one, since it writes the address
		// a second time all the same.
		{name: "a label of a span whose placeholder is the destination", md: "[`x`](￼)\n"},
		{name: "a label ending in a span whose placeholder ends an address", md: "[https://gitlab.example/`x`](https://gitlab.example/￼)\n"},
		{name: "a label of an autolink", md: "[<https://gitlab.example/T/7>](\\<https://gitlab.example/T/7\\>)\n"},
		{name: "a label of raw HTML", md: "[<b>](\\<b\\>)\n"},
		{name: "a label emphasis would format", md: "[https://gitlab.example/_a_](https://gitlab.example/_a_)\n", want: "https://gitlab.example/_a_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ScanGFM(tc.md).SelfLinks, "|"); got != tc.want {
				t.Errorf("self links = %q, want %q", got, tc.want)
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
// opens is outside it; an autolink that begins before a span keeps its
// backticks, since it has the span's own precedence; and a quote line is
// carried, since the quote contains structure and not tags.
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
		// The pipe here is the seventh character of the row rather than the
		// sixth: a split that read every other character as escaped would
		// skip it and close the span across the cells.
		{name: "a span cannot cross a cell at any pipe", md: "| `ab | <c>` |\n| --- | --- |\n", want: "| `ab | <c>` |"},
		// A second span on the line is read after the first one ends, not
		// from inside it.
		{name: "two spans on one line are both elided", md: "- `<a>` and `<b>` <c>\n", want: "- " + gfmCodeSpanPlaceholder + " and " + gfmCodeSpanPlaceholder + " <c>"},
		// A string is closed by the first run of its own length that starts
		// after it, past runs of other lengths, and never by one before it.
		{name: "spans of two lengths interleave", md: "- `a` ``b`` `c` ``d`` <e>\n", want: "- " + gfmCodeSpanPlaceholder + " " + gfmCodeSpanPlaceholder + " " + gfmCodeSpanPlaceholder + " " + gfmCodeSpanPlaceholder + " <e>"},
		{name: "a run of its length before a string closes nothing", md: "- `a` `b <c>\n", want: "- " + gfmCodeSpanPlaceholder + " `b <c>"},
		{name: "a span inside one cell is elided", md: "| `<a>` | x |\n| --- | --- |\n", want: "| " + gfmCodeSpanPlaceholder + " | x |"},
		// An escaped pipe is content of its cell, so the row is tokenized
		// around it and the span beside it still closes. Splitting on it would
		// cut the cell in two and leave a span open across the halves.
		{name: "an escaped pipe does not split a cell", md: "| `<a>` \\| y | z |\n| --- | --- |\n", want: "| " + gfmCodeSpanPlaceholder + " \\| y | z |"},
		{name: "a tag before a span stays", md: "- <a> `x`\n", want: "- <a> " + gfmCodeSpanPlaceholder},
		{name: "a quote line is carried", md: "> <a href=\"http://x\">\n", want: "> <a href=\"http://x\">"},
		// An autolink and a code span have the same precedence and the one
		// that begins first wins, so the backticks inside an autolink are
		// part of it, and a span opened before the "<" holds it. A run left
		// unpaired opens nothing, so the autolink after it is still one.
		{name: "a backtick inside an autolink opens no span", md: "<https://h/a`b`c>\n", want: "<https://h/a`b`c>"},
		{name: "a backtick inside a second autolink opens no span", md: "<https://h/a> <https://h/b`c`d>\n", want: "<https://h/a> <https://h/b`c`d>"},
		{name: "a span right after an autolink closes", md: "<https://h/a>`b`\n", want: "<https://h/a>" + gfmCodeSpanPlaceholder},
		{name: "a span holding an autolink is elided", md: "`<https://h/a>`\n", want: gfmCodeSpanPlaceholder},
		{name: "a run after a span that ends inside an autolink's text opens one", md: "`a <https://h/x`y`z`>\n", want: gfmCodeSpanPlaceholder + "y" + gfmCodeSpanPlaceholder + ">"},
		{name: "an unpaired run before an autolink", md: "``a <https://h/x`y`z>\n", want: "``a <https://h/x`y`z>"},
		// Raw HTML, an email autolink, a backslash escape and a link whose
		// "]" came first share that precedence, so a backtick inside any of
		// them opens nothing, and a span that begins first holds what
		// follows. The first of each pair was red against the walk that knew
		// only spans and URI autolinks.
		{name: "a backtick inside a tag's attribute opens no span", md: "<b title=\"`\">x `y`\n", want: "<b title=\"`\">x " + gfmCodeSpanPlaceholder},
		{name: "a span that begins before a tag holds its start", md: "`<b title=\"`\">x\n", want: gfmCodeSpanPlaceholder + "\">x"},
		{name: "a backtick inside a comment opens no span", md: "<!-- `a --> `b`\n", want: "<!-- `a --> " + gfmCodeSpanPlaceholder},
		{name: "a backtick inside a processing instruction opens no span", md: "<? `a ?> `b`\n", want: "<? `a ?> " + gfmCodeSpanPlaceholder},
		{name: "a backtick inside a declaration opens no span", md: "<!X `a> `b`\n", want: "<!X `a> " + gfmCodeSpanPlaceholder},
		{name: "a backtick inside a CDATA section opens no span", md: "<![CDATA[ `a ]]> `b`\n", want: "<![CDATA[ `a ]]> " + gfmCodeSpanPlaceholder},
		{name: "a backtick inside an email autolink opens no span", md: "<q`r@w.ex> `x`\n", want: "<q`r@w.ex> " + gfmCodeSpanPlaceholder},
		{name: "an escaped backtick opens no span", md: "\\`a`\n", want: "\\`a`"},
		{name: "a run after an escaped backtick opens one", md: "\\``a`\n", want: "\\`" + gfmCodeSpanPlaceholder},
		{name: "a backslash inside a span is a backslash", md: "`a\\`b`\n", want: gfmCodeSpanPlaceholder + "b`"},
		{name: "an escaped angle bracket opens no autolink", md: "\\<https://h/a`b> `c`\n", want: "\\<https://h/a" + gfmCodeSpanPlaceholder + "c`"},
		{name: "a backtick in a destination opens no span", md: "[a](h`b) `c`\n", want: "[a](h`b) " + gfmCodeSpanPlaceholder},
		{name: "a destination ending in a backtick", md: "[a](h`) `c\n", want: "[a](h`) `c"},
		{name: "an escaped bracket leaves the destination text", md: "\\[a](h`b) `c`\n", want: "\\[a](h" + gfmCodeSpanPlaceholder + "c`"},
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
