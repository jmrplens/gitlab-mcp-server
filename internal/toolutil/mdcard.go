package toolutil

import (
	"strconv"
	"strings"
)

// A one-object card is a bulleted field list, and these helpers are the only
// way this repository writes one.
//
// The rule the helpers implement: a table row is an object, not a field. A
// Markdown table carries many objects, one row an object, under column
// headings that name the fields they hold; a card carries one object, one
// field a line. A "| Field | Value |" table is that shape with the information
// taken out of the header, and it was never a third thing — it was a card
// written in the other notation, which is how 27 packages ended up answering
// in one shape and 76 in the other, and four in both (issue #697).
//
// The helpers exist rather than a format constant because a constant is a
// decision a formatter still has to make. A formatter that reaches for
// WriteMdField cannot pick the other shape by accident, and the escaping of a
// value that lands in a list item is settled here, once, instead of in each of
// the packages that write a card — the same reason [WriteMdURL] exists.
//
// None of them formats through fmt. The line is assembled out of writes, so
// the escaping audit sees no template to read and no hole to judge inside
// toolutil: every one of these helpers either escapes its own value or takes a
// value a renderer in this package produced, and [WriteMdFieldRendered] is
// declared to that audit as a sink of its own, so its call sites are judged
// where they are written.

// mdCardFieldOpen and mdCardFieldMid bracket a card field's label.
const (
	mdCardFieldOpen = "- **"
	mdCardFieldMid  = "**: "
)

// WriteMdField appends one field of a card, escaping the value.
//
// [EscapeMdTableCell] is the escaper the escaping audit names for a list item
// as well as for a table cell: both contexts end at a line break, and both are
// ended early by text nobody on this side wrote. A label is escaped too. Every
// label in the tree today is a literal, but one of them is not — the storage
// move renderer labels a field with the entity GitLab named — and a helper
// that is safe for the label it was given is one less thing to remember.
func WriteMdField(b *strings.Builder, label, value string) {
	WriteMdFieldRendered(b, label, EscapeMdTableCell(value))
}

// WriteMdFieldIf appends the field only when the value is non-empty.
//
// It is the form most call sites want: a card that prints "- **Email**: " with
// nothing after it has spent a line telling the reader that GitLab returned
// nothing, which the absent line says as well and shorter.
func WriteMdFieldIf(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	WriteMdField(b, label, value)
}

// WriteMdFieldInt appends a field holding a number.
func WriteMdFieldInt(b *strings.Builder, label string, value int64) {
	WriteMdFieldRendered(b, label, strconv.FormatInt(value, 10))
}

// WriteMdFieldBool appends a field holding a flag, rendered by [BoolEmoji].
//
// A flag reaches a card as a tick or a cross and not as "true", which is the
// rule the tree already follows in 92 of its cards and forgets in 37 of them.
func WriteMdFieldBool(b *strings.Builder, label string, value bool) {
	WriteMdFieldRendered(b, label, BoolEmoji(value))
}

// WriteMdFieldTime appends a field holding a timestamp, rendered by
// [FormatTime], and writes nothing when the timestamp is empty.
func WriteMdFieldTime(b *strings.Builder, label, timestamp string) {
	if timestamp == "" {
		return
	}
	WriteMdFieldRendered(b, label, FormatTime(timestamp))
}

// WriteMdFieldCode appends a field whose value is rendered as a code span, and
// writes nothing when the value is empty.
//
// The value is not entity-escaped, because a code span does not decode
// entities: "&#124;" inside one renders as those six characters rather than as
// the pipe it stands for, so the escaping a table cell needs would corrupt
// exactly the values — a cron expression, a branch pattern, a regex — that are
// written as code in the first place. What the value cannot do instead is end
// its own span: the fence is sized to the longest backtick run inside it, the
// way [MarkdownCodeFence] sizes a block's.
func WriteMdFieldCode(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	WriteMdFieldRendered(b, label, MdInlineCode(value))
}

// WriteMdFieldLink appends a field whose value is a link, rendered by
// [MdTitleLink], which escapes both halves.
func WriteMdFieldLink(b *strings.Builder, label, text, url string) {
	WriteMdFieldRendered(b, label, MdTitleLink(text, url))
}

// WriteMdFieldRendered appends a field whose value is already rendered: a link
// from [MdTitleLink], a tick from [BoolEmoji], a span from [MdInlineCode], a
// number, or a value a formatter composed out of those.
//
// It is the one helper here that escapes nothing, and it is declared as a sink
// in cmd/audit_md_escaping so that a value reaching it raw is reported against
// the package that wrote the call rather than against this line — which is
// also what keeps a //gitlab:allow-unescaped directive working for a card
// field holding an enum GitLab picks from a fixed set.
func WriteMdFieldRendered(b *strings.Builder, label, rendered string) {
	b.WriteString(mdCardFieldOpen)
	b.WriteString(EscapeMdTableCell(label))
	b.WriteString(mdCardFieldMid)
	b.WriteString(rendered)
	b.WriteByte('\n')
}

// MdInlineCode renders a value as a Markdown code span whose fence is long
// enough to contain it: one backtick, or one more than the longest run inside
// the value, with the padding space CommonMark requires when the value itself
// starts or ends with a backtick.
//
// Line breaks collapse to spaces, because a code span is an inline construct
// and a value carrying a newline would end the list item it sits in.
func MdInlineCode(value string) string {
	value = StripControlBytes(value)
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")

	// The fence is sized by [longestBacktickRun], the helper EscapeConsentValue
	// already sizes its own fence with, so both places follow the one rule
	// Markdown itself uses for nesting code.
	fence := strings.Repeat("`", longestBacktickRun(value)+1)
	pad := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		pad = " "
	}
	return fence + pad + value + pad + fence
}
