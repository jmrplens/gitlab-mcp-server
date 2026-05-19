package surfaces

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestStandaloneToolSpecs_ClassifyStandaloneUtilities verifies StandaloneToolSpecs when classify standalone utilities.
func TestStandaloneToolSpecs_ClassifyStandaloneUtilities(t *testing.T) {
	specs := StandaloneToolSpecs(nil)
	discover := findSurfaceSpec(t, specs, "gitlab_discover_project")
	if discover.SurfaceKind != actioncatalog.SurfaceKindRuntimeUtility || !discover.ReadOnly || discover.GroupToolName != "gitlab_discover_project" {
		t.Fatalf("discover spec = %+v, want read-only runtime utility in discover group", discover)
	}
	if !hasActionAlias(discover, "gitlab_discover_project") {
		t.Fatalf("discover compatibility aliases = %+v, want historical tool-name alias", discover.Compatibility.ActionAliases)
	}

	interactive := findSurfaceSpec(t, specs, "gitlab_interactive_issue_create")
	if interactive.SurfaceKind != actioncatalog.SurfaceKindInteractiveUtility || interactive.ReadOnly {
		t.Fatalf("interactive spec = %+v, want non-read-only interactive utility", interactive)
	}
	if len(interactive.CapabilityRequirements) != 1 || interactive.CapabilityRequirements[0] != "elicitation" {
		t.Fatalf("interactive capability requirements = %v, want elicitation", interactive.CapabilityRequirements)
	}
	if !hasActionAlias(interactive, "gitlab_interactive_issue.create") {
		t.Fatalf("interactive compatibility aliases = %+v, want provider-specific issue alias", interactive.Compatibility.ActionAliases)
	}
}

// TestServerMaintenanceToolSpecs_ProjectsUpdateActions verifies updater-backed server maintenance surface metadata.
func TestServerMaintenanceToolSpecs_ProjectsUpdateActions(t *testing.T) {
	if specs := ServerMaintenanceToolSpecs(nil); len(specs) != 0 {
		t.Fatalf("ServerMaintenanceToolSpecs(nil) = %+v, want no specs", specs)
	}

	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{Repository: "owner/repo", CurrentVersion: "1.0.0"}, nil)
	specs := ServerMaintenanceToolSpecs(updater)
	if len(specs) != 2 {
		t.Fatalf("ServerMaintenanceToolSpecs() len = %d, want 2", len(specs))
	}
	checkUpdate := findSurfaceSpec(t, specs, "gitlab_server_check_update")
	if checkUpdate.SurfaceKind != actioncatalog.SurfaceKindServerMaintenance || !checkUpdate.ReadOnly || checkUpdate.GroupToolName != "gitlab_server" {
		t.Fatalf("check update spec = %+v, want read-only server maintenance action", checkUpdate)
	}
	applyUpdate := findSurfaceSpec(t, specs, "gitlab_server_apply_update")
	if !applyUpdate.Destructive || applyUpdate.ReadOnly {
		t.Fatalf("apply update spec = %+v, want destructive non-read-only action", applyUpdate)
	}
}

// TestToolGroupSpecs_ProjectsSurfaceMetadata verifies ToolGroupSpecs projects surface metadata.
func TestToolGroupSpecs_ProjectsSurfaceMetadata(t *testing.T) {
	groups := ToolGroupSpecs(StandaloneToolSpecs(nil))
	if len(groups) != 2 {
		t.Fatalf("ToolGroupSpecs() len = %d, want 2", len(groups))
	}
	var foundInteractive bool
	for _, group := range groups {
		if group.ToolName != "gitlab_interactive" {
			continue
		}
		foundInteractive = true
		if group.SurfaceKind != actioncatalog.SurfaceKindInteractiveUtility || len(group.Actions) != 4 {
			t.Fatalf("interactive group = %+v, want four interactive utility actions", group)
		}
	}
	if !foundInteractive {
		t.Fatalf("ToolGroupSpecs() = %+v, want gitlab_interactive group", groups)
	}
}

