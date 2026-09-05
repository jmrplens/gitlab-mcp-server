package tools

import (
	"encoding/json"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestNewCallIdentifier_SharedCatalogReusesOneResolver verifies servers bound
// to one shared catalog get one resolver per surface, built from the shared
// origin, while a catalog nobody shared gets a resolver of its own and leaves
// nothing in the cache.
func TestNewCallIdentifier_SharedCatalogReusesOneResolver(t *testing.T) {
	shared, err := BuildActionCatalog(nil, ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	origin := shared.SharedOrigin()
	if origin == nil {
		t.Fatal("BuildActionCatalog() returned a catalog with no shared origin")
	}
	key := identifierKey{origin: origin, surface: config.ToolSurfaceDynamic}
	first := NewCallIdentifier(shared, config.ToolSurfaceDynamic)
	cached, ok := sharedIdentifiers.Load(key)
	if !ok {
		t.Fatal("NewCallIdentifier(shared catalog) left nothing in the cache")
	}
	second := NewCallIdentifier(shared.SharedOrigin().BindTo(nil), config.ToolSurfaceDynamic)
	for name, identifier := range map[string]mcpotel.CallIdentifier{"first": first, "second": second, "cached": cached.(mcpotel.CallIdentifier)} {
		t.Run(name, func(t *testing.T) {
			identity, found := identifier.Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": "issue.list"}))
			if !found || identity.ActionID != "issue.list" {
				t.Errorf("Identify(issue.list) = %+v, %t, want the action resolved", identity, found)
			}
		})
	}

	private := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_issue"})
	group.SetAction(actioncatalog.Action{Name: "list", Route: toolutil.RouteAction(nil, testListAction)})
	if addErr := private.AddGroup(group); addErr != nil {
		t.Fatalf("AddGroup() error = %v", addErr)
	}
	if private.SharedOrigin() != nil {
		t.Fatal("a hand-built catalog reported a shared origin")
	}
	resolver := NewCallIdentifier(private, config.ToolSurfaceMeta)
	if identity, found := resolver.Identify("gitlab_issue", rawArgs(t, map[string]any{"action": "list"})); !found || identity.ActionID != "issue.list" {
		t.Errorf("private resolver Identify(gitlab_issue list) = %+v, %t, want issue.list", identity, found)
	}
	if _, leaked := sharedIdentifiers.Load(identifierKey{origin: private, surface: config.ToolSurfaceMeta}); leaked {
		t.Fatal("NewCallIdentifier(private catalog) cached a resolver under a catalog nobody shared")
	}
}

// rawArgs encodes tool arguments the way the wire delivers them.
func rawArgs(t *testing.T, fields map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encoding arguments: %v", err)
	}
	return encoded
}

// TestNewCallIdentifier_EachSurfaceResolvesItsOwnShape is the resolver's whole
// job, asserted one surface at a time because that is how it is built.
//
// The surface is decided before the process starts and cannot change while it
// runs, so the resolver is told which one it serves rather than trying each
// shape until something matches. Asserting them separately is what keeps that
// honest: a single table over a resolver that fell back through all three would
// pass whether or not the surface parameter did anything.
func TestNewCallIdentifier_EachSurfaceResolvesItsOwnShape(t *testing.T) {
	catalog := buildTestCatalog(t)

	tests := []struct {
		name       string
		surface    string
		tool       string
		arguments  any
		wantAction string
		wantDomain string
		wantOK     bool
	}{
		{
			name:       "individual: the declared tool name is looked up",
			surface:    config.ToolSurfaceIndividual,
			tool:       "gitlab_issue_list",
			arguments:  nil,
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:       "meta: the group supplies the domain the action lacks",
			surface:    config.ToolSurfaceMeta,
			tool:       "gitlab_issue",
			arguments:  rawArgs(t, map[string]any{"action": "list"}),
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:       "dynamic: the argument is already canonical",
			surface:    config.ToolSurfaceDynamic,
			tool:       "gitlab_execute_action",
			arguments:  rawArgs(t, map[string]any{"action": "issue.list"}),
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:      "individual: a standalone tool belongs to no action",
			surface:   config.ToolSurfaceIndividual,
			tool:      "gitlab_discover_project",
			arguments: nil,
			wantOK:    false,
		},
		{
			name:      "meta: a standalone tool belongs to no action",
			surface:   config.ToolSurfaceMeta,
			tool:      "gitlab_discover_project",
			arguments: rawArgs(t, map[string]any{}),
			wantOK:    false,
		},
		{
			name:       "meta: an invented action keeps the domain, which is still true",
			surface:    config.ToolSurfaceMeta,
			tool:       "gitlab_issue",
			arguments:  rawArgs(t, map[string]any{"action": "teleport"}),
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:      "dynamic: an invented action resolves to nothing",
			surface:   config.ToolSurfaceDynamic,
			tool:      "gitlab_execute_action",
			arguments: rawArgs(t, map[string]any{"action": "issue.teleport"}),
			wantOK:    false,
		},
		{
			name:      "individual: an invented tool resolves to nothing",
			surface:   config.ToolSurfaceIndividual,
			tool:      "gitlab_not_a_tool",
			arguments: nil,
			wantOK:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			identity, ok := NewCallIdentifier(catalog, tc.surface).Identify(tc.tool, tc.arguments)
			if ok != tc.wantOK {
				t.Fatalf("Identify(%q) ok = %v, want %v (identity %+v)", tc.tool, ok, tc.wantOK, identity)
			}
			if !ok {
				return
			}
			if identity.ActionID != tc.wantAction {
				t.Errorf("action = %q, want %q", identity.ActionID, tc.wantAction)
			}
			if identity.Domain != tc.wantDomain {
				t.Errorf("domain = %q, want %q", identity.Domain, tc.wantDomain)
			}
		})
	}
}

