package actioncatalog

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestCatalog_FromActionMapsRoundTrip_DeterministicActions verifies that a
// catalog built from legacy route maps lists its actions in canonical ID
// order whatever order the maps iterate in, and that the maps read back out
// of it carry the routes' destructive flags and schemas.
func TestCatalog_FromActionMapsRoundTrip_DeterministicActions(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"list": testRoute(false),
			"get":  testRoute(false),
		},
		"gitlab_issue": {
			"delete": testRoute(true),
		},
	}

	catalog := FromActionMaps(routes)
	if _, err := FromActionMapsWithError(routes); err != nil {
		t.Fatalf("FromActionMapsWithError() error = %v", err)
	}
	if catalog.CountGroups() != 2 {
		t.Fatalf("CountGroups() = %d, want 2", catalog.CountGroups())
	}
	if catalog.CountActions() != 3 {
		t.Fatalf("CountActions() = %d, want 3", catalog.CountActions())
	}

	actions := catalog.Actions()
	gotIDs := []string{string(actions[0].ID), string(actions[1].ID), string(actions[2].ID)}
	wantIDs := []string{"issue.delete", "project.get", "project.list"}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("Actions() IDs = %v, want %v", gotIDs, wantIDs)
	}

	roundTrip := catalog.ActionMaps()
	if !roundTrip["gitlab_issue"]["delete"].Destructive {
		t.Fatal("roundTrip issue.delete Destructive = false, want true")
	}
	if roundTrip["gitlab_project"]["get"].InputSchema == nil {
		t.Fatal("roundTrip project.get InputSchema = nil, want schema")
	}
	if ToActionMaps(nil) != nil {
		t.Fatal("ToActionMaps(nil) != nil")
	}
	if ToActionMaps(catalog)["gitlab_project"]["list"].InputSchema == nil {
		t.Fatal("ToActionMaps(catalog) missing project.list schema")
	}
}

// TestFromActionMapsWithError_InvalidToolName_ReturnsError verifies that
// FromActionMapsWithError rejects empty tool names and returns both an
// error and a non-nil (possibly empty) catalog reflecting partial success.
//
// The test feeds an action map keyed by an empty string into
// FromActionMapsWithError and asserts err != nil plus catalog != nil.
// The catalog is non-nil but contains zero groups because the rejected
// entry is discarded by normalizeGroup before AddGroup runs, not stored
// as a "rejected" entry. This protects the catalog validator from
// silently dropping malformed entries before they reach the action
// registry.
func TestFromActionMapsWithError_InvalidToolName_ReturnsError(t *testing.T) {
	catalog, err := FromActionMapsWithError(map[string]toolutil.ActionMap{
		"": {"get": testRoute(false)},
	})
	if err == nil {
		t.Fatal("FromActionMapsWithError() error = nil, want error")
	}
	if catalog == nil {
		t.Fatal("FromActionMapsWithError() catalog = nil, want partial catalog")
	}
	if got := catalog.CountGroups(); got != 0 {
		t.Fatalf("FromActionMapsWithError() CountGroups() = %d, want 0: the rejected group must not be kept", got)
	}
}

// TestFromActionMaps_InvalidToolName_Panics verifies that the panicking
// constructor refuses a route map keyed by an empty tool name instead of
// returning a catalog without it.
func TestFromActionMaps_InvalidToolName_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("FromActionMaps() did not panic for invalid route map")
		}
	}()

	_ = FromActionMaps(map[string]toolutil.ActionMap{
		"": {"get": testRoute(false)},
	})
}

// TestGroup_SetActionAndActionsInOrder_DefensiveBranches verifies the edges
// of a group's action table: a nameless action is ignored, a repeated name
// replaces the action without a second order entry, a group with no recorded
// order lists its actions sorted, and a stale order entry naming no action
// or naming one twice yields each action once.
func TestGroup_SetActionAndActionsInOrder_DefensiveBranches(t *testing.T) {
	group := Group{ToolName: "gitlab_project"}
	group.SetAction(Action{})
	if len(group.Actions) != 0 || len(group.ActionOrder) != 0 {
		t.Fatalf("SetAction(empty) mutated group = %+v, want empty", group)
	}
	group.SetAction(Action{Name: "list", Route: testRoute(false)})
	group.SetAction(Action{Name: "list", Route: testRoute(true)})
	if len(group.ActionOrder) != 1 || !group.Actions["list"].Route.Destructive {
		t.Fatalf("SetAction(replace) group = %+v, want one destructive list action", group)
	}

	fallbackOrder := Group{ToolName: "gitlab_project", Actions: map[string]Action{
		"z": {Name: "z", Route: testRoute(false)},
		"a": {Name: "a", Route: testRoute(false)},
	}}
	actions := fallbackOrder.ActionsInOrder()
	if len(actions) != 2 || actions[0].Name != "a" || actions[1].Name != "z" {
		t.Fatalf("ActionsInOrder() = %+v, want sorted fallback order", actions)
	}

	withStaleOrder := Group{
		ToolName:    "gitlab_project",
		ActionOrder: []string{"get", "missing", "get"},
		Actions:     map[string]Action{"get": {Name: "get", Route: testRoute(false)}},
	}
	ordered := withStaleOrder.ActionsInOrder()
	if len(ordered) != 1 || ordered[0].Name != "get" {
		t.Fatalf("ActionsInOrder(stale order) = %+v, want only get", ordered)
	}
}

// TestCatalog_Clone_SharesSchemasAndOwnsStructure verifies the copy contract
// of the accessors: a clone's groups, actions and slices are its own, so
// adding to or editing them does not reach the original, while the route
// schema maps are shared, since they are frozen and every server in the
// process reads the same ones.
func TestCatalog_Clone_SharesSchemasAndOwnsStructure(t *testing.T) {
	route := testRoute(false)
	route.Aliases = []string{"show"}
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": route},
	})

	cloned := catalog.Clone()
	clonedGroup, ok := cloned.Group("gitlab_project")
	if !ok {
		t.Fatal("cloned Group(gitlab_project) = false")
	}
	originalGroup, foundOriginal := catalog.Group("gitlab_project")
	if !foundOriginal {
		t.Fatal("original Group(gitlab_project) = false")
	}
	clonedRoute := clonedGroup.Actions["get"].Route
	originalRoute := originalGroup.Actions["get"].Route
	if reflect.ValueOf(clonedRoute.InputSchema).UnsafePointer() != reflect.ValueOf(originalRoute.InputSchema).UnsafePointer() {
		t.Fatal("Clone() copied the input schema, want it shared")
	}
	if reflect.ValueOf(clonedRoute.OutputSchema).UnsafePointer() != reflect.ValueOf(originalRoute.OutputSchema).UnsafePointer() {
		t.Fatal("Clone() copied the output schema, want it shared")
	}

	clonedRoute.Aliases[0] = "changed"
	if originalRoute.Aliases[0] != "show" {
		t.Fatalf("original aliases = %v, want unchanged by an edit to the clone's", originalRoute.Aliases)
	}
	if err := cloned.AddAction("gitlab_project", Action{Name: "list", Route: testRoute(false)}); err != nil {
		t.Fatalf("AddAction() error = %v", err)
	}
	if catalog.CountActions() != 1 {
		t.Fatalf("original CountActions() = %d after adding to the clone, want 1", catalog.CountActions())
	}
	if cloned.SharedOrigin() != nil {
		t.Fatal("Clone() carried a shared origin, want none for a catalog that can be added to")
	}
}

