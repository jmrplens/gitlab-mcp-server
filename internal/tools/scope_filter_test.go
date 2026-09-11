// scope_filter_test.go contains unit tests for PAT scope-based tool filtering.
package tools

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestFilterScopeFilteredCatalog_AdminActionsRemovedOnEverySurface is the
// regression for the surface the scope filter used to miss.
//
// Until 3.0.0 the individual surface was filtered by a pass over registered
// tool names, and the filter's keys are meta-tool group names: nothing on that
// surface is ever called gitlab_admin, so the pass matched nothing and all 92
// admin tools stayed listed for a token with no admin_mode. Filtering the
// catalog instead reaches every surface at once, because all three are
// projected from it, so the assertions here are made per surface from one
// filtered catalog: the actions themselves (which is the whole of what the
// dynamic surface registers), the meta dispatcher, and every individual tool
// the admin group projects.
//
// The control rows matter as much as the removals. gitlab_project is not in
// [MetaToolScopes], and a filter that removed it would be a far worse defect
// than the one being fixed.
func TestFilterScopeFilteredCatalog_AdminActionsRemovedOnEverySurface(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

	admin, ok := catalog.Group("gitlab_admin")
	if !ok {
		t.Fatal("source catalog has no gitlab_admin group, so this test proves nothing")
	}
	adminTools := individualToolNamesOfGroup(t, admin)
	adminActions := actionIDsOfGroup(admin)

	// A token with read_api and no admin_mode: the credential the defect was
	// found with, and the one every read-only OAuth application presents.
	scoped, err := FilterScopeFilteredCatalog(catalog, []string{"read_api"})
	if err != nil {
		t.Fatalf("FilterScopeFilteredCatalog() error = %v", err)
	}

	t.Run("dynamic: the catalog carries none of the admin actions", func(t *testing.T) {
		for _, id := range adminActions {
			if _, found := scoped.Action(id); found {
				t.Errorf("action %s survived the scope filter", id)
			}
		}
		if _, found := scoped.Action("project.get"); !found {
			t.Error("project.get was removed, and no scope gates it")
		}
	})

	t.Run("meta: the admin dispatcher is not registered", func(t *testing.T) {
		names := stringSet(registeredNamesForCatalog(t, scoped, false))
		if _, listed := names["gitlab_admin"]; listed {
			t.Error("gitlab_admin is still registered for a token with no admin_mode")
		}
		if _, listed := names["gitlab_project"]; !listed {
			t.Error("gitlab_project is not registered, and no scope gates it")
		}
	})

	t.Run("individual: no tool of the admin group is registered", func(t *testing.T) {
		names := stringSet(registeredNamesForCatalog(t, scoped, true))
		for _, tool := range adminTools {
			if _, listed := names[tool]; listed {
				t.Errorf("%s is still registered for a token with no admin_mode", tool)
			}
		}
		if _, listed := names["gitlab_project_get"]; !listed {
			t.Error("gitlab_project_get is not registered, and no scope gates it")
		}
	})
}

// TestFilterScopeFilteredCatalog_ScopeCombinations_DecideRemoval covers the
// three answers the filter gives a scope list, which are not two: a nil list
// means detection was unavailable and removes nothing, an empty list means the
// token was read and carries nothing and removes every gated group, and a
// populated list is judged scope by scope.
//
// Told apart because the wrong answer is silent in one direction: treating an
// unknown list as empty would remove five domains from a deployment whose
// token detection failed, with nothing said about why.
func TestFilterScopeFilteredCatalog_ScopeCombinations_DecideRemoval(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})
	gated := slices.Sorted(maps.Keys(MetaToolScopes))

	cases := []struct {
		name        string
		scopes      []string
		wantRemoved []string
	}{
		{name: "detection unavailable removes nothing", scopes: nil},
		{name: "every required scope present removes nothing", scopes: []string{"api", "admin_mode", "read_api", "read_user"}},
		{name: "a token with no scopes at all loses every gated group", scopes: []string{}, wantRemoved: gated},
		{name: "a read_api token loses every gated group", scopes: []string{"read_api"}, wantRemoved: gated},
		{name: "an api token without admin_mode loses every gated group", scopes: []string{"api", "read_api"}, wantRemoved: gated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filtered, filterErr := FilterScopeFilteredCatalog(catalog, tc.scopes)
			if filterErr != nil {
				t.Fatalf("FilterScopeFilteredCatalog() error = %v", filterErr)
			}
			for _, group := range tc.wantRemoved {
				if _, found := filtered.Group(group); found {
					t.Errorf("group %s survived scopes %v", group, tc.scopes)
				}
			}
			want := catalog.CountGroups() - len(tc.wantRemoved)
			if got := filtered.CountGroups(); got != want {
				t.Errorf("group count = %d, want %d (source %d minus %d gated)", got, want, catalog.CountGroups(), len(tc.wantRemoved))
			}
		})
	}
}

