package dependencies

import (
	"strings"
	"testing"
)

// injectedLine is the Markdown a component name would carry after a fence of
// its own, and the line the assertions below locate: outside the fenced block
// it is a heading of the server's response rather than part of the SBOM.
const injectedLine = "## Injected heading"

// TestFormatDownloadMarkdown_FenceOutlivesBacktickRunsInTheSBOM checks that the
// block wrapping a CycloneDX export is closed by nothing the export contains.
//
// The component names in an SBOM come out of the project's own dependency
// files, and JSON escaping leaves a backtick alone, so a fixed three-backtick
// fence is closed by the first run of three in the document. Each case carries
// a longer run than the last, and both halves are asserted: that the fence
// outgrew the longest run in the body, and that the injected heading still sits
// inside the fenced span. The second half is what makes the test worth having,
// since a fence of the right length would still prove nothing about output the
// content had already escaped from.
func TestFormatDownloadMarkdown_FenceOutlivesBacktickRunsInTheSBOM(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		longestRun int
	}{
		{
			name:       "no backtick run",
			content:    `{"bomFormat":"CycloneDX","components":[]}` + "\n" + injectedLine,
			longestRun: 0,
		},
		{
			name:       "a three-backtick line",
			content:    `{"components":[{"name":"` + "```" + `"}]}` + "\n```\n" + injectedLine,
			longestRun: 3,
		},
		{
			name:       "a five-backtick line",
			content:    `{"components":[{"name":"x"}]}` + "\n`````\n" + injectedLine,
			longestRun: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := FormatDownloadMarkdown(DownloadOutput{Content: tc.content})

			fence, bodyStart, bodyEnd := fencedSpan(t, rendered)
			if fence <= tc.longestRun {
				t.Errorf("fence is %d backtick(s) long, want more than the %d in the body:\n%s",
					fence, tc.longestRun, rendered)
			}
			injected := strings.Index(rendered, injectedLine)
			if injected < 0 {
				t.Fatalf("the injected line is not in the output at all:\n%s", rendered)
			}
			if injected < bodyStart || injected >= bodyEnd {
				t.Errorf("the injected line is at byte %d, outside the fenced span [%d, %d):\n%s",
					injected, bodyStart, bodyEnd, rendered)
			}
		})
	}
}

// fencedSpan locates the fenced code block in rendered Markdown: how long the
// fence that opened it is, and the byte range its body occupies.
//
// The opening fence is the first line beginning with a run of at least three
// backticks, which may carry an info string; the closing one is the first later
// line that is a run of at least that many and nothing else, which is what
// CommonMark closes a block on.
func fencedSpan(t *testing.T, rendered string) (fence, bodyStart, bodyEnd int) {
	t.Helper()
	offset := 0
	bodyStart, bodyEnd = -1, -1
	for _, line := range strings.SplitAfter(rendered, "\n") {
		text := strings.TrimSuffix(line, "\n")
		run := backtickRun(text)
		switch {
		case fence == 0 && run >= 3:
			fence = run
			bodyStart = offset + len(line)
		case fence > 0 && run >= fence && text == strings.Repeat("`", run):
			return fence, bodyStart, offset
		}
		offset += len(line)
	}
	t.Fatalf("no complete fenced block in the output:\n%s", rendered)
	return 0, 0, 0
}

// backtickRun counts the backticks a line opens with.
func backtickRun(line string) int {
	run := 0
	for run < len(line) && line[run] == '`' {
		run++
	}
	return run
}
