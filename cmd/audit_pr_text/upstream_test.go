// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// upstream_test.go holds the upstream reference rule (issue 1477) to the
// spellings GitLab resolves across projects, and the description half to
// refusing them in the title and the body and naming each one.
package main

import (
	"slices"
	"strings"
	"testing"
)

// TestUpstreamReferences_EachSpellingGitLabResolves_IsFound verifies the
// spellings that make GitLab note a mention on another project's item: a
// project path, nested groups included, followed by !N or #N, and a full
// gitlab.com URL to a merge request or an issue; and the ones that do not: a
// bare #N, which names this repository's own items, a bare !N, a path with no
// number and a URL to something else.
func TestUpstreamReferences_EachSpellingGitLabResolves_IsFound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		line string
		want []string
	}{
		{name: "a merge request of a project", line: "Fixed by gitlab-org/gitlab!266080.", want: []string{"gitlab-org/gitlab!266080"}},
		{name: "an issue of a project", line: "See gitlab-org/gitlab#637548 for why.", want: []string{"gitlab-org/gitlab#637548"}},
		{name: "a nested group", line: "gitlab-org/api/client-go!3063 split it", want: []string{"gitlab-org/api/client-go!3063"}},
		{name: "a project path with dots and underscores", line: "(my.group/sub_group/my-project.go#12)", want: []string{"my.group/sub_group/my-project.go#12"}},
		{name: "inside a code span", line: "`gitlab-org/gitlab#1` is quoted", want: []string{"gitlab-org/gitlab#1"}},
		{
			name: "a merge request URL",
			line: "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_1",
			want: []string{"https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063"},
		},
		{name: "an issue URL over http", line: "<http://gitlab.com/gitlab-org/gitlab/-/issues/1>", want: []string{"http://gitlab.com/gitlab-org/gitlab/-/issues/1"}},
		{name: "a URL in any case", line: "HTTPS://GitLab.com/Group/Project/-/Issues/7", want: []string{"HTTPS://GitLab.com/Group/Project/-/Issues/7"}},
		{
			name: "two on one line, each once",
			line: "a/b!1 and c/d#2 and a/b!1 again",
			want: []string{"a/b!1", "c/d#2"},
		},
		{
			name: "two URLs on one line",
			line: "https://gitlab.com/a/b/-/issues/1 and https://gitlab.com/c/d/-/merge_requests/2",
			want: []string{"https://gitlab.com/a/b/-/issues/1", "https://gitlab.com/c/d/-/merge_requests/2"},
		},
		{
			name: "a path before a URL, in the line's order",
			line: "x/y#3 then https://gitlab.com/a/b/-/issues/4",
			want: []string{"x/y#3", "https://gitlab.com/a/b/-/issues/4"},
		},
		{name: "a number running into a word", line: "a/b#12abc"},
		{name: "a bare issue number", line: "Closes #1477, refs #1114."},
		{name: "a bare merge request number", line: "like !12 here"},
		{name: "a path with no number", line: "gitlab-org/gitlab is the project"},
		{name: "a file anchor", line: "docs/development/upstream-bugs.md#a-permission-refusal"},
		{name: "a line anchor", line: "cmd/server/main.go#L12"},
		{name: "a URL to a pipeline", line: "https://gitlab.com/gitlab-org/gitlab/-/pipelines/12"},
		{name: "a URL to another host", line: "https://example.com/gitlab-org/gitlab/-/issues/1"},
		{name: "a path inside a URL to another host", line: "https://github.com/jmrplens/gitlab-mcp-server/issues/12#issuecomment-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := upstreamReferences(tc.line); !slices.Equal(got, tc.want) {
				t.Errorf("upstreamReferences(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestDescription_AnUpstreamReference_FailsAndIsNamed verifies the rule in
// the gate: a reference in the title or in the body fails the run, its line
// is listed, and the failure names each reference and the register row to
// name instead.
func TestDescription_AnUpstreamReference_FailsAndIsNamed(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "Follow gitlab-org/api/client-go!3063", "Refs #1114\n\nSee https://gitlab.com/gitlab-org/gitlab/-/issues/1 and `gitlab-org/gitlab#2`.\n")
	if absent := lacking(output,
		"title: Follow gitlab-org/api/client-go!3063\n",
		"body:3:See https://gitlab.com/gitlab-org/gitlab/-/issues/1 and `gitlab-org/gitlab#2`.\n",
		"names an item of another project the way GitLab resolves",
		"    gitlab-org/api/client-go!3063\n    https://gitlab.com/gitlab-org/gitlab/-/issues/1\n    gitlab-org/gitlab#2\n",
		"docs/development/upstream-bugs.md",
		"Re-check with: make check-pr-description",
	); len(absent) > 0 {
		t.Errorf("output lacks %q:\n%s", absent, output)
	}
	if strings.Contains(output, "body:1:") {
		t.Errorf("a bare issue number was reported:\n%s", output)
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

// TestDescription_BareNumbers_PassWithEveryRuleNamed verifies that a
// description naming this repository's items by number passes, and that the
// passing line names the new rule among the ones it judged.
func TestDescription_BareNumbers_PassWithEveryRuleNamed(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "Port the gate (#1477)", "Closes #1477\nRefs #1114 and !12\n")
	if code != 0 || !strings.Contains(output, "OK: the title and body carry no em dash, no injected block, no skip command and no upstream reference.") {
		t.Errorf("exit %d, want 0 and the OK line\n%s", code, output)
	}
}
