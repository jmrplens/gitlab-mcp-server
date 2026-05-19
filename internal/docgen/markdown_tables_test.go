package docgen

import (
	"strings"
	"testing"
)

func TestFormatMarkdownTables_FormatsPipeTables(t *testing.T) {
	input := strings.Join([]string{
		"Before",
		"",
		"| Name | Count | Status |",
		"| --- | ---: | :---: |",
		"| short | 1 | ok |",
		"| much longer | 20 | review |",
		"",
		"After",
		"",
	}, "\n")
	want := strings.Join([]string{
		"Before",
		"",
		"| Name        | Count | Status |",
		"| ----------- | ----: | :----: |",
		"| short       |     1 |   ok   |",
		"| much longer |    20 | review |",
		"",
		"After",
		"",
	}, "\n")

	got, changed := FormatMarkdownTables(input)
	if !changed {
		t.Fatal("FormatMarkdownTables changed = false, want true")
	}
	if got != want {
		t.Fatalf("FormatMarkdownTables() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMarkdownTables_SkipsFencedCodeBlocks(t *testing.T) {
	input := strings.Join([]string{
		"```md",
		"| A | B |",
		"| --- | --- |",
		"| unformatted | value |",
		"```",
		"",
	}, "\n")

	got, changed := FormatMarkdownTables(input)
	if changed {
		t.Fatal("FormatMarkdownTables changed fenced code block")
	}
	if got != input {
		t.Fatalf("FormatMarkdownTables() =\n%s\nwant\n%s", got, input)
	}
}

func TestFormatMarkdownTables_PreservesEscapedAndCodePipes(t *testing.T) {
	input := strings.Join([]string{
		"| Pattern | Meaning |",
		"| --- | --- |",
		"| `a|b` | escaped \\| pipe |",
		"",
	}, "\n")
	want := strings.Join([]string{
		"| Pattern | Meaning         |",
		"| ------- | --------------- |",
		"| `a|b`   | escaped \\| pipe |",
		"",
	}, "\n")

	got, changed := FormatMarkdownTables(input)
	if !changed {
		t.Fatal("FormatMarkdownTables changed = false, want true")
	}
	if got != want {
		t.Fatalf("FormatMarkdownTables() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMarkdownTables_PreservesEOFWithoutNewline(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| one | two |"
	want := "| A   | B   |\n| --- | --- |\n| one | two |"

	got, changed := FormatMarkdownTables(input)
	if !changed {
		t.Fatal("FormatMarkdownTables changed = false, want true")
	}
	if got != want {
		t.Fatalf("FormatMarkdownTables() = %q, want %q", got, want)
	}
}

func TestFormatMarkdownTables_IdempotentForFormattedTable(t *testing.T) {
	input := RenderMarkdownTable(
		[]string{"Name", "Count"},
		[]Alignment{AlignLeft, AlignRight},
		[][]string{{"alpha", "10"}},
	)

	got, changed := FormatMarkdownTables(input)
	if changed {
		t.Fatal("FormatMarkdownTables changed already formatted table")
	}
	if got != input {
		t.Fatalf("FormatMarkdownTables() =\n%s\nwant\n%s", got, input)
	}
}