// TestCatalog_AddGroupAndAddActionValidateDuplicates verifies what AddGroup
// and AddAction refuse: a group already present, an action claiming another
// tool, an ID that is not its domain and name, a nil receiver, an empty tool
// name, more than one options value, two actions sharing an ID, and an
// action with no name; and that a refused AddAction leaves the count as it
// was.
func TestCatalog_AddGroupAndAddActionValidateDuplicates(t *testing.T) {
	catalog := NewCatalog()
	group := NewGroup(GroupOptions{ToolName: "gitlab_project"})
	group.SetAction(Action{Name: "get", Route: testRoute(false)})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if err := catalog.AddGroup(group); err == nil {
		t.Fatal("AddGroup(duplicate) error = nil, want error")
	}
	if err := catalog.AddAction("gitlab_project", Action{Name: "list", Route: testRoute(false)}); err != nil {
		t.Fatalf("AddAction() error = %v", err)
	}
	if catalog.CountActions() != 2 {
		t.Fatalf("CountActions() = %d, want 2", catalog.CountActions())
	}
	if err := catalog.AddAction("gitlab_project", Action{Name: "bad", ToolName: "gitlab_issue", Route: testRoute(false)}); err == nil {
		t.Fatal("AddAction(invalid) error = nil, want error")
	}
	if err := catalog.AddAction("gitlab_project", Action{Name: "bad_id", ID: "issue.delete", Route: testRoute(false)}); err == nil {
		t.Fatal("AddAction(non-canonical ID) error = nil, want error")
	}
	if catalog.CountActions() != 2 {
		t.Fatalf("CountActions() after failed AddAction = %d, want 2", catalog.CountActions())
	}
	var nilCatalog *Catalog
	if err := nilCatalog.AddGroup(group); err == nil {
		t.Fatal("nil Catalog AddGroup() error = nil, want error")
	}
	if err := nilCatalog.AddAction("gitlab_project", Action{Name: "get", Route: testRoute(false)}); err == nil {
		t.Fatal("nil Catalog AddAction() error = nil, want error")
	}
	if err := catalog.AddAction("", Action{Name: "get", Route: testRoute(false)}); err == nil {
		t.Fatal("AddAction(empty tool) error = nil, want error")
	}
	if err := catalog.AddAction("gitlab_project", Action{Name: "owned", Route: testRoute(false)}, GroupOptions{}, GroupOptions{}); err == nil {
		t.Fatal("AddAction(multiple group options) error = nil, want error")
	}

	duplicateID := NewGroup(GroupOptions{ToolName: "gitlab_duplicate"})
	duplicateID.SetAction(Action{Name: "one", ID: "duplicate.id", Route: testRoute(false)})
	duplicateID.SetAction(Action{Name: "two", ID: "duplicate.id", Route: testRoute(false)})
	if err := NewCatalog().AddGroup(duplicateID); err == nil {
		t.Fatal("AddGroup(duplicate action ID) error = nil, want error")
	}

	invalidAction := Group{ToolName: "gitlab_project", Actions: map[string]Action{"": {Route: testRoute(false)}}}
	if err := NewCatalog().AddGroup(invalidAction); err == nil {
		t.Fatal("AddGroup(empty action name) error = nil, want error")
	}
}

// TestCatalog_AddGroupInitializesZeroValueCatalog verifies that a zero-value
// Catalog accepts its first group without NewCatalog having run, and that
// two groups sharing a base domain cannot both hold an action of one name,
// since the action ID would be the same.
func TestCatalog_AddGroupInitializesZeroValueCatalog(t *testing.T) {
	var catalog Catalog
	group := NewGroup(GroupOptions{ToolName: "gitlab_project", BaseDomain: "shared"})
	group.SetAction(Action{Name: "get", Route: testRoute(false)})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if catalog.CountGroups() != 1 || catalog.CountActions() != 1 {
		t.Fatalf("counts = groups %d actions %d, want 1/1", catalog.CountGroups(), catalog.CountActions())
	}

	colliding := NewGroup(GroupOptions{ToolName: "gitlab_group", BaseDomain: "shared"})
	colliding.SetAction(Action{Name: "get", Route: testRoute(false)})
	if err := catalog.AddGroup(colliding); err == nil {
		t.Fatal("AddGroup(cross-group duplicate action ID) error = nil, want error")
	}
}

// TestMustAddCatalogGroup_PanicsOnInvariantDrift verifies that the helper the
// clone and filter paths add groups through panics when AddGroup refuses,
// since a group those paths handle was validated when the source catalog
// took it and a refusal now is an invariant that drifted.
func TestMustAddCatalogGroup_PanicsOnInvariantDrift(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("mustAddCatalogGroup() did not panic")
		}
	}()
	mustAddCatalogGroup(nil, Group{}, "test operation")
}

// TestCatalog_AddActionCreatesGroupWithoutOptions verifies that AddAction
// can synthesize a brand new group on demand when the caller does not
// provide any GroupOptions. This exercises the empty-options branch of
// newAddActionGroup (the `len(groupOptions) == 0` path returns a default
// NewGroup(opts)) which is the only uncovered line in that helper.
func TestCatalog_AddActionCreatesGroupWithoutOptions(t *testing.T) {
	catalog := NewCatalog()
	if err := catalog.AddAction("gitlab_no_opts", Action{Name: "list", Route: testRoute(false)}); err != nil {
		t.Fatalf("AddAction() error = %v", err)
	}
	group, ok := catalog.Group("gitlab_no_opts")
	if !ok {
		t.Fatal("Group(gitlab_no_opts) = false, want true")
	}
	if group.ToolName != "gitlab_no_opts" {
		t.Fatalf("group.ToolName = %q, want gitlab_no_opts", group.ToolName)
	}
	action, ok := group.Actions["list"]
	if !ok {
		t.Fatal("expected synthesized group to contain the 'list' action")
	}
	if action.ToolName != "gitlab_no_opts" {
		t.Fatalf("action.ToolName = %q, want gitlab_no_opts", action.ToolName)
	}
}

