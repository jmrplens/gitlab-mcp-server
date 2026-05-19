package docgen

import (
	"strings"
	"unicode/utf8"
)

// Alignment controls how a Markdown table column is padded and marked.
type Alignment int

const (
	// AlignLeft pads cells on the right and uses the default Markdown separator.
	AlignLeft Alignment = iota
	// AlignRight pads cells on the left and uses a right-aligned separator.
	AlignRight
	// AlignCenter pads cells on both sides and uses a centered separator.
	AlignCenter
)

// RenderMarkdownTable returns a Markdown table padded to the widest cell in
// each column so the generated source remains readable as plain text.
func RenderMarkdownTable(headers []string, alignments []Alignment, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = max(utf8.RuneCountInString(header), 3)
	}
	for _, row := range rows {
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	for i := range widths {
		if alignmentAt(alignments, i) == AlignRight {
			widths[i] = max(widths[i], 4)
		}
		if alignmentAt(alignments, i) == AlignCenter {
			widths[i] = max(widths[i], 5)
		}
	}

	var b strings.Builder
	writeMarkdownTableRow(&b, headers, alignments, widths)
	writeMarkdownTableSeparator(&b, alignments, widths)
	for _, row := range rows {
		cells := make([]string, len(headers))
		copy(cells, row)
		writeMarkdownTableRow(&b, cells, alignments, widths)
	}
	return b.String()
}

func writeMarkdownTableRow(b *strings.Builder, cells []string, alignments []Alignment, widths []int) {
	b.WriteByte('|')
	for i, width := range widths {
		b.WriteByte(' ')
		b.WriteString(padMarkdownTableCell(cells[i], alignmentAt(alignments, i), width))
		b.WriteString(" |")
	}
	b.WriteByte('\n')
}

func writeMarkdownTableSeparator(b *strings.Builder, alignments []Alignment, widths []int) {
	b.WriteByte('|')
	for i, width := range widths {
		b.WriteByte(' ')
		b.WriteString(markdownTableSeparator(alignmentAt(alignments, i), width))
		b.WriteString(" |")
	}
	b.WriteByte('\n')
}

func padMarkdownTableCell(cell string, alignment Alignment, width int) string {
	padding := width - utf8.RuneCountInString(cell)
	if padding <= 0 {
		return cell
	}
	spaces := strings.Repeat(" ", padding)
	if alignment == AlignRight {
		return spaces + cell
	}
	if alignment == AlignCenter {
		left := padding / 2
		right := padding - left
		return strings.Repeat(" ", left) + cell + strings.Repeat(" ", right)
	}
	return cell + spaces
}

func markdownTableSeparator(alignment Alignment, width int) string {
	width = max(width, 3)
	if alignment == AlignRight {
		return strings.Repeat("-", width-1) + ":"
	}
	if alignment == AlignCenter {
		return ":" + strings.Repeat("-", width-2) + ":"
	}
	return strings.Repeat("-", width)
}

func alignmentAt(alignments []Alignment, index int) Alignment {
	if index < len(alignments) {
		return alignments[index]
	}
	return AlignLeft
}
