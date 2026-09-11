package toolutil

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Card is the one shape a tool result about a single GitLab object takes, and
// the only writer of its rows: the H2 heading the formatter composes, then one
// "- **Label**: value" item per field, then the object's long text, nested
// objects and nested collections, then the next-step hints last. A Markdown
// table is for a collection of objects that share columns and never for the
// fields of one object, so no method here writes a card row as a table row;
// the one table a Card writes is the nested collection [Card.Table] opens,
// under a heading of its own.
//
// The list shape is the one that survives whatever is written around it. A
// list item is a complete block wherever it lands, so a formatter can extend a
// card another formatter started, and a card can follow a quote, a fence or a
// table. A table row is a row only while nothing but rows has been written
// since its header, and anywhere else it is a line of literal pipes, which is
// how two cards in this tree came to print "| Status | merged |" as text while
// their tests passed on the substring.
//
// A Card streams into the caller's builder rather than buffering a document,
// so a formatter keeps the seams it has today: a fenced body, a rich-content
// note, a second formatter adding rows. What keeps that composable is the
// mark. The card records the builder's length after each of its own writes,
// and when anything else has been written since, the next row starts after a
// blank line, so it opens a list of its own instead of continuing a foreign
// block, and a table or a quote written in between ends where it should.
//
// Every value is escaped at the write that renders it, by the helper the
// value's shape needs, and a caller passes GitLab text as it arrived: the cell
// escaper for an inline value, [MdCodeSpan] for a code span, [MdTitleLink] for
// a link, [WrapGFMBody] for prose. The escaping is idempotent, so a caller
// that still escapes by hand renders unchanged. An empty, nil, blank or zero
// value writes nothing where the zero is an absence: the card shows what
// GitLab sent, and never a label with nothing after it.
type Card struct {
	b *strings.Builder
	// root is the card End writes the hints of and the one that remembers a
	// secret was shown; a card NewCard started is its own root.
	root *Card
	// parent is the card whose list this one continues: a Sub nests its rows
	// inside the parent's item, so a write here is a write of the parent's
	// block too. A root card and a Section have none.
	parent *Card
	// level is the heading level a Section of this card opens: 3 under the H2
	// a root card writes, one deeper per nesting, never past 6.
	level int
	// depth is the list nesting: rows are indented two spaces per level.
	depth int
	// mark is b.Len() after the last write this card or a Sub of it made, and
	// -1 before the first, so a first row on a builder the caller already
	// wrote to separates itself the same way a later one does.
	mark int
	// secrets are the labels Secret wrote, for the hint End adds.
	secrets []string
}

// CardTable is a nested collection inside a card: the table [Card.Table]
// opened, taking one row at a time.
type CardTable struct {
	c *Card
}

// NewCard starts a card in b. A non-empty heading is written as an H2 through
// [EscapeMdHeading]; the formatter composes it, and the whole composition is
// escaped, so it must not begin with '#'. An empty heading writes none, for a
// card that continues under a heading the caller wrote, or a card a shared
// renderer starts on behalf of the formatter that owns the heading.
func NewCard(b *strings.Builder, heading string) *Card {
	c := &Card{b: b, level: 3, mark: -1}
	c.root = c
	if !blank(heading) {
		c.separate()
		fmt.Fprintf(b, "## %s\n\n", EscapeMdHeading(heading))
		c.wrote()
	}
	return c
}

// Field writes "- **label**: value" with both halves through the cell escaper
// and the guidance heading defused. A blank value writes nothing, which is
// how every optional field is written: the card shows what GitLab sent.
func (c *Card) Field(label, value string) {
	c.row(label, cardInline(value))
}

// FieldOr writes value like [Card.Field], or absent in its place when the
// value is blank, for a field whose absence is the answer: "never" for an
// expiry, "unlimited" for a quota, "-" for a slot. A blank absent writes
// nothing.
func (c *Card) FieldOr(label, value, absent string) {
	if blank(value) {
		value = absent
	}
	c.row(label, cardInline(value))
}

