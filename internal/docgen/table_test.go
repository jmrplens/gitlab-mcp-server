package docgen

import (
	"strings"
	"testing"
)

// TestRenderMarkdownTable_AlignsPlainTextColumns verifies left and right
// alignment produce padded Markdown columns for plain ASCII content.
//
// The test renders a package coverage table with one long package name and
// expects right-aligned coverage values. This protects generated documentation
// tables from drifting into hard-to-read raw pipe output.
func TestRenderMarkdownTable_AlignsPlainTextColumns(t *testing.T) {
	got := RenderMarkdownTable(
		[]string{"Package", "Coverage"},
		[]Alignment{AlignLeft, AlignRight},
		[][]string{
			{"cmd/a", "9.0%"},
			{"internal/longer/package", "100.0%"},
		},
	)
	want := strings.Join([]string{
		"| Package                 | Coverage |",
		"| ----------------------- | -------: |",
		"| cmd/a                   |     9.0% |",
		"| internal/longer/package |   100.0% |",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderMarkdownTable() =\n%s\nwant\n%s", got, want)
	}
}

// TestRenderMarkdownTable_DefaultsMissingCellsAndAlignments verifies omitted
// cells and alignments use empty values and left alignment.
//
// The rendered table has two headers but a single row value. The expected output
// pads the missing cell rather than dropping the column, preserving rectangular
// Markdown tables for generated docs.
func TestRenderMarkdownTable_DefaultsMissingCellsAndAlignments(t *testing.T) {
	got := RenderMarkdownTable(
		[]string{"A", "B"},
		nil,
		[][]string{{"value"}},
	)
	want := strings.Join([]string{
		"| A     | B   |",
		"| ----- | --- |",
		"| value |     |",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderMarkdownTable() =\n%s\nwant\n%s", got, want)
	}
}

// TestRenderMarkdownTable_EmptyHeaders_ReturnsEmptyString verifies a table with
// no headers renders as no Markdown content.
//
// Rows without headers cannot form a valid pipe table, so the expected result is
// an empty string instead of malformed output.
func TestRenderMarkdownTable_EmptyHeaders_ReturnsEmptyString(t *testing.T) {
	got := RenderMarkdownTable(nil, nil, [][]string{{"ignored"}})
	if got != "" {
		t.Fatalf("RenderMarkdownTable() = %q, want empty string", got)
	}
}

// TestRenderMarkdownTable_UnicodeContent_UsesRuneWidths verifies Unicode text is
// padded by rune width rather than byte length.
//
// The test renders accented words and expects aligned columns, ensuring generated
// docs remain readable when labels contain non-ASCII characters.
func TestRenderMarkdownTable_UnicodeContent_UsesRuneWidths(t *testing.T) {
	got := RenderMarkdownTable(
		[]string{"Word", "Meaning"},
		[]Alignment{AlignLeft, AlignLeft},
		[][]string{
			{"café", "accented"},
			{"niño", "child"},
		},
	)
	want := strings.Join([]string{
		"| Word | Meaning  |",
		"| ---- | -------- |",
		"| café | accented |",
		"| niño | child    |",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderMarkdownTable() =\n%s\nwant\n%s", got, want)
	}
}

// TestRenderMarkdownTable_WideContent_ExpandsColumns verifies cell content wider
// than the header determines the rendered column width.
//
// The single row contains a long value, and the expected table expands the Value
// column to fit it without truncation.
func TestRenderMarkdownTable_WideContent_ExpandsColumns(t *testing.T) {
	got := RenderMarkdownTable(
		[]string{"Key", "Value"},
		[]Alignment{AlignLeft, AlignLeft},
		[][]string{{"short", "this value is intentionally wider than the header"}},
	)
	want := strings.Join([]string{
		"| Key   | Value                                             |",
		"| ----- | ------------------------------------------------- |",
		"| short | this value is intentionally wider than the header |",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderMarkdownTable() =\n%s\nwant\n%s", got, want)
	}
}

// TestRenderMarkdownTable_SingleColumn_RendersTable verifies one-column input
// still produces a valid Markdown table.
//
// The expected output includes header, separator, and both rows, covering the
// smallest valid table shape used by generated documentation.
func TestRenderMarkdownTable_SingleColumn_RendersTable(t *testing.T) {
	got := RenderMarkdownTable(
		[]string{"Only"},
		[]Alignment{AlignLeft},
		[][]string{{"one"}, {"three"}},
	)
	want := strings.Join([]string{
		"| Only  |",
		"| ----- |",
		"| one   |",
		"| three |",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderMarkdownTable() =\n%s\nwant\n%s", got, want)
	}
}