// TestStandaloneToolSpecs_ProjectPoliciesAndReadOnlyFilter verifies StandaloneToolSpecs when project policies and read only filter.
func TestStandaloneToolSpecs_ProjectPoliciesAndReadOnlyFilter(t *testing.T) {
	specs := StandaloneToolSpecs(nil)
	for _, spec := range specs {
		if spec.SafeModePolicy != surfaceSafeModeGlobalWrapper || spec.ReadOnlyPolicy != surfaceReadOnlyGlobalFilter {
			t.Fatalf("spec %s policies = safe:%q readonly:%q, want global wrappers", spec.Name, spec.SafeModePolicy, spec.ReadOnlyPolicy)
		}
	}

	groups := ToolGroupSpecs(specs)
	interactive := findGroupSpec(t, groups, "gitlab_interactive")
	if len(interactive.CapabilityRequirements) != 1 || interactive.CapabilityRequirements[0] != "elicitation" {
		t.Fatalf("interactive capability requirements = %v, want elicitation", interactive.CapabilityRequirements)
	}

	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{ReadOnlyOnly: true})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}
	if _, ok := catalog.Action(actioncatalog.ActionID("discover_project.resolve")); !ok {
		t.Fatal("read-only surface catalog missing discover_project.resolve")
	}
	if _, ok := catalog.Action(actioncatalog.ActionID("interactive.issue_create")); ok {
		t.Fatal("read-only surface catalog included mutating interactive.issue_create")
	}
}

// TestAddToolCatalog_ExcludesBySurfaceAndGroupName verifies catalog projection filters explicit tool and group names.
func TestAddToolCatalog_ExcludesBySurfaceAndGroupName(t *testing.T) {
	specs := StandaloneToolSpecs(nil)
	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{ExcludeToolNames: []string{" ", "gitlab_discover_project", "gitlab_interactive_issue_create"}})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}
	if _, ok := catalog.Action(actioncatalog.ActionID("discover_project.resolve")); ok {
		t.Fatal("catalog included discover_project.resolve despite excluded group name")
	}
	if _, ok := catalog.Action(actioncatalog.ActionID("interactive.issue_create")); ok {
		t.Fatal("catalog included interactive.issue_create despite excluded surface tool name")
	}
	if _, ok := catalog.Action(actioncatalog.ActionID("interactive.mr_create")); !ok {
		t.Fatal("catalog missing interactive.mr_create after excluding only issue_create")
	}
}

// TestAddToolCatalog_DuplicateGroupIncludesActionLabel verifies duplicate group errors identify the first action label.
func TestAddToolCatalog_DuplicateGroupIncludesActionLabel(t *testing.T) {
	specs := []actioncatalog.SurfaceToolSpec{findSurfaceSpec(t, StandaloneToolSpecs(nil), "gitlab_discover_project")}
	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{})
	if err != nil {
		t.Fatalf("AddToolCatalog(first) error = %v", err)
	}
	_, err = AddToolCatalog(catalog, specs, CatalogOptions{})
	if err == nil {
		t.Fatal("AddToolCatalog(second) error = nil, want duplicate group error")
	}
	if !strings.Contains(err.Error(), "gitlab_discover_project.resolve") {
		t.Fatalf("duplicate error = %v, want surface action label", err)
	}
}

// TestAddToolCatalog_BuildGroupError verifies duplicate action names in one
// surface group are reported while building the catalog group.
func TestAddToolCatalog_BuildGroupError(t *testing.T) {
	specs := []actioncatalog.SurfaceToolSpec{
		testSurfaceToolSpec("gitlab_test_one", "run"),
		testSurfaceToolSpec("gitlab_test_two", "run"),
	}
	_, err := AddToolCatalog(nil, specs, CatalogOptions{})
	if err == nil {
		t.Fatal("AddToolCatalog() error = nil, want duplicate action error")
	}
	if !strings.Contains(err.Error(), "build surface tool group gitlab_test") {
		t.Fatalf("error = %v, want build group context", err)
	}
}

// TestSurfaceGroupActionLabel_EmptyGroup verifies labels fall back to the tool
// name when a group has no actions.
func TestSurfaceGroupActionLabel_EmptyGroup(t *testing.T) {
	label := surfaceGroupActionLabel(actioncatalog.Group{ToolName: "gitlab_empty"})
	if label != "gitlab_empty" {
		t.Fatalf("surfaceGroupActionLabel() = %q, want gitlab_empty", label)
	}
}

