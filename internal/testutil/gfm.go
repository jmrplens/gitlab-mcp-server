package testutil

import (
	"regexp"
	"strconv"
	"strings"
)

// GFMHintsHeading is the heading the server writes over its next-step hints,
// spelled here so the line model can find the section without importing the
// package that writes it.
const GFMHintsHeading = "\U0001F4A1 **Next steps:**"

// GFMFinding is one place a rendered Markdown document does not have the
// structure its formatter meant to write, judged by the block rules a GFM
// renderer applies to the bytes the client receives.
type GFMFinding struct {
	// Rule names the rule: T1 to T4 and T3b for tables, B1 to B3 for block
	// separation, H1 for the guidance section.
	Rule string
	// Line is the 1-based line the finding sits on.
	Line int
	// Text is that line as written.
	Text string
	// Detail says what the renderer makes of it.
	Detail string
}

// GFMTable is one table the model recognized: a pipe line followed by a
// delimiter row, and the body rows that follow.
type GFMTable struct {
	// Header and Delimiter are 0-based line indexes.
	Header, Delimiter int
	// Columns is the header's cell count.
	Columns int
	// Rows are the 0-based indexes of the body rows.
	Rows []int
}

// GFMHintSection is one guidance section: its heading line and the bullets
// directly under it.
type GFMHintSection struct {
	Line    int
	Bullets []string
}

// GFMBareLine is one line as the raw-tag rule must read it: the line as it
// was written, and the same line with every inline code span elided, since a
// span makes its contents literal and HTML-escaped.
type GFMBareLine struct {
	// Line is the 1-based line the text sits on.
	Line int
	// Text is that line as written, which is what a finding quotes.
	Text string
	// Bare is the line with every code span and its delimiters replaced by a
	// placeholder, which is what a finding scans.
	Bare string
}

// GFMDocument is what the line model read out of one rendered document.
type GFMDocument struct {
	Lines    []string
	Tables   []GFMTable
	Hints    []GFMHintSection
	Findings []GFMFinding
	// Headings, Items, Rows, Cells and Links are the structure the hostile
	// comparison holds constant: the ATX headings, the top-level list items,
	// the table body rows and their cells, and the link destinations, all
	// outside fences and quotes. Content is every line outside a fence or a
	// quote.
	Headings []string
	Items    []string
	Rows     int
	Cells    int
	Links    []string
	Content  []string
	// Bare is where a raw tag is a live tag rather than text: every line
	// outside a fence, its code spans elided. It carries quote lines, which
	// Content does not, because a blockquote contains structure and not tags
	// and a renderer takes an anchor inside one exactly as it takes one
	// outside.
	Bare []GFMBareLine
}

// gfmKind is the block a line opens, outside a fence.
type gfmKind int

const (
	gfmBlank gfmKind = iota
	gfmFence
	gfmQuote
	gfmHeading
	gfmBreak
	gfmItem
	gfmPipe
	gfmIndented
	gfmText
)

var (
	gfmHeadingRe   = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)
	gfmBreakRe     = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	gfmSetextRe    = regexp.MustCompile(`^ {0,3}(?:-+|=+)[ \t]*$`)
	gfmItemRe      = regexp.MustCompile(`^( *)(?:[-*+]|\d{1,9}[.)])(?: |$)`)
	gfmDelimCellRe = regexp.MustCompile(`^:?-+:?$`)
	gfmLabelRe     = regexp.MustCompile(`^\*\*[^*\n]+(?:\*\*:|:\*\*)`)
	// gfmLinkRe reads an inline link: a bracketed label and a destination.
	// A bare "](url)" with no opening bracket before it is text, so a value
	// that carries one opens nothing unless it lands inside a label.
	//
	// A label character is either one that is not a bracket, a backslash or a
	// newline, or a backslash and whatever follows it. That second half is
	// CommonMark's own rule twice over: an escaped character "is treated as a
	// regular character and does not have its usual Markdown meaning", and
	// brackets are allowed in link text when they are backslash-escaped. A
	// class that accepts the backslash without giving it that meaning reads
	// the escaped "]" of a label EscapeMdLinkLabel wrote as the end of the
	// label, and reports every link this server builds as a link to whatever
	// destination the value inside it names.
	gfmLinkRe = regexp.MustCompile(`\[(?:[^\[\]\\\n]|\\.)*\]\(([^)\s]+)\)`)
)

// gfmCodeSpanPlaceholder stands in for an elided code span: the object
// replacement character, spelled by its code point so the source carries no
// invisible byte. It holds no character a renderer acts on, so a rule reading
// a bare line can neither see structure the span hid nor lose the position
// the span occupied.
const gfmCodeSpanPlaceholder = string(rune(0xFFFC))