// TestCatalog_AddGroup_SameNameCollisionRejectedBeforeDuplicateIDCheck
// documents why a SAME-NAME collision never reaches the intra-group
// duplicate action ID branch in AddGroup (the "duplicate action id %q"
// return at the top of the ActionsInOrder loop): two layers of defense sit
// above it for that specific case.
//   - normalizeAction requires explicit IDs to match `<domain>.<name>`,
//     so two actions sharing both name and an explicit ID must also agree
//     on domain, or the second is rejected as a normalization mismatch
//     before AddGroup's duplicate-ID check ever runs.
//   - ActionsInOrder deduplicates by map key before AddGroup sees the
//     slice, so two map entries that share a name cannot both surface
//     as "duplicate IDs" — SetAction already folded them into one entry.
//
// This does NOT mean the duplicate-ID branch itself is unreachable: two
// DIFFERENT names can still collide on ActionID by overriding per-action
// Domain so domain+"."+name produces the same string for both, since
// normalizeAction only checks an action's ID against its own domain/name,
// never against sibling actions. See
// TestCatalog_AddGroup_IntraGroupDuplicateIDViaDomainOverlap for that path.
//
// We assert the documented contract below: feeding two actions that share
// both a name and an explicit invalid ID is rejected at the normalization
// step, never reaching the duplicate-ID branch by this route.
func TestCatalog_AddGroup_SameNameCollisionRejectedBeforeDuplicateIDCheck(t *testing.T) {
	group := NewGroup(GroupOptions{ToolName: "gitlab_dup"})
	group.SetAction(Action{Name: "one", ID: "shared.invalid", Route: testRoute(false)})
	group.SetAction(Action{Name: "two", ID: "shared.invalid", Route: testRoute(false)})
	err := NewCatalog().AddGroup(group)
	if err == nil {
		t.Fatal("AddGroup() error = nil, want normalization error for mismatched explicit ID")
	}
	if !strings.Contains(err.Error(), "has id") {
		t.Fatalf("err = %q, want it to come from normalizeAction (mention 'has id')", err.Error())
	}
}

// TestCatalog_AddGroup_IntraGroupDuplicateIDViaDomainOverlap verifies the
// intra-group "duplicate action id %q" branch in AddGroup — the one
// TestCatalog_AddGroup_SameNameCollisionRejectedBeforeDuplicateIDCheck
// documents as unreachable through same-name collisions. It is reachable
// through a different route:
// ActionID is literally domain+"."+name, and normalizeAction only checks
// that an action's own explicit ID (if any) matches its own domain+name —
// it never compares across actions. Two actions with DIFFERENT Name values
// (so SetAction's name-keyed map never folds them together) can still
// compute to the SAME ActionID by choosing complementary per-action Domain
// overrides: domain "foo.bar" + name "dup" and domain "foo" + name
// "bar.dup" both yield "foo.bar.dup". Neither action's ID mismatches its
// own domain/name, so normalizeGroup accepts both, and the collision is
// only caught by the seenIDs check this test targets.
func TestCatalog_AddGroup_IntraGroupDuplicateIDViaDomainOverlap(t *testing.T) {
	group := NewGroup(GroupOptions{ToolName: "gitlab_probe"})
	group.SetAction(Action{Name: "dup", Domain: "foo.bar", Route: testRoute(false)})
	group.SetAction(Action{Name: "bar.dup", Domain: "foo", Route: testRoute(false)})

	err := NewCatalog().AddGroup(group)
	if err == nil {
		t.Fatal("AddGroup() error = nil, want duplicate action id error")
	}
	if err.Error() != `duplicate action id "foo.bar.dup"` {
		t.Fatalf("err = %q, want duplicate action id \"foo.bar.dup\"", err.Error())
	}
}

// TestCatalog_AddActionCreatesGroupWithMetadata verifies that AddAction
// synthesizes a missing group from the options it is given, so the group
// carries a description, a read-only flag, icons and a formatter like one
// built the usual way, and that options naming a different tool are refused.
func TestCatalog_AddActionCreatesGroupWithMetadata(t *testing.T) {
	catalog := NewCatalog()
	formatResult := func(any) *mcp.CallToolResult { return nil }
	err := catalog.AddAction("gitlab_discover_project", Action{Name: "resolve", Route: testRoute(false)}, GroupOptions{
		Description:  "Resolve git remotes to projects.",
		Icons:        toolutil.IconProject,
		ReadOnly:     true,
		FormatResult: formatResult,
	})
	if err != nil {
		t.Fatalf("AddAction() error = %v", err)
	}

	group, ok := catalog.Group("gitlab_discover_project")
	if !ok {
		t.Fatal("Group(gitlab_discover_project) = false")
	}
	if group.Description == "" || !group.ReadOnly || len(group.Icons) == 0 || group.FormatResult == nil {
		t.Fatalf("group metadata = %+v, want description, read-only, icons, and formatter", group)
	}
	if _, hasResolve := group.Actions["resolve"]; !hasResolve {
		t.Fatal("group missing resolve action")
	}
	mismatchErr := NewCatalog().AddAction("gitlab_discover_project", Action{Name: "resolve", Route: testRoute(false)}, GroupOptions{ToolName: "gitlab_project"})
	if mismatchErr == nil {
		t.Fatal("AddAction(mismatched group options) error = nil, want error")
	}
}

// TestCatalog_AddGroupPreservesFormatter verifies that the formatter a group
// was built with is still on the group read back out of the catalog.
func TestCatalog_AddGroupPreservesFormatter(t *testing.T) {
	group := NewGroup(GroupOptions{
		ToolName: "gitlab_project",
		FormatResult: func(any) *mcp.CallToolResult {
			return nil
		},
	})
	group.SetAction(Action{Name: "get", Route: testRoute(false)})
	catalog := NewCatalog()
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	got, ok := catalog.Group("gitlab_project")
	if !ok {
		t.Fatal("Group(gitlab_project) = false")
	}
	if got.FormatResult == nil {
		t.Fatal("Group(gitlab_project) FormatResult = nil, want preserved formatter")
	}
}

// TestCatalog_LookupsAndNilReceivers verifies that every accessor answers a
// nil catalog with nothing rather than a panic, that a lookup of a group or
// action the catalog does not hold reports false, and that one it does hold
// comes back normalized with its name, domain and schema URI.
func TestCatalog_LookupsAndNilReceivers(t *testing.T) {
	var nilCatalog *Catalog
	if _, ok := nilCatalog.Group("gitlab_project"); ok {
		t.Fatal("nil Group() ok = true, want false")
	}
	if _, ok := nilCatalog.Action("project.get"); ok {
		t.Fatal("nil Action() ok = true, want false")
	}
	if nilCatalog.Groups() != nil || nilCatalog.Actions() != nil || nilCatalog.ActionMaps() != nil || nilCatalog.Clone() != nil {
		t.Fatal("nil catalog accessors returned non-nil values")
	}
	if nilCatalog.CountGroups() != 0 || nilCatalog.CountActions() != 0 {
		t.Fatal("nil catalog counts are non-zero")
	}

	catalog := FromActionMaps(map[string]toolutil.ActionMap{"gitlab_project": {"get": testRoute(false)}})
	if _, ok := catalog.Group("gitlab_missing"); ok {
		t.Fatal("Group(missing) ok = true, want false")
	}
	if _, ok := catalog.Action("missing.action"); ok {
		t.Fatal("Action(missing) ok = true, want false")
	}
	action, ok := catalog.Action("project.get")
	if !ok || action.Name != "get" || action.Domain != "project" || action.SchemaURI == "" {
		t.Fatalf("Action(project.get) = %+v, %t; want normalized action", action, ok)
	}
}