// TestNewCallIdentifier_OneSurfaceDoesNotAnswerForAnother is what the surface
// parameter buys, and the assertion that fails if it is ignored.
//
// A resolver that tried every shape would answer all of these, which sounds
// harmless and is not: it would mean the code cannot state what it knows, and a
// reader would reasonably conclude the surfaces overlap when only one is ever
// registered.
func TestNewCallIdentifier_OneSurfaceDoesNotAnswerForAnother(t *testing.T) {
	catalog := buildTestCatalog(t)

	individual := NewCallIdentifier(catalog, config.ToolSurfaceIndividual)
	meta := NewCallIdentifier(catalog, config.ToolSurfaceMeta)
	dynamic := NewCallIdentifier(catalog, config.ToolSurfaceDynamic)

	if identity, ok := individual.Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": "issue.list"})); ok {
		t.Errorf("the individual resolver answered a dynamic call: %+v", identity)
	}
	if identity, ok := dynamic.Identify("gitlab_issue_list", nil); ok {
		t.Errorf("the dynamic resolver answered an individual call: %+v", identity)
	}
	if identity, ok := meta.Identify("gitlab_issue_list", nil); ok {
		t.Errorf("the meta resolver answered an individual call: %+v", identity)
	}
	if identity, ok := dynamic.Identify("gitlab_issue", rawArgs(t, map[string]any{"action": "list"})); ok {
		t.Errorf("the dynamic resolver answered a meta call, whose bare action is not a canonical id: %+v", identity)
	}
}

// TestNewCallIdentifier_UnknownSurfaceBehavesAsTheDefault covers a value that
// cannot reach here from configuration but could from a caller.
//
// Dynamic is the server's own default when nothing is set, so matching it
// produces no surprise. Inventing a fourth behavior, or resolving nothing at
// all, would make a wiring mistake show up as silently missing telemetry rather
// than as telemetry for the default surface.
func TestNewCallIdentifier_UnknownSurfaceBehavesAsTheDefault(t *testing.T) {
	catalog := buildTestCatalog(t)

	identity, ok := NewCallIdentifier(catalog, "no-such-surface").
		Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": "issue.list"}))
	if !ok {
		t.Fatal("an unknown surface resolved nothing")
	}
	if identity.ActionID != "issue.list" {
		t.Errorf("action = %q, want issue.list", identity.ActionID)
	}
}