// Int writes a number that is an answer at zero: an ID, or a count GitLab
// always sends.
func (c *Card) Int(label string, v int64) {
	c.row(label, strconv.FormatInt(v, 10))
}

// Count writes a number only when it is not zero, for a limit or a count
// whose zero means GitLab did not say.
func (c *Card) Count(label string, v int64) {
	if v == 0 {
		return
	}
	c.Int(label, v)
}

// Bool writes the flag as [BoolEmoji] renders it, a tick or a cross. It is
// for a flag whose true reads as a good thing; a negative one, such as
// revoked, locked or expired, is written by [Card.Warn].
func (c *Card) Bool(label string, v bool) {
	c.row(label, BoolEmoji(v))
}

// BoolPtr writes the flag when GitLab sent it and nothing for nil.
func (c *Card) BoolPtr(label string, v *bool) {
	if v == nil {
		return
	}
	c.Bool(label, *v)
}

// Flag writes "- <emoji> **label**" when on is true and nothing otherwise, for
// a condition worth stating only when it holds: confidential, archived, a
// draft. An empty emoji writes the label alone.
func (c *Card) Flag(emoji, label string, on bool) {
	if !on {
		return
	}
	c.separate()
	c.b.WriteString(c.indent())
	if emoji == "" {
		fmt.Fprintf(c.b, "- **%s**\n", cardInline(label))
	} else {
		fmt.Fprintf(c.b, "- %s **%s**\n", cardInline(emoji), cardInline(label))
	}
	c.wrote()
}

// Warn is the negative-polarity flag: revoked, locked, expired, has failures.
// It writes "- <warning> **label**" when on, so the condition is marked with a
// warning sign and never with the tick [BoolEmoji] gives a true, which on
// "Revoked" reads as success.
func (c *Card) Warn(label string, on bool) {
	c.Flag(EmojiWarning, label, on)
}

// Time writes an RFC 3339 timestamp in the display form [FormatTime] gives,
// or nothing when it is empty. A time.Time is passed as [RFC3339Ptr] of it.
func (c *Card) Time(label, rfc3339 string) {
	c.row(label, FormatTime(rfc3339))
}

// Link writes [MdTitleLink] of text and url: a link when url is set, the
// escaped text alone when it is not, and nothing when both are blank. A blank
// text with a url links the url itself, so a link never has an empty label.
func (c *Card) Link(label, text, url string) {
	if blank(text) {
		text = url
	}
	c.row(label, MdTitleLink(text, url))
}

// URL writes the "URL" row, the address linked to itself, or nothing when it
// is empty.
func (c *Card) URL(url string) {
	c.Link("URL", url, url)
}

// Code writes the value as a code span through [MdCodeSpan], for a value the
// reader has to copy exactly: a key, a path, a SHA, a command. Nothing inside
// the span is Markdown and no entity is written. A blank value writes nothing.
func (c *Card) Code(label, value string) {
	c.row(label, MdCodeSpan(value))
}

// Secret writes a value that is shown once, as [Card.Code] does, and records
// it so that [Card.End] adds the hint to store it, since a token or a secret
// GitLab returns on creation cannot be retrieved again. A blank value writes
// nothing and adds no hint.
func (c *Card) Secret(label, secret string) {
	span := MdCodeSpan(secret)
	if span == "" {
		return
	}
	c.row(label, span)
	c.root.noteSecret(label)
}

// Text writes prose a person typed into GitLab: a description, a note body, a
// commit message. A one-line body stays on the field's line, through the cell
// escaper with the guidance heading defused. A longer one becomes a blockquote
// under the label through [WrapGFMBody], indented so the quote belongs to the
// item, and nothing in it can add a field, a heading or a list item to the
// card. Trailing line breaks are dropped first, so a one-line body that ends
// in a newline stays inline. A blank body writes nothing.
func (c *Card) Text(label, body string) {
	body = strings.TrimRight(body, "\r\n")
	if blank(body) {
		return
	}
	if !strings.ContainsAny(body, "\r\n") {
		c.row(label, cardInline(body))
		return
	}
	c.separate()
	c.b.WriteString(c.indent())
	fmt.Fprintf(c.b, "- **%s**:\n", cardInline(label))
	quoteIndent := c.indent() + "  "
	for line := range strings.SplitSeq(WrapGFMBody(body), "\n") {
		c.b.WriteString(quoteIndent)
		c.b.WriteString(line)
		c.b.WriteString("\n")
	}
	c.wrote()
}

