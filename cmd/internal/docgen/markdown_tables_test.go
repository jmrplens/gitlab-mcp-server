package docgen

import (
	"strings"
	"testing"
)

// TestFormatMarkdownTables_TableDriven verifies Markdown table normalization
// across formatting, skip, line-ending, and malformed-input scenarios.
//
// The table covers ordinary pipe tables, fenced code blocks, escaped and code
// pipes, missing trailing newlines, CRLF input, ragged rows, table termination,
// idempotent formatted content, empty content, and invalid separators. Each
// case asserts both rendered output and whether a change was reported.
func TestFormatMarkdownTables_TableDriven(t *testing.T) {
	formattedTable := RenderMarkdownTable(
		[]string{"Name", "Count"},
		[]Alignment{AlignLeft, AlignRight},
		[][]string{{"alpha", "10"}},
	)

	tests := []struct {
		name        string
		input       string
		want        string
		wantChanged bool
	}{
		{
			name: "formats pipe tables",
			input: strings.Join([]string{
				"Before",
				"",
				"| Name | Count | Status |",
				"| --- | ---: | :---: |",
				"| short | 1 | ok |",
				"| much longer | 20 | review |",
				"",
				"After",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"Before",
				"",
				"| Name        | Count | Status |",
				"| ----------- | ----: | :----: |",
				"| short       |     1 |   ok   |",
				"| much longer |    20 | review |",
				"",
				"After",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "skips fenced code blocks",
			input: strings.Join([]string{
				"```md",
				"| A | B |",
				"| --- | --- |",
				"| unformatted | value |",
				"```",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"```md",
				"| A | B |",
				"| --- | --- |",
				"| unformatted | value |",
				"```",
				"",
			}, "\n"),
		},
		{
			name: "preserves escaped and code pipes",
			input: strings.Join([]string{
				"| Pattern | Meaning |",
				"| --- | --- |",
				"| `a|b` | escaped \\| pipe |",
				"| ``a|b`` | code span |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Pattern | Meaning         |",
				"| ------- | --------------- |",
				"| `a|b`   | escaped \\| pipe |",
				"| ``a|b`` | code span       |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name:        "preserves eof without newline",
			input:       "| A | B |\n| --- | --- |\n| one | two |",
			want:        "| A   | B   |\n| --- | --- |\n| one | two |",
			wantChanged: true,
		},
		{
			name: "preserves crlf line endings",
			input: strings.Join([]string{
				"| A | B |\r",
				"| --- | ---: |\r",
				"| one | 2 |\r",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A   |    B |\r",
				"| --- | ---: |\r",
				"| one |    2 |\r",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "normalizes ragged rows",
			input: strings.Join([]string{
				"| A | B | C |",
				"| --- | --- | --- |",
				"| one | two |",
				"| extra | values | ignored | more |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A     | B      | C       |",
				"| ----- | ------ | ------- |",
				"| one   | two    |         |",
				"| extra | values | ignored |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "stops table at non table content",
			input: strings.Join([]string{
				"| A | B |",
				"| --- | --- |",
				"| one | two |",
				"not a table",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A   | B   |",
				"| --- | --- |",
				"| one | two |",
				"not a table",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name:        "idempotent for formatted table",
			input:       formattedTable,
			want:        formattedTable,
			wantChanged: false,
		},
		{
			name:        "empty content",
			input:       "",
			want:        "",
			wantChanged: false,
		},
		{
			name: "leaves invalid short separator unchanged",
			input: strings.Join([]string{
				"| A | B |",
				"| -- | --- |",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A | B |",
				"| -- | --- |",
				"| one | two |",
				"",
			}, "\n"),
		},
		{
			name: "leaves empty separator cell unchanged",
			input: strings.Join([]string{
				"| A | B |",
				"|  | --- |",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A | B |",
				"|  | --- |",
				"| one | two |",
				"",
			}, "\n"),
		},
		{
			name: "leaves separator column mismatch unchanged",
			input: strings.Join([]string{
				"| A | B |",
				"| --- |",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A | B |",
				"| --- |",
				"| one | two |",
				"",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := FormatMarkdownTables(tt.input)
			if changed != tt.wantChanged {
				t.Fatalf("FormatMarkdownTables changed = %v, want %v", changed, tt.wantChanged)
			}
			if got != tt.want {
				t.Fatalf("FormatMarkdownTables() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// TestMarkdownTableHelpers_EdgeCases verifies low-level table helper behavior
// for invalid separators and line-ending preservation.
//
// The test rejects zero-column separators and checks that rendered trailing
// newlines are kept or trimmed based on the source table metadata, preserving the
// formatter's exact-file behavior.
func TestMarkdownTableHelpers_EdgeCases(t *testing.T) {
	if _, ok := parseMarkdownTableSeparator("| --- |", 0); ok {
		t.Fatal("parseMarkdownTableSeparator accepted zero columns")
	}
	if got := applyMarkdownTableLineEnding("rendered\n", nil); got != "rendered\n" {
		t.Fatalf("applyMarkdownTableLineEnding() = %q, want rendered newline", got)
	}
	if got := applyMarkdownTableLineEnding("rendered\n", []markdownLine{{Raw: "raw", Text: "raw"}}); got != "rendered" {
		t.Fatalf("applyMarkdownTableLineEnding() = %q, want trimmed newline", got)
	}
}
