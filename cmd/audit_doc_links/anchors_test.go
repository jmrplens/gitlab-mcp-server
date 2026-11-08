// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// anchors_test.go holds the two heading-slug dialects, which the script
// implemented with no test of its own: GitHub's for a file read in the
// repository and Starlight's for a page of the site, where they agree and the
// three places they part company (a repeated heading, a join control, the
// page's _top), and how a heading's source becomes the text that is slugged.
package main

import (
	"maps"
	"slices"
	"testing"
)

// anchorsOf returns the sorted anchors content answers to in d.
func anchorList(content string, d dialect) []string {
	return slices.Sorted(maps.Keys(collectAnchors(content, d)))
}

// TestDialects_RepeatedHeadings_AreNumberedEachTheirOwnWay verifies the
// repeat rules: GitHub counts per original slug, so a third heading "Foo 1"
// takes the foo-1 the second already holds; github-slugger walks up until a
// name is free, so it takes foo-1-1.
func TestDialects_RepeatedHeadings_AreNumberedEachTheirOwnWay(t *testing.T) {
	t.Parallel()

	page := "## Foo\n\n## Foo\n\n## Foo 1\n\n## Foo\n"
	if got, want := anchorList(page, gitHub), []string{"foo", "foo-1", "foo-2"}; !slices.Equal(got, want) {
		t.Errorf("GitHub anchors %q, want %q", got, want)
	}
	if got, want := anchorList(page, starlight), []string{"_top", "foo", "foo-1", "foo-1-1", "foo-2"}; !slices.Equal(got, want) {
		t.Errorf("Starlight anchors %q, want %q", got, want)
	}
}

// TestSlugBase_KeepsLettersMarksNumbersConnectorsAndHyphens verifies the
// characters each dialect keeps and drops, and that a join control survives
// in GitHub's alone.
func TestSlugBase_KeepsLettersMarksNumbersConnectorsAndHyphens(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text, gitHub, starlight string
	}{
		{text: "Configuración del ¿servidor?", gitHub: "configuración-del-servidor", starlight: "configuración-del-servidor"},
		{text: "The `make check` target (v2.0)", gitHub: "the-make-check-target-v20", starlight: "the-make-check-target-v20"},
		{text: "snake_case and a-hyphen", gitHub: "snake_case-and-a-hyphen", starlight: "snake_case-and-a-hyphen"},
		{text: "Roman Ⅻ and ² superscript", gitHub: "roman-ⅻ-and--superscript", starlight: "roman-ⅻ-and--superscript"},
		{text: "Emoji \U0001F469\U0000200D\U0001F4BB dev", gitHub: "emoji-\U0000200D-dev", starlight: "emoji--dev"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()

			if got := slugBase(tc.text, true); got != tc.gitHub {
				t.Errorf("GitHub slug %q, want %q", got, tc.gitHub)
			}
			if got := slugBase(tc.text, false); got != tc.starlight {
				t.Errorf("Starlight slug %q, want %q", got, tc.starlight)
			}
		})
	}
}

// TestHeadingText_IsWhatAReaderSees verifies the source a heading drops
// before it is slugged: closing hashes, comments (nested ones included),
// the URL of a link or an image, a reference label, tags, and emphasis
// written with underscores, which a name keeps.
func TestHeadingText_IsWhatAReaderSees(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Trailing hashes ##":           "Trailing hashes",
		"Comment <!-- hidden --> here": "Comment  here",
		// The first comment ends at the first closing marker, and what is
		// left after it opens no other, so it stays, as it did in the script.
		"Nested <!--<!-- x -->--> end":         "Nested --> end",
		"[Link](https://example.com/(a)) text": "Link text",
		"![Image](x.png) caption":              "Image caption",
		"Ref [label][id] here":                 "Ref label here",
		"Tag <b>bold</b> <<b>i> nested":        "Tag bold  nested",
		"_word_ and __strong__":                "word and strong",
		"audit_md_escaping stays":              "audit_md_escaping stays",
		"Mixed _a_b_ and snake_case_ and _x__": "Mixed _a_b_ and snake_case_ and x",
		"  spaced  ":                           "spaced",
		"__":                                   "__",
		"é_x_":                                 "é_x_",
		"_é_ ok":                               "é ok",
		"a _b_c":                               "a _b_c",
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			if got := headingText(raw); got != want {
				t.Errorf("headingText(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

// TestCollectAnchors_EveryHeadingShape_IsRead verifies the headings a page
// carries: ATX, Setext under equals and under dashes, equals under a line
// carrying a pipe, an explicit id or name on any element, and what is not a
// heading: a row of dashes under a line carrying a pipe, which is a table, a
// dash under an empty list item, indented code, a hash with no space, seven
// hashes, the frontmatter's title and anything in a fence.
func TestCollectAnchors_EveryHeadingShape_IsRead(t *testing.T) {
	t.Parallel()

	page := "---\ntitle: Front\nmore: x\n---\n" +
		"# Title\n" +
		"Setext One\n==========\n\n" +
		"Setext Two\n----------\n\n" +
		"a | b\n--|--\n\n" +
		"c | d\n=====\n\n" +
		"e | f\n-----\n\n" +
		"-\n-\n\n" +
		"> quote\n---\n\n" +
		"<a id=\"explicit\"></a> <span name='named'>x</span>\n\n" +
		"    ## indented\n\n" +
		"#nospace\n\n" +
		"####### seven\n\n" +
		"```\n## fenced\n```\n" +
		"#\n"
	want := []string{"", "c--d", "explicit", "named", "setext-one", "setext-two", "title"}
	if got := anchorList(page, gitHub); !slices.Equal(got, want) {
		t.Errorf("anchors %q, want %q", got, want)
	}
}

// TestIsParagraphLine_TellsTextFromABlockThatOwnsItsLine verifies what can
// be the text of a Setext heading.
func TestIsParagraphLine_TellsTextFromABlockThatOwnsItsLine(t *testing.T) {
	t.Parallel()

	for line, want := range map[string]bool{
		"Plain text": true, "   three spaces": true, "-word": true, "1.5 liters": true,
		"": false, "  ": false, "    code": false, "> quote": false, "# h": false, "| t |": false,
		"<div>": false, "- item": false, "-": false, "* item": false, "+": false, "1. item": false, "2)": false,
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			if got := isParagraphLine(line); got != want {
				t.Errorf("isParagraphLine(%q) = %v, want %v", line, got, want)
			}
		})
	}
}