// Markdown writes a value the formatter composed from parts that are already
// escaped, such as a link built by [MdTitleLink] beside a status word, or a
// code span beside a count. It is the one row whose value is written as
// given, which is why the escaping gate judges the call site rather than this
// method: the write here is a plain concatenation with no hole of its own, and
// a value that reaches it raw is the formatter's finding, at the line that
// composed it.
func (c *Card) Markdown(label, composed string) {
	if blank(composed) {
		return
	}
	c.separate()
	c.b.WriteString(c.indent() + "- **" + cardInline(label) + "**: " + composed + "\n")
	c.wrote()
}

// Sub writes a label-only row and returns the card that writes the rows
// nested under it, indented two spaces further: the shape of a small nested
// object, an author or a milestone, whose fields belong beside the parent's.
// The nested rows continue the parent's list, so a parent row written after
// them follows directly.
func (c *Card) Sub(label string) *Card {
	c.separate()
	c.b.WriteString(c.indent())
	fmt.Fprintf(c.b, "- **%s**:\n", cardInline(label))
	c.wrote()
	return &Card{b: c.b, root: c.root, parent: c, level: c.level, depth: c.depth + 1, mark: c.b.Len()}
}

// Section writes a heading one level below the card's own, through
// [EscapeMdHeading], and returns the card that writes the section's rows: the
// shape of a large nested object, a pipeline inside a merge request, a commit
// inside a release. Its rows start at column zero under the heading, and a
// section of a section goes one level deeper, to H6 at most. A blank title
// opens no heading and the returned card continues the same list. The fields
// of the object itself are written before its sections, since a row written
// after one lands under the section's heading.
func (c *Card) Section(title string) *Card {
	if blank(title) {
		return &Card{b: c.b, root: c.root, parent: c, level: c.level, depth: c.depth, mark: c.b.Len()}
	}
	c.heading(title)
	return &Card{b: c.b, root: c.root, level: min(c.level+1, 6), mark: c.b.Len()}
}

// Table writes a nested collection's heading, one level below the card's own
// and omitted when the title is blank, then the table header for columns, and
// returns the table that takes the rows. The table is written at column zero
// whatever the card's depth, and the card's next row starts after a blank
// line, so the table ends where its rows end. A table with no columns writes
// only the heading.
func (c *Card) Table(title string, columns ...string) *CardTable {
	if blank(title) {
		c.endBlock()
	} else {
		c.heading(title)
	}
	if len(columns) > 0 {
		cells := make([]string, len(columns))
		for i, column := range columns {
			cells[i] = cardInline(column)
		}
		c.b.WriteString(markdownTableLine(cells))
		c.b.WriteString(markdownTableSeparator(len(cells)))
	}
	return &CardTable{c: c}
}

// Row writes one row of the collection. The cells are written as given, so
// the caller renders each one: [EscapeMdTableCell] for text, [MdTitleLink]
// for a link, [MdCodeSpanCell] for a code span, [FormatTime] for a timestamp.
// A link is a finished construct the cell escaper would turn back into text,
// which is why this is the one write in a card the caller escapes for, on the
// same terms as [MarkdownTableRow], and why the escaping gate judges the call
// site rather than this method. A row with no cells writes nothing.
func (t *CardTable) Row(cells ...string) {
	if len(cells) == 0 {
		return
	}
	t.c.b.WriteString(markdownTableLine(cells))
}

