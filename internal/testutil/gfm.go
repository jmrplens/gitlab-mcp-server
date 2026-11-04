// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package testutil

import (
	"cmp"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
// was written, the same line with every inline code span elided, since a
// span makes its contents literal and HTML-escaped, and the raw HTML on it
// that a renderer passes through.
type GFMBareLine struct {
	// Line is the 1-based line the text sits on.
	Line int
	// Text is that line as written, which is what a finding quotes.
	Text string
	// Bare is the line with every code span and its delimiters replaced by a
	// placeholder.
	Bare string
	// RawHTML is every piece of raw HTML on the line that a renderer passes
	// through as markup, as written and in order: an open or closing tag, a
	// comment, a processing instruction, a declaration, a CDATA section. It
	// is what the raw-tag rule reads. A tag written inside a code span, after
	// a backslash that escapes its "<", or inside the description of an
	// image, which a renderer writes as the image's alt text, is text and is
	// not here.
	RawHTML []string
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
	//
	// Links holds the inline links, the images and the autolinks of each line,
	// in the order a renderer writes them: an inline link's destination and
	// an image's source as written, an autolink's address, and an email
	// autolink's address as the mailto: link a client opens, since the
	// address alone would read as a relative destination. <url> is a link a
	// client renders and follows exactly as it does [text](url), and
	// <user@host> is one to mailto:user@host. A link inside an image's
	// description is not here, because a renderer writes that description as
	// the image's alt text.
	//
	// Links is CommonMark's: an address written as plain text is not one.
	// GFM's literal autolinks, which make a renderer link a www., http:// or
	// https:// address, or an email address, written as text, are
	// LiteralLinks.
	Headings []string
	Items    []string
	Rows     int
	Cells    int
	Links    []string
	Content  []string
	// LiteralLinks are GFM's literal autolinks outside fences and quotes, in
	// the order a renderer writes them, each recorded as the destination a
	// client opens: an http or https address as written, a www. address after
	// "http://", and an email address after "mailto:". They are kept apart from
	// Links because the formatters write none of them on purpose: every one is
	// an address that reached the page as text, so a rule reads them on their
	// own. [gfmWalk.literal] says how one is read.
	LiteralLinks []string
	// SelfLinks are the inline links, outside fences, quotes and code spans,
	// whose label is their own destination, [url](url): the address written
	// twice where the autolink <url> writes it once (issue 1396). Each is
	// recorded as the destination, as written. An image is not one, since no
	// autolink can stand in for it.
	//
	// The label is its destination when the link renders as that autolink
	// does, the destination's text inside the anchor: the label holds plain
	// text and nothing that renders as markup (no code span, raw HTML,
	// autolink or image), and its characters are the destination's once
	// CommonMark's backslash escapes and character references are resolved on
	// both sides, so [https://h/a\_b](https://h/a_b) is one, and so is
	// [https://h/?a&amp;b](https://h/?a&b). The comparison is between
	// characters, before a renderer percent-encodes the destination into an
	// href, which the specification leaves to the renderer: the long form an
	// address keeps when it holds a pipe, [https://h/a&#124;b](https://h/a%7Cb),
	// is not one. Nor is a link whose address no autolink can hold, which
	// [gfmAutolinkableRe] states: [https://h/a b](<https://h/a b>) renders as
	// its own address, yet the long form is its only spelling, so it writes
	// nothing twice that could be written once.
	//
	// Three choices are deliberate. Emphasis is not modeled, so a label whose
	// "*" or "_" form emphasis, which renders without them, is still recorded
	// when its characters are the destination's: it writes the address a
	// second time all the same, which is what the rule counts, and GFM's
	// strikethrough, text between tildes, is read the same way. A numeric
	// reference to a control character other than a tab, a line feed, a form
	// feed and a carriage return (C0, DEL and C1) or to a noncharacter
	// (U+FDD0 to U+FDEF and the last two code points of each plane) is that
	// code point, as the specification reads it, where micromark writes
	// U+FFFD, and commonmark.js reads a C1 one through the Windows-1252 table.
	// And a reference link is not read at all, for the reason [walkInline]
	// gives.
	SelfLinks []string
	// Bare is where a raw tag is a live tag rather than text: every line
	// outside a fence, its code spans elided and its raw HTML listed. It
	// carries quote lines, which Content does not, because a blockquote
	// contains structure and not tags and a renderer takes an anchor inside
	// one exactly as it takes one outside.
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
	// gfmEscapeRe reads a backslash escape at the start of the text: a
	// backslash and the ASCII punctuation character it makes literal. A
	// backslash before anything else is a backslash.
	gfmEscapeRe = regexp.MustCompile(`^\\[!"#$%&'()*+,\-./:;<=>?@\[\\\]^_\x60{|}~]`)
	// gfmReferenceRe reads a character reference at the start of the text, as
	// CommonMark spells one between "&" and ";": a decimal number of one to
	// seven digits, its first group; a hexadecimal one of one to six after an
	// "x" or an "X", its second; or a name, which is a reference only when
	// HTML5 defines it, the question [gfmReference] asks.
	gfmReferenceRe = regexp.MustCompile(`^&(?:#([0-9]{1,7})|#[Xx]([0-9A-Fa-f]{1,6})|[A-Za-z][A-Za-z0-9]{1,31});`)
	// gfmAngleRe reads what a "<" at the start of the text opens, in the order
	// CommonMark tries it: an autolink, an absolute URI between angle brackets
	// whose address is the first group (a scheme of two to thirty-two
	// characters, a colon, and anything but a space, an ASCII control
	// character other than U+0000, which [walkInline] reads as U+FFFD, '<' or
	// '>'); an email autolink, whose address is the second group; and raw
	// HTML, which sets neither group: an open tag with its attributes, a
	// closing tag, a comment, a processing instruction, a declaration or a
	// CDATA section, spelled as in the specification's section on raw HTML
	// with the whitespace one line can hold. An unquoted attribute value is a
	// nonempty run of anything but a space, a tab, a line ending, '"', "'",
	// '=', '<', '>' and the backtick, so every other control character can be
	// part of one.
	//
	// It is written out whole, rather than joined from named parts, because
	// the mutation tool cannot see a package-level declaration and reads the
	// operator of a joined one as never covered.
	gfmAngleRe = regexp.MustCompile(`^(?:<([A-Za-z][A-Za-z0-9+.-]{1,31}:[^\x01-\x20<>\x7f]*)>|<([A-Za-z0-9.!#$%&'*+/=?^_\x60{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)>|<[A-Za-z][A-Za-z0-9-]*(?:[ \t]+[A-Za-z_:][A-Za-z0-9_.:-]*(?:[ \t]*=[ \t]*(?:[^"'=<>\x60 \t\r\n]+|'[^']*'|"[^"]*"))?)*[ \t]*/?>|</[A-Za-z][A-Za-z0-9-]*[ \t]*>|<!-->|<!--->|<!--.*?-->|<\?.*?\?>|<![A-Za-z][^>]*>|<!\[CDATA\[.*?\]\]>)`)
	// gfmAutolinkableRe matches an address toolutil.MdAutolink writes as the
	// autolink <url>: an http or https address, its scheme in either case,
	// with something after "://" and no space, ASCII control character, '<',
	// '>' or pipe, which would end the autolink or the table cell it sits in.
	// Only such an address makes a self-labeled link one an autolink could
	// replace.
	gfmAutolinkableRe = regexp.MustCompile(`(?i)^https?://[^\x00-\x20<>|\x7f]+$`)
	// gfmResourceOpenRe reads the opening of an inline link's resource, the
	// parenthesis right after the label and the whitespace after it.
	gfmResourceOpenRe = regexp.MustCompile(`^\([ \t]*`)
	// gfmEnclosedRe reads a destination between angle brackets, which may hold
	// a space and may be empty, and holds no unescaped '<' or '>'. The
	// destination is its group.
	gfmEnclosedRe = regexp.MustCompile(`^<((?:[^<>\\]|\\.)*)>`)
	// gfmResourceCloseRe reads the rest of a resource after its destination:
	// an optional title, separated from the destination by whitespace and
	// quoted with double quotes, single quotes or parentheses, then
	// whitespace and the closing parenthesis. A backslash and the character
	// after it are read as one, which is the escape rule wherever that
	// character could close the title.
	gfmResourceCloseRe = regexp.MustCompile(`^(?:[ \t]+(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\((?:[^()\\]|\\.)*\)))?[ \t]*\)`)
	// gfmLiteralEmailRe reads the longest email address a literal autolink can
	// open with: a run of the characters a local part holds, an "@", and a
	// domain of letters, digits, '-' and '_' in which a '.' is taken only when
	// a letter or a digit follows it. Whether the address is one is decided
	// on that longest run, which [gfmLiteralEmail] asks, and never on a
	// shorter one: micromark does not shorten an address to make it link.
	gfmLiteralEmailRe = regexp.MustCompile(`^[A-Za-z0-9+._-]+@(?:[A-Za-z0-9_-]|\.[A-Za-z0-9])*`)
	// gfmLiteralWwwRe reads the "www." a www. address opens with, in either
	// case, and the character after it, without which there is no address.
	gfmLiteralWwwRe = regexp.MustCompile(`^[Ww]{3}\..`)
	// gfmLiteralHTTPRe reads the scheme an http or https address opens with,
	// in any letter case, the "//" after its colon, and the character after
	// them, which must be neither an ASCII control character, nor whitespace,
	// nor Unicode punctuation or a symbol. U+0000 is a control character here
	// and U+FFFD, which micromark reads it as, a symbol: both refuse.
	gfmLiteralHTTPRe = regexp.MustCompile(`^(?i:https?)://[^\x00-\x20\x7f\pP\pS\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`)
	// gfmTrailRe matches where a literal autolink's domain or path ends: at a
	// run of trailing punctuation (one of !"')*,.:;?_~, a "]", or a character
	// reference such as &amp;) that reaches the end of the text, a "<",
	// whitespace, or a "]" right before a "(" or a "[". The punctuation is
	// then not part of the address. Anything else after the run makes it part
	// of the address, as micromark's trail check reads it.
	gfmTrailRe = regexp.MustCompile(`^(?:[!"')*,.:;?_~]|&[A-Za-z]+;|\])*(?:$|[<\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]|\][(\[])`)
)

// The character sets a literal autolink is read with. gfmWhitespace is what
// micromark reads as whitespace, JavaScript's \s: the ASCII tab, line feed,
// vertical tab, form feed, carriage return and space, the Unicode space
// separators, the line and paragraph separators, and U+FEFF. gfmPathPunctuation
// are the characters at which a path asks whether the trail begins.
// gfmEmailRefuses are the characters an email address cannot follow: one its
// local part holds, and '/'. gfmWwwFollows are the characters a www. address
// can follow, besides the start of the text. gfmLetters are the ASCII
// letters, which an http address cannot follow and an email address must end
// in. Each is written out whole, gfmEmailRefuses too, for the reason
// gfmAngleRe is.
const (
	gfmWhitespace      = "\t\n\v\f\r \U000000A0\U00001680\U00002000\U00002001\U00002002\U00002003\U00002004\U00002005\U00002006\U00002007\U00002008\U00002009\U0000200A\U00002028\U00002029\U0000202F\U0000205F\U00003000\U0000FEFF"
	gfmPathPunctuation = `!"&')*,.:;<?]_~`
	gfmLetters         = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	gfmEmailRefuses    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+-._/"
	gfmWwwFollows      = " \t(*_[]~"
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
// Inline content is read by one walk per line, or per cell of a table row,
// for the three things the rules ask of it: the code spans, whose contents
// are literal; the raw HTML a renderer passes through; and the links a
// client follows. [walkInline] says what that walk models.
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
//
// The scanner is written in ifs rather than tagless switches throughout: a
// case expression carries no statement counter, so the mutation gate reports
// every mutant of one as not covered whatever the tests assert.
func (s *gfmScanner) classify() {
	fence := 0
	for i, line := range s.doc.Lines {
		if run, ok := fenceRun(line); ok {
			if fence == 0 {
				fence = run
			} else if run >= fence {
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
	if strings.TrimSpace(line) == "" {
		return gfmBlank
	}
	if strings.HasPrefix(trimmed, ">") && leadingSpaces(line) <= 3 {
		return gfmQuote
	}
	if gfmHeadingRe.MatchString(line) {
		return gfmHeading
	}
	if gfmBreakRe.MatchString(line) {
		return gfmBreak
	}
	if gfmItemRe.MatchString(line) {
		return gfmItem
	}
	if strings.HasPrefix(trimmed, "|") && leadingSpaces(line) <= 3 {
		return gfmPipe
	}
	if leadingSpaces(line) >= 2 {
		return gfmIndented
	}
	return gfmText
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
		if escaped {
			cell.WriteRune(r)
			escaped = false
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
			continue
		}
		cell.WriteRune(r)
		escaped = r == '\\'
	}
	// The last cell is appended whatever the line held, so there is always a
	// first cell to look at; and a leading pipe has already closed one cell
	// before that last one, so cutting the first leaves a last.
	cells = append(cells, strings.TrimSpace(cell.String()))
	if cells[0] == "" && strings.HasPrefix(strings.TrimSpace(line), "|") {
		cells = cells[1:]
	}
	if cells[len(cells)-1] == "" && strings.HasSuffix(strings.TrimSpace(line), "|") {
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
	// A table's own lines are never asked about: the loop resumes at the
	// line that ended the table, which body leaves unmarked.
	for i := 0; i+1 < len(lines); i++ {
		if s.kinds[i] != gfmPipe || s.kinds[i+1] != gfmPipe || !isDelimiterRow(lines[i+1]) {
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
		if previous == gfmText && gfmSetextRe.MatchString(lines[i]) {
			s.finding("B1", i, "a rule directly under a paragraph line turns that line into a setext heading")
			continue
		}
		if previous == gfmItem && s.kinds[i] == gfmText {
			s.finding("B2", i, "a line directly after a list item lazily continues that item")
			continue
		}
		// A label line is a text line unless a fence holds it, and a line
		// after a text line is outside any fence, so the kind of the line
		// above is the one kind this rule needs to read: asking for the
		// current line's as well asked nothing a test could tell apart.
		if previous == gfmText && gfmLabelRe.MatchString(lines[i]) && gfmLabelRe.MatchString(lines[i-1]) {
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
		inline := s.inline(i, line)
		s.doc.Bare = append(s.doc.Bare, GFMBareLine{Line: i + 1, Text: line, Bare: inline.bare, RawHTML: inline.html})
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
		for _, link := range inline.links {
			s.doc.Links = append(s.doc.Links, link.dest)
			if link.self {
				s.doc.SelfLinks = append(s.doc.SelfLinks, link.dest)
			}
		}
		for _, link := range inline.literals {
			s.doc.LiteralLinks = append(s.doc.LiteralLinks, link.dest)
		}
	}
}

// inline reads the inline content of one line.
//
// A row of a recognized table is read per cell, because GFM splits a row on
// its unescaped pipes before anything parses the inline content, so neither a
// span nor a link can cross a cell boundary; every other line is walked
// whole, which is what CommonMark does to a paragraph, a pipe line no table
// owns included.
func (s *gfmScanner) inline(i int, line string) gfmInline {
	if !s.inTable[i] {
		return walkInline(line)
	}
	var row gfmInline
	var bare []string
	for _, segment := range gfmPipeSegments(line) {
		cell := walkInline(segment)
		bare = append(bare, cell.bare)
		row.links = append(row.links, cell.links...)
		row.literals = append(row.literals, cell.literals...)
		row.html = append(row.html, cell.html...)
	}
	row.bare = strings.Join(bare, "|")
	return row
}

// gfmInline is what one walk read out of a line or a cell: the text with its
// code spans elided, its links and its literal autolinks, each in the order a
// renderer writes them, and its live raw HTML.
type gfmInline struct {
	bare     string
	links    []gfmLink
	literals []gfmLink
	html     []string
}

// gfmLink is one link the walk read: where it opens, which orders it among the
// others, the destination as recorded in [GFMDocument.Links], and whether it
// is an inline link whose label is that destination.
type gfmLink struct {
	at   int
	dest string
	self bool
}

// gfmOpener is a "[" or a "![" that may still open a link or an image: where
// it sits, where its label begins in the bare text, how many code spans and
// links and how much raw HTML the walk had read when it opened, which is
// where an image's description begins in each and what tells a label of
// plain text from one holding a construct, and whether a link formed inside
// it, which leaves it nothing but a bracket.
type gfmOpener struct {
	at, label, spans, links, html int
	image, inactive               bool
}

// gfmWalk is the state of one inline walk. runs indexes the line's backtick
// strings by length, each length's starts left to right, which is where a
// span's closing string is looked up; spans counts the spans the walk read.
type gfmWalk struct {
	line     string
	runs     map[int][]int
	bare     strings.Builder
	spans    int
	links    []gfmLink
	literals []gfmLink
	html     []string
	openers  []gfmOpener
}

// walkInline reads the inline content of a line, or of one table cell, the way
// CommonMark does: left to right, a construct read whole where it begins.
//
// Four constructs share one precedence, and the one that begins first wins: a
// code span, an autolink (an absolute URI or an email address between angle
// brackets), raw HTML, and a backslash escape. Nothing inside one of them
// opens anything else, so a backtick in an autolink's address or in a tag's
// quoted attribute opens no span, and a tag or an autolink inside a span is
// text. A backtick string of length n opens a span that the next string of
// exactly length n closes, and a string with no such partner is literal and
// opens nothing. A backslash before an ASCII punctuation character makes that
// character literal, so an escaped backtick opens no span, an escaped "<"
// opens no autolink and no tag, and an escaped bracket opens no label; inside
// a span, an autolink or a tag a backslash is only a backslash, so it does
// not keep a span from closing.
//
// Links are read with CommonMark's bracket rule rather than a pattern: a "["
// or a "![" outside those constructs opens a label, the next "]" closes the
// latest one still open, and a link or an image forms when a resource,
// "(destination "title")", follows that "]". The resource is read as written,
// so a backtick in a destination opens no span. Links do not nest: once a
// link forms, every "[" still open around it is only a bracket. An image's
// description is its alt text, so a link, an autolink or a tag written inside
// one is not live, and is not recorded.
//
// U+0000 is read as the U+FFFD the specification replaces it with, as
// micromark does before it parses anything, and U+FFFD is no control
// character: a destination and an autolink's address may hold a U+0000, and
// it is written to the bare text and recorded as written.
//
// GFM's literal autolinks are read last, at a byte that begins none of the
// rest, as [gfmWalk.literal] says, and consume the address they read, so
// nothing inside one opens anything else.
//
// Where a renderer departs from the specification the walk follows the
// specification: micromark refuses a "!" in the local part of an email
// autolink and a "/" in an unquoted attribute value, which CommonMark 0.31.2
// allows, and accepts an unescaped "(" inside a title in parentheses, which
// CommonMark 0.31.2 refuses. Two things are not modeled: what one line cannot
// show (a reference link and its definition, a construct that continues onto
// the next line, raw HTML that opens a block), and the limit of 32 nested
// parentheses micromark puts on a destination.
//
// The walk ranges over the line's bytes and skips what a construct consumed,
// and every loop it calls ranges too, so no loop here advances by an amount
// it computed: a mutant that inverts a condition then gives a wrong answer,
// which a test sees, rather than a walk that never ends, which the mutation
// gate can only report as timed out.
func walkInline(line string) gfmInline {
	w := &gfmWalk{line: line, runs: backtickRuns(line)}
	next := 0
	for i := range len(line) {
		if i < next {
			continue
		}
		if end, consumed := w.construct(i); consumed {
			next = end
			continue
		}
		w.bare.WriteByte(line[i])
	}
	slices.SortFunc(w.links, func(a, b gfmLink) int { return cmp.Compare(a.at, b.at) })
	return gfmInline{bare: w.bare.String(), links: w.links, literals: w.literals, html: w.html}
}

// construct reads whatever begins at byte i and returns the index just past
// it, with true when it consumed those bytes and wrote them to the bare text
// itself. A "[" opens a label and is written by the walk, like any byte that
// begins nothing. A literal autolink is tried last, since none of the other
// constructs opens with a character one can open with.
func (w *gfmWalk) construct(i int) (int, bool) {
	rest := w.line[i:]
	if escape := gfmEscapeRe.FindString(rest); escape != "" {
		w.bare.WriteString(escape)
		return i + len(escape), true
	}
	if rest[0] == '`' {
		return w.codeSpan(i), true
	}
	if rest[0] == '<' {
		return w.angle(i)
	}
	if strings.HasPrefix(rest, "![") {
		w.bare.WriteString("![")
		w.open(i, w.bare.Len(), true)
		return i + 2, true
	}
	if rest[0] == '[' {
		w.open(i, w.bare.Len()+1, false)
		return 0, false
	}
	if rest[0] == ']' {
		return w.closeLabel(i)
	}
	if dest, n := w.literal(i); n > 0 {
		w.literals = append(w.literals, gfmLink{at: i, dest: dest})
		w.bare.WriteString(rest[:n])
		return i + n, true
	}
	return 0, false
}

// literal reads the GFM literal autolink that begins at byte i, the way
// micromark-extension-gfm-autolink-literal 2.1.0 reads one, and returns the
// destination a client opens and the address's length, or a length of zero
// when none begins there. Three kinds are tried, in micromark's order:
//
//   - an email address, at a character its local part holds that does not
//     follow one, nor a '/': the destination is "mailto:" and the address;
//   - a www. address, at the start of the text or after a space, a tab, or one
//     of "(*_[]~": the destination is "http://" and the address;
//   - an http or https address, at a scheme that does not follow an ASCII
//     letter: the destination is the address as written.
//
// The character before byte i is the byte before it as written, since that
// is the character micromark last read whatever construct it closed: the
// backtick of a span, the '>' of a tag, the character an escape made literal.
// A byte that ends a multibyte character is none of the characters these
// rules name, as the character it ends is not.
//
// None opens while a "[" or a "![" is still open, which is micromark's rule:
// it reads no literal autolink after a bracket no "]" has closed yet, a
// link's label included, so an address in a label is never one.
func (w *gfmWalk) literal(i int) (dest string, length int) {
	if len(w.openers) > 0 {
		return "", 0
	}
	previous := ' '
	if i > 0 {
		previous = rune(w.line[i-1])
	}
	rest := w.line[i:]
	if !strings.ContainsRune(gfmEmailRefuses, previous) {
		if n := gfmLiteralEmail(rest); n > 0 {
			return "mailto:" + rest[:n], n
		}
	}
	if strings.ContainsRune(gfmWwwFollows, previous) && gfmLiteralWwwRe.MatchString(rest) {
		if n, ok := gfmLiteralAddress(rest); ok {
			// GFM itself prefixes a www. autolink with http://, so the model
			// must too; nothing is fetched. The trailing NOSONAR suppresses
			// go:S5332 (cleartext protocol heuristic).
			return "http://" + rest[:n], n // NOSONAR
		}
	}
	if strings.ContainsRune(gfmLetters, previous) {
		return "", 0
	}
	scheme := gfmLiteralHTTPRe.FindString(rest)
	if scheme == "" {
		return "", 0
	}
	start := strings.Index(scheme, "//") + 2
	n, ok := gfmLiteralAddress(rest[start:])
	if !ok {
		return "", 0
	}
	return rest[:start+n], start + n
}

// gfmLiteralEmail returns the length of the email address s opens with, or
// zero when it opens none: the longest run [gfmLiteralEmailRe] reads is one
// when its domain holds a '.' and its last character is an ASCII letter. A
// '.' in that domain is always followed by a letter or a digit, so a domain
// with a dot has the character after it as well.
func gfmLiteralEmail(s string) int {
	address := gfmLiteralEmailRe.FindString(s)
	domain := address[strings.IndexByte(address, '@')+1:]
	if !strings.Contains(domain, ".") || !strings.ContainsRune(gfmLetters, rune(address[len(address)-1])) {
		return 0
	}
	return len(address)
}

// gfmLiteralAddress returns the length of the domain and the path s opens
// with, as a www. address and an http address after its "//" read them, and
// false when the domain is no domain.
func gfmLiteralAddress(s string) (int, bool) {
	n, ok := gfmLiteralDomain(s)
	if !ok {
		return 0, false
	}
	return n + gfmLiteralPath(s[n:]), true
}

// gfmLiteralDomain returns the length of the domain s opens with and whether
// it is one. It runs up to whitespace, or Unicode punctuation or a symbol
// other than '-', with U+0000 read as the U+FFFD it is to micromark, which is
// a symbol; a '.' or a '_' is part of it unless the trail begins there
// ([gfmTrailRe]). It is no domain when its last two segments, the text after
// its last dot and the text between its last two, hold an underscore. Its
// first character is never one that ends it, since each caller has read a
// "w" or the character after "//" already.
func gfmLiteralDomain(s string) (int, bool) {
	end, last, lastButOne := len(s), false, false
	for i, r := range s {
		if r == '.' || r == '_' {
			if gfmTrailRe.MatchString(s[i:]) {
				end = i
				break
			}
			if r == '.' {
				lastButOne = last
			}
			last = r == '_'
			continue
		}
		if strings.ContainsRune(gfmWhitespace, r) || r == 0 || r != '-' && unicode.In(r, unicode.P, unicode.S) {
			end = i
			break
		}
	}
	return end, !last && !lastButOne
}

// gfmLiteralPath returns the length of the path s opens with, after a
// domain: it runs up to whitespace, or up to a character of
// [gfmPathPunctuation] where the trail begins. A ")" that closes a "(" the
// path opened is part of it whatever follows, and one that closes none is
// part of it only when the trail does not begin there, and is counted as
// closed all the same, as micromark counts it.
func gfmLiteralPath(s string) int {
	open, closed := 0, 0
	for i, r := range s {
		if r == '(' {
			open++
			continue
		}
		if r == ')' && closed < open {
			closed++
			continue
		}
		if strings.ContainsRune(gfmPathPunctuation, r) {
			if gfmTrailRe.MatchString(s[i:]) {
				return i
			}
			if r == ')' {
				closed++
			}
			continue
		}
		if strings.ContainsRune(gfmWhitespace, r) {
			return i
		}
	}
	return len(s)
}

// codeSpan reads the backtick string at byte i: the span it opens, elided
// into [gfmCodeSpanPlaceholder], when a later string of the same length
// closes it, and the string as literal text when none does. The closing
// string is looked for among the line's runs as written, since nothing
// inside a span is anything but text: it is the first run of that length
// that starts after byte i, found by a binary search of that length's starts,
// so a line of many spans is not read again from its first run for each one.
func (w *gfmWalk) codeSpan(i int) int {
	rest := w.line[i:]
	n := len(rest) - len(strings.TrimLeft(rest, "`"))
	starts := w.runs[n]
	if k, _ := slices.BinarySearch(starts, i+1); k < len(starts) {
		w.spans++
		w.bare.WriteString(gfmCodeSpanPlaceholder)
		return starts[k] + n
	}
	w.bare.WriteString(rest[:n])
	return i + n
}

// angle reads what the "<" at byte i opens: an autolink, an email autolink or
// raw HTML, each recorded and consumed whole, and nothing when it opens none
// of them, which leaves the "<" text.
func (w *gfmWalk) angle(i int) (int, bool) {
	m := gfmAngleRe.FindStringSubmatch(w.line[i:])
	if m == nil {
		return 0, false
	}
	end := i + len(m[0])
	w.bare.WriteString(m[0])
	if m[1] != "" {
		w.links = append(w.links, gfmLink{at: i, dest: m[1]})
		return end, true
	}
	if m[2] != "" {
		w.links = append(w.links, gfmLink{at: i, dest: "mailto:" + m[2]})
		return end, true
	}
	w.html = append(w.html, m[0])
	return end, true
}

// open records a label opened at byte i, whose text begins at offset label of
// the bare text.
func (w *gfmWalk) open(i, label int, image bool) {
	w.openers = append(w.openers, gfmOpener{at: i, label: label, spans: w.spans, links: len(w.links), html: len(w.html), image: image})
}

// closeLabel reads the "]" at byte i. It closes the latest label still open,
// and a link or an image forms when that label is not only a bracket and a
// resource follows; otherwise the "]" is text and the label is gone either
// way, which is how a bracket pair that forms nothing stays text.
func (w *gfmWalk) closeLabel(i int) (int, bool) {
	if len(w.openers) == 0 {
		return 0, false
	}
	o := w.openers[len(w.openers)-1]
	w.openers = w.openers[:len(w.openers)-1]
	if o.inactive {
		return 0, false
	}
	dest, n, ok := gfmResource(w.line[i+1:])
	if !ok {
		return 0, false
	}
	label := w.bare.String()[o.label:]
	if o.image {
		w.links, w.html = w.links[:o.links], w.html[:o.html]
	} else {
		for k := range w.openers {
			if !w.openers[k].image {
				w.openers[k].inactive = true
			}
		}
	}
	w.links = append(w.links, gfmLink{at: o.at, dest: dest, self: w.selfLabel(o, label, dest)})
	end := i + 1 + n
	w.bare.WriteString(w.line[i:end])
	return end, true
}

// selfLabel reports whether the label o opened, read as label from the bare
// text, renders as the link's destination, the way the autolink of that
// destination renders: a label of plain text, holding no code span, autolink,
// image or raw HTML, whose characters are the destination's once
// [gfmResolve] has read both, and whose address an autolink can hold
// ([gfmAutolinkableRe]), since only then does the autolink write it once. An
// image's label is its alt text, which no autolink writes, and an empty
// destination is no address. [GFMDocument.SelfLinks] says what the
// comparison leaves out.
func (w *gfmWalk) selfLabel(o gfmOpener, label, dest string) bool {
	if o.image || dest == "" || w.spans > o.spans || len(w.links) > o.links || len(w.html) > o.html {
		return false
	}
	address := gfmResolve(dest)
	return gfmResolve(label) == address && gfmAutolinkableRe.MatchString(address)
}

// gfmResolve returns s as CommonMark renders the characters of text outside a
// code span, an autolink and raw HTML, and of a link destination: a backslash
// escape as the punctuation character it makes literal, a character reference
// as the character it stands for, a U+0000 byte as the U+FFFD the
// specification replaces it with, and every other byte as it is. It ranges
// over s and skips what an escape or a reference consumed, as [walkInline]
// does over a line.
func gfmResolve(s string) string {
	var text strings.Builder
	next := 0
	for i := range len(s) {
		if i < next {
			continue
		}
		if s[i] == 0 {
			text.WriteString("�")
			continue
		}
		if gfmEscapeRe.MatchString(s[i:]) {
			text.WriteByte(s[i+1])
			next = i + 2
			continue
		}
		if m := gfmReferenceRe.FindStringSubmatch(s[i:]); m != nil {
			text.WriteString(gfmReference(m))
			next = i + len(m[0])
			continue
		}
		text.WriteByte(s[i])
	}
	return text.String()
}

// gfmReference returns what a character reference renders as, given its
// match of gfmReferenceRe. A numeric one is the character it numbers, read as
// the specification reads it: U+0000, a surrogate and a number past U+10FFFF
// are U+FFFD, and every other code point is itself. A named one is the
// character, or the two, that HTML5 gives the name, and a name HTML5 does not
// define is no reference, so it is text as written. [html.UnescapeString]
// knows every name, and it also reads the legacy ones HTML5 accepts without a
// semicolon, such as "&amp" in "&ampx;", which CommonMark does not: the name
// is one exactly when the semicolon was part of what it decoded, so decoding
// the reference whole differs from decoding it without its semicolon and
// writing the semicolon after.
func gfmReference(m []string) string {
	if m[1] != "" {
		return gfmCodePoint(m[1], 10)
	}
	if m[2] != "" {
		return gfmCodePoint(m[2], 16)
	}
	decoded := html.UnescapeString(m[0])
	if decoded == html.UnescapeString(strings.TrimSuffix(m[0], ";"))+";" {
		return m[0]
	}
	return decoded
}

// gfmCodePoint returns the character a numeric reference's digits number in
// base: U+FFFD for zero, and for a number that is no Unicode scalar value,
// which converting it to a string already gives.
func gfmCodePoint(digits string, base int) string {
	code, _ := strconv.ParseInt(digits, base, 32)
	if code == 0 {
		return string(utf8.RuneError)
	}
	return string(rune(code))
}

// gfmResource reads the resource s opens with, "(destination "title")", and
// returns the destination as written and the resource's length, with false
// when s opens none.
func gfmResource(s string) (dest string, length int, ok bool) {
	open := gfmResourceOpenRe.FindString(s)
	if open == "" {
		return "", 0, false
	}
	dest, n, ok := gfmDestination(s[len(open):])
	if !ok {
		return "", 0, false
	}
	closing := gfmResourceCloseRe.FindString(s[len(open)+n:])
	if closing == "" {
		return "", 0, false
	}
	return dest, len(open) + n + len(closing), true
}

// gfmDestination reads the link destination s opens with: one between angle
// brackets, whose text is the destination, or one written bare, which may be
// empty when the resource ends at once. It returns the destination, the
// length it takes in s, and false when s opens none.
func gfmDestination(s string) (dest string, length int, ok bool) {
	if strings.HasPrefix(s, "<") {
		m := gfmEnclosedRe.FindStringSubmatch(s)
		if m == nil {
			return "", 0, false
		}
		return m[1], len(m[0]), true
	}
	n, ok := gfmBareDestination(s)
	return s[:n], n, ok
}

// gfmBareDestination returns the length of the bare destination s opens with,
// and false when it does not end: it runs up to a space or an ASCII control
// character other than U+0000, which [walkInline] reads as U+FFFD, or up to a
// ")" that closes no "(" inside it, and a parenthesis a backslash escapes is
// not one. One that still has a "(" open where it ends, or that reaches the
// end of s, which leaves the resource no ")", is no destination.
func gfmBareDestination(s string) (int, bool) {
	depth, escaped := 0, false
	for i := range len(s) {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if (c != 0 && c <= ' ') || c == 0x7f || (c == ')' && depth == 0) {
			return i, depth == 0
		}
		escaped = gfmEscapeRe.MatchString(s[i:])
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
		}
	}
	return 0, false
}

// backtickSpan is one maximal run of backticks: where it starts and how many
// backticks it holds.
type backtickSpan struct {
	start, length int
}

// backtickRuns indexes the maximal runs of backticks in line by length: for
// each length, where the runs of that length start, left to right, which is
// the order [gfmWalk.codeSpan] searches.
func backtickRuns(line string) map[int][]int {
	var runs []backtickSpan
	for i := range len(line) {
		if line[i] != '`' {
			continue
		}
		if i > 0 && line[i-1] == '`' {
			runs[len(runs)-1].length++
			continue
		}
		runs = append(runs, backtickSpan{start: i, length: 1})
	}
	starts := map[int][]int{}
	for _, run := range runs {
		starts[run.length] = append(starts[run.length], run.start)
	}
	return starts
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
		if escaped {
			segment.WriteRune(r)
			escaped = false
			continue
		}
		if r == '|' {
			segments = append(segments, segment.String())
			segment.Reset()
			continue
		}
		segment.WriteRune(r)
		escaped = r == '\\'
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
