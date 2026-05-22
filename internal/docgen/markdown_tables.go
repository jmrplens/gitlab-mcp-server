package docgen

import (
	"strings"
)

type markdownLine struct {
	Raw  string
	Text string
	EOL  string
}

// FormatMarkdownTables normalizes GitHub-flavored Markdown pipe tables in content.
// Tables inside fenced code blocks are left unchanged.
func FormatMarkdownTables(content string) (string, bool) {
	lines := splitMarkdownLines(content)
	if len(lines) == 0 {
		return content, false
	}

	var b strings.Builder
	b.Grow(len(content))
	changed := false
	inFence := false

	for i := 0; i < len(lines); {
		line := lines[i]
		if isMarkdownFence(line.Text) {
			inFence = !inFence
			b.WriteString(line.Raw)
			i++
			continue
		}

		if !inFence && i+1 < len(lines) {
			rendered, end, tableChanged, ok := formatMarkdownTableAt(lines, i)
			if ok {
				changed = changed || tableChanged
				b.WriteString(rendered)
				i = end
				continue
			}
		}

		b.WriteString(line.Raw)
		i++
	}

	if !changed {
		return content, false
	}
	return b.String(), true
}

func formatMarkdownTableAt(lines []markdownLine, start int) (rendered string, end int, changed, ok bool) {
	headers, headerOK := parseMarkdownTableRow(lines[start].Text)
	alignments, separatorOK := parseMarkdownTableSeparator(lines[start+1].Text, len(headers))
	if !headerOK || !separatorOK {
		return "", start, false, false
	}
	rows, end := collectMarkdownTableRows(lines, start+2, len(headers))
	rendered = RenderMarkdownTable(headers, alignments, rows)
	rendered = applyMarkdownTableLineEnding(rendered, lines[start:end])
	original := joinMarkdownLines(lines[start:end])
	return rendered, end, rendered != original, true
}

func collectMarkdownTableRows(lines []markdownLine, start, width int) (rows [][]string, end int) {
	rows = make([][]string, 0)
	end = start
	for end < len(lines) {
		if strings.TrimSpace(lines[end].Text) == "" {
			break
		}
		row, ok := parseMarkdownTableRow(lines[end].Text)
		if !ok {
			break
		}
		rows = append(rows, normalizeMarkdownTableRow(row, width))
		end++
	}
	return rows, end
}

func splitMarkdownLines(content string) []markdownLine {
	if content == "" {
		return nil
	}
	parts := strings.SplitAfter(content, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}

	lines := make([]markdownLine, 0, len(parts))
	for _, part := range parts {
		line := markdownLine{Raw: part, Text: part}
		textNoLF, okLF := strings.CutSuffix(line.Text, "\n")
		if okLF {
			line.Text = textNoLF
			line.EOL = "\n"
			textNoCR, okCR := strings.CutSuffix(line.Text, "\r")
			if okCR {
				line.Text = textNoCR
				line.EOL = "\r\n"
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func isMarkdownFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func parseMarkdownTableRow(line string) ([]string, bool) {
	if !hasUnescapedPipe(line) {
		return nil, false
	}

	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	if strings.HasSuffix(trimmed, "|") && !isEscapedAt(trimmed, len(trimmed)-1) {
		trimmed = trimmed[:len(trimmed)-1]
	}

	cells := splitMarkdownTableCells(trimmed)
	return cells, true
}

func splitMarkdownTableCells(line string) []string {
	cells := make([]string, 0, strings.Count(line, "|")+1)
	var cell strings.Builder
	inCode := false
	codeDelimiterLen := 0
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '`' && !isEscapedAt(line, i) {
			run := 1
			for j := i + 1; j < len(line) && line[j] == '`'; j++ {
				run++
			}
			if !inCode {
				inCode = true
				codeDelimiterLen = run
			} else if run == codeDelimiterLen {
				inCode = false
				codeDelimiterLen = 0
			}
			cell.WriteString(line[i : i+run])
			i += run - 1
			continue
		}
		if ch == '|' && !inCode && !isEscapedAt(line, i) {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
			continue
		}
		cell.WriteByte(ch)
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	return cells
}

func parseMarkdownTableSeparator(line string, columns int) ([]Alignment, bool) {
	if columns == 0 {
		return nil, false
	}
	cells, ok := parseMarkdownTableRow(line)
	if !ok || len(cells) != columns {
		return nil, false
	}

	alignments := make([]Alignment, len(cells))
	for i, cell := range cells {
		compact := strings.ReplaceAll(strings.TrimSpace(cell), " ", "")
		if compact == "" {
			return nil, false
		}
		leftColon := strings.HasPrefix(compact, ":")
		rightColon := strings.HasSuffix(compact, ":")
		marker := strings.Trim(compact, ":")
		if len(marker) < 3 || strings.Trim(marker, "-") != "" {
			return nil, false
		}
		switch {
		case leftColon && rightColon:
			alignments[i] = AlignCenter
		case rightColon:
			alignments[i] = AlignRight
		default:
			alignments[i] = AlignLeft
		}
	}
	return alignments, true
}

func normalizeMarkdownTableRow(row []string, columns int) []string {
	if len(row) == columns {
		return row
	}
	out := make([]string, columns)
	copy(out, row)
	return out
}

func hasUnescapedPipe(line string) bool {
	for i := range len(line) {
		if line[i] == '|' && !isEscapedAt(line, i) {
			return true
		}
	}
	return false
}

func isEscapedAt(s string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func applyMarkdownTableLineEnding(rendered string, original []markdownLine) string {
	if len(original) == 0 {
		return rendered
	}
	eol := original[0].EOL
	if eol == "" {
		eol = "\n"
	}
	if eol != "\n" {
		rendered = strings.ReplaceAll(rendered, "\n", eol)
	}
	if original[len(original)-1].EOL == "" {
		rendered = strings.TrimSuffix(rendered, eol)
	}
	return rendered
}

func joinMarkdownLines(lines []markdownLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.Raw)
	}
	return b.String()
}
