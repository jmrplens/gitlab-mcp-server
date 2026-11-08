// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMakeMetaHandler_UnroutedAction_IsAnsweredWithTheCauseTheContextCarries
// verifies the meta dispatcher's fallback for an action its routes do not
// carry (issue 1282): the cause the call middleware handed it when the action
// was withheld, its own "unknown action" list otherwise, and the list too
// when nothing handed it an answer at all. A nil answer installs nothing.
func TestMakeMetaHandler_UnroutedAction_IsAnsweredWithTheCauseTheContextCarries(t *testing.T) {
	routes := ActionMap{"list": Route(func(context.Context, map[string]any) (any, error) { return "ran", nil })}
	handler := MakeMetaHandler("gitlab_demo", routes, nil)
	answer := WithheldAnswer(func(toolName, action string) (string, bool) {
		if toolName == "gitlab_demo" && action == "removed" {
			return `gitlab_demo: action "removed" exists but is not available: the cause`, true
		}
		return "", false
	})
	cases := []struct {
		name   string
		ctx    context.Context
		action string
		want   string
	}{
		{name: "a withheld action", ctx: WithWithheldAnswer(context.Background(), answer), action: "removed", want: `gitlab_demo: action "removed" exists but is not available: the cause`},
		{name: "an action nothing withheld", ctx: WithWithheldAnswer(context.Background(), answer), action: "nope", want: `gitlab_demo: unknown action "nope". Valid actions: list`},
		{name: "no answer handed over", ctx: context.Background(), action: "removed", want: `gitlab_demo: unknown action "removed". Valid actions: list`},
		{name: "a nil answer", ctx: WithWithheldAnswer(context.Background(), nil), action: "removed", want: `gitlab_demo: unknown action "removed". Valid actions: list`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, err := handler(tc.ctx, &mcp.CallToolRequest{}, MetaToolInput{Action: tc.action})
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("handler = %+v, %v; want a refusal", result, err)
			}
			if text := result.Content[0].(*mcp.TextContent).Text; !strings.HasPrefix(text, tc.want) {
				t.Errorf("handler text = %q, want it to begin %q", text, tc.want)
			}
		})
	}
}
