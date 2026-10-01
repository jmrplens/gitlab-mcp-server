// fine_grained_test.go pins what a fine-grained session is listed and what a
// call to a registered tool is checked against before its arguments are
// decoded: the per-tool action sets of each surface, the listing decision, the
// streaming read of a meta tool's action, and the two middlewares built on
// them.
package toolvisibility

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// demoCatalog is two meta groups: gitlab_demo, whose allowed action every
// session runs and whose denied one (reachable by its historical spelling
// "forbidden" too) no fine-grained token does, and gitlab_blocked, whose one
// action no fine-grained token runs.
func demoCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	route := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return "ran", nil })
	denied := route
	denied.CompatibilityAliases = []string{"forbidden"}
	catalog := actioncatalog.NewCatalog()
	demo := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_demo", BaseDomain: "demo", SurfaceKind: actioncatalog.SurfaceKindMetaGroup})
	demo.SetAction(actioncatalog.Action{Name: "allowed", Route: route, IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_demo_allowed"}})
	demo.SetAction(actioncatalog.Action{Name: "denied", Route: denied, IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_demo_denied"}})
	blocked := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_blocked", BaseDomain: "blocked", SurfaceKind: actioncatalog.SurfaceKindMetaGroup})
	blocked.SetAction(actioncatalog.Action{Name: "only", Route: route, IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_blocked_only"}})
	for _, group := range []actioncatalog.Group{demo, blocked} {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error = %v", group.ToolName, err)
		}
	}
	return catalog
}

// demoAuthority is phase A over a table that denies demo.denied and
// blocked.only and holds no row for demo.allowed.
func demoAuthority() *finegrained.Authority {
	denial := &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace", Effect: finegrained.EffectNull}
	return finegrained.Unevaluated(&finegrained.Table{
		Version: "19.4.1-ee",
		Actions: []finegrained.Requirement{{ID: "blocked.only", Denied: denial}, {ID: "demo.denied", Denied: denial}},
	}, finegrained.FallbackNone, "")
}

// boundTo is a request context bound to a client carrying authority.
func boundTo(authority *finegrained.Authority) context.Context {
	client := gitlabclient.NewUnboundClient("https://gitlab.example.com")
	client.SetAuthority(authority)
	return gitlabclient.WithClient(context.Background(), client)
}

// TestNewToolActions_MapsEachSurfacesToolsToTheirActions verifies which
// actions each registered tool runs: an individual tool its own and a meta tool
// every action of its group, a standalone utility its own on both, and on the
// dynamic surface nothing, since its two tools run no action of their own.
func TestNewToolActions_MapsEachSurfacesToolsToTheirActions(t *testing.T) {
	catalog := demoCatalog(t)
	standalone := gitlabtools.StandaloneActionIDs()
	var anyStandalone string
	for name := range standalone {
		anyStandalone = name
		break
	}
	cases := []struct {
		surface string
		want    map[string][]string
	}{
		{surface: config.ToolSurfaceIndividual, want: map[string][]string{
			"gitlab_demo_allowed": {"demo.allowed"}, "gitlab_demo_denied": {"demo.denied"}, "gitlab_blocked_only": {"blocked.only"},
			anyStandalone: {standalone[anyStandalone]},
		}},
		{surface: config.ToolSurfaceMeta, want: map[string][]string{
			"gitlab_demo": {"demo.allowed", "demo.denied"}, "gitlab_blocked": {"blocked.only"},
			anyStandalone: {standalone[anyStandalone]},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			ids := NewToolActions(tc.surface, catalog).ActionIDs()
			for name, want := range tc.want {
				if !slices.Equal(ids[name], want) {
					t.Errorf("ActionIDs()[%s] = %v, want %v", name, ids[name], want)
				}
			}
			if len(ids) != len(standalone)+len(tc.want)-1 {
				t.Errorf("ActionIDs() holds %d tools, want the catalog's %d and the %d standalone utilities", len(ids), len(tc.want)-1, len(standalone))
			}
		})
	}
	t.Run(config.ToolSurfaceDynamic, func(t *testing.T) {
		if ids := NewToolActions(config.ToolSurfaceDynamic, catalog).ActionIDs(); len(ids) != 0 {
			t.Errorf("ActionIDs() = %v, want nothing on the dynamic surface", ids)
		}
	})
	t.Run("an action projected as no individual tool", func(t *testing.T) {
		unnamed := actioncatalog.NewCatalog()
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_unnamed", BaseDomain: "unnamed", SurfaceKind: actioncatalog.SurfaceKindMetaGroup})
		group.SetAction(actioncatalog.Action{Name: "only", Route: toolutil.Route(func(context.Context, map[string]any) (any, error) { return "ran", nil })})
		if err := unnamed.AddGroup(group); err != nil {
			t.Fatalf("AddGroup error = %v", err)
		}
		ids := NewToolActions(config.ToolSurfaceIndividual, unnamed).ActionIDs()
		if _, mapped := ids[""]; mapped || len(ids) != len(standalone) {
			t.Errorf("ActionIDs() = %v, want only the %d standalone utilities", ids, len(standalone))
		}
	})
	t.Run("the map is the caller's", func(t *testing.T) {
		actions := NewToolActions(config.ToolSurfaceMeta, catalog)
		actions.ActionIDs()["gitlab_demo"][0] = "changed"
		if got := actions.ActionIDs()["gitlab_demo"][0]; got != "demo.allowed" {
			t.Errorf("changing a returned list changed the index: %q", got)
		}
	})
}

// TestToolActions_Listed_HidesOnlyWhatTheSessionCannotRun verifies the listing
// decision: every tool for a session with no authority; for a fine-grained one,
// an individual tool by its action, a meta tool while any action of its group
// is listed, and a tool that runs no catalog action always.
func TestToolActions_Listed_HidesOnlyWhatTheSessionCannotRun(t *testing.T) {
	catalog := demoCatalog(t)
	individual := NewToolActions(config.ToolSurfaceIndividual, catalog)
	meta := NewToolActions(config.ToolSurfaceMeta, catalog)
	authority := demoAuthority()
	cases := []struct {
		name      string
		actions   *ToolActions
		authority *finegrained.Authority
		tool      string
		want      bool
	}{
		{name: "no authority lists a denied tool", actions: individual, tool: "gitlab_demo_denied", want: true},
		{name: "an allowed individual tool", actions: individual, authority: authority, tool: "gitlab_demo_allowed", want: true},
		{name: "a denied individual tool", actions: individual, authority: authority, tool: "gitlab_demo_denied", want: false},
		{name: "a tool no catalog action runs", actions: individual, authority: authority, tool: "gitlab_find_action", want: true},
		{name: "a meta tool with an allowed action", actions: meta, authority: authority, tool: "gitlab_demo", want: true},
		{name: "a meta tool whose every action is denied", actions: meta, authority: authority, tool: "gitlab_blocked", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.actions.Listed(tc.authority, tc.tool); got != tc.want {
				t.Errorf("Listed(%s) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

// TestToolActions_Filter_KeepsOrderAndTheSDKsOwnTools verifies a listing
// narrowed for a fine-grained session keeps the listed tools in their order,
// as the very pointers the SDK listed, and that a listing it narrows nothing of
// is returned as it came.
func TestToolActions_Filter_KeepsOrderAndTheSDKsOwnTools(t *testing.T) {
	actions := NewToolActions(config.ToolSurfaceIndividual, demoCatalog(t))
	allowed := &mcp.Tool{Name: "gitlab_demo_allowed"}
	denied := &mcp.Tool{Name: "gitlab_demo_denied"}
	find := &mcp.Tool{Name: "gitlab_find_action"}
	tools := []*mcp.Tool{find, denied, nil, allowed}

	got := actions.Filter(demoAuthority(), tools)
	if want := []*mcp.Tool{find, nil, allowed}; !slices.Equal(got, want) {
		t.Errorf("Filter = %v, want %v", got, want)
	}
	kept := []*mcp.Tool{find, allowed}
	if same := actions.Filter(demoAuthority(), kept); &same[0] != &kept[0] {
		t.Error("Filter copied a listing it removed nothing from")
	}
}

// TestToolActions_CallAction_NamesTheActionACallRuns verifies which action a
// call is checked against: an individual or standalone tool's own; a meta
// tool's action argument resolved through its group's routes, a historical
// spelling included; and none for a tool that runs no catalog action, an
// action the group does not have, or arguments the call check cannot read.
func TestToolActions_CallAction_NamesTheActionACallRuns(t *testing.T) {
	catalog := demoCatalog(t)
	individual := NewToolActions(config.ToolSurfaceIndividual, catalog)
	meta := NewToolActions(config.ToolSurfaceMeta, catalog)
	cases := []struct {
		name      string
		actions   *ToolActions
		tool      string
		arguments string
		want      string
		named     bool
	}{
		{name: "an individual tool", actions: individual, tool: "gitlab_demo_denied", arguments: `{}`, want: "demo.denied", named: true},
		{name: "a meta action", actions: meta, tool: "gitlab_demo", arguments: `{"action":"denied","params":{"x":1}}`, want: "demo.denied", named: true},
		{name: "a historical spelling", actions: meta, tool: "gitlab_demo", arguments: `{"action":"Forbidden"}`, want: "demo.denied", named: true},
		{name: "an action the group lacks", actions: meta, tool: "gitlab_demo", arguments: `{"action":"nope"}`},
		{name: "no action", actions: meta, tool: "gitlab_demo", arguments: `{"params":{}}`},
		{name: "a tool no catalog action runs", actions: meta, tool: "gitlab_find_action", arguments: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, named := tc.actions.CallAction(tc.tool, json.RawMessage(tc.arguments))
			if got != tc.want || named != tc.named {
				t.Errorf("CallAction(%s, %s) = %q, %v; want %q, %v", tc.tool, tc.arguments, got, named, tc.want, tc.named)
			}
		})
	}
	t.Run("a route no catalog named", func(t *testing.T) {
		unnamed := &ToolActions{groups: map[string]toolGroup{"gitlab_raw": {routes: toolutil.ActionMap{"go": toolutil.ActionRoute{}}}}}
		if got, named := unnamed.CallAction("gitlab_raw", json.RawMessage(`{"action":"go"}`)); named {
			t.Errorf("CallAction = %q, %v; want nothing named", got, named)
		}
	})
}

// TestActionArgument_ReadsTheActionMemberAlone verifies the action member is
// read out of a call's arguments wherever it sits, past members of any shape,
// and that anything that is not an object holding it as one string reads as
// none, a name given twice included.
func TestActionArgument_ReadsTheActionMemberAlone(t *testing.T) {
	cases := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "first", arguments: `{"action":"list","params":{}}`, want: "list"},
		{name: "after nested members", arguments: `{"params":{"a":[1,{"b":"c"}]},"confirm":true,"action":"get"}`, want: "get"},
		{name: "absent", arguments: `{"params":{}}`},
		{name: "not a string", arguments: `{"action":7}`},
		{name: "not an object", arguments: `["action","list"]`},
		{name: "empty", arguments: ``},
		{name: "truncated after the name", arguments: `{"action"`},
		{name: "truncated in another member", arguments: `{"params":{"a":`},
		{name: "a name given twice", arguments: `{"action":"list","action":"delete"}`},
		{name: "unterminated", arguments: `{"action":"list"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := actionArgument(json.RawMessage(tc.arguments)); got != tc.want {
				t.Errorf("actionArgument(%s) = %q, want %q", tc.arguments, got, tc.want)
			}
		})
	}
}

// listing answers a tools/list with the given tools, a cursor and _meta, so a
// test can tell a narrowed copy kept them.
func listing(tools ...*mcp.Tool) mcp.MethodHandler {
	return func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{Tools: tools, NextCursor: "next", Meta: mcp.Meta{"k": "v"}}, nil
	}
}

// TestListingMiddleware_NarrowsOnlyAFineGrainedListing verifies the listing a
// fine-grained session is served drops what it may not run as a copy keeping
// the cursor and _meta, and that every other listing, the server's own, one
// served before registration published the tool sets, another method's result
// and an error, passes through untouched.
func TestListingMiddleware_NarrowsOnlyAFineGrainedListing(t *testing.T) {
	actions := NewToolActions(config.ToolSurfaceIndividual, demoCatalog(t))
	allowed := &mcp.Tool{Name: "gitlab_demo_allowed"}
	denied := &mcp.Tool{Name: "gitlab_demo_denied"}
	ready := func() *ToolActions { return actions }
	session := boundTo(demoAuthority())
	request := &mcp.ServerRequest[*mcp.ListToolsParams]{Params: &mcp.ListToolsParams{}}

	t.Run("a fine-grained listing", func(t *testing.T) {
		upstream := listing(denied, allowed)
		result, err := ListingMiddleware(ready)(upstream)(session, methodToolsList, request)
		got, ok := result.(*mcp.ListToolsResult)
		if err != nil || !ok {
			t.Fatalf("ListingMiddleware = %T, %v", result, err)
		}
		if !slices.Equal(got.Tools, []*mcp.Tool{allowed}) || got.NextCursor != "next" || got.Meta["k"] != "v" {
			t.Errorf("ListingMiddleware = %+v, want the allowed tool with the cursor and _meta kept", got)
		}
	})
	passes := []struct {
		name    string
		ctx     context.Context
		method  string
		actions func() *ToolActions
	}{
		{name: "a classic listing", ctx: context.Background(), method: methodToolsList, actions: ready},
		{name: "the server's own listing", ctx: toolutil.WithInternalInspection(session), method: methodToolsList, actions: ready},
		{name: "before registration", ctx: session, method: methodToolsList, actions: func() *ToolActions { return nil }},
		{name: "another method", ctx: session, method: "resources/list", actions: ready},
	}
	for _, tc := range passes {
		t.Run(tc.name, func(t *testing.T) {
			upstream := listing(denied, allowed)
			want, _ := upstream(tc.ctx, tc.method, request)
			got, err := ListingMiddleware(tc.actions)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
				return want, nil
			})(tc.ctx, tc.method, request)
			if err != nil || got != want {
				t.Errorf("ListingMiddleware = %+v, %v; want the result as the SDK built it", got, err)
			}
		})
	}
	t.Run("an error", func(t *testing.T) {
		failed := func(context.Context, string, mcp.Request) (mcp.Result, error) { return nil, context.Canceled }
		if got, err := ListingMiddleware(ready)(failed)(session, methodToolsList, request); got != nil || err != context.Canceled { //nolint:errorlint // the very error is expected back
			t.Errorf("ListingMiddleware = %v, %v; want the error as it came", got, err)
		}
	})
	t.Run("a result that is not a listing", func(t *testing.T) {
		other := &mcp.CallToolResult{}
		got, err := ListingMiddleware(ready)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return other, nil
		})(session, methodToolsList, request)
		if err != nil || got != other {
			t.Errorf("ListingMiddleware = %v, %v; want the result as it came", got, err)
		}
	})
	t.Run("a nil listing", func(t *testing.T) {
		var empty *mcp.ListToolsResult
		got, err := ListingMiddleware(ready)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return empty, nil
		})(session, methodToolsList, request)
		if listed, ok := got.(*mcp.ListToolsResult); err != nil || !ok || listed != nil {
			t.Errorf("ListingMiddleware = %v, %v; want the nil listing as it came, not a copy of it", got, err)
		}
	})
}

// TestCallMiddleware_AnswersAWithheldCallBeforeTheArgumentsAreRead verifies a
// fine-grained session's call to a tool whose action it may not run is answered
// with the reason without the call going further in, and that every other call,
// another method, one before registration, one this cannot name, an allowed one
// and a classic session's, goes on.
func TestCallMiddleware_AnswersAWithheldCallBeforeTheArgumentsAreRead(t *testing.T) {
	actions := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t))
	ready := func() *ToolActions { return actions }
	session := boundTo(demoAuthority())
	call := func(name, arguments string) mcp.Request {
		return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: name, Arguments: json.RawMessage(arguments)}}
	}
	reached := &mcp.CallToolResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return reached, nil }

	t.Run("a withheld call", func(t *testing.T) {
		result, err := CallMiddleware(ready)(next)(session, methodToolsCall, call("gitlab_demo", `{"action":"denied","params":"not an object"}`))
		got, ok := result.(*mcp.CallToolResult)
		if err != nil || !ok || got == reached || !got.IsError {
			t.Fatalf("CallMiddleware = %+v, %v; want the withheld answer", result, err)
		}
		text := got.Content[0].(*mcp.TextContent).Text
		if !strings.HasPrefix(text, `action "demo.denied" exists but is not available to a fine-grained personal access token`) {
			t.Errorf("CallMiddleware text = %q", text)
		}
	})
	passes := []struct {
		name    string
		ctx     context.Context
		method  string
		req     mcp.Request
		actions func() *ToolActions
	}{
		{name: "an allowed call", ctx: session, method: methodToolsCall, req: call("gitlab_demo", `{"action":"allowed"}`), actions: ready},
		{name: "a classic session", ctx: context.Background(), method: methodToolsCall, req: call("gitlab_demo", `{"action":"denied"}`), actions: ready},
		{name: "a call this cannot name", ctx: session, method: methodToolsCall, req: call("gitlab_demo", `{"action":"nope"}`), actions: ready},
		{name: "before registration", ctx: session, method: methodToolsCall, req: call("gitlab_demo", `{"action":"denied"}`), actions: func() *ToolActions { return nil }},
		{name: "no raw parameters", ctx: session, method: methodToolsCall, req: &mcp.ServerRequest[*mcp.CallToolParams]{Params: &mcp.CallToolParams{Name: "gitlab_demo"}}, actions: ready},
		{name: "another method", ctx: session, method: methodToolsList, req: call("gitlab_demo", `{"action":"denied"}`), actions: ready},
	}
	for _, tc := range passes {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := CallMiddleware(tc.actions)(next)(tc.ctx, tc.method, tc.req); err != nil || got != reached {
				t.Errorf("CallMiddleware = %+v, %v; want the call to go on", got, err)
			}
		})
	}
}