// TestCatalog_ValidateRejectsInvalidCatalogs verifies each refusal of
// Catalog.Validate by the text it reports: a nil handler, a nil input schema,
// a route with no binder, a schema URI naming another action or another
// tool, an alias two actions claim, a group with no tool name and an action
// with no name.
func TestCatalog_ValidateRejectsInvalidCatalogs(t *testing.T) {
	tests := []struct {
		name    string
		catalog *Catalog
		want    string
	}{
		{
			name: "nil handler",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}}},
			}),
			want: "nil handler",
		},
		{
			name: "nil schema",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: toolutil.ActionRoute{Handler: testHandler}},
			}),
			want: "nil input schema",
		},
		{
			name: "no binder",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: testRouteWithoutBinder(false)},
			}),
			want: "has no binder",
		},
		{
			name: "schema uri names another action",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: testRoute(false), SchemaURI: "gitlab://schema/meta/gitlab_project/list"},
			}),
			want: "malformed schema URI",
		},
		{
			name: "schema uri names another tool",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: testRoute(false), SchemaURI: "gitlab://schema/meta/gitlab_issue/get"},
			}),
			want: "malformed schema URI",
		},
		{
			name: "ambiguous alias",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: testRoute(false), Aliases: []string{"project.show"}},
				{Name: "list", Route: testRoute(false), Aliases: []string{"project.show"}},
			}),
			want: "maps to both",
		},
		{
			name:    "missing group tool name",
			catalog: &Catalog{groups: map[string]Group{"": {ToolName: "", Actions: map[string]Action{"get": {Name: "get", Route: testRoute(false)}}}}},
			want:    errToolNameRequired,
		},
		{
			name:    "missing action name",
			catalog: &Catalog{groups: map[string]Group{"gitlab_project": {ToolName: "gitlab_project", ActionOrder: []string{""}, Actions: map[string]Action{"": {Route: testRoute(false)}}}}},
			want:    "action name is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.catalog.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestCatalog_ValidateAcceptsValidAndRejectsNil verifies that Validate refuses
// a nil catalog, accepts a well-formed one whose action lists an alias twice
// beside a blank one (a repeat on one action is not a claim by two), and
// exempts the dynamic controllers alone from the binder rule.
func TestCatalog_ValidateAcceptsValidAndRejectsNil(t *testing.T) {
	var nilCatalog *Catalog
	if err := nilCatalog.Validate(); err == nil {
		t.Fatal("nil Validate() error = nil, want error")
	}
	catalog := catalogWithActions(t, "gitlab_project", []Action{
		{Name: "get", Route: testRoute(false), Aliases: []string{"", "project.show", "Project.Show"}},
	})
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v, want an alias repeated on one action accepted", err)
	}

	// The dynamic controllers are the one exemption from the binder rule:
	// gitlab_find_action and gitlab_execute_action close over the registry
	// rather than a client, so there is no client for a binder to take.
	controllers := NewCatalog()
	controllerGroup := NewGroup(GroupOptions{ToolName: "gitlab_dynamic", SurfaceKind: SurfaceKindDynamicController})
	controllerGroup.SetAction(Action{Name: "find", Route: testRouteWithoutBinder(false)})
	if err := controllers.AddGroup(controllerGroup); err != nil {
		t.Fatalf("AddGroup(dynamic controllers) error = %v", err)
	}
	if err := controllers.Validate(); err != nil {
		t.Fatalf("Validate(dynamic controllers) error = %v, want the binder rule not to apply", err)
	}
}

// TestCatalog_FiltersCloneWithoutMutatingSource verifies that each filter
// returns a narrowed copy and leaves the source catalog with every group it
// had, that a nil catalog filters to nil, and that an empty exclusion or
// allow-list keeps everything.
func TestCatalog_FiltersCloneWithoutMutatingSource(t *testing.T) {
	catalog := NewCatalog()
	readGroup := NewGroup(GroupOptions{ToolName: "gitlab_search", ReadOnly: true})
	readGroup.SetAction(Action{Name: "code", Route: testRoute(false)})
	writeGroup := NewGroup(GroupOptions{ToolName: "gitlab_project"})
	writeGroup.SetAction(Action{Name: "create", Route: testRoute(false)})
	if err := catalog.AddGroup(readGroup); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if err := catalog.AddGroup(writeGroup); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	if got := catalog.FilterExcludedTools([]string{"gitlab_project"}).CountGroups(); got != 1 {
		t.Fatalf("FilterExcludedTools CountGroups() = %d, want 1", got)
	}
	if got := catalog.FilterReadOnlyGroups().CountGroups(); got != 1 {
		t.Fatalf("FilterReadOnlyGroups CountGroups() = %d, want 1", got)
	}
	if got := catalog.FilterAllowedToolNames([]string{"gitlab_project"}).CountActions(); got != 1 {
		t.Fatalf("FilterAllowedToolNames CountActions() = %d, want 1", got)
	}
	filtered := catalog.Filter(FilterOptions{
		ExcludeTools:     []string{"gitlab_project"},
		ReadOnlyOnly:     true,
		AllowedToolNames: []string{"gitlab_search"},
	})
	if filtered.CountGroups() != 1 || filtered.CountActions() != 1 {
		t.Fatalf("Filter() counts = groups %d actions %d, want 1/1", filtered.CountGroups(), filtered.CountActions())
	}
	if catalog.CountGroups() != 2 {
		t.Fatalf("source CountGroups() = %d, want 2", catalog.CountGroups())
	}
	var nilCatalog *Catalog
	if nilCatalog.FilterExcludedTools(nil) != nil || nilCatalog.FilterReadOnlyGroups() != nil || nilCatalog.FilterAllowedToolNames(nil) != nil || nilCatalog.Filter(FilterOptions{}) != nil {
		t.Fatal("nil catalog filters returned non-nil values")
	}
	if got := catalog.FilterExcludedTools(nil).CountGroups(); got != 2 {
		t.Fatalf("FilterExcludedTools(nil) CountGroups() = %d, want 2", got)
	}
	if got := catalog.FilterAllowedToolNames(nil).CountGroups(); got != 2 {
		t.Fatalf("FilterAllowedToolNames(nil) CountGroups() = %d, want 2", got)
	}
}

// catalogWithActions supports catalog with actions assertions in actioncatalog tests.
func catalogWithActions(t *testing.T, toolName string, actions []Action) *Catalog {
	t.Helper()
	group := NewGroup(GroupOptions{ToolName: toolName})
	for _, action := range actions {
		group.SetAction(action)
	}
	catalog := NewCatalog()
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	return catalog
}

