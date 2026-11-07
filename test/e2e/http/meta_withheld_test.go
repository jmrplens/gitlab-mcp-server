// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// meta_withheld_test.go drives the answer a meta tool gives a call naming an
// action its group withheld, over the real binary in HTTP mode (issue 1282):
// the cause the dynamic surface gives the same call, where the SDK used to
// refuse the action as a value outside the tool's enum, and the one narrowing
// that stays unnamed, an operator's exclusion by name.
package httpe2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// metaCall spells a tools/call to a meta tool, the action's own arguments
// under params, with the parameter header protocol 2026-07-28 requires
// beside the action argument.
func metaCall(id int, tool, action, params string) request {
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{%s,"name":%q,"arguments":{"action":%q,"params":%s}}}`,
		id, protocolMeta, tool, action, params,
	)
	return authorizedCall(body, withAction(action))
}

// TestMetaWithheld_EachCauseIsAnsweredAsOnTheDynamicSurface calls, on the meta
// surface of a Free instance, an action the tier withholds and one read-only
// mode withholds, from tools the session still lists, and checks the answer
// says the action exists and gives its cause in the dynamic surface's words
// rather than the enum's refusal.
func TestMetaWithheld_EachCauseIsAnsweredAsOnTheDynamicSurface(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	cases := []struct {
		name   string
		flags  []string
		tool   string
		action string
		params string
		want   []string
	}{
		{
			name: "the tier, pinned", flags: []string{"--tier=free"},
			tool: "gitlab_project", action: "approval_config_get", params: `{"project_id":"1"}`,
			want: []string{`gitlab_project: action \"approval_config_get\" exists but is not available`, "it needs GitLab Premium or Ultimate", "this deployment is configured for GitLab Free"},
		},
		{
			name: "the tier, detected",
			tool: "gitlab_project", action: "approval_config_get", params: `{"project_id":"1"}`,
			want: []string{`gitlab_project: action \"approval_config_get\" exists but is not available`, "this instance's tier was detected as GitLab Free"},
		},
		{
			name: "the operator's read-only mode", flags: []string{"--read-only"},
			tool: "gitlab_issue", action: "create", params: `{"project_id":"42","title":"never created"}`,
			want: []string{`gitlab_issue: action \"create\" exists but is not available`, "this deployment is configured to withhold it", "Ask the operator"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, map[string]string{"GITLAB_MCP_TOOL_SURFACE": "meta"}, append([]string{"--gitlab-url=" + gitlab.url}, tc.flags...)...)
			got := srv.do(t, metaCall(1, tc.tool, tc.action, tc.params)).body
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("the answer does not say %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "validating") || strings.Contains(got, "Valid actions") {
				t.Errorf("the answer is the refusal of a value outside the enum, not the cause:\n%s", got)
			}
		})
	}
}

// TestMetaWithheld_AnActionExcludedByNameStaysUnnamed checks the narrowing the
// answer leaves out: an action the operator excluded by name is refused as the
// value outside its tool's enum it is, and never named back as withheld,
// which would contradict the exclusion (AUT-004).
func TestMetaWithheld_AnActionExcludedByNameStaysUnnamed(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, map[string]string{"GITLAB_MCP_TOOL_SURFACE": "meta", "GITLAB_MCP_EXCLUDE_TOOLS": "issue.create"}, "--gitlab-url="+gitlab.url)

	got := srv.do(t, metaCall(1, "gitlab_issue", "create", `{"project_id":"42","title":"never created"}`)).body
	if strings.Contains(got, "exists but is not available") || strings.Contains(got, "withhold") {
		t.Errorf("an excluded action was named back as withheld:\n%s", got)
	}
	if !strings.Contains(got, "/properties/action") {
		t.Errorf("an excluded action was not refused by its tool's enum:\n%s", got)
	}
}
