// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// withheld_test.go pins how a meta call naming an action its tool's group
// withheld is answered (issue 1282): with the cause the dynamic surface gives,
// before the SDK refuses the action as a value outside the tool's enum, for
// each of the three causes a record holds; and how every other call is left
// where it was going.
package toolvisibility

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// metaCall is a tools/call for a meta tool with raw arguments, the shape the
// middleware reads before the SDK decodes anything.
func metaCall(name, arguments string) mcp.Request {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: name, Arguments: json.RawMessage(arguments)}}
}

// answeredText is the text of a tool result the middleware answered with, or
// a failure when it let the call through instead.
func answeredText(t *testing.T, result mcp.Result, err error, reached mcp.Result) string {
	t.Helper()
	got, ok := result.(*mcp.CallToolResult)
	if err != nil || !ok || result == reached || !got.IsError {
		t.Fatalf("NarrowedCallMiddleware = %+v, %v; want the withheld answer", result, err)
	}
	return got.Content[0].(*mcp.TextContent).Text
}

// servedMetaIndex is the index a meta server of cfg publishes: its served
// catalog's tools, with the action names of the tier's meta tools and the
// record of what cfg's narrowing withheld attached, as cmd/server attaches
// them.
func servedMetaIndex(t *testing.T, cfg *config.ServerConfig) (*ToolActions, *actioncatalog.Withheld) {
	t.Helper()
	client := gitlabtools.UnboundClient(false)
	catalog, withheldActions, err := gitlabtools.SharedMetaCatalog(client, cfg)
	if err != nil {
		t.Fatalf("SharedMetaCatalog() error = %v", err)
	}
	keys, err := gitlabtools.MetaActionKeys(false, cfg.Tier)
	if err != nil {
		t.Fatalf("MetaActionKeys() error = %v", err)
	}
	withheld := &actioncatalog.Withheld{}
	withheld.RecordActions(withheldActions.ByTokenScope, withheldActions.ByOperator)
	withheld.RecordTier(withheldActions.ByTier, cfg.Tier, cfg.TierExplicit)
	return NewToolActions(config.ToolSurfaceMeta, catalog).Withhold(keys, withheld), withheld
}

// TestNarrowedCallMiddleware_WithheldMetaAction_AnswersEachCauseAsTheDynamicSurfaceDoes
// drives the middleware with the meta catalog a server of each configuration
// registers and checks a call naming an action the configuration withheld is
// answered with the dynamic surface's text for the same record, the meta
// tool's name and the action argument in it, and nothing is dispatched: the
// tier (a Premium action on a detected Free tier), the credential's scope (a
// write for a token carrying read_api) and the operator's read-only mode.
func TestNarrowedCallMiddleware_WithheldMetaAction_AnswersEachCauseAsTheDynamicSurfaceDoes(t *testing.T) {
	cases := []struct {
		name     string
		cfg      *config.ServerConfig
		tool     string
		action   string
		id       string
		contains string
	}{
		{
			name: "tier", cfg: &config.ServerConfig{Tier: edition.Free},
			tool: "gitlab_project", action: "approval_config_get", id: "project.approval_config_get",
			contains: "it needs GitLab Premium or Ultimate, and this instance's tier was detected as GitLab Free",
		},
		{
			name: "token scope", cfg: &config.ServerConfig{Tier: edition.Free, TokenScopes: []string{"read_api"}, ReadAPIOnly: true},
			tool: "gitlab_issue", action: "create", id: "issue.create",
			contains: "GitLab requires the api scope for it and the credential in use does not carry it",
		},
		{
			name: "operator", cfg: &config.ServerConfig{Tier: edition.Free, ReadOnly: true},
			tool: "gitlab_issue", action: "create", id: "issue.create",
			contains: "this deployment is configured to withhold it",
		},
	}
	reached := &mcp.CallToolResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return reached, nil }
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index, withheld := servedMetaIndex(t, tc.cfg)
			want, ok := withheld.Message(tc.tool, tc.action, tc.id)
			if !ok {
				t.Fatalf("the record holds no %s for this configuration", tc.id)
			}
			result, err := NarrowedCallMiddleware(func() *ToolActions { return index })(next)(context.Background(), methodToolsCall,
				metaCall(tc.tool, `{"action":"`+tc.action+`","params":{"project_id":"1"}}`))
			if text := answeredText(t, result, err, reached); text != want || !strings.Contains(text, tc.contains) {
				t.Errorf("NarrowedCallMiddleware text = %q, want %q naming %q", text, want, tc.contains)
			}
			if !strings.HasPrefix(want, tc.tool+`: action "`+tc.action+`" exists but is not available`) {
				t.Errorf("the answer %q does not name the meta tool and the action argument", want)
			}
		})
	}

	t.Run("an action the operator excluded by name is not named", func(t *testing.T) {
		index, _ := servedMetaIndex(t, &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"issue.create"}})
		result, err := NarrowedCallMiddleware(func() *ToolActions { return index })(next)(context.Background(), methodToolsCall,
			metaCall("gitlab_issue", `{"action":"create"}`))
		if err != nil || result != reached {
			t.Errorf("NarrowedCallMiddleware = %+v, %v; want the call left to the SDK's enum refusal (AUT-004)", result, err)
		}
	})
}