// individualToolNamesOfGroup returns the individual tool name every action of
// group projects, which is what the individual surface registers and what the
// scope filter's own keys never match.
func individualToolNamesOfGroup(t *testing.T, group actioncatalog.Group) []string {
	t.Helper()
	names := make([]string, 0, len(group.Actions))
	for _, action := range group.Actions {
		if name := action.IndividualTool.Name; name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("group %s projects no individual tool, so nothing can be asserted about that surface", group.ToolName)
	}
	slices.Sort(names)
	return names
}

// actionIDsOfGroup returns the canonical IDs of a group's actions.
func actionIDsOfGroup(group actioncatalog.Group) []actioncatalog.ActionID {
	ids := make([]actioncatalog.ActionID, 0, len(group.Actions))
	for _, action := range group.Actions {
		ids = append(ids, action.ID)
	}
	slices.SortFunc(ids, func(a, b actioncatalog.ActionID) int { return strings.Compare(string(a), string(b)) })
	return ids
}

// registeredNamesForCatalog registers catalog on a fresh server, on the
// individual surface when individual is true and the meta surface otherwise,
// and returns the tool names a client would be served.
func registeredNamesForCatalog(t *testing.T, catalog *actioncatalog.Catalog, individual bool) []string {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000, SchemaCache: testSchemaCache})
	if individual {
		RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{})
	} else {
		RegisterMetaCatalog(server, catalog)
	}
	return toolNames(t, server)
}

// TestFilterScopeFilteredCatalog_MissingAdminMode verifies that catalog-level
// scope filtering removes the same admin-mode groups without mutating the source.
func TestFilterScopeFilteredCatalog_MissingAdminMode(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

	t.Run("source contains admin", func(t *testing.T) {
		if _, ok := catalog.Group("gitlab_admin"); !ok {
			t.Fatal("source catalog missing gitlab_admin")
		}
	})

	t.Run("removes admin and preserves project", func(t *testing.T) {
		filtered, filterErr := FilterScopeFilteredCatalog(catalog, []string{"read_api"})
		if filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", filterErr)
		}
		if _, ok := filtered.Group("gitlab_admin"); ok {
			t.Fatal("filtered catalog still contains gitlab_admin")
		}
		if _, ok := filtered.Group("gitlab_project"); !ok {
			t.Fatal("filtered catalog removed ungated gitlab_project")
		}
	})

	t.Run("source remains unchanged", func(t *testing.T) {
		if _, filterErr := FilterScopeFilteredCatalog(catalog, []string{"read_api"}); filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", filterErr)
		}
		if _, ok := catalog.Group("gitlab_admin"); !ok {
			t.Fatal("source catalog was mutated")
		}
	})

	t.Run("nil scopes return clone", func(t *testing.T) {
		unfiltered, filterErr := FilterScopeFilteredCatalog(catalog, nil)
		if filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog(nil) error = %v", filterErr)
		}
		if unfiltered == catalog {
			t.Fatal("nil token scopes should return a cloned catalog")
		}
		if unfiltered.CountGroups() != catalog.CountGroups() {
			t.Fatalf("nil-scope group count = %d, want %d", unfiltered.CountGroups(), catalog.CountGroups())
		}
	})
}

