// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// meta_withheld_test.go drives the answer a meta tool gives a call naming an
// action its group withheld, over the real binary on stdio (issue 1282): the
// cause the dynamic surface gives the same call, for the tier, the token's
// scope and the operator's read-only mode, where the SDK used to refuse the
// action as a value outside the tool's enum, which reads as a typo.
package stdioe2e

import (
	"maps"
	"strings"
	"testing"
)

// TestMetaWithheld_EachCauseIsAnsweredAsOnTheDynamicSurface calls, on the meta
// surface of a Free instance, an action each narrowing withholds from a tool
// the session still lists, and checks the answer names the action, says it
// exists, gives the cause in the dynamic surface's words, and was given before
// GitLab was asked for anything: no issue was created.
func TestMetaWithheld_EachCauseIsAnsweredAsOnTheDynamicSurface(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		scopes    []string
		arguments string
		want      []string
	}{
		{
			name:      "the tier, pinned",
			env:       map[string]string{"GITLAB_MCP_TIER": "free"},
			arguments: `{"name":"gitlab_project","arguments":{"action":"approval_config_get","params":{"project_id":"1"}}}`,
			want: []string{
				`gitlab_project: action "approval_config_get" exists but is not available`, "it needs GitLab Premium or Ultimate",
				"this deployment is configured for GitLab Free", "GITLAB_MCP_TIER",
			},
		},
		{
			name:      "the tier, detected",
			arguments: `{"name":"gitlab_project","arguments":{"action":"approval_config_get","params":{"project_id":"1"}}}`,
			want:      []string{`gitlab_project: action "approval_config_get" exists but is not available`, "this instance's tier was detected as GitLab Free"},
		},
		{
			name:      "the token's scope",
			scopes:    []string{"read_api"},
			arguments: `{"name":"gitlab_issue","arguments":{"action":"create","params":{"project_id":"42","title":"never created"}}}`,
			want: []string{
				`gitlab_issue: action "create" exists but is not available to this session`,
				"GitLab requires the api scope for it and the credential in use does not carry it", "Reauthorize with the api scope",
			},
		},
		{
			name:      "the operator's read-only mode",
			env:       map[string]string{"GITLAB_MCP_READ_ONLY": "true"},
			arguments: `{"name":"gitlab_issue","arguments":{"action":"create","params":{"project_id":"42","title":"never created"}}}`,
			want:      []string{`gitlab_issue: action "create" exists but is not available`, "this deployment is configured to withhold it", "Ask the operator"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = tc.scopes
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "meta"
			maps.Copy(env, tc.env)
			s := startSession(t, env)

			text := resultText(t, s.call(t, request(1, "tools/call", tc.arguments)))
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("the answer %q does not say %q", text, want)
				}
			}
			for _, unwanted := range []string{"validating", "enum", "Valid actions"} {
				if strings.Contains(text, unwanted) {
					t.Errorf("the answer %q is the refusal of a value outside the enum (%q), not the cause", text, unwanted)
				}
			}
			if created := fake.issuesCreated.Load(); created != 0 {
				t.Errorf("the instance was asked to create %d issues, want none", created)
			}
		})
	}
}
