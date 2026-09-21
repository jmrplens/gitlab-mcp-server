package docgen

import (
	"slices"
	"strings"
	"testing"
)

// formatTableCase is one Markdown document put to FormatMarkdownTables, with
// the text it has to produce and whether it has to report a change.
type formatTableCase struct {
	name        string
	input       string
	want        string
	wantChanged bool
}

// runFormatTableCases drives each case as its own subtest, asserting the
// rendered document and the reported change together: a formatter that
// produces the right text and lies about having touched it leaves a generator
// believing its artifact is fresh.
func runFormatTableCases(t *testing.T, tests []formatTableCase) {
	t.Helper()
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

	runFormatTableCases(t, []formatTableCase{
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
	})
}

// TestFormatMarkdownTables_EdgeCases_TableDriven verifies the document shapes
// the table above happens to avoid, each of which decides a branch every other
// fixture here settles the same way: a row that ends in something other than a
// pipe, a pipe or a backtick the author escaped, a code span that runs to the
// very end of a line, a separator that is not one, an explicit left alignment,
// a second table after the first, and a CRLF document whose last line carries
// no line ending at all.
func TestFormatMarkdownTables_EdgeCases_TableDriven(t *testing.T) {
	runFormatTableCases(t, []formatTableCase{
		{
			name: "row without a trailing pipe keeps its last cell",
			input: strings.Join([]string{
				"| Name | Note |",
				"| --- | --- |",
				"| alpha | ends without a pipe",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Name  | Note                |",
				"| ----- | ------------------- |",
				"| alpha | ends without a pipe |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "row ending with an escaped pipe keeps it",
			input: strings.Join([]string{
				"| Pattern | Meaning |",
				"| --- | --- |",
				"| trailing | a \\|",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Pattern  | Meaning |",
				"| -------- | ------- |",
				"| trailing | a \\|    |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "code span closes at the end of a row",
			input: strings.Join([]string{
				"| Meaning | Sample |",
				"| --- | --- |",
				"| note | ``a``|",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Meaning | Sample |",
				"| ------- | ------ |",
				"| note    | ``a``  |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "double backtick span holds a single backtick",
			input: strings.Join([]string{
				"| Span | Note |",
				"| --- | --- |",
				"| ``a`b`` | note |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Span    | Note |",
				"| ------- | ---- |",
				"| ``a`b`` | note |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "skips tilde fenced code blocks",
			input: strings.Join([]string{
				"~~~md",
				"| A | B |",
				"| --- | --- |",
				"| unformatted | value |",
				"~~~",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"~~~md",
				"| A | B |",
				"| --- | --- |",
				"| unformatted | value |",
				"~~~",
				"",
			}, "\n"),
		},
		{
			name: "escaped backtick opens no code span",
			input: strings.Join([]string{
				"| Escape | Note |",
				"| --- | --- |",
				"| a \\` | b |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Escape | Note |",
				"| ------ | ---- |",
				"| a \\`   | b    |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "leaves a separator line without a pipe unchanged",
			input: strings.Join([]string{
				"| A | B |",
				"---",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A | B |",
				"---",
				"| one | two |",
				"",
			}, "\n"),
		},
		{
			name: "normalizes an explicitly left aligned separator",
			input: strings.Join([]string{
				"| A | B |",
				"| :--- | --- |",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A   | B   |",
				"| --- | --- |",
				"| one | two |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "a paragraph whose only pipe is escaped is not a row",
			input: strings.Join([]string{
				"A sentence with an escaped \\| pipe.",
				"| A | B |",
				"| --- | --- |",
				"| one | two |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"A sentence with an escaped \\| pipe.",
				"| A   | B   |",
				"| --- | --- |",
				"| one | two |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "cell beginning with an escaped pipe",
			input: strings.Join([]string{
				"| Cell | Note |",
				"| --- | --- |",
				"|\\|a | b |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Cell | Note |",
				"| ---- | ---- |",
				"| \\|a  | b    |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "escaped backslash leaves the next pipe a delimiter",
			input: strings.Join([]string{
				"| Cell | Note |",
				"| --- | --- |",
				"| a\\\\| b |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| Cell | Note |",
				"| ---- | ---- |",
				"| a\\\\  | b    |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name: "two tables in one document",
			input: strings.Join([]string{
				"| A | B |",
				"| --- | --- |",
				"| one | two |",
				"",
				"| C | D |",
				"| --- | --- |",
				"| three | four |",
				"",
			}, "\n"),
			want: strings.Join([]string{
				"| A   | B   |",
				"| --- | --- |",
				"| one | two |",
				"",
				"| C     | D    |",
				"| ----- | ---- |",
				"| three | four |",
				"",
			}, "\n"),
			wantChanged: true,
		},
		{
			name:        "crlf table without a final newline",
			input:       "| A | B |\r\n| --- | ---: |\r\n| one | 2 |",
			want:        "| A   |    B |\r\n| --- | ---: |\r\n| one |    2 |",
			wantChanged: true,
		},
	})
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

// TestNormalizeMarkdownTableRow_RaggedRows_MatchTheColumnCount verifies a row
// handed on to the renderer carries exactly one cell per column: a short row
// is padded with empty cells and a long one is cut.
//
// It is asserted here rather than through the formatter because the renderer
// sizes its own cell slice from the header, so a row that kept its own width
// would come out looking the same; the invariant this states is what makes
// reading rows by column index safe anywhere else.
func TestNormalizeMarkdownTableRow_RaggedRows_MatchTheColumnCount(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns int
		want    []string
	}{
		{name: "short row is padded", row: []string{"one"}, columns: 3, want: []string{"one", "", ""}},
		{name: "long row is cut", row: []string{"a", "b", "c", "d"}, columns: 2, want: []string{"a", "b"}},
		{name: "exact row is kept", row: []string{"a", "b"}, columns: 2, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMarkdownTableRow(tt.row, tt.columns); !slices.Equal(got, tt.want) {
				t.Errorf("normalizeMarkdownTableRow(%q, %d) = %q, want %q", tt.row, tt.columns, got, tt.want)
			}
		})
	}
}