// ScanGFM reads a rendered Markdown document with a line model of the GFM
// block rules and reports where the structure the client renders is not the
// one the formatter wrote.
//
// The model is deliberately small: fences (a backtick run of three or more at
// the start of a line, closed by a run at least as long) and blockquote lines
// are skipped as content; list items, ATX headings, thematic breaks and
// indented continuations are recognized by how their line opens; a table is a
// pipe line followed by a delimiter row, and its cells split on every pipe
// not preceded by a backslash, because GFM splits inside code spans too. It
// is a test oracle for the shapes this server writes, not a renderer.
//
// Inline content is read in two places only, and each says which rule it
// serves: a link is a bracketed label whose brackets are escaped or matched,
// and a bare line is a line whose code spans have been elided, since what a
// span holds is literal.
func ScanGFM(md string) *GFMDocument {
	doc := &GFMDocument{Lines: strings.Split(md, "\n")}
	s := &gfmScanner{doc: doc, kinds: make([]gfmKind, len(doc.Lines)), inTable: make([]bool, len(doc.Lines))}
	s.classify()
	s.tables()
	s.blocks()
	s.hints()
	s.structure()
	return doc
}

// gfmScanner holds one scan's state: the kind of every line and which lines
// a recognized table owns.
type gfmScanner struct {
	doc     *GFMDocument
	kinds   []gfmKind
	inTable []bool
}

// classify decides what block each line opens, following fences so that a
// row, a heading or a bullet inside a code block is content.
func (s *gfmScanner) classify() {
	fence := 0
	for i, line := range s.doc.Lines {
		if run, ok := fenceRun(line); ok {
			switch {
			case fence == 0:
				fence = run
			case run >= fence:
				fence = 0
			}
			s.kinds[i] = gfmFence
			continue
		}
		if fence > 0 {
			s.kinds[i] = gfmFence
			continue
		}
		s.kinds[i] = classifyLine(line)
	}
}

// classifyLine reads the block one line opens outside a fence.
func classifyLine(line string) gfmKind {
	trimmed := strings.TrimLeft(line, " \t")
	switch {
	case strings.TrimSpace(line) == "":
		return gfmBlank
	case strings.HasPrefix(trimmed, ">") && leadingSpaces(line) <= 3:
		return gfmQuote
	case gfmHeadingRe.MatchString(line):
		return gfmHeading
	case gfmBreakRe.MatchString(line):
		return gfmBreak
	case gfmItemRe.MatchString(line):
		return gfmItem
	case strings.HasPrefix(trimmed, "|") && leadingSpaces(line) <= 3:
		return gfmPipe
	case leadingSpaces(line) >= 2:
		return gfmIndented
	default:
		return gfmText
	}
}

// leadingSpaces counts the spaces a line opens with.
func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// fenceRun reads the backtick run a line opens with, after the indentation
// CommonMark allows in front of one; a run shorter than three opens a code
// span, not a block.
func fenceRun(line string) (int, bool) {
	indent := leadingSpaces(line)
	if indent > 3 {
		return 0, false
	}
	run := 0
	for indent+run < len(line) && line[indent+run] == '`' {
		run++
	}
	if run < 3 {
		return 0, false
	}
	return run, true
}

