// Package mdshape judges the shape of one rendered Markdown response: whether
// a block of it is a table, a card, or the two mixed into a block that renders
// as neither.
//
// It reads the finished string rather than the source that built it, which is
// the whole reason it exists. A formatter writes its card across conditionals,
// loops and helpers in three packages, and a gate that read the source would
// have to model all of it to answer a question the output answers by itself.
package mdshape

import (
	"fmt"
	"strings"
)

// Kind names one way a block can be wrong.
type Kind string

const (
	// KindHeaderless is a block of pipe rows that no delimiter row turns into
	// a table. GFM renders those rows as literal text, pipes included, which
	// is the defect that started the audit: a formatter appending to a card
	// was given table rows.
	KindHeaderless Kind = "pipe-rows-without-a-delimiter"
	// KindFieldInTable is a card field written inside a table block. The line
	// ends the table at the delimiter, so the header renders as a table with
	// no body and every row after it renders as literal text inside the list
	// item the field opened.
	KindFieldInTable Kind = "card-field-inside-a-table"
	// KindMixed is a block holding both shapes in some other order.
	KindMixed Kind = "mixed-shapes-in-one-block"
	// KindBoldPrefix is a field written as "**Label:** value", a third shape
	// that is neither a card field nor a table row.
	KindBoldPrefix Kind = "bold-prefix-field"
)

// Kinds lists every kind of finding, in the order a report names them: the
// two that cost a block its structure outright, then the one that mixes two
// shapes in some other way, then the third-shape sweep.
func Kinds() []Kind {
	return []Kind{KindHeaderless, KindFieldInTable, KindMixed, KindBoldPrefix}
}

// Shape names what a block of lines is.
type Shape string

const (
	// ShapeTable is a header row, a delimiter row, and rows under them.
	ShapeTable Shape = "table"
	// ShapeCard is a run of "- **Label**: value" field lines.
	ShapeCard Shape = "card"
	// ShapeOther is everything else: prose, headings, quotes, code, a plain
	// bulleted list of hints.
	ShapeOther Shape = "other"
)

// Finding is one block that does not hold together.
type Finding struct {
	Kind Kind   `json:"kind"`
	Line int    `json:"line"`
	Text string `json:"text"`
	Why  string `json:"why"`
}

// Report is what one rendered response amounts to: the shapes it is built out
// of, and the blocks that are not one of them.
type Report struct {
	Shapes   []Shape   `json:"shapes"`
	Findings []Finding `json:"findings"`
}

// Cards reports how many blocks of the response are one-object cards.
func (r Report) Cards() int { return r.count(ShapeCard) }

// Tables reports how many blocks of the response are tables.
func (r Report) Tables() int { return r.count(ShapeTable) }

func (r Report) count(want Shape) int {
	n := 0
	for _, s := range r.Shapes {
		if s == want {
			n++
		}
	}
	return n
}

// Lint splits a rendered Markdown response into blocks and judges each one.
//
// A blank line separates blocks, which is what separates them for a Markdown
// parser as well: a table ends at one, and so does a list. Lines inside a
// fenced code block are skipped whole, because a job log or a repository file
// quoted into a response is free to contain anything, pipe rows included, and
// none of it is this response's own shape.
func Lint(md string) Report {
	var report Report
	var block []line
	inFence, fence := false, ""

	flush := func() {
		if len(block) == 0 {
			return
		}
		shape, findings := judge(block)
		report.Shapes = append(report.Shapes, shape)
		report.Findings = append(report.Findings, findings...)
		block = nil
	}

	for number, text := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(text)
		if run := openingFence(trimmed); run != "" && !inFence {
			flush()
			inFence, fence = true, run
			continue
		}
		if inFence {
			if strings.HasPrefix(trimmed, fence) {
				inFence, fence = false, ""
			}
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		block = append(block, line{number: number + 1, text: trimmed})
	}
	flush()
	return report
}

// line is one non-blank line of a block, kept with the number it was written
// on so a finding points at it.
type line struct {
	number int
	text   string
}

