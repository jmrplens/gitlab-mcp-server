// markdown_test.go asserts that the merge request formatters contain the text
// GitLab hands them, rather than letting it reshape the document around it.
//
// The rendering of ordinary values is covered by merge_requests_test.go; what
// is here is the hostile half, and it asserts structure rather than comparing
// against a golden string, so it keeps failing for the right reason after a
// column is added or a label is reworded.
package mergerequests

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// hostileBranch is a branch name a person can create and push.
//
// It is not a contrivance: git check-ref-format forbids a space, a control
// byte and the characters "~^:?*[" and backslash, and says nothing about the
// pipe or the angle brackets, so GitLab accepts all three through the same
// check. This server's own branch.create writes one.
const hostileBranch = "feat|x<img src=q onerror=alert(1)>"

// hostileTitle is what a person types into a merge request title, an issue
// title or a label, none of which GitLab constrains to a character set. The
// newline is the half that ends a table row.
const hostileTitle = "Fix login | urgent\nsecond line"

// assertTableRowsWellFormed fails when a row of a Markdown table carries a
// different number of cells than the header above it, which is exactly what an
// unescaped pipe or newline in a cell produces.
func assertTableRowsWellFormed(t *testing.T, rendered string) {
	t.Helper()
	want := -1
	for line := range strings.SplitSeq(rendered, "\n") {
		if !strings.HasPrefix(line, "|") {
			want = -1
			continue
		}
		got := strings.Count(line, "|")
		switch {
		case want < 0:
			want = got
		case got != want:
			t.Errorf("table row %q has %d pipes, want %d\n---\n%s", line, got, want, rendered)
		}
	}
}

// assertContained fails when text a person wrote reached the page able to
// change the document around it.
//
// A raw '<' is the one that matters most: a renderer takes raw HTML ahead of
// whatever surrounds it, so a title carrying an anchor tag arrives at a client
// as a working link to a host that is not GitLab.
func assertContained(t *testing.T, rendered string) {
	t.Helper()
	if strings.Contains(rendered, "<img") {
		t.Errorf("rendered Markdown carries a raw tag\n---\n%s", rendered)
	}
	if !strings.Contains(rendered, "&lt;img") {
		t.Errorf("rendered Markdown does not entity-encode the angle bracket\n---\n%s", rendered)
	}
	if !strings.Contains(rendered, "&#124;") {
		t.Errorf("rendered Markdown does not encode the pipe\n---\n%s", rendered)
	}
}

// hostileOutput is a merge request whose every free-text field carries the
// characters the escapers exist for.
func hostileOutput() Output {
	return Output{
		IID: 1, Title: hostileTitle, State: testStateOpened,
		SourceBranch: hostileBranch, TargetBranch: hostileBranch,
		Author:    &toolutil.BasicUserOutput{Username: hostileBranch},
		Assignees: []*toolutil.BasicUserOutput{{Username: hostileBranch}},
		Reviewers: []*toolutil.BasicUserOutput{{Username: hostileBranch}},
		Labels:    []string{hostileTitle},
		Milestone: &toolutil.MRMilestoneOutput{Title: hostileTitle},
		WebURL:    "https://gitlab.example.com/g/p/-/merge_requests/1",
	}
}

// TestFormatMarkdown_HostileBranchAndTitle_StayContained pins the detail view.
//
// It is a list of single-line values rather than a table, so the newline is
// what breaks it: every value below has to arrive on the line the formatter
// wrote for it, with no tag opened along the way.
func TestFormatMarkdown_HostileBranchAndTitle_StayContained(t *testing.T) {
	md := FormatMarkdown(hostileOutput())
	assertContained(t, md)
	for _, label := range []string{"**Source**", "**Assignees**", "**Reviewers**", "**Milestone**", "**Labels**"} {
		t.Run(label, func(t *testing.T) {
			line := lineWith(md, label)
			if line == "" {
				t.Fatalf("no %s line in\n---\n%s", label, md)
			}
			if strings.Contains(line, "\n") {
				t.Errorf("%s line %q was split across lines", label, line)
			}
		})
	}
}

// TestFormatListMarkdown_HostileBranchAndTitle_StayInOneCell pins the tables.
//
// A branch name and a title are cells here, so a pipe in either would add a
// column to its row and shift every heading after it.
func TestFormatListMarkdown_HostileBranchAndTitle_StayInOneCell(t *testing.T) {
	mr := hostileOutput()
	cases := []struct {
		name     string
		rendered string
	}{
		{name: "ListOutput", rendered: FormatListMarkdown(ListOutput{MergeRequests: []Output{mr}})},
		{
			name: "IssuesClosedOutput",
			rendered: FormatIssuesClosedMarkdown(IssuesClosedOutput{Issues: []issues.Output{{
				IID: 2, Title: hostileTitle, State: "opened", Labels: []string{hostileTitle},
			}}}),
		},
		{
			name: "DependenciesOutput",
			rendered: FormatDependenciesMarkdown(DependenciesOutput{Dependencies: []DependencyOutput{{
				ID: 3, BlockingMergeRequest: &BlockingMergeRequestOutput{IID: 4, Title: hostileTitle, State: "opened"},
			}}}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertTableRowsWellFormed(t, tc.rendered)
		})
	}
}

// TestFormatDependencyMarkdown_HostileBranch_StaysContained pins the blocking
// merge request detail, whose branches were rendered raw beside an escaped
// title.
func TestFormatDependencyMarkdown_HostileBranch_StaysContained(t *testing.T) {
	md := FormatDependencyMarkdown(DependencyOutput{
		ID: 3,
		BlockingMergeRequest: &BlockingMergeRequestOutput{
			IID: 4, Title: hostileTitle, State: "opened",
			SourceBranch: hostileBranch, TargetBranch: hostileBranch,
		},
	})
	assertContained(t, md)
}

// lineWith returns the single rendered line carrying label, which is how a
// test asserts that a value stayed on the line written for it rather than
// continuing onto lines of its own.
func lineWith(rendered, label string) string {
	for line := range strings.SplitSeq(rendered, "\n") {
		if strings.Contains(line, label) {
			return line
		}
	}
	return ""
}
