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
// spellings that make GitLab note a mention on another project's or group's
// item: a project path, nested groups included and an absolute one too,
// followed by !N or #N; a single segment followed by #N, which GitLab reads
// as a top-level group's work item; a group path followed by &N for an epic;
// the bracketed [issue:], [work_item:] and [epic:] forms with a path; and a
// full gitlab.com URL to a merge request, an issue, an incident, a work item,
// an epic or a commit, with or without the /-/ and at a group's address. It
// also verifies the ones that do not: a bare #N, which names this
// repository's own items, a bare !N, a project of this repository's namespace
// named alone before !N, a path with no number, an ampersand that names no
// epic, a commit pinned as owner/repo@sha, and a URL to something else or to
// another host, the way a GitHub issue passes.
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
		{name: "an absolute project path", line: "Fixed by /gitlab-org/gitlab!266080.", want: []string{"/gitlab-org/gitlab!266080"}},
		{name: "an absolute path opening the line", line: "/gitlab-org/gitlab#1 is it", want: []string{"/gitlab-org/gitlab#1"}},
		{name: "another forge's issue spelled the same way", line: "explained by golang/go#81238", want: []string{"golang/go#81238"}},
		{name: "a work item of a top-level group", line: "Part of the epic gitlab-org#19972, as a work item.", want: []string{"gitlab-org#19972"}},
		{name: "a work item of an absolute group path", line: "see /gitlab-org#19972", want: []string{"/gitlab-org#19972"}},
		{name: "a project of this namespace before #N, a group's too", line: "sibling-project#12 opening the line", want: []string{"sibling-project#12"}},
		{name: "another forge's repository named alone", line: "(go-sdk#1266)", want: []string{"go-sdk#1266"}},
		{name: "an epic of a group", line: "Part of gitlab-org&19972.", want: []string{"gitlab-org&19972"}},
		{name: "an epic of a subgroup", line: "(gitlab-org/plan-stage&4)", want: []string{"gitlab-org/plan-stage&4"}},
		{name: "inside a code span", line: "`gitlab-org/gitlab#1` is quoted", want: []string{"gitlab-org/gitlab#1"}},
		{name: "a bracketed issue", line: "see [issue:gitlab-org/gitlab/637548]", want: []string{"[issue:gitlab-org/gitlab/637548]"}},
		{name: "a bracketed work item of a group", line: "[work_item:gitlab-org/12] next", want: []string{"[work_item:gitlab-org/12]"}},
		{name: "a bracketed epic", line: "[epic:/gitlab-org/19972]", want: []string{"[epic:/gitlab-org/19972]"}},
		{
			name: "a merge request URL",
			line: "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_1",
			want: []string{"https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063"},
		},
		{name: "an issue URL over http", line: "<http://gitlab.com/gitlab-org/gitlab/-/issues/1>", want: []string{"http://gitlab.com/gitlab-org/gitlab/-/issues/1"}},
		{name: "a URL in any case", line: "HTTPS://GitLab.com/Group/Project/-/Issues/7", want: []string{"HTTPS://GitLab.com/Group/Project/-/Issues/7"}},
		{
			name: "a merge request URL without the dash",
			line: "https://gitlab.com/gitlab-org/gitlab/merge_requests/1",
			want: []string{"https://gitlab.com/gitlab-org/gitlab/merge_requests/1"},
		},
		{
			name: "an incident URL",
			line: "https://gitlab.com/gitlab-com/gl-infra/production/-/issues/incident/5",
			want: []string{"https://gitlab.com/gitlab-com/gl-infra/production/-/issues/incident/5"},
		},
		{
			name: "a work item URL of a project",
			line: "https://gitlab.com/gitlab-org/gitlab/-/work_items/2",
			want: []string{"https://gitlab.com/gitlab-org/gitlab/-/work_items/2"},
		},
		{
			name: "a work item URL of a group",
			line: "https://gitlab.com/groups/gitlab-org/-/work_items/3",
			want: []string{"https://gitlab.com/groups/gitlab-org/-/work_items/3"},
		},
		{
			name: "an epic URL",
			line: "https://gitlab.com/groups/gitlab-org/-/epics/19972",
			want: []string{"https://gitlab.com/groups/gitlab-org/-/epics/19972"},
		},
		{
			name: "a commit URL",
			line: "https://gitlab.com/gitlab-org/api/client-go/-/commit/0123abc4567def",
			want: []string{"https://gitlab.com/gitlab-org/api/client-go/-/commit/0123abc4567def"},
		},
		{
			name: "a commit URL with its full SHA, without the dash and with a view",
			line: "<https://gitlab.com/gitlab-org/gitlab/commit/FA7117DB83C52D787AE5A89F350999701505D78E?view=parallel>",
			want: []string{"https://gitlab.com/gitlab-org/gitlab/commit/FA7117DB83C52D787AE5A89F350999701505D78E"},
		},
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
		{
			name: "every spelling on one line, in the line's order",
			line: "[epic:g/1] g&2 https://gitlab.com/g/p/-/issues/3 g/p!4",
			want: []string{"[epic:g/1]", "g&2", "https://gitlab.com/g/p/-/issues/3", "g/p!4"},
		},
		{name: "a number running into a word", line: "a/b#12abc"},
		{name: "a bare issue number", line: "Closes #1477, refs #1114."},
		{name: "a bare merge request number", line: "like !12 here"},
		{name: "a project of this namespace before !N", line: "other!3 and other-project!4"},
		{name: "a GitHub action pinned to a commit", line: "uses: ossf/scorecard-action@fa7117db83c52d787ae5a89f350999701505d78e"},
		{name: "a bracketed issue of this project", line: "[issue:12]"},
		{name: "a bracketed issue of this namespace", line: "[issue:sibling-project/12]"},
		{name: "a bare epic number", line: "&12 alone"},
		{name: "a redirect in a shell line", line: "go test ./... 2>&1 | tee log"},
		{name: "an ampersand between words", line: "R&D and Q&A"},
		{name: "a path with no number", line: "gitlab-org/gitlab is the project"},
		{name: "a file anchor", line: "docs/development/upstream-bugs.md#a-permission-refusal"},
		{name: "a line anchor", line: "cmd/server/main.go#L12"},
		{name: "a URL to a pipeline", line: "https://gitlab.com/gitlab-org/gitlab/-/pipelines/12"},
		{name: "a URL to a list of merge requests", line: "https://gitlab.com/gitlab-org/gitlab/-/merge_requests"},
		{name: "a URL to a list of commits", line: "https://gitlab.com/gitlab-org/gitlab/-/commits/main"},
		{name: "a commit URL with a SHA too short to be one", line: "https://gitlab.com/gitlab-org/gitlab/-/commit/0123ab"},
		{name: "a commit URL to another host", line: "https://github.com/golang/go/commit/0123abc4567def"},
		{name: "a URL to another host", line: "https://example.com/gitlab-org/gitlab/-/issues/1"},
		{name: "a path inside a URL to another host", line: "https://github.com/jmrplens/gitlab-mcp-server/issues/12#issuecomment-1"},
		{name: "a GitHub issue URL", line: "explained by https://github.com/golang/go/issues/81238"},
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
// is listed, and the failure names each reference, the register row to name
// instead, how to write an item of another forge so that it passes, and how a
// Dependabot pull request's description is cut and its run repeated.
func TestDescription_AnUpstreamReference_FailsAndIsNamed(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "Follow gitlab-org/api/client-go!3063", "Refs #1114\n\nSee https://gitlab.com/gitlab-org/gitlab/-/issues/1 and `gitlab-org/gitlab#2`.\n")
	if absent := lacking(output,
		"title: Follow gitlab-org/api/client-go!3063\n",
		"body:3:See https://gitlab.com/gitlab-org/gitlab/-/issues/1 and `gitlab-org/gitlab#2`.\n",
		"names an item of another project or group the way GitLab resolves",
		"    gitlab-org/api/client-go!3063\n    https://gitlab.com/gitlab-org/gitlab/-/issues/1\n    gitlab-org/gitlab#2\n",
		"docs/development/upstream-bugs.md",
		"such as a GitHub issue as\nowner/repo#N or repo#N, is refused too",
		"write it as its github.com URL, which passes, or name it in words",
		"A Dependabot pull request fails here when its description copies a\ndependency's release notes, changelog or commit list",
		"Keep what Dependabot writes outside its <details>\nblocks, delete the blocks",
		"editing a description starts no run",
		"    gh pr edit <number> --body-file <file>\n    gh run rerun <run id> --failed\n",
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
