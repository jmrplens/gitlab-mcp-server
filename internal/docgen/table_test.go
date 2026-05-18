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

func TestRenderMarkdownTable_EmptyHeaders_ReturnsEmptyString(t *testing.T) {
	got := RenderMarkdownTable(nil, nil, [][]string{{"ignored"}})
	if got != "" {
		t.Fatalf("RenderMarkdownTable() = %q, want empty string", got)
	}
}

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
