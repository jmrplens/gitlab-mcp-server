package repositorysubmodules

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// injectedLine is the Markdown a pusher would put after their own fence, and
// the line the assertions below locate: if it renders outside the fenced block,
// it is a heading of the server's response rather than a line of a file.
const injectedLine = "## Injected heading"

// TestFormatReadMarkdown_FenceOutlivesBacktickRunsInTheContent checks that the
// block wrapping a submodule's file is closed by nothing the file contains.
//
// The file is written by whoever can push to the submodule's project, so a
// fixed three-backtick fence is closed by the first run of three in the body
// and everything after it renders as Markdown of this response. Each case
// carries a longer run than the last, and both halves are asserted: that the
// fence outgrew the longest run in the body, and that the injected heading
// still sits inside the fenced span. The second half is what makes the test
// worth having, since a fence of the right length would still prove nothing
// about output the content had already escaped from.
func TestFormatReadMarkdown_FenceOutlivesBacktickRunsInTheContent(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		longestRun int
	}{
		{name: "no backtick run", content: "int main() {}\n" + injectedLine, longestRun: 0},
		{name: "a three-backtick line", content: "```\n" + injectedLine, longestRun: 3},
		{name: "a five-backtick line", content: "`````\n" + injectedLine, longestRun: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatReadMarkdown(ReadOutput{
				FileName:        "main.c",
				FilePath:        "src/main.c",
				SubmodulePath:   "libs/core",
				ResolvedProject: "org/project",
				CommitSHA:       "abc123def456789",
				Size:            int64(len(tc.content)),
				Content:         tc.content,
			})

			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("first content is %T, want text", result.Content[0])
			}
			assertFenceContains(t, text.Text, tc.longestRun)
		})
	}
}

// assertFenceContains checks that the rendered Markdown holds one fenced block,
// that its fence is longer than the longest backtick run of the body, and that
// the injected line is inside it.
func assertFenceContains(t *testing.T, rendered string, longestRun int) {
	t.Helper()
	fence, bodyStart, bodyEnd := fencedSpan(t, rendered)
	if fence <= longestRun {
		t.Errorf("fence is %d backtick(s) long, want more than the %d in the body:\n%s", fence, longestRun, rendered)
	}
	injected := strings.Index(rendered, injectedLine)
	if injected < 0 {
		t.Fatalf("the injected line is not in the output at all:\n%s", rendered)
	}
	if injected < bodyStart || injected >= bodyEnd {
		t.Errorf("the injected line is at byte %d, outside the fenced span [%d, %d):\n%s",
			injected, bodyStart, bodyEnd, rendered)
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
