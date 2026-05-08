package actionregistry

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
}

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

	duplicateID := NewGroup(GroupOptions{ToolName: "gitlab_duplicate"})
	duplicateID.SetAction(Action{Name: "one", ID: "duplicate.id", Route: testRoute(false)})
	duplicateID.SetAction(Action{Name: "two", ID: "duplicate.id", Route: testRoute(false)})
	if err := NewCatalog().AddGroup(duplicateID); err == nil {
		t.Fatal("AddGroup(duplicate action ID) error = nil, want error")
	}
}

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

func testHandler(context.Context, map[string]any) (any, error) {
	return map[string]any{"ok": true}, nil
}