// testRoute supports test route assertions in actioncatalog tests. It carries
// a binder because [Catalog.Validate] requires one of every catalog action but
// the dynamic controllers; [testRouteWithoutBinder] is the route that does not.
func testRoute(destructive bool) toolutil.ActionRoute {
	return testRouteWithoutBinder(destructive).WithBoundHandler(nil, func(*gitlabclient.Client) toolutil.ActionFunc {
		return testHandler
	})
}

// testRouteWithoutBinder is testRoute's route with its handler installed
// directly, the shape a catalog refuses.
func testRouteWithoutBinder(destructive bool) toolutil.ActionRoute {
	return toolutil.ActionRoute{
		Handler:     testHandler,
		Destructive: destructive,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "integer"},
			},
		},
		OutputSchema: map[string]any{"type": "object"},
	}
}

// testHandler supports test handler assertions in actioncatalog tests.
func testHandler(context.Context, map[string]any) (any, error) {
	return map[string]any{"ok": true}, nil
}

// TestCatalog_FilterReadOnlyActionsKeepsReadsInMixedGroups verifies that
// read-only filtering works at action granularity: a mixed group keeps its
// read-only actions instead of disappearing wholesale, the surviving group is
// marked read-only so annotations and read-only tool pruning agree with its
// contents, groups left with no read-only action are dropped, and mutating
// actions never survive.
func TestCatalog_FilterReadOnlyActionsKeepsReadsInMixedGroups(t *testing.T) {
	catalog := NewCatalog()

	mixed := NewGroup(GroupOptions{ToolName: "gitlab_issue"})
	mixed.SetAction(Action{Name: "list", Route: testRoute(false), ReadOnly: true})
	mixed.SetAction(Action{Name: "get", Route: testRoute(false), ReadOnly: true})
	mixed.SetAction(Action{Name: "create", Route: testRoute(false)})

	writeOnly := NewGroup(GroupOptions{ToolName: "gitlab_project_alias"})
	writeOnly.SetAction(Action{Name: "create", Route: testRoute(false)})

	readOnly := NewGroup(GroupOptions{ToolName: "gitlab_search", ReadOnly: true})
	readOnly.SetAction(Action{Name: "code", Route: testRoute(false), ReadOnly: true})

	if err := catalog.AddGroup(mixed); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if err := catalog.AddGroup(writeOnly); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if err := catalog.AddGroup(readOnly); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	filtered := catalog.FilterReadOnlyActions()

	if got := filtered.CountGroups(); got != 2 {
		t.Fatalf("CountGroups() = %d, want 2 (mixed group survives, write-only group dropped)", got)
	}
	issueGroup, ok := filtered.Group("gitlab_issue")
	if !ok {
		t.Fatal("gitlab_issue group missing: a mixed group must keep its read-only actions")
	}
	if got := len(issueGroup.Actions); got != 2 {
		t.Errorf("gitlab_issue actions = %d, want 2 (list, get)", got)
	}
	if _, mutating := issueGroup.Actions["create"]; mutating {
		t.Error("gitlab_issue kept the mutating create action")
	}
	if !issueGroup.ReadOnly {
		t.Error("surviving group must be marked ReadOnly so derived annotations match its contents")
	}
	if _, dropped := filtered.Group("gitlab_project_alias"); dropped {
		t.Error("group with no read-only action must be dropped")
	}
	if catalog.CountActions() != 5 {
		t.Errorf("source catalog mutated: CountActions() = %d, want 5", catalog.CountActions())
	}

	var nilCatalog *Catalog
	if nilCatalog.FilterReadOnlyActions() != nil {
		t.Error("nil catalog FilterReadOnlyActions() must return nil")
	}
}

// TestCatalog_WithSafeModePreviewsRewritesOnlyMutatingActions verifies that
// safe mode is applied at action granularity: mutating actions get a preview
// handler naming the canonical action ID and lose their destructive flag
// (nothing executes, so nothing needs confirming), read-only actions keep their
// real handler, groups are never dropped, and the source catalog is untouched.
func TestCatalog_WithSafeModePreviewsRewritesOnlyMutatingActions(t *testing.T) {
	executed := false
	realHandler := func(context.Context, map[string]any) (any, error) {
		executed = true
		return "real", nil
	}

	catalog := NewCatalog()
	group := NewGroup(GroupOptions{ToolName: "gitlab_issue"})
	group.SetAction(Action{Name: "list", Route: toolutil.ActionRoute{Handler: realHandler}, ReadOnly: true})
	group.SetAction(Action{Name: "create", Route: toolutil.ActionRoute{Handler: realHandler, Destructive: true}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	previewed := catalog.WithSafeModePreviews()

	safeGroup, ok := previewed.Group("gitlab_issue")
	if !ok {
		t.Fatal("gitlab_issue group missing: safe mode must not drop groups")
	}
	if got := len(safeGroup.Actions); got != 2 {
		t.Fatalf("actions = %d, want 2 (safe mode previews, never removes)", got)
	}

	readAction := safeGroup.Actions["list"]
	if _, err := readAction.Route.Handler(context.Background(), nil); err != nil {
		t.Fatalf("read-only handler error = %v", err)
	}
	if !executed {
		t.Error("read-only action must keep executing its real handler under safe mode")
	}

	executed = false
	writeAction := safeGroup.Actions["create"]
	result, err := writeAction.Route.Handler(context.Background(), map[string]any{"title": "x"})
	if err != nil {
		t.Fatalf("mutating handler error = %v", err)
	}
	if executed {
		t.Error("mutating action executed its real handler under safe mode")
	}
	preview, isPreview := result.(toolutil.SafeModePreview)
	if !isPreview {
		t.Fatalf("mutating handler returned %T, want toolutil.SafeModePreview", result)
	}
	if preview.Status != "blocked" || preview.Mode != "safe" {
		t.Errorf("preview status/mode = %q/%q, want blocked/safe", preview.Status, preview.Mode)
	}
	if preview.Tool != "issue.create" {
		t.Errorf("preview tool = %q, want the canonical action ID issue.create", preview.Tool)
	}
	if string(preview.Params) != `{"title":"x"}` {
		t.Errorf("preview params = %s, want the would-be call arguments", preview.Params)
	}
	if writeAction.Route.Destructive || writeAction.Destructive {
		t.Error("previewed action must not stay destructive: nothing executes, so nothing needs confirmation")
	}

	if sourceAction := catalog.Groups()[0].Actions["create"]; !sourceAction.Route.Destructive {
		t.Error("source catalog mutated: original action lost its destructive flag")
	}

	var nilCatalog *Catalog
	if nilCatalog.WithSafeModePreviews() != nil {
		t.Error("nil catalog WithSafeModePreviews() must return nil")
	}
}

// recordingRoute builds a typed route whose handler records the client it
// ran under, which is how a test observes what BindTo did.
func recordingRoute(client *gitlabclient.Client, seen **gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, func(_ context.Context, client *gitlabclient.Client, _ struct{}) (string, error) {
		*seen = client
		return "ok", nil
	})
}

// TestCatalog_MarkSharedAndSharedOrigin verifies the identity a catalog
// offers to caches: nothing for an ordinary catalog, itself once marked
// shared, and the shared catalog for every catalog bound from it, with the
// route schemas registered as shared by the marking. A nil catalog answers
// nil to both without panicking.
func TestCatalog_MarkSharedAndSharedOrigin(t *testing.T) {
	var nilCatalog *Catalog
	nilCatalog.MarkShared()
	if nilCatalog.SharedOrigin() != nil || nilCatalog.BindTo(nil) != nil {
		t.Fatal("nil catalog SharedOrigin() or BindTo() returned something")
	}

	var seen *gitlabclient.Client
	// The list route carries ad hoc schema maps: a typed route's schemas come
	// from the process-wide type cache and are shared by construction, which
	// would make the "not shared yet" half of this test vacuous.
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": recordingRoute(nil, &seen), "list": testRoute(false)},
	})
	if catalog.SharedOrigin() != nil {
		t.Fatal("an unshared catalog reported a shared origin")
	}
	action, _ := catalog.Action("project.list")
	if toolutil.SchemaShared(action.Route.InputSchema) {
		t.Fatal("the route schema was registered as shared before MarkShared")
	}

	catalog.MarkShared()
	if catalog.SharedOrigin() != catalog {
		t.Fatal("a shared catalog does not report itself as its origin")
	}
	if !toolutil.SchemaShared(action.Route.InputSchema) || !toolutil.SchemaShared(action.Route.OutputSchema) {
		t.Fatal("MarkShared() did not register the route schemas")
	}

	client := &gitlabclient.Client{}
	bound := catalog.BindTo(client)
	if bound == catalog || bound.SharedOrigin() != catalog {
		t.Fatal("BindTo() did not return a distinct catalog with the shared one as its origin")
	}
	if rebound := bound.BindTo(client); rebound.SharedOrigin() != catalog {
		t.Fatal("binding a bound catalog lost the shared origin")
	}
	if cloned := bound.Clone(); cloned.SharedOrigin() != nil {
		t.Fatal("Clone() of a bound catalog carried its origin, want none")
	}
	if filtered := bound.FilterReadOnlyGroups(); filtered.SharedOrigin() != nil {
		t.Fatal("a filtered catalog carried the origin, want none: it is a different action set")
	}
}

