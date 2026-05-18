package docgen

import (
	"strings"
	"testing"
)

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