// judge names a block's shape and reports what is wrong with it.
//
// A table may open partway down a block: GFM lets a table interrupt a
// paragraph, so the summary line a list formatter writes above its header is
// not a defect and is not reported as one. What the block may not do is carry
// pipe rows no delimiter row turns into a table, or carry a card field on
// either side of a table, which ends the table at that line.
func judge(block []line) (Shape, []Finding) {
	var findings []Finding
	for _, l := range block {
		if isBoldPrefixField(l.text) {
			findings = append(findings, Finding{
				Kind: KindBoldPrefix, Line: l.number, Text: l.text,
				Why: `a card field is written "- **Label**: value"; this line is a third shape`,
			})
		}
	}

	hasField := anyLine(block, isCardField)
	first := firstPipeRow(block)
	if first < 0 {
		if hasField {
			return ShapeCard, findings
		}
		return ShapeOther, findings
	}

	// A pipe row is only a table row once a delimiter row under it says so.
	// Until then GFM renders the row as text, pipes included, which is the
	// defect that started the audit.
	if first+1 >= len(block) || !isDelimiterRow(block[first+1].text) {
		kind, why := KindHeaderless, "a pipe row renders as literal text until a delimiter row under it turns the block into a table"
		if hasField {
			kind, why = KindMixed, "the block holds both card fields and pipe rows, so it renders as neither"
		}
		findings = append(findings, Finding{Kind: kind, Line: block[first].number, Text: block[first].text, Why: why})
		return ShapeOther, findings
	}

	for _, l := range block[:first] {
		if isCardField(l.text) {
			findings = append(findings, Finding{
				Kind: KindMixed, Line: l.number, Text: l.text,
				Why: "a card field above a table in the same block keeps the table from opening",
			})
		}
	}
	for _, l := range block[first+2:] {
		if isPipeRow(l.text) {
			continue
		}
		kind, why := KindMixed, "the line is not a table row, so the table ends above it"
		if isCardField(l.text) {
			kind, why = KindFieldInTable, "the table ends at this line, leaving a header with no body and every row below it as literal text"
		}
		findings = append(findings, Finding{Kind: kind, Line: l.number, Text: l.text, Why: why})
	}
	return ShapeTable, findings
}

// firstPipeRow returns the index of the block's first pipe row, or -1.
func firstPipeRow(block []line) int {
	for i, l := range block {
		if isPipeRow(l.text) {
			return i
		}
	}
	return -1
}

// anyLine reports whether one of the block's lines satisfies pred.
func anyLine(block []line, pred func(string) bool) bool {
	for _, l := range block {
		if pred(l.text) {
			return true
		}
	}
	return false
}

// isPipeRow reports whether a line is written as a table row.
func isPipeRow(text string) bool {
	return strings.HasPrefix(text, "|")
}

// isDelimiterRow reports whether a line is a table's delimiter row: cells of
// dashes, each optionally carrying the colons that align a column.
func isDelimiterRow(text string) bool {
	if !strings.HasPrefix(text, "|") || !strings.HasSuffix(text, "|") || len(text) < 3 {
		return false
	}
	for cell := range strings.SplitSeq(strings.Trim(text, "|"), "|") {
		cell = strings.TrimSpace(cell)
		cell = strings.TrimPrefix(cell, ":")
		cell = strings.TrimSuffix(cell, ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

// isCardField reports whether a line is a card field: a list item whose label
// is bold and whose value follows a colon.
func isCardField(text string) bool {
	rest, ok := cutListMarker(text)
	if !ok || !strings.HasPrefix(rest, "**") {
		return false
	}
	label, after, closed := strings.Cut(rest[2:], "**")
	return closed && label != "" && strings.HasPrefix(after, ":")
}

// isBoldPrefixField reports whether a line is written "**Label:** value" or
// "**Label**: value" with no list marker in front of it.
func isBoldPrefixField(text string) bool {
	if !strings.HasPrefix(text, "**") {
		return false
	}
	label, after, closed := strings.Cut(text[2:], "**")
	if !closed || label == "" || after == "" {
		return false
	}
	return strings.HasSuffix(label, ":") || strings.HasPrefix(after, ":")
}

// cutListMarker strips a bullet or an ordered marker off a line, reporting
// whether the line had one.
func cutListMarker(text string) (string, bool) {
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(text, bullet); ok {
			return rest, true
		}
	}
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits+1 >= len(text) {
		return "", false
	}
	if (text[digits] == '.' || text[digits] == ')') && text[digits+1] == ' ' {
		return text[digits+2:], true
	}
	return "", false
}

// openingFence returns the backtick or tilde run that opens a fenced code
// block on this line, or "" when the line opens none.
func openingFence(text string) string {
	for _, char := range []byte{'`', '~'} {
		run := 0
		for run < len(text) && text[run] == char {
			run++
		}
		if run >= 3 {
			return text[:run]
		}
	}
	return ""
}

// String renders a finding the way a report lists it.
func (f Finding) String() string {
	return fmt.Sprintf("line %d: %s: %s (%s)", f.Line, f.Kind, f.Text, f.Why)
}
