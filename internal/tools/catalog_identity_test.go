package tools

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// TestNewCallIdentifier_ResolvesEverySurface is the whole reason this resolver
// exists, so it asserts all three shapes against the real catalog rather than a
// fixture.
//
// The tool name is the operation on exactly one surface. On the other two it is
// either a domain that needs the action argument to mean anything, or, on the
// default surface, one of two names covering roughly a thousand operations.
// Instrumentation keyed on the tool name alone would record nothing useful for
// the deployment shape most people run.
func TestNewCallIdentifier_ResolvesEverySurface(t *testing.T) {
	catalog := buildTestCatalog(t)
	identifier := NewCallIdentifier(catalog)

	tests := []struct {
		name       string
		tool       string
		arguments  any
		wantAction string
		wantDomain string
		wantOK     bool
	}{
		{
			name:       "individual: the declared tool name is looked up",
			tool:       "gitlab_issue_list",
			arguments:  map[string]any{},
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:       "meta: the group supplies the domain the action lacks",
			tool:       "gitlab_issue",
			arguments:  map[string]any{"action": "list"},
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:       "dynamic: the argument is already canonical",
			tool:       "gitlab_execute_action",
			arguments:  map[string]any{"action": "issue.list"},
			wantAction: "issue.list",
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:      "a standalone tool belongs to no action",
			tool:      "gitlab_discover_project",
			arguments: map[string]any{},
			wantOK:    false,
		},
		{
			name:      "an invented tool resolves to nothing rather than guessing",
			tool:      "gitlab_not_a_tool",
			arguments: map[string]any{"action": "list"},
			wantOK:    false,
		},
		{
			name:       "meta with an invented action keeps the domain, which is still true",
			tool:       "gitlab_issue",
			arguments:  map[string]any{"action": "teleport"},
			wantDomain: "issue",
			wantOK:     true,
		},
		{
			name:      "arguments that are not a map yield nothing, not a panic",
			tool:      "gitlab_execute_action",
			arguments: "issue.list",
			wantOK:    false,
		},
		{
			name:      "nil arguments yield nothing",
			tool:      "gitlab_execute_action",
			arguments: nil,
			wantOK:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			identity, ok := identifier.Identify(tc.tool, tc.arguments)
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

// TestNewCallIdentifier_ResolvesAnAlias covers the case the dynamic surface
// exists for.
//
// gitlab_execute_action accepts compatibility aliases as well as canonical ids,
// so a resolver that understood only canonical ids would silently drop exactly
// the calls where a model reached for a name that used to be right. Those are
// the ones worth seeing in a trace.
func TestNewCallIdentifier_ResolvesAnAlias(t *testing.T) {
	catalog := buildTestCatalog(t)
	identifier := NewCallIdentifier(catalog)

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

	identity, ok := identifier.Identify("gitlab_execute_action", map[string]any{"action": alias})
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
	identifier := NewCallIdentifier(catalog)

	for _, action := range catalog.Actions() {
		id := string(action.ID)
		identity, ok := identifier.Identify("gitlab_execute_action", map[string]any{"action": id})
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
// The two actions are ordered so that the alias is inserted after the canonical
// id it collides with, which is the case the guard exists for. The reverse
// order is asserted too, because a guard that only worked one way round would
// pass the first case and still be wrong.
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
			identifier := newCallIdentifier(tc.actions)

			identity, ok := identifier.Identify("gitlab_execute_action", map[string]any{"action": "issue.close"})
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
	identifier := NewCallIdentifier(nil)

	if identity, ok := identifier.Identify("gitlab_issue_list", map[string]any{}); ok {
		t.Errorf("a nil catalog resolved something: %+v", identity)
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

var _ mcpotel.CallIdentifier = NewCallIdentifier(nil)