// GFMCells splits one table line into its cells the way GFM does: on every
// pipe not preceded by a backslash, the entity &#124; left alone, with the
// one leading and one trailing pipe the house style writes removed.
func GFMCells(line string) []string {
	var cells []string
	var cell strings.Builder
	escaped := false
	for _, r := range strings.TrimSpace(line) {
		switch {
		case escaped:
			cell.WriteRune(r)
			escaped = false
		case r == '\\':
			cell.WriteRune(r)
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteRune(r)
		}
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	if len(cells) > 0 && cells[0] == "" && strings.HasPrefix(strings.TrimSpace(line), "|") {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" && strings.HasSuffix(strings.TrimSpace(line), "|") {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// isDelimiterRow reports whether a pipe line is a table delimiter: every cell
// a run of dashes with an optional alignment colon at either end.
func isDelimiterRow(line string) bool {
	cells := GFMCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !gfmDelimCellRe.MatchString(strings.ReplaceAll(cell, " ", "")) {
			return false
		}
	}
	return true
}

// tables recognizes every table and applies the table rules: T1, a header
// must open a block of its own; T2, the header and the delimiter agree on the
// cell count; T3, a line after the delimiter that is not a row is absorbed
// as one or ends the table under the rows it was meant to sit beside; T3b, a
// row's cell count is the header's; T4, a pipe line no table owns is a row
// rendered as text.
func (s *gfmScanner) tables() {
	lines := s.doc.Lines
	for i := 0; i+1 < len(lines); i++ {
		if s.kinds[i] != gfmPipe || s.kinds[i+1] != gfmPipe || !isDelimiterRow(lines[i+1]) || s.inTable[i] {
			continue
		}
		table := GFMTable{Header: i, Delimiter: i + 1, Columns: len(GFMCells(lines[i]))}
		s.inTable[i], s.inTable[i+1] = true, true
		if i > 0 && !opensBlock(s.kinds[i-1]) {
			s.finding("T1", i, "a table header directly under a paragraph or list line lazily continues it, so no table renders")
		}
		if delimiter := len(GFMCells(lines[i+1])); delimiter != table.Columns {
			s.finding("T2", i+1, "the delimiter row has "+strconv.Itoa(delimiter)+" cell(s) and the header "+strconv.Itoa(table.Columns)+", so no table renders")
		}
		end := s.body(&table, i+2)
		s.doc.Tables = append(s.doc.Tables, table)
		i = end - 1
	}
	for i, line := range lines {
		if s.kinds[i] == gfmPipe && !s.inTable[i] && strings.HasSuffix(strings.TrimSpace(line), "|") {
			s.finding("T4", i, "a pipe row outside any table renders as text")
		}
	}
}

// body reads a table's rows from the line after the delimiter to the line
// that ends the table, and returns the index of that line.
func (s *gfmScanner) body(table *GFMTable, start int) int {
	lines := s.doc.Lines
	j := start
	for ; j < len(lines) && s.kinds[j] != gfmBlank; j++ {
		kind := s.kinds[j]
		if kind == gfmHeading || kind == gfmBreak || kind == gfmFence || kind == gfmQuote {
			break
		}
		if kind == gfmItem {
			s.finding("T3", j, "a list item ends the table, and every row written after it is text")
			break
		}
		if kind != gfmPipe {
			s.finding("T3", j, "a line written directly after the last row is absorbed as one more row")
		} else if cells := len(GFMCells(lines[j])); cells != table.Columns {
			s.finding("T3b", j, "the row has "+strconv.Itoa(cells)+" cell(s) and the header "+strconv.Itoa(table.Columns))
		}
		s.inTable[j] = true
		table.Rows = append(table.Rows, j)
		s.doc.Rows++
		s.doc.Cells += len(GFMCells(lines[j]))
	}
	return j
}

// opensBlock reports whether a line of this kind lets a table header after
// it open a table: a blank line, a heading or a rule does, and a paragraph,
// a list item or a row does not.
func opensBlock(kind gfmKind) bool {
	return kind == gfmBlank || kind == gfmHeading || kind == gfmBreak || kind == gfmFence
}

// blocks applies the block-separation rules: B1, a rule directly under a
// paragraph line turns it into a setext heading; B2, a line directly after a
// list item continues that item; B3, consecutive bullet-less label lines
// render as one paragraph.
func (s *gfmScanner) blocks() {
	lines := s.doc.Lines
	for i := 1; i < len(lines); i++ {
		previous := s.kinds[i-1]
		switch {
		case previous == gfmText && gfmSetextRe.MatchString(lines[i]):
			s.finding("B1", i, "a rule directly under a paragraph line turns that line into a setext heading")
		case previous == gfmItem && s.kinds[i] == gfmText:
			s.finding("B2", i, "a line directly after a list item lazily continues that item")
		case previous == gfmText && s.kinds[i] == gfmText && gfmLabelRe.MatchString(lines[i]) && gfmLabelRe.MatchString(lines[i-1]):
			s.finding("B3", i, "consecutive bullet-less label lines render as one run-on paragraph")
		}
	}
}

// hints reads every guidance section: the heading and the bullets directly
// under it. More than one section is a finding (H1), since ExtractHints reads
// only a leading or a trailing one; whether the one section agrees with what
// ExtractHints returns is the harness's comparison, which owns that function.
func (s *gfmScanner) hints() {
	for i, line := range s.doc.Lines {
		if s.kinds[i] == gfmFence || s.kinds[i] == gfmQuote || strings.TrimSpace(line) != GFMHintsHeading {
			continue
		}
		section := GFMHintSection{Line: i}
		for j := i + 1; j < len(s.doc.Lines) && s.kinds[j] == gfmItem; j++ {
			section.Bullets = append(section.Bullets, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s.doc.Lines[j]), "-")))
		}
		s.doc.Hints = append(s.doc.Hints, section)
	}
	if len(s.doc.Hints) > 1 {
		s.finding("H1", s.doc.Hints[1].Line, "a second guidance section, which ExtractHints cannot read as the server's")
	}
}

