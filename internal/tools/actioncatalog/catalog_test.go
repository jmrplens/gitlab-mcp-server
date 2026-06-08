package actioncatalog

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCatalog_FromActionMapsRoundTrip_DeterministicActions verifies Catalog when from action maps round trip deterministic actions.
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

// TestFromActionMapsWithError_InvalidToolName_ReturnsError verifies FromActionMapsWithError returns error with invalid tool name.
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
}

// TestFromActionMaps_InvalidToolName_Panics verifies FromActionMaps when invalid tool name panics.
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

// TestGroup_SetActionAndActionsInOrder_DefensiveBranches verifies Group when set action and actions in order defensive branches.
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

// TestCatalog_CloneDefensivelyCopiesRoutes verifies Catalog when clone defensively copies routes.
func TestCatalog_CloneDefensivelyCopiesRoutes(t *testing.T) {
	catalog := FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": testRoute(false)},
	})

	cloned := catalog.Clone()
	clonedGroup, ok := cloned.Group("gitlab_project")
	if !ok {
		t.Fatal("cloned Group(gitlab_project) = false")
	}
	clonedRoute := clonedGroup.Actions["get"].Route
	clonedRoute.InputSchema["changed"] = true

	originalGroup, foundOriginal := catalog.Group("gitlab_project")
	if !foundOriginal {
		t.Fatal("original Group(gitlab_project) = false")
	}
	if _, hasChanged := originalGroup.Actions["get"].Route.InputSchema["changed"]; hasChanged {
		t.Fatal("mutating cloned schema changed original catalog")
	}
}

// TestCatalog_AddGroupAndAddActionValidateDuplicates verifies Catalog when add group and add action validate duplicates.
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

// TestCatalog_AddGroupInitializesZeroValueCatalog verifies AddGroup supports a
// zero-value catalog and detects cross-group action ID collisions.
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

// TestMustAddCatalogGroup_PanicsOnInvariantDrift verifies MustAddCatalogGroup when panics on invariant drift.
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

// TestCatalog_AddGroup_DuplicateActionIDDeadBranch documents why the
// intra-group duplicate action ID branch in AddGroup (the
// "duplicate action id %q" return at the top of the ActionsInOrder loop)
// cannot be reached through the public API. The two layers of defense
// above it make it unreachable:
//   - normalizeAction requires explicit IDs to match `<domain>.<name>`,
//     so two different names always produce two different IDs after
//     normalization, and any two actions that share a name get folded
//     together by SetAction (which uses the trimmed name as the map
//     key, overwriting the prior entry).
//   - ActionsInOrder deduplicates by map key before AddGroup sees the
//     slice, so two map entries that share a name cannot both surface
//     as "duplicate IDs".
//
// We assert the documented contract below: feeding two actions with the
// same explicit invalid ID is rejected at the normalization step, and
// the duplicate-ID branch never has to fire.
func TestCatalog_AddGroup_DuplicateActionIDDeadBranch(t *testing.T) {
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

// TestCatalog_AddActionCreatesGroupWithMetadata verifies Catalog when add action creates group with metadata.
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

// TestCatalog_AddGroupPreservesFormatter verifies Catalog when add group preserves formatter.
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

// TestCatalog_LookupsAndNilReceivers verifies Catalog when lookups and nil receivers.
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

// TestCatalog_ValidateRejectsInvalidCatalogs covers Catalog with table-driven subtests for validate rejects invalid catalogs.
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
			name: "bad schema uri",
			catalog: catalogWithActions(t, "gitlab_project", []Action{
				{Name: "get", Route: testRoute(false), SchemaURI: "gitlab://schema/meta/gitlab_project/list"},
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

// TestCatalog_ValidateAcceptsValidAndRejectsNil verifies Catalog when validate accepts valid and rejects nil.
func TestCatalog_ValidateAcceptsValidAndRejectsNil(t *testing.T) {
	var nilCatalog *Catalog
	if err := nilCatalog.Validate(); err == nil {
		t.Fatal("nil Validate() error = nil, want error")
	}
	catalog := catalogWithActions(t, "gitlab_project", []Action{
		{Name: "get", Route: testRoute(false), Aliases: []string{"", "project.show"}},
	})
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
}

// TestCatalog_FiltersCloneWithoutMutatingSource verifies Catalog when filters clone without mutating source.
func TestCatalog_FiltersCloneWithoutMutatingSource(t *testing.T) {
	catalog := NewCatalog()
	readGroup := NewGroup(GroupOptions{ToolName: "gitlab_search", ReadOnly: true})
	readGroup.SetAction(Action{Name: "code", Route: testRoute(false)})
	writeGroup := NewGroup(GroupOptions{ToolName: "gitlab_project"})
	writeGroup.SetAction(Action{Name: "create", Route: testRoute(false)})
	for _, group := range []Group{readGroup, writeGroup} {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup() error = %v", err)
		}
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

// testRoute supports test route assertions in actioncatalog tests.
func testRoute(destructive bool) toolutil.ActionRoute {
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