// Fence writes a body inside a fenced code block sized by
// [MarkdownFencedBlock], under a heading one level below the card's own when
// the title is not blank: a file, a template, a job log, a diff. An empty body
// writes nothing, heading included.
func (c *Card) Fence(title, lang, body string) {
	if body == "" {
		return
	}
	if blank(title) {
		c.endBlock()
	} else {
		c.heading(title)
	}
	c.b.WriteString(MarkdownFencedBlock(lang, body))
}

// Note writes a paragraph of the server's own prose after the rows and
// sections and before the hints: a rendering note, a confirmation sentence.
// It is one line, with control bytes dropped and line breaks collapsed, so it
// can add no structure; prose GitLab wrote belongs in [Card.Text], which quotes
// it. A blank note writes nothing.
func (c *Card) Note(prose string) {
	prose = strings.TrimSpace(codeSpanText(prose))
	if prose == "" {
		return
	}
	c.endBlock()
	c.b.WriteString(prose)
	c.b.WriteString("\n")
}

// End writes the next-step hints through [WriteHints] and is always the last
// write of a card: a row written after it lands below the guidance section,
// where [ExtractHints] no longer finds one. A secret the card showed adds its
// hint ahead of the caller's, with the label escaped like the row that showed
// it, since the hint is a list item too. It is called once, on the card
// NewCard started; on a nested card it writes the same section.
func (c *Card) End(hints ...string) {
	root := c.root
	all := make([]string, 0, len(root.secrets)+len(hints))
	for _, label := range root.secrets {
		all = append(all, "Store the "+strings.ToLower(EscapeMdTableCell(label))+" securely. It cannot be retrieved later")
	}
	all = append(all, hints...)
	WriteHints(c.b, all...)
}

// row writes one "- **label**: rendered" item, where rendered is the value
// already through the helper its shape needs. A blank rendering writes nothing.
func (c *Card) row(label, rendered string) {
	if blank(rendered) {
		return
	}
	c.separate()
	c.b.WriteString(c.indent())
	fmt.Fprintf(c.b, "- **%s**: %s\n", cardInline(label), rendered)
	c.wrote()
}

// heading writes a section heading at the card's section level, after a blank
// line: what follows it is a section's rows or a table's, never this card's
// list.
func (c *Card) heading(title string) {
	c.endBlock()
	fmt.Fprintf(c.b, "%s %s\n\n", strings.Repeat("#", c.level), EscapeMdHeading(title))
}

// separate ends whatever the builder holds that this card did not write, so
// the next row the card writes opens a list of its own. Nothing is written
// when the card's own last line is the builder's last line, which is how
// consecutive rows stay one list.
func (c *Card) separate() {
	if c.b.Len() == c.mark {
		return
	}
	c.endBlock()
}

// endBlock leaves the builder empty or ending in a blank line, whatever it
// ends with now, so a heading, a table, a fence or a note opens a block of its
// own rather than continuing the last line as a lazy paragraph or a row.
func (c *Card) endBlock() {
	endBlock(c.b)
}

// wrote records that the builder's last line is this card's, and its parents'
// where this card nests inside their item.
func (c *Card) wrote() {
	for x := c; x != nil; x = x.parent {
		x.mark = x.b.Len()
	}
}

// indent is the list indentation of this card's rows.
func (c *Card) indent() string {
	return strings.Repeat("  ", c.depth)
}

// noteSecret records a label Secret wrote, once.
func (c *Card) noteSecret(label string) {
	if slices.Contains(c.secrets, label) {
		return
	}
	c.secrets = append(c.secrets, label)
}

// cardInline renders a GitLab-authored value on a line the card wrote: the
// cell escaper collapses line breaks, drops control bytes and neutralizes the
// pipe, the tag and the link, and the server's guidance heading is defused so
// a value carrying it is shown as the text it is. It is the containment the
// prompts package applies to every inline value, moved here because a tool
// result annotated for the assistant is model-instruction payload on the same
// terms as a prompt message.
func cardInline(s string) string {
	return DefuseHintsHeading(EscapeMdTableCell(s))
}

// blank reports whether s holds nothing a reader would see.
func blank(s string) bool {
	return strings.TrimSpace(s) == ""
}
