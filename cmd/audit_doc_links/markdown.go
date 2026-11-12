// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"strings"
)

// fenceMarker matches a line that opens or closes a fenced code block: up
// to three spaces, a run of three or more backticks or tildes, and the rest
// of the line, the info string of an opening fence.
var fenceMarker = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

// inlineLink matches an inline link or image and captures its destination,
// angle-bracketed or bare, before an optional title.
var inlineLink = regexp.MustCompile(`!?\[[^\]]*\]\(\s*(<[^>]+>|[^)\s]+)(?:\s+"[^"]*")?\s*\)`)

// referenceDefinition matches a link reference definition and captures its
// label and its destination. A label opening with a caret is a footnote,
// which [referenceTarget] leaves out.
var referenceDefinition = regexp.MustCompile(`^\s*\[([^\]]+)\]:\s*(<[^>]+>|\S+)`)

// frontmatter matches the YAML block a page opens with.
var frontmatter = regexp.MustCompile(`(?s)^---[ \t]*\r?\n.*?\r?\n(?:---|\.{3})[ \t]*(?:\r?\n|$)`)

// eachProseLine visits the lines a reader is shown as prose, which is every
// line outside a fenced code block, with its number counted from 1.
//
// A fence closes on the marker that opened it, at the same length or longer,
// and on nothing else. A toggle that flipped on any line starting a fence
// would close a four-backtick block at the three-backtick sample inside it,
// and read every line after that in the opposite state: in
// .github/instructions/*.md, where Markdown samples are nested that way,
// whole sections were once scanned as prose or skipped as code depending on
// how many inner fences had gone by.
func eachProseLine(content string, visit func(line string, number int)) {
	var fenceChar byte
	fenceLength := 0
	for index, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		marker := fenceMarker.FindStringSubmatch(line)
		if fenceLength > 0 {
			if len(marker) == 3 && marker[1][0] == fenceChar && len(marker[1]) >= fenceLength && strings.TrimSpace(marker[2]) == "" {
				fenceLength = 0
			}
			continue
		}
		if len(marker) == 3 {
			fenceChar, fenceLength = marker[1][0], len(marker[1])
			continue
		}
		visit(line, index+1)
	}
}

// inlineLinks returns the destinations of the inline links and images on a
// line, read outside its code spans.
func inlineLinks(line string) []string {
	var targets []string
	for _, match := range inlineLink.FindAllStringSubmatch(withoutCodeSpans(line), -1) {
		targets = append(targets, match[1])
	}
	return targets
}

// referenceTarget returns the destination a link reference definition on the
// line names, and whether the line is one. A footnote definition is not.
func referenceTarget(line string) (string, bool) {
	match := referenceDefinition.FindStringSubmatch(line)
	if len(match) != 3 || strings.HasPrefix(match[1], "^") {
		return "", false
	}
	return match[2], true
}

// withoutCodeSpans blanks the contents of every backtick span, keeping the
// line the same length so a reported column still points at the right place.
//
// Text inside a code span is quoted, not linked: prose about a formatter that
// must not hand-write `[%s](%s)` was read as a link to a file named `%s` and
// failed this check, which is the checker being wrong about Markdown rather
// than the sentence being wrong about the code.
//
// A span is what the regular expression the script used matched, with its
// backtracking: at each position, the longest run of backticks that a body
// and an equal run not followed by another backtick can close, the shortest
// such body first, and a body that is one character other than a backtick or
// starts and ends with one.
func withoutCodeSpans(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if end, open, ok := codeSpanAt(line, i); ok {
			b.WriteString(line[i : i+open])
			b.WriteString(strings.Repeat(" ", end-open-(i+open)))
			b.WriteString(line[end-open : end])
			i = end
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// codeSpanAt reports whether a code span opens at index i of line, and
// where it ends and how long its fences are.
func codeSpanAt(line string, i int) (end, fence int, ok bool) {
	run := 0
	for i+run < len(line) && line[i+run] == '`' {
		run++
	}
	for open := run; open > 0; open-- {
		body := i + open
		if body >= len(line) || line[body] == '`' {
			continue
		}
		for closing := body + 1; closing+open <= len(line); closing++ {
			// A body ends with a character other than a backtick; one of a
			// single character is the one checked above.
			if line[closing-1] == '`' {
				continue
			}
			if strings.Count(line[closing:closing+open], "`") == open && (closing+open == len(line) || line[closing+open] != '`') {
				return closing + open, open, true
			}
		}
	}
	return 0, 0, false
}

// withoutFrontmatter drops the YAML block a page opens with. It carries the
// title Starlight renders the <h1> from, and GitHub draws it as a table: a
// heading in neither, and a "title: x" line above the closing "---" that
// would otherwise read as a Setext heading in both.
func withoutFrontmatter(content string) string {
	if match := frontmatter.FindStringIndex(content); match != nil {
		return content[match[1]:]
	}
	return content
}