// TestToolGroupSpecs_EmptyAndInvalidInputs verifies grouping handles empty input and fails fast on invalid specs.
func TestToolGroupSpecs_EmptyAndInvalidInputs(t *testing.T) {
	if groups := ToolGroupSpecs(nil); groups != nil {
		t.Fatalf("ToolGroupSpecs(nil) = %+v, want nil", groups)
	}

	defer func() {
		panicValue := recover()
		if panicValue == nil {
			t.Fatal("ToolGroupSpecs(invalid) did not panic")
		}
		if !strings.Contains(panicValue.(error).Error(), "project surface tool broken") {
			t.Fatalf("panic = %v, want invalid surface tool context", panicValue)
		}
	}()
	ToolGroupSpecs([]actioncatalog.SurfaceToolSpec{{Name: "broken"}})
}

// TestSurfaceGroupingHelpers_CoverReadOnlyAndStringSetBranches verifies local helper behavior that shapes catalog groups.
func TestSurfaceGroupingHelpers_CoverReadOnlyAndStringSetBranches(t *testing.T) {
	if readOnlyGroup(nil) {
		t.Fatal("readOnlyGroup(nil) = true, want false")
	}
	readOnlySpec := toolutil.NewActionSpec("get", toolutil.RouteFunc(func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil }), toolutil.ActionSpecOptions{ReadOnly: true})
	mutatingSpec := toolutil.NewActionSpec("create", toolutil.RouteFunc(func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil }), toolutil.ActionSpecOptions{})
	if !readOnlyGroup([]toolutil.ActionSpec{readOnlySpec}) {
		t.Fatal("readOnlyGroup(read-only) = false, want true")
	}
	if readOnlyGroup([]toolutil.ActionSpec{readOnlySpec, mutatingSpec}) {
		t.Fatal("readOnlyGroup(mixed) = true, want false")
	}

	set := stringSet([]string{" alpha ", "", "beta"})
	if _, ok := set["alpha"]; !ok {
		t.Fatalf("stringSet() = %+v, want trimmed alpha", set)
	}
	if _, ok := set[""]; ok {
		t.Fatalf("stringSet() = %+v, want empty value skipped", set)
	}

	filtered := filterToolSpecs([]actioncatalog.SurfaceToolSpec{testSurfaceToolSpec("gitlab_test_one", "run")}, CatalogOptions{ExcludeToolNames: []string{"gitlab_test"}})
	if len(filtered) != 0 {
		t.Fatalf("filterToolSpecs(group exclusion) = %+v, want empty", filtered)
	}
}

func testSurfaceToolSpec(name, actionName string) actioncatalog.SurfaceToolSpec {
	return actioncatalog.SurfaceToolSpec{
		Name:          name,
		Description:   "Test surface.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    actionName,
		SurfaceKind:   actioncatalog.SurfaceKindRuntimeUtility,
		Route:         toolutil.RouteFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil }),
		OwnerPackage:  "surfaces",
		ReadOnly:      true,
	}
}

// findSurfaceSpec locates surface spec fixture data for assertions.
func findSurfaceSpec(t *testing.T, specs []actioncatalog.SurfaceToolSpec, name string) actioncatalog.SurfaceToolSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("surface spec %q not found in %+v", name, specs)
	return actioncatalog.SurfaceToolSpec{}
}

// findGroupSpec locates group spec fixture data for assertions.
func findGroupSpec(t *testing.T, groups []actioncatalog.CatalogGroupSpec, name string) actioncatalog.CatalogGroupSpec {
	t.Helper()
	for _, group := range groups {
		if group.ToolName == name {
			return group
		}
	}
	t.Fatalf("catalog group spec %q not found in %+v", name, groups)
	return actioncatalog.CatalogGroupSpec{}
}

// hasActionAlias reports whether has action alias.
func hasActionAlias(spec actioncatalog.SurfaceToolSpec, alias string) bool {
	for _, actionAlias := range spec.Compatibility.ActionAliases {
		if actionAlias.Alias == alias {
			return true
		}
	}
	return false
}