// TestNarrowedCallMiddleware_LeavesEveryOtherCallWhereItWasGoing verifies the
// calls the middleware does not answer: an action the group serves, under its
// name or a historical spelling, an action no record holds, a group tool this
// server never registered, whose call keeps the SDK's "unknown tool", a call
// with no readable action, an index nothing was attached to, the time before
// registration, and every other method.
func TestNarrowedCallMiddleware_LeavesEveryOtherCallWhereItWasGoing(t *testing.T) {
	withheld := &actioncatalog.Withheld{}
	withheld.RecordActions(nil, []string{"demo.removed", "gone.only"})
	keys := map[string]map[string]string{
		"gitlab_demo": {"allowed": "demo.allowed", "removed": "demo.removed"},
		"gitlab_gone": {"only": "gone.only"},
	}
	attached := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t)).Withhold(keys, withheld)
	bare := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t))
	reached := &mcp.CallToolResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return reached, nil }

	cases := []struct {
		name    string
		method  string
		req     mcp.Request
		actions func() *ToolActions
	}{
		{name: "a served action", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":"allowed"}`), actions: func() *ToolActions { return attached }},
		{name: "a served action's historical spelling", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":"forbidden"}`), actions: func() *ToolActions { return attached }},
		{name: "an action no record holds", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":"nope"}`), actions: func() *ToolActions { return attached }},
		{name: "a group tool never registered", method: methodToolsCall, req: metaCall("gitlab_gone", `{"action":"only"}`), actions: func() *ToolActions { return attached }},
		{name: "no readable action", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":7}`), actions: func() *ToolActions { return attached }},
		{name: "an index nothing was attached to", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":"removed"}`), actions: func() *ToolActions { return bare }},
		{name: "before registration", method: methodToolsCall, req: metaCall("gitlab_demo", `{"action":"removed"}`), actions: func() *ToolActions { return nil }},
		{name: "no raw parameters", method: methodToolsCall, req: &mcp.ServerRequest[*mcp.CallToolParams]{Params: &mcp.CallToolParams{Name: "gitlab_demo"}}, actions: func() *ToolActions { return attached }},
		{name: "another method", method: methodToolsList, req: metaCall("gitlab_demo", `{"action":"removed"}`), actions: func() *ToolActions { return attached }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NarrowedCallMiddleware(tc.actions)(next)(context.Background(), tc.method, tc.req)
			if err != nil || result != reached {
				t.Errorf("NarrowedCallMiddleware = %+v, %v; want the call passed through", result, err)
			}
		})
	}

	t.Run("a withheld action of a registered group is answered and logged", func(t *testing.T) {
		var logged bytes.Buffer
		original := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
		t.Cleanup(func() { slog.SetDefault(original) })
		result, err := NarrowedCallMiddleware(func() *ToolActions { return attached })(next)(context.Background(), methodToolsCall,
			metaCall("gitlab_demo", `{"action":"Removed"}`))
		const want = `gitlab_demo: action "Removed" exists but is not available: this deployment is configured to withhold it`
		if text := answeredText(t, result, err, reached); !strings.HasPrefix(text, want) {
			t.Errorf("NarrowedCallMiddleware text = %q, want it to begin %q", text, want)
		}
		for _, field := range []string{`"msg":"tool call refused"`, `"tool":"gitlab_demo/Removed"`, `"reason":"unknown_action"`} {
			t.Run(field, func(t *testing.T) {
				if !strings.Contains(logged.String(), field) {
					t.Errorf("the log does not carry %s:\n%s", field, logged.String())
				}
			})
		}
	})
}

// TestNarrowedCallMiddleware_PassedCall_CarriesTheAnswerForTheDispatcher
// verifies a call the middleware lets through carries the withheld answer
// for the meta dispatcher, which consults it for the action it arrives at
// after the parameter-dependent rewrite, and that the answer it carries is
// the middleware's own: the cause for a withheld action of the tool, nothing
// for anything else.
func TestNarrowedCallMiddleware_PassedCall_CarriesTheAnswerForTheDispatcher(t *testing.T) {
	withheld := &actioncatalog.Withheld{}
	withheld.RecordActions(nil, []string{"demo.removed"})
	keys := map[string]map[string]string{"gitlab_demo": {"removed": "demo.removed"}}
	index := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t)).Withhold(keys, withheld)
	routes := toolutil.ActionMap{"allowed": toolutil.Route(func(context.Context, map[string]any) (any, error) { return "ran", nil })}
	handler := toolutil.MakeMetaHandler("gitlab_demo", routes, nil)

	var answered string
	next := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		result, _, _ := handler(ctx, &mcp.CallToolRequest{}, toolutil.MetaToolInput{Action: "removed"})
		answered = result.Content[0].(*mcp.TextContent).Text
		return result, nil
	}
	if _, err := NarrowedCallMiddleware(func() *ToolActions { return index })(next)(context.Background(), methodToolsCall,
		metaCall("gitlab_demo", `{"action":"allowed"}`)); err != nil {
		t.Fatalf("NarrowedCallMiddleware error = %v", err)
	}
	if !strings.HasPrefix(answered, `gitlab_demo: action "removed" exists but is not available`) {
		t.Errorf("the dispatcher answered %q, want the withheld cause it was handed", answered)
	}
	if _, ok := index.withheldAnswer("gitlab_demo", "allowed"); ok {
		t.Error("withheldAnswer answered an action no record holds")
	}
}

// TestToolActions_Withhold_AttachesNothingToNoIndexOrFromNoRecord verifies
// the two ways Withhold attaches nothing: to a nil index, which it returns as
// it came, and from a nil record, which leaves the index answering nothing.
func TestToolActions_Withhold_AttachesNothingToNoIndexOrFromNoRecord(t *testing.T) {
	var none *ToolActions
	if got := none.Withhold(map[string]map[string]string{}, &actioncatalog.Withheld{}); got != nil {
		t.Errorf("Withhold() on a nil index = %v, want nil", got)
	}
	index := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t))
	if got := index.Withhold(map[string]map[string]string{"gitlab_demo": {"removed": "demo.removed"}}, nil); got != index || got.withheldKeys != nil {
		t.Errorf("Withhold(nil record) = %+v, want the same index with nothing attached", got)
	}
	if _, ok := index.WithheldMessage("gitlab_demo", json.RawMessage(`{"action":"removed"}`)); ok {
		t.Error("an index nothing was attached to answered a withheld call")
	}
}
