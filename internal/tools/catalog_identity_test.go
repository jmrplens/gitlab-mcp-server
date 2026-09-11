package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	cached, ok := sharedIdentifiers.Peek(key)
	if !ok {
		t.Fatal("NewCallIdentifier(shared catalog) left nothing in the cache")
	}
	second := NewCallIdentifier(shared.SharedOrigin().BindTo(nil), config.ToolSurfaceDynamic)
	for name, identifier := range map[string]mcpotel.CallIdentifier{"first": first, "second": second, "cached": cached} {
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
	if _, leaked := sharedIdentifiers.Peek(identifierKey{origin: private, surface: config.ToolSurfaceMeta}); leaked {
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

// TestIdentifierActions_IndividualSurfaceReadsRegistrationOrder covers the one
// surface whose resolver cannot read the catalog's own order.
//
// An individual tool name can be declared by more than one action, registration
// binds it to the first one it visits, and that walk is group by group in tool
// name order. The catalog's own action list is sorted by canonical id instead,
// which is a different order whenever a group's tool name and its domain do not
// sort alike. Reading the wrong one names an action whose handler never ran
// under that name, which is how every gitlab_commit_list call was once recorded
// as repository.file_history.
func TestIdentifierActions_IndividualSurfaceReadsRegistrationOrder(t *testing.T) {
	catalog := registrationOrderTestCatalog(t)

	if got := actionIDsInOrder(identifierActions(catalog, config.ToolSurfaceIndividual)); !slices.Equal(got, []string{"zeta.list", "alpha.list"}) {
		t.Errorf("individual order = %v, want the registration order [zeta.list alpha.list]", got)
	}
	for _, surface := range []string{config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(surface, func(t *testing.T) {
			if got := actionIDsInOrder(identifierActions(catalog, surface)); !slices.Equal(got, []string{"alpha.list", "zeta.list"}) {
				t.Errorf("%s order = %v, want the catalog's own order [alpha.list zeta.list]", surface, got)
			}
		})
	}

	registered := registeredIndividualTools(t, catalog)
	identity, ok := NewCallIdentifier(catalog, config.ToolSurfaceIndividual).Identify(sharedOrderToolName, nil)
	if !ok {
		t.Fatalf("%s resolved to nothing", sharedOrderToolName)
	}
	action, found := catalog.Action(actioncatalog.ActionID(identity.ActionID))
	if !found {
		t.Fatalf("resolved action %q is not in the catalog", identity.ActionID)
	}
	if got, want := action.IndividualTool.Description, registered[sharedOrderToolName].Description; got != want {
		t.Fatalf("resolved %s (%q), but registration served %q", identity.ActionID, got, want)
	}
}

// sharedOrderToolName is the individual tool name both groups of
// [registrationOrderTestCatalog] declare, so which action owns it is decided by
// the order alone.
const sharedOrderToolName = "gitlab_test_order_shared"

// registrationOrderTestCatalog returns two groups whose tool names sort the
// opposite way to their domains, so the registration walk and the catalog's
// canonical order disagree about which action is first.
func registrationOrderTestCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	for _, declared := range []struct {
		toolName    string
		domain      string
		description string
	}{
		{toolName: "gitlab_test_order_first", domain: "zeta", description: "Zeta."},
		{toolName: "gitlab_test_order_second", domain: "alpha", description: "Alpha."},
	} {
		spec := toolutil.NewActionSpec("list", toolutil.RouteAction(nil,
			func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}), toolutil.ActionSpecOptions{
			ReadOnly:       true,
			OwnerPackage:   "tools",
			IndividualTool: toolutil.IndividualToolSpec{Name: sharedOrderToolName, Title: "Shared", Description: declared.description},
		})
		group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
			ToolName:     declared.toolName,
			BaseDomain:   declared.domain,
			Title:        "Order probe",
			Description:  "Order probe group.",
			OwnerPackage: "tools",
			SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		}, []toolutil.ActionSpec{spec})
		if err != nil {
			t.Fatalf("GroupFromSpecs(%s) error = %v", declared.toolName, err)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			t.Fatalf("AddGroup(%s) error = %v", declared.toolName, addErr)
		}
	}
	return catalog
}

