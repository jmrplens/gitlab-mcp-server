// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// markdown_test.go holds the reading of a page to the lines a reader is
// shown as prose, the links on them and the code spans they are not read in.
package main

import (
	"slices"
	"strconv"
	"testing"
)

// proseLines returns the numbered lines eachProseLine visits.
func proseLines(content string) []string {
	var lines []string
	eachProseLine(content, func(line string, number int) {
		lines = append(lines, strconv.Itoa(number)+":"+line)
	})
	return lines
}

// TestEachProseLine_AFenceClosesOnItsOwnMarkerAlone verifies the fence rule:
// a block closes on a marker of its own character at least as long and with
// nothing after it, so a shorter or other fence inside it is content, and a
// carriage return is not part of a line.
func TestEachProseLine_AFenceClosesOnItsOwnMarkerAlone(t *testing.T) {
	t.Parallel()

	content := "one\r\n" +
		"````markdown\n" +
		"```\n" +
		"inside\n" +
		"```` trailing\n" +
		"~~~~\n" +
		"````\n" +
		"two\n" +
		"   ~~~\n" +
		"```\n" +
		"~~~~~\n" +
		"three"
	want := []string{"1:one", "8:two", "12:three"}
	if got := proseLines(content); !slices.Equal(got, want) {
		t.Errorf("prose lines %q, want %q", got, want)
	}
}

// TestInlineLinks_AreReadOutsideCodeSpans verifies the destinations a line
// links to: bare or in angle brackets, with a title, an image's, and none of
// those quoted in a code span of any length.
func TestInlineLinks_AreReadOutsideCodeSpans(t *testing.T) {
	t.Parallel()

	line := "[a](a.md) ![i](<i b.png> \"t\") `[c](c.md)` ``[d](`d.md`)`` [e]( e.md ) `[f](f.md)"
	want := []string{"a.md", "<i b.png>", "e.md", "f.md"}
	if got := inlineLinks(line); !slices.Equal(got, want) {
		t.Errorf("links %q, want %q", got, want)
	}
}

// TestWithoutCodeSpans_BlanksWhatTheScriptsExpressionMatched verifies the
// spans the script's expression found, its backtracking included: the
// longest opening run a body and an equal closing run can close, a closing
// run not followed by another backtick, a body that is not a backtick at
// either end, and an opening that never closes left as it is. Each answer
// below is the one the script's own function gave, but for one count: a
// body is blanked a space per byte, where the script counted UTF-16 units,
// which changes no link either reads.
func TestWithoutCodeSpans_BlanksWhatTheScriptsExpressionMatched(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"a `b` c":     "a ` ` c",
		"``a ` b``":   "``     ``",
		"``a`":        "`` `",
		"`a``b`":      "`    `",
		"`a`` b":      "`a`` b",
		"no spans":    "no spans",
		"```":         "```",
		"` `x":        "` `x",
		"`é`":         "`  `",
		"`a` and `b`": "` ` and ` `",
		"x``y``z":     "x`` ``z",
		"``":          "``",
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			if got := withoutCodeSpans(line); got != want {
				t.Errorf("withoutCodeSpans(%q) = %q, want %q", line, got, want)
			}
		})
	}
}

// TestReferenceTarget_ReadsADefinitionAndNotAFootnote verifies the link
// reference definitions a line carries.
func TestReferenceTarget_ReadsADefinitionAndNotAFootnote(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line, target string
		ok           bool
	}{
		{line: "[ref]: docs/a.md", target: "docs/a.md", ok: true},
		{line: "  [ref]: <docs/a b.md> \"title\"", target: "<docs/a b.md>", ok: true},
		{line: "[^1]: a footnote", ok: false},
		{line: "text [ref]: not at the start", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()

			if target, ok := referenceTarget(tc.line); target != tc.target || ok != tc.ok {
				t.Errorf("referenceTarget(%q) = %q, %v, want %q, %v", tc.line, target, ok, tc.target, tc.ok)
			}
		})
	}
}

// TestWithoutFrontmatter_DropsTheOpeningBlockAlone verifies the frontmatter
// a page opens with, closed by three dashes or three dots, and a block that
// does not open the page, which is kept.
func TestWithoutFrontmatter_DropsTheOpeningBlockAlone(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"---\ntitle: x\n---\nbody":       "body",
		"---\r\ntitle: x\r\n...\r\nbody": "body",
		"---\ntitle: x\n---":             "",
		"body\n---\ntitle: x\n---\n":     "body\n---\ntitle: x\n---\n",
		"---\nnever closed\n":            "---\nnever closed\n",
	}
	for content, want := range cases {
		t.Run(content, func(t *testing.T) {
			t.Parallel()

			if got := withoutFrontmatter(content); got != want {
				t.Errorf("withoutFrontmatter(%q) = %q, want %q", content, got, want)
			}
		})
	}
}
