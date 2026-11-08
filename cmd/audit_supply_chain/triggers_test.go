// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// TestCheckTriggers_Workflows_ReportsWhatRule9Refuses runs rule 9 over small
// workflows, one per case. The first case is the regression the rule exists
// for: auto-assign.yml triggered by pull_request_target while its condition
// still named pull_request, which skips the job on every pull request and
// reports success. Each case lists the fragments the problems must carry, one
// per problem, and an empty list means the workflow passes.
func TestCheckTriggers_Workflows_ReportsWhatRule9Refuses(t *testing.T) {
	cases := []struct {
		name     string
		workflow string
		want     []string
	}{
		{
			name: "condition naming an event the workflow is not triggered by",
			workflow: `
on:
  pull_request_target:
    types: [opened]
jobs:
  assign:
    if: github.event_name == 'pull_request'
    steps:
      - run: gh api --method POST /repos/o/r/issues/1/assignees
`,
			want: []string{"job assign tests github.event_name == 'pull_request'"},
		},
		{
			name: "condition naming the trigger",
			workflow: `
on:
  issues:
    types: [opened]
  pull_request_target:
    types: [opened]
jobs:
  assign:
    if: >-
      (github.event_name == 'issues' && github.event.issue.user.login != 'x') ||
      (github.event_name == "pull_request_target" && github.event.pull_request.user.login != 'x')
    steps:
      - run: gh api --method POST /repos/o/r/issues/1/assignees
`,
		},
		{
			name: "step condition naming an event the list form leaves out",
			workflow: `
on: [push, pull_request]
jobs:
  build:
    steps:
      - if: github.event_name == 'schedule'
        run: make all
`,
			want: []string{"job build step 1 tests github.event_name == 'schedule'"},
		},
		{
			name: "single event spelled as a string",
			workflow: `
on: push
jobs:
  build:
    if: github.event_name == 'push'
    steps:
      - run: make all
`,
		},
		{
			name: "checkout under pull_request_target",
			workflow: `
on:
  pull_request_target:
jobs:
  label:
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/labeler@bf12e9b00b37c5c0ca2b87b79b2daf7891dbda13 # v7
`,
			want: []string{"job label step 1 checks out code in a workflow triggered by pull_request_target"},
		},
		{
			name: "checkout under pull_request",
			workflow: `
on:
  pull_request:
jobs:
  build:
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, _, err := parseWorkflow(tc.workflow)
			if err != nil {
				t.Fatalf("parseWorkflow() error = %v", err)
			}
			problems := checkTriggers("w.yml", doc)
			if len(problems) != len(tc.want) {
				t.Fatalf("checkTriggers() = %q, want %d problems carrying %q", problems, len(tc.want), tc.want)
			}
			for i, fragment := range tc.want {
				if !strings.Contains(problems[i], fragment) {
					t.Errorf("checkTriggers()[%d] = %q, want it to carry %q", i, problems[i], fragment)
				}
			}
		})
	}
}

// TestCheckTriggers_RepositoryWorkflows_Pass holds this repository's own
// workflows to rule 9, so auto-assign.yml and labeler.yml, the two triggered
// by pull_request_target, keep naming that event and checking out nothing.
func TestCheckTriggers_RepositoryWorkflows_Pass(t *testing.T) {
	workflows, err := loadWorkflows(repositoryRoot(t))
	if err != nil {
		t.Fatalf("loadWorkflows() error = %v", err)
	}
	for _, file := range workflows {
		if file.doc == nil {
			continue
		}
		if problems := checkTriggers(file.path, file.doc); len(problems) != 0 {
			t.Errorf("checkTriggers(%s) = %q, want none", file.path, problems)
		}
	}
}