// TestFilterScopeFilteredCatalog_LogsOnlyTheGroupsItRemoved covers the one line
// an operator gets about a credential narrowing their surface.
//
// It is how a deployment learns that its token, not its configuration, is why a
// domain is missing, so it has to appear when a group was removed and say how
// many; and it must stay silent when the token satisfies every requirement, or
// an operator reading a startup log sees a narrowing that never happened and
// goes looking for a scope that is already there.
func TestFilterScopeFilteredCatalog_LogsOnlyTheGroupsItRemoved(t *testing.T) {
	catalog := scopeGatedTestCatalog(t)

	t.Run("a removed group is named with the count", func(t *testing.T) {
		output := captureSlogOutput(t)

		filtered, err := FilterScopeFilteredCatalog(catalog, []string{"read_api"})
		if err != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", err)
		}
		if _, found := filtered.Group("gitlab_admin"); found {
			t.Fatal("gitlab_admin survived a token with no admin_mode, so this case proves nothing")
		}
		if !strings.Contains(output.String(), `"removed":1`) || !strings.Contains(output.String(), "gitlab_admin") {
			t.Errorf("startup log = %s, want the removed group named and counted", output.String())
		}
	})

	t.Run("a token that satisfies every requirement is not told otherwise", func(t *testing.T) {
		output := captureSlogOutput(t)

		if _, err := FilterScopeFilteredCatalog(catalog, []string{"api", "admin_mode"}); err != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", err)
		}
		if strings.Contains(output.String(), "scope-filtered catalog groups removed") {
			t.Errorf("startup log = %s, want nothing reported when nothing was removed", output.String())
		}
	})
}

// scopeGatedTestCatalog returns a two-group catalog: one group MetaToolScopes
// gates on admin_mode and one it does not name at all, so a filtered run
// removes exactly one of them.
func scopeGatedTestCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	for _, toolName := range []string{"gitlab_admin", "gitlab_scope_filter_fixture"} {
		action := actioncatalog.Action{
			Name:         "list",
			OwnerPackage: "tools",
			Route:        toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}},
		}
		options := actioncatalog.GroupOptions{
			ToolName:     toolName,
			OwnerPackage: "tools",
			SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		}
		if err := catalog.AddAction(toolName, action, options); err != nil {
			t.Fatalf("AddAction(%s) error = %v", toolName, err)
		}
	}
	return catalog
}

// TestFilterScopeFilteredCatalog_NilCatalog verifies scope filtering handles a
// nil source catalog by returning an empty catalog.
//
// The test expects no error, a non-nil result, and zero groups or actions. This
// keeps callers safe when filtering is invoked before catalog construction.
func TestFilterScopeFilteredCatalog_NilCatalog(t *testing.T) {
	filtered, err := FilterScopeFilteredCatalog(nil, []string{"read_api"})
	if err != nil {
		t.Fatalf("FilterScopeFilteredCatalog(nil) error = %v", err)
	}
	if filtered == nil {
		t.Fatal("FilterScopeFilteredCatalog(nil) returned nil catalog")
	}
	if filtered.CountGroups() != 0 || filtered.CountActions() != 0 {
		t.Fatalf("filtered counts = groups %d actions %d, want empty catalog", filtered.CountGroups(), filtered.CountActions())
	}
}

// scopeFilterTestCatalog returns a one-group catalog the scope filter keeps
// whole: its tool name is in no MetaToolScopes entry, so every token scope
// carries it through to the rebuild.
func scopeFilterTestCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	action := actioncatalog.Action{
		Name:         "list",
		OwnerPackage: "tools",
		Route:        toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}},
	}
	options := actioncatalog.GroupOptions{
		ToolName:     "gitlab_scope_filter_fixture",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
	}
	if err := catalog.AddAction(options.ToolName, action, options); err != nil {
		t.Fatalf("AddAction() error: %v", err)
	}
	return catalog
}

// failAddFilteredGroup makes the rebuild step fail and returns the restore. The
// real call re-adds a group the source catalog already normalized under a name
// unique there, so no input reaches the guard; the seam is what lets the guard
// be tested rather than merely trusted.
func failAddFilteredGroup(t *testing.T) func() {
	t.Helper()
	original := addFilteredGroup
	addFilteredGroup = func(*actioncatalog.Catalog, actioncatalog.Group) error {
		return errors.New("group refused")
	}
	return func() { addFilteredGroup = original }
}

