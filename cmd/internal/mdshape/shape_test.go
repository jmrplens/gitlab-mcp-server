// shape_test.go covers the block classifier: the shapes it names, the defects
// it reports, and the responses it must leave alone.
package mdshape

import "testing"

// TestLintShapes verifies what Lint names each block of a response.
func TestLintShapes(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []Shape
	}{
		{
			name: "a card is a card",
			md:   "## Runner #7\n\n- **Name**: runner\n- **Status**: online\n",
			want: []Shape{ShapeOther, ShapeCard},
		},
		{
			name: "a table is a table",
			md:   "## Runners (2)\n\n| ID | Name |\n| --- | --- |\n| 1 | a |\n| 2 | b |\n",
			want: []Shape{ShapeOther, ShapeTable},
		},
		{
			name: "a summary line above a header does not stop the table opening",
			md:   "Showing 2 of 40 results\n| ID | Name |\n| --- | --- |\n| 1 | a |\n",
			want: []Shape{ShapeTable},
		},
		{
			name: "an aligned delimiter is a delimiter",
			md:   "| ID | Name |\n|---:|:-----|\n| 1 | a |\n",
			want: []Shape{ShapeTable},
		},
		{
			name: "a hint list is neither",
			md:   "---\n\U0001F4A1 **Next steps:**\n- Use action 'get'\n",
			want: []Shape{ShapeOther},
		},
		{
			name: "pipe rows inside a fenced block are not this response's shape",
			md:   "## Job log\n\n```\n| not | a | table |\n```\n",
			want: []Shape{ShapeOther},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Lint(tt.md)
			if len(report.Findings) != 0 {
				t.Fatalf("Lint reported %d finding(s), want none: %v", len(report.Findings), report.Findings)
			}
			if len(report.Shapes) != len(tt.want) {
				t.Fatalf("Lint named %v, want %v", report.Shapes, tt.want)
			}
			for i, shape := range report.Shapes {
				if shape != tt.want[i] {
					t.Errorf("block %d is %q, want %q", i, shape, tt.want[i])
				}
			}
		})
	}
}

// TestLintFindings verifies the defects Lint reports, one per shape of defect.
func TestLintFindings(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want Kind
		line int
	}{
		{
			name: "a card field inside a table ends the table at that line",
			md:   "| Property | Value |\n| --- | --- |\n- **ID**: 42\n| IID | 7 |\n",
			want: KindFieldInTable,
			line: 3,
		},
		{
			name: "pipe rows appended to a card render as neither",
			md:   "- **Token**: glrt-a\n| Runners Token | glrt-a |\n",
			want: KindMixed,
			line: 2,
		},
		{
			name: "pipe rows no delimiter row turns into a table",
			md:   "| Added | 3 |\n| Removed | 1 |\n",
			want: KindHeaderless,
			line: 1,
		},
		{
			name: "a bold-prefix field is a third shape",
			md:   "**Name:** hook\n",
			want: KindBoldPrefix,
			line: 1,
		},
		{
			name: "prose below a table ends it",
			md:   "| ID | Name |\n| --- | --- |\n| 1 | a |\nTotal: 1\n",
			want: KindMixed,
			line: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Lint(tt.md)
			if len(report.Findings) != 1 {
				t.Fatalf("Lint reported %d finding(s), want 1: %v", len(report.Findings), report.Findings)
			}
			got := report.Findings[0]
			if got.Kind != tt.want {
				t.Errorf("Lint reported %q, want %q", got.Kind, tt.want)
			}
			if got.Line != tt.line {
				t.Errorf("Lint pointed at line %d, want %d", got.Line, tt.line)
			}
			if got.String() == "" {
				t.Error("a finding renders as an empty string")
			}
		})
	}
}

// TestReportCounts verifies the census a report carries.
func TestReportCounts(t *testing.T) {
	report := Lint("- **ID**: 1\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n- **ID**: 2\n")
	if got := report.Cards(); got != 2 {
		t.Errorf("Cards() = %d, want 2", got)
	}
	if got := report.Tables(); got != 1 {
		t.Errorf("Tables() = %d, want 1", got)
	}
}