// TestNewCallIdentifier_ReadsTheWireShape is the regression for a defect the
// first version of these tests could not see.
//
// A tools/call arriving over the wire is CallToolParamsRaw, whose Arguments
// field is json.RawMessage: the SDK deliberately leaves decoding to the tool
// handler. The first resolver read only map[string]any, so it compiled, ran,
// passed every test built on maps, and would have recorded no action for a
// single real request on the two surfaces that need one.
func TestNewCallIdentifier_ReadsTheWireShape(t *testing.T) {
	identifier := NewCallIdentifier(buildTestCatalog(t), config.ToolSurfaceDynamic)

	for _, tc := range []struct {
		name      string
		arguments any
	}{
		{name: "raw JSON, as the wire delivers it", arguments: json.RawMessage(`{"action":"issue.list"}`)},
		{name: "raw JSON with other fields alongside", arguments: json.RawMessage(`{"project_id":"a/b","action":"issue.list","per_page":20}`)},
		{name: "a byte slice", arguments: []byte(`{"action":"issue.list"}`)},
		{name: "a map, as an in-process caller builds it", arguments: map[string]any{"action": "issue.list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, ok := identifier.Identify("gitlab_execute_action", tc.arguments)
			if !ok {
				t.Fatal("resolved to nothing; every real tools/call would carry no action")
			}
			if identity.ActionID != "issue.list" {
				t.Errorf("action = %q, want issue.list", identity.ActionID)
			}
		})
	}
}

// TestNewCallIdentifier_MalformedArgumentsResolveToNothing pins the failure
// mode. Arguments that do not parse are the handler's business to reject, and
// telemetry has no standing to complain about them first: the only correct
// outcome is no attribute, never a panic and never an error.
func TestNewCallIdentifier_MalformedArgumentsResolveToNothing(t *testing.T) {
	identifier := NewCallIdentifier(buildTestCatalog(t), config.ToolSurfaceDynamic)

	cases := []struct {
		name      string
		arguments any
	}{
		{name: "truncated_object", arguments: json.RawMessage(`{"action":`)},
		{name: "array_not_object", arguments: json.RawMessage(`[]`)},
		{name: "numeric_action", arguments: json.RawMessage(`{"action":42}`)},
		{name: "empty_payload", arguments: json.RawMessage(``)},
		{name: "json_null", arguments: json.RawMessage(`null`)},
		{name: "not_json_at_all", arguments: 42},
		{name: "nil_arguments", arguments: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if identity, ok := identifier.Identify("gitlab_execute_action", tc.arguments); ok {
				t.Errorf("arguments %v resolved to %+v", tc.arguments, identity)
			}
		})
	}
}

// TestNewCallIdentifier_ResolvesAnAlias covers the case the dynamic surface
// exists for.
//
// gitlab_execute_action accepts compatibility aliases as well as canonical ids,
// so a resolver that understood only canonical ids would silently drop exactly
// the calls where a model reached for a name that used to be right. Those are
// the ones worth seeing in a trace.
func TestNewCallIdentifier_ResolvesAnAlias(t *testing.T) {
	catalog := buildTestCatalog(t)
	identifier := NewCallIdentifier(catalog, config.ToolSurfaceDynamic)

	var alias, wantID string
	for _, action := range catalog.Actions() {
		if len(action.Aliases) > 0 {
			alias, wantID = action.Aliases[0], string(action.ID)
			break
		}
	}
	if alias == "" {
		t.Skip("no action in the catalog declares an alias")
	}

	identity, ok := identifier.Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": alias}))
	if !ok {
		t.Fatalf("alias %q resolved to nothing; a model using it would produce an unattributed span", alias)
	}
	if identity.ActionID != wantID {
		t.Errorf("alias %q resolved to %q, want %q", alias, identity.ActionID, wantID)
	}
}

// TestNewCallIdentifier_EveryCanonicalIDResolvesToItself sweeps the real
// catalog, which is the assertion worth having about today's data: roughly a
// thousand ids, every one of which a model may send to gitlab_execute_action.
//
// It does NOT exercise the alias guard, and saying so matters. No action in the
// current catalog declares an alias colliding with another action's canonical
// id, so this test would keep passing if that guard were deleted. The test
// below is the one that fails when it is.
func TestNewCallIdentifier_EveryCanonicalIDResolvesToItself(t *testing.T) {
	catalog := buildTestCatalog(t)
	identifier := NewCallIdentifier(catalog, config.ToolSurfaceDynamic)

	for _, action := range catalog.Actions() {
		id := string(action.ID)
		identity, ok := identifier.Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": id}))
		if !ok {
			t.Errorf("canonical id %q resolved to nothing", id)
			continue
		}
		if identity.ActionID != id {
			t.Errorf("canonical id %q resolved to %q", id, identity.ActionID)
		}
	}
}