// structure collects the headings, the top-level items and the links, for a
// comparison between two renders of one fixture, and the bare lines the
// raw-tag rule reads.
func (s *gfmScanner) structure() {
	for i, line := range s.doc.Lines {
		if s.kinds[i] == gfmFence {
			continue
		}
		bare := s.bareLine(i, line)
		s.doc.Bare = append(s.doc.Bare, GFMBareLine{Line: i + 1, Text: line, Bare: bare})
		if s.kinds[i] == gfmQuote {
			continue
		}
		switch s.kinds[i] {
		case gfmHeading:
			s.doc.Headings = append(s.doc.Headings, strings.TrimSpace(line))
		case gfmItem:
			if leadingSpaces(line) < 2 {
				s.doc.Items = append(s.doc.Items, strings.TrimSpace(line))
			}
		}
		s.doc.Content = append(s.doc.Content, line)
		// Links are read off the bare line, not the raw one: a code span takes
		// precedence over a link in CommonMark, so a bracketed address inside
		// one is the text a formatter deliberately moved into a span and not
		// a destination a client would follow. Reading the raw line reported a
		// link the page does not have, against exactly the value that had
		// been contained.
		for _, m := range gfmLinkRe.FindAllStringSubmatch(bare, -1) {
			s.doc.Links = append(s.doc.Links, m[1])
		}
	}
}

// bareLine elides the inline code spans of one line.
//
// A row of a recognized table is tokenized per cell first, because GFM splits
// a row on its unescaped pipes before anything parses the inline content, so
// a span cannot cross a cell boundary; every other line is walked whole,
// which is what CommonMark does to a paragraph, a pipe line no table owns
// included.
func (s *gfmScanner) bareLine(i int, line string) string {
	if !s.inTable[i] {
		return elideCodeSpans(line)
	}
	segments := gfmPipeSegments(line)
	for j, segment := range segments {
		segments[j] = elideCodeSpans(segment)
	}
	return strings.Join(segments, "|")
}

// elideCodeSpans replaces every inline code span of a line, its delimiters
// included, with [gfmCodeSpanPlaceholder].
//
// The walk is the spec's rule read left to right: a backtick string of length
// n opens a span that the next backtick string of exactly length n closes,
// and a run with no such partner is literal text that opens nothing. Reading
// it in that order settles the one precedence question this has by
// construction: a "<" written before the first backtick is outside every span
// the walk recognizes, so a raw tag cannot be hidden by a code span opened
// after it.
func elideCodeSpans(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		open := backtickRun(line, i)
		if closer := nextBacktickRun(line, i+open, open); closer >= 0 {
			b.WriteString(gfmCodeSpanPlaceholder)
			i = closer + open
			continue
		}
		b.WriteString(line[i : i+open])
		i += open
	}
	return b.String()
}

// backtickRun returns the length of the run of backticks at i.
func backtickRun(line string, i int) int {
	run := 0
	for i+run < len(line) && line[i+run] == '`' {
		run++
	}
	return run
}

// nextBacktickRun returns the index of the first run of exactly n backticks
// at or after i, or -1 when the line holds none: a longer or a shorter run
// does not close a span and is stepped over whole.
func nextBacktickRun(line string, i, n int) int {
	for j := i; j < len(line); {
		if line[j] != '`' {
			j++
			continue
		}
		run := backtickRun(line, j)
		if run == n {
			return j
		}
		j += run
	}
	return -1
}

// gfmPipeSegments splits a line on the pipes GFM splits a row on: the same
// unescaped-pipe rule [GFMCells] applies, without its trimming and without
// dropping the outer empties, since these pieces are rejoined rather than
// read as cells.
func gfmPipeSegments(line string) []string {
	var segments []string
	var segment strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			segment.WriteRune(r)
			escaped = false
		case r == '\\':
			segment.WriteRune(r)
			escaped = true
		case r == '|':
			segments = append(segments, segment.String())
			segment.Reset()
		default:
			segment.WriteRune(r)
		}
	}
	return append(segments, segment.String())
}

// finding records one finding at a 0-based line.
func (s *gfmScanner) finding(rule string, line int, detail string) {
	s.doc.Findings = append(s.doc.Findings, GFMFinding{Rule: rule, Line: line + 1, Text: s.doc.Lines[line], Detail: detail})
}

// Rules lists the rules the findings hit, each once, in rule order.
func (d *GFMDocument) Rules() []string {
	seen := map[string]bool{}
	var rules []string
	for _, f := range d.Findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			rules = append(rules, f.Rule)
		}
	}
	return rules
}