// TestCatalog_BindTo_RebindsEveryHandlerAndSharesTheRest verifies a bound
// catalog runs every action under the client it was bound to, while the
// route metadata, the group metadata and the action index are the shared
// catalog's own.
func TestCatalog_BindTo_RebindsEveryHandlerAndSharesTheRest(t *testing.T) {
	var seen *gitlabclient.Client
	buildClient, clientA, clientB := &gitlabclient.Client{}, &gitlabclient.Client{}, &gitlabclient.Client{}
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": recordingRoute(buildClient, &seen), "list": recordingRoute(buildClient, &seen)},
	})
	catalog.MarkShared()

	boundA, boundB := catalog.BindTo(clientA), catalog.BindTo(clientB)
	for _, id := range []ActionID{"project.get", "project.list"} {
		t.Run(string(id), func(t *testing.T) {
			actionA, okA := boundA.Action(id)
			actionB, okB := boundB.Action(id)
			shared, okShared := catalog.Action(id)
			if !okA || !okB || !okShared {
				t.Fatalf("Action(%s) missing from a catalog: A %t, B %t, shared %t", id, okA, okB, okShared)
			}
			if _, err := actionA.Route.Handler(context.Background(), map[string]any{}); err != nil || seen != clientA {
				t.Errorf("catalog A ran %s under %p (error %v), want client A %p", id, seen, err, clientA)
			}
			if _, err := actionB.Route.Handler(context.Background(), map[string]any{}); err != nil || seen != clientB {
				t.Errorf("catalog B ran %s under %p (error %v), want client B %p", id, seen, err, clientB)
			}
			if _, err := shared.Route.Handler(context.Background(), map[string]any{}); err != nil || seen != buildClient {
				t.Errorf("the shared catalog ran %s under %p, want the build client %p untouched by binding", id, seen, buildClient)
			}
			if reflect.ValueOf(actionA.Route.InputSchema).UnsafePointer() != reflect.ValueOf(actionB.Route.InputSchema).UnsafePointer() {
				t.Errorf("bound catalogs carry different input schemas for %s, want one shared map", id)
			}
		})
	}
	groupA, _ := boundA.Group("gitlab_project")
	if len(groupA.ActionOrder) != 2 || boundA.CountActions() != 2 || boundA.CountGroups() != 1 {
		t.Errorf("bound catalog shape = %d groups, %d actions, order %v; want the shared catalog's", boundA.CountGroups(), boundA.CountActions(), groupA.ActionOrder)
	}
}

// TestCatalog_WithSafeModePreviews_SurvivesBinding verifies the previews a
// safe-mode catalog carries stay previews after the catalog is bound to a
// client: both halves of the route were replaced, so binding does not
// rebuild the real handler.
func TestCatalog_WithSafeModePreviews_SurvivesBinding(t *testing.T) {
	var seen *gitlabclient.Client
	route := recordingRoute(&gitlabclient.Client{}, &seen)
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"delete": route},
	})
	previewed := catalog.WithSafeModePreviews()
	if err := previewed.Validate(); err != nil {
		t.Fatalf("Validate(previewed) error = %v", err)
	}
	bound := previewed.BindTo(&gitlabclient.Client{})
	action, _ := bound.Action("project.delete")
	result, err := action.Route.Handler(context.Background(), map[string]any{"project_id": "1"})
	if err != nil {
		t.Fatalf("preview handler error = %v", err)
	}
	if _, isPreview := result.(toolutil.SafeModePreview); !isPreview {
		t.Fatalf("bound safe-mode handler returned %T, want a SafeModePreview", result)
	}
	if seen != nil {
		t.Fatal("binding a previewed catalog rebuilt the real handler, which ran")
	}
}

// TestCatalog_Validate_RefusesAReplacedHandler verifies catalog validation
// carries the route binding guard: an action whose handler was assigned
// directly after the constructor set a binder is refused, naming the action.
func TestCatalog_Validate_RefusesAReplacedHandler(t *testing.T) {
	var seen *gitlabclient.Client
	broken := recordingRoute(nil, &seen)
	broken.Handler = func(context.Context, map[string]any) (any, error) { return "replaced", nil }
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": broken},
	})
	err := catalog.Validate()
	if err == nil || !strings.Contains(err.Error(), `action "project.get"`) || !strings.Contains(err.Error(), "without its binder") {
		t.Fatalf("Validate() error = %v, want the replaced handler refused by action", err)
	}
}