// TestFilterScopeFilteredCatalog_RebuildFails_ReportsWhichGroup verifies the
// rebuild failure names the group it stopped at: the catalog it would have
// returned is incomplete, so the caller is given the error instead, and the
// message has to say enough to find the offending group.
func TestFilterScopeFilteredCatalog_RebuildFails_ReportsWhichGroup(t *testing.T) {
	restore := failAddFilteredGroup(t)
	defer restore()

	filtered, err := FilterScopeFilteredCatalog(scopeFilterTestCatalog(t), []string{"api"})

	if err == nil {
		t.Fatal("FilterScopeFilteredCatalog() error = nil, want the rebuild failure")
	}
	if filtered != nil {
		t.Errorf("catalog = %+v, want none when the rebuild failed", filtered)
	}
	if !strings.Contains(err.Error(), "gitlab_scope_filter_fixture") {
		t.Errorf("error = %q, want it to name the group it stopped at", err)
	}
}

// TestCatalogRelevantScopes_EqualComponentsFilterIdentically runs the filter
// over the property that makes the narrowed cache key safe: two token scope
// lists with the same canonical components produce the same filtered catalog
// and the same withheld sets, so serving both from one shared catalog serves
// neither a catalog it did not earn.
//
// It drives FilterActionCatalog rather than comparing keys, because the key
// is only correct in terms of what the filter does with a scope list: the
// ways this equivalence could be broken (a prefix match, a scope-implication
// rule, a rule keyed on absence or on the length of the list, a second
// requirements map) all leave [CatalogFilterKey] and [catalogRelevantScopes]
// looking exactly as they do now. The list of them is beside
// [catalogRelevantScopes].
//
// The last case is what keeps the rest from being vacuous: a list carrying
// none of the required scopes must filter differently from one carrying them
// all, or the filter would be removing nothing and every list would agree.
func TestCatalogRelevantScopes_EqualComponentsFilterIdentically(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})
	required := requiredScopeUniverse(t)
	// Scopes GitLab issues that MetaToolScopes never asks about, so no list
	// below changes its canonical components by carrying them.
	noise := []string{"api", "read_api", "read_user", "read_repository", "write_repository", "read_registry", "write_registry", "create_runner", "manage_runner", "ai_features", "k8s_proxy", "sudo"}
	noise = slices.DeleteFunc(noise, func(scope string) bool { return slices.Contains(required, scope) })
	reversed := slices.Clone(required)
	slices.Reverse(reversed)

	cases := []struct {
		name  string
		left  []string
		right []string
	}{
		{
			name:  "the required scopes, alone and among scopes the filter never reads",
			left:  required,
			right: append(slices.Clone(noise), required...),
		},
		{
			name:  "the same scopes reordered, repeated and interleaved",
			left:  append(slices.Clone(required), required...),
			right: append(append(slices.Clone(reversed), noise...), reversed...),
		},
		{
			name:  "no required scope at all, alone and among the others",
			left:  []string{},
			right: noise,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if left, right := catalogRelevantScopes(tc.left), catalogRelevantScopes(tc.right); !slices.Equal(left, right) {
				t.Fatalf("the two lists have components %v and %v, so this case is not about the equivalence", left, right)
			}
			leftActions, leftWithheld := filterByTokenScopes(t, catalog, tc.left)
			rightActions, rightWithheld := filterByTokenScopes(t, catalog, tc.right)
			if !slices.Equal(leftActions, rightActions) {
				t.Errorf("the two lists kept %d and %d actions (%s), want one filtered catalog for both",
					len(leftActions), len(rightActions), firstDifference(leftActions, rightActions))
			}
			// Compared field by field and reported by count: the withheld
			// lists carry every alias of every removed action, and a failure
			// naming them all would be unreadable.
			if !slices.Equal(leftWithheld.ByTokenScope, rightWithheld.ByTokenScope) {
				t.Errorf("withheld by token scope = %d and %d keys (%s), want the same narrowing reported to both",
					len(leftWithheld.ByTokenScope), len(rightWithheld.ByTokenScope), firstDifference(leftWithheld.ByTokenScope, rightWithheld.ByTokenScope))
			}
			if !slices.Equal(leftWithheld.ByOperator, rightWithheld.ByOperator) || !slices.Equal(leftWithheld.ExcludedByName, rightWithheld.ExcludedByName) {
				t.Errorf("withheld by operator = %d and %d keys, excluded by name = %d and %d keys; want the same for both",
					len(leftWithheld.ByOperator), len(rightWithheld.ByOperator), len(leftWithheld.ExcludedByName), len(rightWithheld.ExcludedByName))
			}
		})
	}

	t.Run("a missing required scope filters differently", func(t *testing.T) {
		withAll, _ := filterByTokenScopes(t, catalog, required)
		withNone, withheld := filterByTokenScopes(t, catalog, []string{})
		if len(withNone) >= len(withAll) {
			t.Fatalf("a token with none of %v kept %d actions and one with all of them kept %d, want strictly fewer", required, len(withNone), len(withAll))
		}
		if len(withheld.ByTokenScope) == 0 {
			t.Error("nothing was reported withheld by token scope, so the equivalence above compares two catalogs the filter never narrowed")
		}
	})
}

