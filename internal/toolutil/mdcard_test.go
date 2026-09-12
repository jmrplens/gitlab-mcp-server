// mdcard_test.go covers the one-object card helpers: the shape they write, the
// escaping each of them applies, and the fields they leave out.
package toolutil

import (
	"strings"
	"testing"
)

// TestWriteMdField verifies the shape and the escaping of a card field.
func TestWriteMdField(t *testing.T) {
	tests := []struct {
		name  string
		label string
		value string
		want  string
	}{
		{
			name:  "plain value",
			label: "Name",
			value: "runner-01",
			want:  "- **Name**: runner-01\n",
		},
		{
			name:  "a pipe in the value is escaped",
			label: "Name",
			value: "a|b",
			want:  "- **Name**: a&#124;b\n",
		},
		{
			name:  "a newline in the value collapses, so the item cannot end early",
			label: "Description",
			value: "first\nsecond",
			want:  "- **Description**: first second\n",
		},
		{
			name:  "a pipe in the label is escaped too",
			label: "a|b",
			value: "x",
			want:  "- **a&#124;b**: x\n",
		},
		{
			name:  "an empty value still writes the field",
			label: "Email",
			value: "",
			want:  "- **Email**: \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteMdField(&b, tt.label, tt.value)
			if got := b.String(); got != tt.want {
				t.Errorf("WriteMdField(%q, %q) = %q, want %q", tt.label, tt.value, got, tt.want)
			}
		})
	}
}

// TestWriteMdFieldIf verifies that an empty value writes no line at all.
func TestWriteMdFieldIf(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "a value is written", value: "x", want: "- **Email**: x\n"},
		{name: "an empty value is skipped", value: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteMdFieldIf(&b, "Email", tt.value)
			if got := b.String(); got != tt.want {
				t.Errorf("WriteMdFieldIf(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestWriteMdFieldTypedHelpers verifies the helpers that render their own
// value: a number, a flag, a timestamp, a link.
func TestWriteMdFieldTypedHelpers(t *testing.T) {
	tests := []struct {
		name  string
		write func(b *strings.Builder)
		want  string
	}{
		{
			name:  "an integer",
			write: func(b *strings.Builder) { WriteMdFieldInt(b, "ID", 42) },
			want:  "- **ID**: 42\n",
		},
		{
			name:  "a true flag is a tick",
			write: func(b *strings.Builder) { WriteMdFieldBool(b, "Paused", true) },
			want:  "- **Paused**: " + EmojiSuccess + "\n",
		},
		{
			name:  "a false flag is a cross",
			write: func(b *strings.Builder) { WriteMdFieldBool(b, "Paused", false) },
			want:  "- **Paused**: " + EmojiCross + "\n",
		},
		{
			name:  "a timestamp is formatted",
			write: func(b *strings.Builder) { WriteMdFieldTime(b, "Created", "2026-03-14T09:30:00Z") },
			want:  "- **Created**: 14 Mar 2026 09:30 UTC\n",
		},
		{
			name:  "an empty timestamp writes nothing",
			write: func(b *strings.Builder) { WriteMdFieldTime(b, "Created", "") },
			want:  "",
		},
		{
			name:  "a link",
			write: func(b *strings.Builder) { WriteMdFieldLink(b, "Pipeline", "#7", "https://gitlab.example.com/p/7") },
			want:  "- **Pipeline**: [#7](https://gitlab.example.com/p/7)\n",
		},
		{
			name:  "a code span",
			write: func(b *strings.Builder) { WriteMdFieldCode(b, "Cron", "0 4 * * *") },
			want:  "- **Cron**: `0 4 * * *`\n",
		},
		{
			name:  "an empty code value writes nothing",
			write: func(b *strings.Builder) { WriteMdFieldCode(b, "Cron", "") },
			want:  "",
		},
		{
			name:  "a rendered value is written as it arrived",
			write: func(b *strings.Builder) { WriteMdFieldRendered(b, "Token", "glrt-a|b") },
			want:  "- **Token**: glrt-a|b\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			tt.write(&b)
			if got := b.String(); got != tt.want {
				t.Errorf("card field = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMdInlineCode verifies that a value cannot end the span it is written in,
// and that the entity escaping a table cell needs is not applied to one.
func TestMdInlineCode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain", value: "main", want: "`main`"},
		{name: "a pipe survives, because a span decodes no entity", value: "a|b", want: "`a|b`"},
		{name: "a backtick inside is contained by a longer fence", value: "a`b", want: "``a`b``"},
		{name: "a run of two is contained by three", value: "a``b", want: "```a``b```"},
		{name: "a leading backtick is padded", value: "`x", want: "`` `x ``"},
		{name: "a newline collapses", value: "a\nb", want: "`a b`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MdInlineCode(tt.value); got != tt.want {
				t.Errorf("MdInlineCode(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