// TestCatalog_ActionMetadataFallsBackToTheRouteOnlyWhenUnset verifies that
// an action's aliases, tags, usage and related actions come from its route
// exactly when the action carries none of its own: an action given its own
// keeps them even though the route's differ.
//
// Every value here is distinct from every other, on the route and on the
// action alike, so a fallback that reads the wrong list, or one that
// overwrites what the action carried, is seen by the value that arrives.
func TestCatalog_ActionMetadataFallsBackToTheRouteOnlyWhenUnset(t *testing.T) {
	route := testRoute(false)
	route.Aliases = []string{"route-alias"}
	route.Tags = []string{"route-tag"}
	route.Usage = "route usage"
	route.RelatedActions = []string{"route.related"}

	tests := []struct {
		name               string
		action             Action
		wantAliases        []string
		wantTags           []string
		wantUsage          string
		wantRelatedActions []string
	}{
		{
			name:               "an action carrying none takes the route's",
			action:             Action{Name: "get", Route: route},
			wantAliases:        []string{"route-alias"},
			wantTags:           []string{"route-tag"},
			wantUsage:          "route usage",
			wantRelatedActions: []string{"route.related"},
		},
		{
			name: "an action carrying its own keeps them",
			action: Action{
				Name:           "get",
				Route:          route,
				Aliases:        []string{"action-alias"},
				Tags:           []string{"action-tag"},
				Usage:          "action usage",
				RelatedActions: []string{"action.related"},
			},
			wantAliases:        []string{"action-alias"},
			wantTags:           []string{"action-tag"},
			wantUsage:          "action usage",
			wantRelatedActions: []string{"action.related"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := catalogWithActions(t, "gitlab_project", []Action{tt.action})
			got, ok := catalog.Action("project.get")
			if !ok {
				t.Fatal("Action(project.get) = false, want the action indexed")
			}
			if !slices.Equal(got.Aliases, tt.wantAliases) {
				t.Errorf("aliases = %v, want %v", got.Aliases, tt.wantAliases)
			}
			if !slices.Equal(got.Tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", got.Tags, tt.wantTags)
			}
			if got.Usage != tt.wantUsage {
				t.Errorf("usage = %q, want %q", got.Usage, tt.wantUsage)
			}
			if !slices.Equal(got.RelatedActions, tt.wantRelatedActions) {
				t.Errorf("related actions = %v, want %v", got.RelatedActions, tt.wantRelatedActions)
			}
		})
	}
}

// filterTestCatalog builds the two-group catalog the Filter tests narrow: a
// read-only gitlab_search group and a mutating gitlab_project group.
func filterTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog := NewCatalog()
	readGroup := NewGroup(GroupOptions{ToolName: "gitlab_search", ReadOnly: true})
	readGroup.SetAction(Action{Name: "code", Route: testRoute(false), ReadOnly: true})
	writeGroup := NewGroup(GroupOptions{ToolName: "gitlab_project"})
	writeGroup.SetAction(Action{Name: "create", Route: testRoute(false)})
	if err := catalog.AddGroup(readGroup); err != nil {
		t.Fatalf("AddGroup(gitlab_search) error = %v", err)
	}
	if err := catalog.AddGroup(writeGroup); err != nil {
		t.Fatalf("AddGroup(gitlab_project) error = %v", err)
	}
	return catalog
}

// catalogGroupNames returns the tool names of every group in the catalog,
// sorted, so a test can compare the survivors of a filter as one value.
func catalogGroupNames(catalog *Catalog) []string {
	names := make([]string, 0, catalog.CountGroups())
	for _, group := range catalog.Groups() {
		names = append(names, group.ToolName)
	}
	return names
}

// TestCatalog_Filter_AppliesEachOptionOnItsOwn verifies that Filter honors
// each of its three options when it is the only one set, and all of them
// together, by the groups that survive.
//
// The one Filter test before this set all three at once on a catalog where
// any two of them already produced the final answer, so an allow-list that
// was never applied passed.
func TestCatalog_Filter_AppliesEachOptionOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		opts FilterOptions
		want []string
	}{
		{name: "nothing set keeps every group", opts: FilterOptions{}, want: []string{"gitlab_project", "gitlab_search"}},
		{name: "exclusion alone removes the named group", opts: FilterOptions{ExcludeTools: []string{"gitlab_search"}}, want: []string{"gitlab_project"}},
		{name: "read-only alone keeps the read-only group", opts: FilterOptions{ReadOnlyOnly: true}, want: []string{"gitlab_search"}},
		{name: "allow-list alone keeps the named group", opts: FilterOptions{AllowedToolNames: []string{"gitlab_project"}}, want: []string{"gitlab_project"}},
		{name: "allow-list naming nothing present keeps no group", opts: FilterOptions{AllowedToolNames: []string{"gitlab_issue"}}, want: []string{}},
		{
			name: "every option composes",
			opts: FilterOptions{ExcludeTools: []string{"gitlab_project"}, ReadOnlyOnly: true, AllowedToolNames: []string{"gitlab_search"}},
			want: []string{"gitlab_search"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := filterTestCatalog(t)
			filtered := catalog.Filter(tt.opts)
			if got := catalogGroupNames(filtered); !slices.Equal(got, tt.want) {
				t.Errorf("Filter(%+v) groups = %v, want %v", tt.opts, got, tt.want)
			}
			if got := catalog.CountGroups(); got != 2 {
				t.Errorf("source CountGroups() = %d after Filter, want 2", got)
			}
			if filtered == catalog {
				t.Error("Filter() returned the source catalog, want a copy")
			}
		})
	}
}

// TestCatalog_AddAction_ToOneGroup_LeavesTheOthersIndexed verifies that
// adding an action to one existing group re-indexes that group alone: the
// actions of every other group stay in the catalog's action index.
//
// AddAction rebuilds the target group by dropping its actions from the index
// first, selecting them by tool name, so a selection that matched too widely
// would silently unindex a neighbor.
func TestCatalog_AddAction_ToOneGroup_LeavesTheOthersIndexed(t *testing.T) {
	catalog := filterTestCatalog(t)
	if err := catalog.AddAction("gitlab_project", Action{Name: "get", Route: testRoute(false), ReadOnly: true}); err != nil {
		t.Fatalf("AddAction() error = %v", err)
	}
	if got := catalogActionIDs(catalog); !slices.Equal(got, []string{"project.create", "project.get", "search.code"}) {
		t.Errorf("actions after AddAction = %v, want project.create, project.get and search.code all indexed", got)
	}
	if _, ok := catalog.Action("search.code"); !ok {
		t.Error("Action(search.code) = false after adding to gitlab_project, want the other group's action still indexed")
	}
	if group, ok := catalog.Group("gitlab_project"); !ok || !slices.Equal(group.ActionOrder, []string{"create", "get"}) {
		t.Errorf("gitlab_project order = %v (found %t), want [create get]", group.ActionOrder, ok)
	}
}