// requiredScopeUniverse returns every scope MetaToolScopes requires, sorted
// and deduplicated: the whole of what a scope list's canonical components can
// be drawn from. Read from the map so a new requirement joins the test
// without an edit.
func requiredScopeUniverse(t *testing.T) []string {
	t.Helper()
	seen := map[string]struct{}{}
	for _, scopes := range MetaToolScopes {
		for _, scope := range scopes {
			seen[scope] = struct{}{}
		}
	}
	universe := slices.Sorted(maps.Keys(seen))
	if len(universe) == 0 {
		t.Fatal("MetaToolScopes requires no scope at all, so the equivalence proves nothing")
	}
	return universe
}

// firstDifference names the first position at which two lists disagree, for a
// failure message that has to stay readable over lists thousands of entries
// long.
func firstDifference(left, right []string) string {
	for i := range min(len(left), len(right)) {
		if left[i] != right[i] {
			return fmt.Sprintf("first difference at %d: %q against %q", i, left[i], right[i])
		}
	}
	return fmt.Sprintf("one is a prefix of the other, %d entries longer", max(len(left), len(right))-min(len(left), len(right)))
}

// filterByTokenScopes narrows a catalog for a token carrying scopes and
// returns the action IDs it kept, sorted, together with what the narrowing
// withheld.
func filterByTokenScopes(t *testing.T, catalog *actioncatalog.Catalog, scopes []string) ([]string, WithheldActions) {
	t.Helper()
	filtered, withheld, err := FilterActionCatalog(catalog, &config.ServerConfig{TokenScopes: scopes})
	if err != nil {
		t.Fatalf("FilterActionCatalog(%v) error = %v", scopes, err)
	}
	kept := make([]string, 0, filtered.CountActions())
	for _, action := range filtered.Actions() {
		kept = append(kept, string(action.ID))
	}
	slices.Sort(kept)
	return kept, withheld
}

// TestAllScopesPresent_Scenarios_CorrectResult tests the allScopesPresent helper.
func TestAllScopesPresent_Scenarios_CorrectResult(t *testing.T) {
	tests := []struct {
		name     string
		scopes   map[string]struct{}
		required []string
		want     bool
	}{
		{
			name:     "empty required",
			scopes:   map[string]struct{}{"api": {}},
			required: nil,
			want:     true,
		},
		{
			name:     "all present",
			scopes:   map[string]struct{}{"api": {}, "admin_mode": {}},
			required: []string{"api", "admin_mode"},
			want:     true,
		},
		{
			name:     "one missing",
			scopes:   map[string]struct{}{"api": {}},
			required: []string{"api", "admin_mode"},
			want:     false,
		},
		{
			name:     "all missing",
			scopes:   map[string]struct{}{},
			required: []string{"api"},
			want:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allScopesPresent(tc.scopes, tc.required)
			if got != tc.want {
				t.Errorf("allScopesPresent() = %v, want %v", got, tc.want)
			}
		})
	}
}

// toolNames returns the names of all tools registered on the server.
func toolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}
