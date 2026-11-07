// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// tier_params_test.go drives the answer a call gets for a param the tier
// left out of a served action's schema, over the real binary on stdio (issue
// 1239): the tier it needs, on each surface, before anything is sent.
package stdioe2e

import (
	"strings"
	"testing"
)

// TestTierParams_APremiumParamOnAFreeInstanceIsAnsweredWithItsTier scopes a
// board to an assignee on a pinned Free instance on each surface: the
// individual one, where the SDK refused the param as an unexpected property,
// the meta one, where it was sent and dropped by GitLab in silence, and the
// dynamic one, where it was listed among the unknown params.
func TestTierParams_APremiumParamOnAFreeInstanceIsAnsweredWithItsTier(t *testing.T) {
	const scoped = `"project_id":"42","board_id":7,"assignee_id":5`
	cases := []struct {
		surface string
		call    string
		opens   string
	}{
		{surface: "individual", call: `{"name":"gitlab_board_update","arguments":{` + scoped + `}}`, opens: "gitlab_board_update: "},
		{surface: "meta", call: `{"name":"gitlab_project","arguments":{"action":"board_update","params":{` + scoped + `}}}`, opens: "gitlab_project/board_update: "},
		{surface: "dynamic", call: `{"name":"gitlab_execute_action","arguments":{"action":"project.board_update","params":{` + scoped + `}}}`, opens: "gitlab_execute_action/project.board_update: "},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = tc.surface
			env["GITLAB_MCP_TIER"] = "free"
			s := startSession(t, env)

			text := resultText(t, s.call(t, request(1, "tools/call", tc.call)))
			want := tc.opens + `param "assignee_id" needs GitLab Premium or Ultimate, and this deployment is configured for GitLab Free, so the action's schema at this tier leaves it out and nothing was sent.`
			if !strings.HasPrefix(text, want) {
				t.Errorf("the answer %q does not begin %q", text, want)
			}
		})
	}
}