// TestNewCallIdentifier_AliasNeverShadowsACanonicalID builds the collision the
// real catalog does not currently contain.
//
// Nothing forbids one action from declaring an alias that is another action's
// canonical id, and if that ever happens the map-building order decides which
// one wins, which is a coin flip dressed as behavior. A synthetic catalog is
// the only way to assert the rule: against real data the guard is invisible.
//
// Both orderings are asserted, because a guard that only worked one way round
// would pass the first case and still be wrong.
func TestNewCallIdentifier_AliasNeverShadowsACanonicalID(t *testing.T) {
	victim := actioncatalog.Action{ID: "issue.close", Domain: "issue", Name: "close"}
	shadower := actioncatalog.Action{
		ID:      "merge_request.close",
		Domain:  "merge_request",
		Name:    "close",
		Aliases: []string{"issue.close"},
	}

	for _, tc := range []struct {
		name    string
		actions []actioncatalog.Action
	}{
		{name: "canonical id first", actions: []actioncatalog.Action{victim, shadower}},
		{name: "alias first", actions: []actioncatalog.Action{shadower, victim}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identifier := newCallIdentifier(tc.actions, config.ToolSurfaceDynamic)

			identity, ok := identifier.Identify("gitlab_execute_action", rawArgs(t, map[string]any{"action": "issue.close"}))
			if !ok {
				t.Fatal("issue.close resolved to nothing")
			}
			if identity.ActionID != "issue.close" {
				t.Errorf("issue.close resolved to %q; another action's alias shadowed a canonical id", identity.ActionID)
			}
		})
	}
}

// TestNewCallIdentifier_NilCatalogIsUsable pins the degradation. Forgetting to
// wire a catalog must cost the action attribute, not the process: this runs on
// every tool call, and a nil dereference there would be a crash on the happy
// path.
func TestNewCallIdentifier_NilCatalogIsUsable(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceIndividual, config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(surface, func(t *testing.T) {
			identifier := NewCallIdentifier(nil, surface)
			if identity, ok := identifier.Identify("gitlab_issue_list", nil); ok {
				t.Errorf("a nil catalog resolved something on %s: %+v", surface, identity)
			}
		})
	}
}

// buildTestCatalog returns the real canonical catalog, so these tests fail when
// the catalog changes shape rather than when a fixture drifts from it.
func buildTestCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{})
	if err != nil {
		t.Fatalf("building the action catalog: %v", err)
	}
	return catalog
}

var _ mcpotel.CallIdentifier = NewCallIdentifier(nil, config.ToolSurfaceDynamic)

// TestNewCallIdentifier_MetaCallWithoutAnAction_StillNamesTheDomain covers a
// dispatcher call whose arguments name no action.
//
// It happens whenever a model calls a meta-tool with an incomplete argument
// object, which is the shape a retry loop produces. The domain is still true —
// the tool name proves it — and recording it is what keeps such a call visible
// on a dashboard grouped by domain, rather than dropping out of the telemetry
// as though it had never arrived.
func TestNewCallIdentifier_MetaCallWithoutAnAction_StillNamesTheDomain(t *testing.T) {
	t.Parallel()

	identifier := NewCallIdentifier(buildTestCatalog(t), config.ToolSurfaceMeta)

	tests := []struct {
		name      string
		arguments any
	}{
		{name: "no arguments at all", arguments: nil},
		{name: "an argument map without an action", arguments: map[string]any{"project_id": "42"}},
		{name: "an action that is not a string", arguments: map[string]any{"action": 42}},
		{name: "an action the catalog does not have", arguments: map[string]any{"action": "invented"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			identity, ok := identifier.Identify("gitlab_issue", tt.arguments)

			if !ok {
				t.Fatal("the call resolved to nothing; a dashboard grouped by domain would lose it entirely")
			}
			if identity.Domain != "issue" {
				t.Errorf("domain = %q, want the domain the tool name proves", identity.Domain)
			}
			if identity.ActionID != "" {
				t.Errorf("action = %q, want none recorded for a call that named none", identity.ActionID)
			}
		})
	}
}