// groupMetadataOptions returns group options with every metadata field set
// to a value no other field shares, and exactly one of the three boolean
// fields set, named by flag: "read_only", "enterprise_only" or
// "gitlab_com_only".
//
// A rebuild of a group is a block of twelve plain assignments, which is the
// shape neither gate can report on, and the three flags are what a fixture
// setting them all to true cannot tell apart.
func groupMetadataOptions(flag string) GroupOptions {
	return GroupOptions{
		ToolName:               "gitlab_fixture",
		Title:                  "Fixture title",
		Description:            "Fixture description.",
		Icons:                  []mcp.Icon{{Source: "data:image/svg+xml;base64,fixture", MIMEType: "image/svg+xml", Sizes: []string{"any"}}},
		ReadOnly:               flag == "read_only",
		FormatResult:           groupMetadataFormatter,
		BaseDomain:             "fixture_domain",
		EnterpriseOnly:         flag == "enterprise_only",
		GitLabDotComOnly:       flag == "gitlab_com_only",
		CapabilityRequirements: []string{"fixture-capability"},
		OwnerPackage:           "fixturepkg",
		SurfaceKind:            SurfaceKindGitLabAction,
	}
}

// groupMetadataFormatter is the formatter groupMetadataOptions carries. A
// func cannot be compared, so the assertion runs it and reads the text back.
func groupMetadataFormatter(any) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "fixture formatter"}}}
}

// assertGroupMetadata holds every metadata field of got to want, one field
// at a time so the report names the one that moved.
func assertGroupMetadata(t *testing.T, got Group, want GroupOptions) {
	t.Helper()
	if got.ToolName != want.ToolName {
		t.Errorf("tool name = %q, want %q", got.ToolName, want.ToolName)
	}
	if got.Title != want.Title {
		t.Errorf("title = %q, want %q", got.Title, want.Title)
	}
	if got.Description != want.Description {
		t.Errorf("description = %q, want %q", got.Description, want.Description)
	}
	if !reflect.DeepEqual(got.Icons, want.Icons) {
		t.Errorf("icons = %+v, want %+v", got.Icons, want.Icons)
	}
	if got.ReadOnly != want.ReadOnly {
		t.Errorf("read-only = %t, want %t", got.ReadOnly, want.ReadOnly)
	}
	if got.FormatResult == nil {
		t.Error("formatter = nil, want the fixture's")
	} else if result := got.FormatResult(nil); len(result.Content) != 1 || result.Content[0].(*mcp.TextContent).Text != "fixture formatter" {
		t.Errorf("formatter result = %+v, want the fixture formatter's text", result)
	}
	if got.BaseDomain != want.BaseDomain {
		t.Errorf("base domain = %q, want %q", got.BaseDomain, want.BaseDomain)
	}
	if got.EnterpriseOnly != want.EnterpriseOnly {
		t.Errorf("enterprise-only = %t, want %t", got.EnterpriseOnly, want.EnterpriseOnly)
	}
	if got.GitLabDotComOnly != want.GitLabDotComOnly {
		t.Errorf("gitlab.com-only = %t, want %t", got.GitLabDotComOnly, want.GitLabDotComOnly)
	}
	if !slices.Equal(got.CapabilityRequirements, want.CapabilityRequirements) {
		t.Errorf("capability requirements = %v, want %v", got.CapabilityRequirements, want.CapabilityRequirements)
	}
	if got.OwnerPackage != want.OwnerPackage {
		t.Errorf("owner package = %q, want %q", got.OwnerPackage, want.OwnerPackage)
	}
	if got.SurfaceKind != want.SurfaceKind {
		t.Errorf("surface kind = %q, want %q", got.SurfaceKind, want.SurfaceKind)
	}
}

// TestCatalog_Rebuilds_KeepEveryGroupMetadataField verifies that every path
// which rebuilds a group from its own metadata hands each field on: reading
// it back, cloning, binding, narrowing to read-only actions, replacing
// writes with previews, excluding one of its actions, and adding an action
// to it. Each runs three times, with one boolean field set per run.
//
// Six of those paths copy the group's fields into a fresh GroupOptions by
// hand, and a field left out of one copy is a group that loses its icons,
// its formatter or its tier on one surface and keeps them on the others.
func TestCatalog_Rebuilds_KeepEveryGroupMetadataField(t *testing.T) {
	rebuilds := []struct {
		name    string
		rebuild func(t *testing.T, catalog *Catalog) (Group, bool)
		// forcesReadOnly marks the one path that sets the flag on purpose.
		forcesReadOnly bool
	}{
		{name: "read back", rebuild: func(_ *testing.T, c *Catalog) (Group, bool) { return c.Group("gitlab_fixture") }},
		{name: "clone", rebuild: func(_ *testing.T, c *Catalog) (Group, bool) { return c.Clone().Group("gitlab_fixture") }},
		{name: "bind", rebuild: func(_ *testing.T, c *Catalog) (Group, bool) {
			return c.BindTo(&gitlabclient.Client{}).Group("gitlab_fixture")
		}},
		{name: "read-only actions", forcesReadOnly: true, rebuild: func(_ *testing.T, c *Catalog) (Group, bool) {
			return c.FilterReadOnlyActions().Group("gitlab_fixture")
		}},
		{name: "safe-mode previews", rebuild: func(_ *testing.T, c *Catalog) (Group, bool) {
			return c.WithSafeModePreviews().Group("gitlab_fixture")
		}},
		{name: "exclude one action", rebuild: func(t *testing.T, c *Catalog) (Group, bool) {
			t.Helper()
			filtered, unmatched := c.FilterExcludedToolNames([]string{"fixture_domain.delete"})
			if len(unmatched) != 0 {
				t.Fatalf("unmatched = %v, want the action found", unmatched)
			}
			return filtered.Group("gitlab_fixture")
		}},
		{name: "add an action", rebuild: func(t *testing.T, c *Catalog) (Group, bool) {
			t.Helper()
			if err := c.AddAction("gitlab_fixture", Action{Name: "list", Route: testRoute(false), ReadOnly: true}); err != nil {
				t.Fatalf("AddAction() error = %v", err)
			}
			return c.Group("gitlab_fixture")
		}},
	}
	flags := []string{"read_only", "enterprise_only", "gitlab_com_only"}
	for _, rebuild := range rebuilds {
		for _, flag := range flags {
			t.Run(rebuild.name+"/"+flag, func(t *testing.T) {
				want := groupMetadataOptions(flag)
				group := NewGroup(want)
				group.SetAction(Action{Name: "get", Route: testRoute(false), ReadOnly: true})
				group.SetAction(Action{Name: "delete", Route: testRoute(true), Destructive: true})
				catalog := NewCatalog()
				if err := catalog.AddGroup(group); err != nil {
					t.Fatalf("AddGroup() error = %v", err)
				}
				got, ok := rebuild.rebuild(t, catalog)
				if !ok {
					t.Fatal("Group(gitlab_fixture) = false after the rebuild, want the group kept")
				}
				if rebuild.forcesReadOnly {
					want.ReadOnly = true
				}
				assertGroupMetadata(t, got, want)
			})
		}
	}
}