// actionIDsInOrder lists canonical action ids in the order they were given.
func actionIDsInOrder(actions []actioncatalog.Action) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, string(action.ID))
	}
	return ids
}

// TestNewCallIdentifier_ActionMissingEitherName_ClaimsNoTool covers the guard
// that decides which tool names the meta index answers for.
//
// An action indexes its tool name only when it also carries a domain, because
// the pair is what names a catalog action: an entry with one half missing would
// make the resolver claim a call it can say nothing about, and a dashboard
// grouped by domain would gain a row named by the empty string. Every action a
// catalog builds carries both, so only a hand-built one reaches this, which is
// why the resolver can be built from a plain slice.
func TestNewCallIdentifier_ActionMissingEitherName_ClaimsNoTool(t *testing.T) {
	cases := []struct {
		name   string
		action actioncatalog.Action
		tool   string
	}{
		{
			name:   "an action with a tool name and no domain",
			action: actioncatalog.Action{ID: "ghost.list", Name: "list", ToolName: "gitlab_ghost"},
			tool:   "gitlab_ghost",
		},
		{
			name:   "an action with a domain and no tool name",
			action: actioncatalog.Action{ID: "ghost.list", Name: "list", Domain: "ghost"},
			tool:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identifier := newCallIdentifier([]actioncatalog.Action{tc.action}, config.ToolSurfaceMeta)

			if identity, ok := identifier.Identify(tc.tool, rawArgs(t, map[string]any{"action": "list"})); ok {
				t.Errorf("Identify(%q) = %+v, true; want a tool the index cannot name to resolve to nothing", tc.tool, identity)
			}
			dispatch, isDispatcher := identifier.(mcpotel.DispatchIdentifier)
			if !isDispatcher {
				t.Fatalf("the meta resolver is a %T, which names no dispatched route", identifier)
			}
			if identity, ok := dispatch.IdentifyDispatch(tc.tool, "list"); ok {
				t.Errorf("IdentifyDispatch(%q) = %+v, true; want a tool the index cannot name to resolve to nothing", tc.tool, identity)
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

// TestNewCallIdentifier_IndividualNameBelongsToTheRegisteredAction verifies
// that when two actions declare one individual tool name, the resolver names
// the action whose projection registration kept.
//
// alpha is declared before beta and sorts before it, so registration keeps
// alpha while a resolver reading the actions by canonical ID and keeping the
// last would name beta. The two carry different descriptions, which is how the
// test reads, from the served tool list, which of them registration kept.
func TestNewCallIdentifier_IndividualNameBelongsToTheRegisteredAction(t *testing.T) {
	spec := func(actionName, description string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(actionName, toolutil.RouteAction(nil,
			func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}), toolutil.ActionSpecOptions{
			ReadOnly:       true,
			OwnerPackage:   "tools",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_shared", Title: "Shared", Description: description},
		})
	}
	catalog := testIndividualCatalog(t, spec("alpha", "Alpha."), spec("beta", "Beta."))

	registered := registeredIndividualTools(t, catalog)
	identity, ok := NewCallIdentifier(catalog, config.ToolSurfaceIndividual).Identify("gitlab_test_shared", nil)
	if !ok {
		t.Fatal("gitlab_test_shared resolved to nothing")
	}
	action, found := catalog.Action(actioncatalog.ActionID(identity.ActionID))
	if !found {
		t.Fatalf("resolved action %q is not in the catalog", identity.ActionID)
	}
	if got, want := action.IndividualTool.Description, registered["gitlab_test_shared"].Description; got != want {
		t.Fatalf("resolved %s (%q), but registration served %q", identity.ActionID, got, want)
	}
}

// TestNewCallIdentifier_TelemetryNamesTheRouteTheDispatcherRan verifies, with
// the real catalog, the real dispatchers and the real middleware, that a call
// the dispatcher rewrites is recorded under the action that ran.
//
// gitlab_environment with action get and an environment name runs
// protected_get, on the meta surface and on the dynamic one, which re-enters
// the same meta handler. The middleware predicts environment.get from the
// arguments before anything runs; the span has to end up naming
// environment.protected_get.
func TestNewCallIdentifier_TelemetryNamesTheRouteTheDispatcherRan(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"name":"production"}`)
	}))
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	// A name where get expects a numeric id is what sends get to
	// protected_get, and unlike an environment parameter it passes the
	// dynamic surface's schema check for environment.get.
	params := map[string]any{"project_id": "1", "environment_id": "production"}
	metaHandler := toolutil.MakeMetaHandler("gitlab_environment", catalog.ActionMaps()["gitlab_environment"], markdownForResult)
	registry := dynamic.NewRegistryFromCatalog(catalog)

	memberParams := map[string]any{"project_id": "1", "user_id": 5}
	tests := map[string]struct {
		surface    string
		tool       string
		action     string
		params     map[string]any
		wantAction string
		run        func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		"meta": {
			surface: config.ToolSurfaceMeta, tool: "gitlab_environment", action: "get", params: params, wantAction: "environment.protected_get",
			run: func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, _, runErr := metaHandler(ctx, req, MetaToolInput{Action: "get", Params: params})
				return result, runErr
			},
		},
		"dynamic": {
			surface: config.ToolSurfaceDynamic, tool: dynamic.ExecuteActionToolName, action: "environment.get", params: params, wantAction: "environment.protected_get",
			run: func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, _, runErr := registry.Execute(ctx, req, dynamic.ExecuteInput{Action: "environment.get", Params: params})
				return result, runErr
			},
		},
		// A compatibility alias the argument-based identifier does not know,
		// for a destructive action sent without confirm: gitlab_execute_action
		// refuses it before any meta handler runs.
		"dynamic refusal of a compatibility alias": {
			surface: config.ToolSurfaceDynamic, tool: dynamic.ExecuteActionToolName, action: "project.member_remove", params: memberParams, wantAction: "project.member_delete",
			run: func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, _, runErr := registry.Execute(ctx, req, dynamic.ExecuteInput{Action: "project.member_remove", Params: memberParams})
				return result, runErr
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := recordSpans(t)
			handler := mcpotel.Middleware(mcpotel.Options{Identifier: NewCallIdentifier(catalog, tc.surface), Surface: tc.surface})(
				func(ctx context.Context, _ string, req mcp.Request) (mcp.Result, error) {
					return tc.run(ctx, req.(*mcp.CallToolRequest))
				},
			)
			arguments, marshalErr := json.Marshal(map[string]any{"action": tc.action, "params": tc.params})
			if marshalErr != nil {
				t.Fatalf("json.Marshal() error = %v", marshalErr)
			}
			req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: tc.tool, Arguments: arguments}}
			if _, callErr := handler(context.Background(), "tools/call", req); callErr != nil {
				t.Fatalf("tools/call error = %v", callErr)
			}

			// The GitLab request the route makes records a client span of
			// its own; the tools/call span is the server one.
			var server []sdktrace.ReadOnlySpan
			for _, span := range recorder.Ended() {
				if span.SpanKind() == trace.SpanKindServer {
					server = append(server, span)
				}
			}
			if len(server) != 1 {
				t.Fatalf("recorded %d server spans, want 1", len(server))
			}
			if got := spanString(server[0], mcpotel.AttrActionID); got != tc.wantAction {
				t.Errorf("span action = %q, want %s, the route the dispatcher chose", got, tc.wantAction)
			}
		})
	}
}

// recordSpans installs a real tracer provider that keeps finished spans in
// memory for the rest of the test, restoring the global one afterwards.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})
	return recorder
}

// spanString reads one string attribute off a recorded span, empty when absent.
func spanString(span sdktrace.ReadOnlySpan, key attribute.Key) string {
	for _, kv := range span.Attributes() {
		if kv.Key == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

// ambiguousIndividualToolOwners names every individual tool that more than one
// action declares, with the action registration binds it to. The siblings are
// deliberate: each is one handler projected under a second meta name, and the
// individual surface serves it once.
var ambiguousIndividualToolOwners = map[string]string{
	// repository.file_history is the same commits.List under a path-scoped name.
	"gitlab_commit_list": "repository.commit_list",
	// issue.list_group is the same issues.ListGroup; the group surface owns
	// the individual tool's description.
	"gitlab_issue_list_group": "group.issues",
	// user.me is the same users.Current under a friendlier name.
	"gitlab_user_current": "user.current",
}

// TestNewCallIdentifier_AmbiguousIndividualNamesResolveToTheirOwner verifies,
// on the real catalog and every licensing tier, that each individual tool name
// several actions declare resolves to the action registration binds it to.
//
// The siblings project identical tools, so nothing a client can see tells them
// apart and the served tool list cannot be the oracle here, as it is for the
// synthetic catalog above: the owners are declared instead. A name declared
// twice that is missing from the declaration fails, and so does a declaration
// no tier needs any more, so a new sibling is a decision rather than a
// telemetry label chosen by sort order.
func TestNewCallIdentifier_AmbiguousIndividualNamesResolveToTheirOwner(t *testing.T) {
	needed := make(map[string]bool)
	for _, tier := range []edition.Tier{edition.Free, edition.Premium, edition.Ultimate} {
		t.Run(tier.String(), func(t *testing.T) {
			for _, name := range checkAmbiguousIndividualOwners(t, tier) {
				needed[name] = true
			}
		})
	}
	for name := range ambiguousIndividualToolOwners {
		if !needed[name] {
			t.Errorf("ambiguousIndividualToolOwners declares %s, which no tier declares twice any more", name)
		}
	}
}

// checkAmbiguousIndividualOwners resolves every individual tool name the tier's
// catalog declares more than once, reports each that does not resolve to its
// declared owner, and returns the names it found declared more than once.
func checkAmbiguousIndividualOwners(t *testing.T, tier edition.Tier) []string {
	t.Helper()
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(%s) error = %v", tier, err)
	}
	identifier := NewCallIdentifier(catalog, config.ToolSurfaceIndividual)
	var ambiguous []string
	for name, count := range declaredIndividualNames(catalog) {
		if count < 2 {
			continue
		}
		ambiguous = append(ambiguous, name)
		owner, known := ambiguousIndividualToolOwners[name]
		if !known {
			t.Errorf("%s is declared by %d actions and has no owner in ambiguousIndividualToolOwners", name, count)
			continue
		}
		if identity, ok := identifier.Identify(name, nil); !ok || identity.ActionID != owner {
			t.Errorf("%s resolved to %q, want %q", name, identity.ActionID, owner)
		}
	}
	return ambiguous
}

// declaredIndividualNames counts the actions that declare each individual tool
// name, trimmed as registration trims it.
func declaredIndividualNames(catalog *actioncatalog.Catalog) map[string]int {
	declared := make(map[string]int)
	for _, action := range catalog.Actions() {
		if name := strings.TrimSpace(action.IndividualTool.Name); name != "" {
			declared[name]++
		}
	}
	return declared
}

// registeredIndividualTools registers catalog on a fresh server with the
// options cmd/server registers the individual surface with, and returns the
// served tools by name.
func registeredIndividualTools(t *testing.T, catalog *actioncatalog.Catalog) map[string]*mcp.Tool {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000, SchemaCache: testSchemaCache})
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{IncludeStandaloneUtilities: true})
	tools := make(map[string]*mcp.Tool)
	for _, tool := range listToolsFromServer(t, server) {
		tools[tool.Name] = tool
	}
	return tools
}
