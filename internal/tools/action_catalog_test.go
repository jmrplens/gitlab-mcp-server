package tools

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestBuildActionCatalog_IncludesBaseEnterpriseAndMCPActions verifies BuildActionCatalog includes base enterprise and MCP actions.
func TestBuildActionCatalog_IncludesBaseEnterpriseAndMCPActions(t *testing.T) {
	t.Run("base", func(t *testing.T) {
		base, err := BuildActionCatalog(nil, ActionCatalogOptions{})
		if err != nil {
			t.Fatalf("BuildActionCatalog(base) error = %v", err)
		}
		if base.CountGroups() == 0 || base.CountActions() == 0 {
			t.Fatalf("base catalog counts = groups %d actions %d, want non-zero", base.CountGroups(), base.CountActions())
		}
		if _, ok := base.Action("project.list"); !ok {
			t.Fatal("base catalog missing project.list")
		}
		if _, ok := base.Action("server.status"); ok {
			t.Fatal("base catalog contains server.status without IncludeMCP")
		}
	})

	t.Run("mcp", func(t *testing.T) {
		withMCP, err := BuildActionCatalog(nil, ActionCatalogOptions{IncludeMCP: true})
		if err != nil {
			t.Fatalf("BuildActionCatalog(with MCP) error = %v", err)
		}
		if _, ok := withMCP.Action("server.status"); !ok {
			t.Fatal("MCP catalog missing server.status")
		}
	})

	t.Run("enterprise", func(t *testing.T) {
		base, err := BuildActionCatalog(nil, ActionCatalogOptions{})
		if err != nil {
			t.Fatalf("BuildActionCatalog(base) error = %v", err)
		}
		enterprise, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
		if err != nil {
			t.Fatalf("BuildActionCatalog(enterprise) error = %v", err)
		}
		if enterprise.CountActions() <= base.CountActions() {
			t.Fatalf("enterprise action count = %d, want greater than base %d", enterprise.CountActions(), base.CountActions())
		}
	})
}

// TestBuildActionCatalog_DoesNotUseMetaRegistrationCapture verifies BuildActionCatalog does not use meta registration capture.
func TestBuildActionCatalog_DoesNotUseMetaRegistrationCapture(t *testing.T) {
	source, err := os.ReadFile("action_catalog.go")
	if err != nil {
		t.Fatalf("ReadFile(action_catalog.go) error = %v", err)
	}
	for _, forbidden := range []string{"CaptureMetaToolDefinitions", "registerAllMetaGroups("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("action_catalog.go contains %q; catalog construction must use ActionSpec groups directly", forbidden)
		}
	}
}

// TestBuildActionCatalog_CapturesInlineAndDelegatedGroups verifies BuildActionCatalog captures inline and delegated groups.
func TestBuildActionCatalog_CapturesInlineAndDelegatedGroups(t *testing.T) {
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	for _, actionID := range []string{"project.list", "search.code", "runner.list", "analyze.issue_summary"} {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(actionID)); !ok {
				t.Fatalf("catalog missing %s", actionID)
			}
		})
	}

	group, ok := catalog.Group("gitlab_analyze")
	if !ok {
		t.Fatal("catalog missing gitlab_analyze group")
	}
	if !group.ReadOnly {
		t.Fatal("gitlab_analyze group should be read-only")
	}
	if group.FormatResult == nil {
		t.Fatal("gitlab_analyze group should preserve its custom formatter")
	}
}

// TestBuildActionCatalog_KeyBuilderRoutes verifies that representative split
// meta builders preserve catalog metadata. It builds the enterprise catalog and
// checks expected tool names, derived action IDs and domains, destructive flags,
// schema URIs, and input schemas so future builder moves cannot silently weaken
// the canonical action catalog.
func TestBuildActionCatalog_KeyBuilderRoutes(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

	testCases := []struct {
		name        string
		toolName    string
		actionName  string
		destructive bool
	}{
		{name: "source project list", toolName: "gitlab_project", actionName: "list"},
		{name: "source repository file delete", toolName: "gitlab_repository", actionName: "file_delete", destructive: true},
		{name: "collaboration merge request merge", toolName: "gitlab_merge_request", actionName: "merge", destructive: true},
		{name: "delivery package download", toolName: "gitlab_package", actionName: "download"},
		{name: "admin access token revoke", toolName: "gitlab_access", actionName: "token_project_revoke", destructive: true},
		{name: "admin group list", toolName: "gitlab_group", actionName: "list"},
		{name: "enterprise vulnerability list", toolName: "gitlab_vulnerability", actionName: "list"},
		{name: "enterprise geo delete", toolName: "gitlab_geo", actionName: "delete", destructive: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group, ok := catalog.Group(tc.toolName)
			if !ok {
				t.Fatalf("catalog missing group %s", tc.toolName)
			}
			if group.ToolName != tc.toolName {
				t.Fatalf("group.ToolName = %q, want %q", group.ToolName, tc.toolName)
			}

			action, ok := group.Actions[tc.actionName]
			if !ok {
				t.Fatalf("%s missing action %s", tc.toolName, tc.actionName)
			}
			wantDomain := actioncatalog.DomainFromToolName(tc.toolName)
			wantID := actioncatalog.ActionID(wantDomain + "." + tc.actionName)
			if action.ID != wantID {
				t.Fatalf("action.ID = %q, want %q", action.ID, wantID)
			}
			if action.Domain != wantDomain {
				t.Fatalf("action.Domain = %q, want %q", action.Domain, wantDomain)
			}
			if action.Route.Destructive != tc.destructive {
				t.Fatalf("action.Route.Destructive = %v, want %v", action.Route.Destructive, tc.destructive)
			}
			wantSchemaURI := toolutil.MetaSchemaURI(tc.toolName, tc.actionName)
			if action.SchemaURI != wantSchemaURI {
				t.Fatalf("action.SchemaURI = %q, want %q", action.SchemaURI, wantSchemaURI)
			}
			if action.Route.InputSchema == nil {
				t.Fatal("action.Route.InputSchema is nil")
			}
		})
	}
}

// TestBuildActionCatalog_EnterpriseAndGitLabDotComGates verifies catalog gating
// for base, enterprise, and GitLab.com enterprise surfaces. It compares action
// presence across those catalogs so enterprise-only routes and GitLab.com-only
// Orbit routes remain registered only for the intended surfaces.
func TestBuildActionCatalog_EnterpriseAndGitLabDotComGates(t *testing.T) {
	base := mustBuildActionCatalog(t, nil, ActionCatalogOptions{})
	enterprise := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})
	gitLabDotComEnterprise := mustBuildActionCatalog(t, newGitLabDotComClient(t), ActionCatalogOptions{Enterprise: true})

	for _, actionID := range []actioncatalog.ActionID{
		"merge_train.list_project",
		"audit_event.list_instance",
		"dora_metrics.project",
		"dependency.list",
		"vulnerability.list",
		"security_finding.list",
		"project.push_rule_get",
		"group.epic_list",
		"issue.iteration_list_project",
	} {
		t.Run(string(actionID), func(t *testing.T) {
			assertCatalogMissingAction(t, base, actionID)
			assertCatalogHasAction(t, enterprise, actionID)
			assertCatalogHasAction(t, gitLabDotComEnterprise, actionID)
		})
	}

	for _, actionID := range []actioncatalog.ActionID{"orbit.status", "orbit.dsl"} {
		assertCatalogMissingAction(t, base, actionID)
		assertCatalogMissingAction(t, enterprise, actionID)
		assertCatalogHasAction(t, gitLabDotComEnterprise, actionID)
	}
}

// TestBuildMCPActionGroup_NilUpdaterOmitsUpdateActions verifies BuildMCPActionGroup when nil updater omits update actions.
func TestBuildMCPActionGroup_NilUpdaterOmitsUpdateActions(t *testing.T) {
	group := BuildMCPActionGroup(nil, nil)
	if _, ok := group.Actions["status"]; !ok {
		t.Fatal("BuildMCPActionGroup(nil updater) missing status action")
	}
	if _, ok := group.Actions["apply_update"]; ok {
		t.Fatal("BuildMCPActionGroup(nil updater) contains apply_update")
	}
	if group.ToolName != "gitlab_server" || group.Description == "" || len(group.Icons) == 0 {
		t.Fatalf("BuildMCPActionGroup metadata = %+v, want tool name, description, and icons", group)
	}
}

// TestBuildActionCatalog_WithUpdaterIncludesUpdateSchemas verifies updater-backed
// server maintenance actions remain valid in the dynamic action catalog.
func TestBuildActionCatalog_WithUpdaterIncludesUpdateSchemas(t *testing.T) {
	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Mode:           autoupdate.ModeCheck,
		Repository:     "owner/repo",
		CurrentVersion: "1.0.0",
	}, autoupdate.EmptySource{})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{IncludeMCP: true, Updater: updater})
	if err != nil {
		t.Fatalf("BuildActionCatalog(with updater) error = %v", err)
	}

	for _, actionID := range []actioncatalog.ActionID{"server.check_update", "server.apply_update"} {
		action, ok := catalog.Action(actionID)
		if !ok {
			t.Fatalf("catalog missing %s", actionID)
		}
		if action.Route.InputSchema == nil {
			t.Fatalf("%s Route.InputSchema is nil", actionID)
		}
		if got := action.Route.InputSchema["type"]; got != "object" {
			t.Fatalf("%s input schema type = %v, want object", actionID, got)
		}
	}
}

// TestBuildActionCatalog_UsesCanonicalActionSpecs verifies BuildActionCatalog uses canonical action specs.
func TestBuildActionCatalog_UsesCanonicalActionSpecs(t *testing.T) {
	spec := toolutil.NewActionSpec("list", testCatalogActionRoute("search"), toolutil.ActionSpecOptions{
		Aliases:           []string{"group.search"},
		Tags:              []string{"Group"},
		Usage:             "Use to list groups with optional search filtering.",
		RelatedActions:    []string{"group.get"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{"search": {SemanticRole: "group_search_query"}},
		ReadOnly:          true,
		Idempotent:        true,
		OpenWorld:         true,
		OwnerPackage:      "groups",
		IndividualTool:    toolutil.IndividualToolSpec{Name: "gitlab_group_list", Title: "List groups"},
	})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_group", Actions: []toolutil.ActionSpec{spec}}}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	action, ok := catalog.Action("group.list")
	if !ok {
		t.Fatal("catalog missing group.list")
	}
	if action.Usage != "Use to list groups with optional search filtering." || action.OwnerPackage != "groups" {
		t.Fatalf("action metadata = %+v, want spec metadata", action)
	}
	if !slices.Contains(action.Aliases, "group.search") || !slices.Contains(action.Tags, "group") || !slices.Contains(action.RelatedActions, "group.get") {
		t.Fatalf("action search metadata = aliases %+v tags %+v related %+v", action.Aliases, action.Tags, action.RelatedActions)
	}
	if action.Route.ParameterGuidance["search"].SemanticRole != "group_search_query" {
		t.Fatalf("route guidance = %+v, want spec guidance", action.Route.ParameterGuidance)
	}
}

// TestBuildActionCatalog_UsesCollectedActionSpecGuidance verifies BuildActionCatalog uses collected action spec guidance.
func TestBuildActionCatalog_UsesCollectedActionSpecGuidance(t *testing.T) {
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	action, ok := catalog.Action("job.token_scope_remove_project")
	if !ok {
		t.Fatal("catalog missing job.token_scope_remove_project")
	}
	if !action.SpecBacked {
		t.Fatal("job.token_scope_remove_project is not spec-backed")
	}
	guidance := action.Route.ParameterGuidance
	if guidance["project_id"].SemanticRole != "scope_owner_project" {
		t.Fatalf("project_id guidance = %+v, want canonical spec guidance", guidance["project_id"])
	}
	if guidance["target_project_id"].SemanticRole != "target_project" {
		t.Fatalf("target_project_id guidance = %+v, want canonical spec guidance", guidance["target_project_id"])
	}
}

// TestBuildActionCatalog_ExplicitSpecOverridesCatalogRoute verifies BuildActionCatalog when explicit spec overrides catalog route.
func TestBuildActionCatalog_ExplicitSpecOverridesCatalogRoute(t *testing.T) {
	spec := toolutil.NewActionSpec("token_scope_add_project", testCatalogActionRoute("project_id"), toolutil.ActionSpecOptions{
		ParameterGuidance: map[string]toolutil.ParameterGuidance{"project_id": {SemanticRole: "spec_scope_project"}},
	})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_job", Actions: []toolutil.ActionSpec{spec}}}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	action, ok := catalog.Action("job.token_scope_add_project")
	if !ok {
		t.Fatal("catalog missing job.token_scope_add_project")
	}
	guidance := action.Route.ParameterGuidance
	if guidance["project_id"].SemanticRole != "spec_scope_project" {
		t.Fatalf("project_id guidance = %+v, want spec guidance", guidance["project_id"])
	}
}

// TestBuildActionCatalog_AcceptsExplicitSpecGroupActions verifies BuildActionCatalog accepts explicit spec group actions.
func TestBuildActionCatalog_AcceptsExplicitSpecGroupActions(t *testing.T) {
	spec := toolutil.NewActionSpec("not_captured", testCatalogActionRoute("project_id"), toolutil.ActionSpecOptions{})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_project", Actions: []toolutil.ActionSpec{spec}}}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if _, ok := catalog.Action("project.not_captured"); !ok {
		t.Fatal("catalog missing project.not_captured")
	}
}

// TestBuildActionCatalog_DuplicateMCPGroupReturnsContext verifies IncludeMCP
// reports duplicate server-group registration with MCP action context.
func TestBuildActionCatalog_DuplicateMCPGroupReturnsContext(t *testing.T) {
	spec := toolutil.NewActionSpec("custom_status", testCatalogActionRoute(), toolutil.ActionSpecOptions{})

	_, err := BuildActionCatalog(nil, ActionCatalogOptions{
		IncludeMCP: true,
		SpecGroups: []ActionSpecGroup{{
			ToolName:    "gitlab_server",
			Description: "Custom server group.",
			Actions:     []toolutil.ActionSpec{spec},
		}},
	})
	if err == nil {
		t.Fatal("expected duplicate MCP group error")
	}
	if !strings.Contains(err.Error(), "add MCP action group") {
		t.Fatalf("BuildActionCatalog() error = %v, want MCP group context", err)
	}
}

// TestBuildActionCatalog_ActionSpecMapErrorReturnsContext verifies invalid
// explicit actions fail while deriving a generated group description.
func TestBuildActionCatalog_ActionSpecMapErrorReturnsContext(t *testing.T) {
	_, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{
		ToolName: "gitlab_invalid",
		Actions:  []toolutil.ActionSpec{{Name: ""}},
	}}})
	if err == nil {
		t.Fatal("expected invalid action spec error")
	}
	if !strings.Contains(err.Error(), `build catalog group "gitlab_invalid"`) {
		t.Fatalf("BuildActionCatalog() error = %v, want group context", err)
	}
}

// TestMergeActionSpecGroupOverrides_HandlesBlankOverrideMetadata verifies MergeActionSpecGroupOverrides handles blank override metadata.
func TestMergeActionSpecGroupOverrides_HandlesBlankOverrideMetadata(t *testing.T) {
	base := []ActionSpecGroup{{ToolName: "gitlab_project", Actions: []toolutil.ActionSpec{
		toolutil.NewActionSpec("get", testCatalogActionRoute("project_id"), toolutil.ActionSpecOptions{}),
		toolutil.NewActionSpec("list", testCatalogActionRoute("search"), toolutil.ActionSpecOptions{}),
	}}}
	overrides := []ActionSpecGroup{
		{ToolName: "", Actions: []toolutil.ActionSpec{toolutil.NewActionSpec("ignored", testCatalogActionRoute(), toolutil.ActionSpecOptions{})}},
		{ToolName: "gitlab_project", Actions: []toolutil.ActionSpec{{Name: ""}, toolutil.NewActionSpec("get", testCatalogActionRoute("id"), toolutil.ActionSpecOptions{})}},
	}

	merged := mergeActionSpecGroupOverrides(base, overrides)
	if len(merged) != 2 {
		t.Fatalf("merged groups = %+v, want invalid override plus consolidated group", merged)
	}
	if len(merged[1].Actions) != 3 || merged[1].Actions[0].Name != "list" || merged[1].Actions[2].Name != "get" {
		t.Fatalf("merged specs = %+v, want list, invalid override, then get", merged[1].Actions)
	}
}

// TestMergeActionSpecGroupOverrides_PreservesInvalidBaseGroup verifies base
// groups without a tool name are carried through for downstream validation.
func TestMergeActionSpecGroupOverrides_PreservesInvalidBaseGroup(t *testing.T) {
	base := []ActionSpecGroup{{ToolName: " "}}
	overrides := []ActionSpecGroup{{ToolName: "gitlab_project", Actions: []toolutil.ActionSpec{
		toolutil.NewActionSpec("list", testCatalogActionRoute(), toolutil.ActionSpecOptions{}),
	}}}

	merged := mergeActionSpecGroupOverrides(base, overrides)
	if len(merged) != 2 {
		t.Fatalf("merged groups = %+v, want invalid base plus override", merged)
	}
	if strings.TrimSpace(merged[0].ToolName) != "" {
		t.Fatalf("first merged group = %+v, want invalid base first", merged[0])
	}
}

// TestMergeActionSpecGroup_MetadataOverrides verifies explicit groups can
// override all group-level metadata while preserving base actions not replaced.
func TestMergeActionSpecGroup_MetadataOverrides(t *testing.T) {
	base := ActionSpecGroup{
		ToolName:     "gitlab_project",
		Title:        "Base title",
		Description:  "Base description",
		Actions:      []toolutil.ActionSpec{toolutil.NewActionSpec("list", testCatalogActionRoute(), toolutil.ActionSpecOptions{})},
		OwnerPackage: "baseowner",
	}
	formatter := func(any) *mcp.CallToolResult { return &mcp.CallToolResult{} }
	override := ActionSpecGroup{
		ToolName:               " gitlab_project ",
		Title:                  "Override title",
		Description:            "Override description",
		Icons:                  []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}},
		ReadOnly:               true,
		BaseDomain:             "project_override",
		EnterpriseOnly:         true,
		GitLabDotComOnly:       true,
		CapabilityRequirements: []string{"roots"},
		FormatResult:           formatter,
		OwnerPackage:           "overrideowner",
		SurfaceKind:            actioncatalog.SurfaceKindRuntimeUtility,
		Actions:                []toolutil.ActionSpec{toolutil.NewActionSpec("get", testCatalogActionRoute(), toolutil.ActionSpecOptions{})},
	}

	merged := mergeActionSpecGroup(base, override)
	if merged.ToolName != "gitlab_project" || merged.Title != "Override title" || merged.Description != "Override description" {
		t.Fatalf("merged basic metadata = %+v", merged)
	}
	if len(merged.Icons) != 1 || merged.Icons[0].Source == "" {
		t.Fatalf("merged icons = %+v, want override icon", merged.Icons)
	}
	if !merged.ReadOnly || merged.BaseDomain != "project_override" || !merged.EnterpriseOnly || !merged.GitLabDotComOnly {
		t.Fatalf("merged flags = %+v, want override flags", merged)
	}
	if !slices.Equal(merged.CapabilityRequirements, []string{"roots"}) {
		t.Fatalf("capability requirements = %+v, want roots", merged.CapabilityRequirements)
	}
	if merged.FormatResult == nil || merged.FormatResult(nil) == nil {
		t.Fatal("merged formatter was not preserved")
	}
	if merged.OwnerPackage != "overrideowner" || merged.SurfaceKind != actioncatalog.SurfaceKindRuntimeUtility {
		t.Fatalf("merged owner/surface = %q/%q", merged.OwnerPackage, merged.SurfaceKind)
	}
	if len(merged.Actions) != 2 || merged.Actions[0].Name != "list" || merged.Actions[1].Name != "get" {
		t.Fatalf("merged actions = %+v, want base list plus override get", merged.Actions)
	}
}

// TestBuildActionCatalog_InvalidExplicitGroupReturnsContext verifies invalid
// explicit groups fail with catalog-group context instead of surfacing raw validation errors.
func TestBuildActionCatalog_InvalidExplicitGroupReturnsContext(t *testing.T) {
	_, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_invalid"}}})
	if err == nil {
		t.Fatal("BuildActionCatalog() error = nil, want invalid group error")
	}
	if !strings.Contains(err.Error(), `build catalog group "gitlab_invalid"`) {
		t.Fatalf("BuildActionCatalog() error = %v, want group context", err)
	}
}

// TestEnsureActionSpecOwners_FillsMissingOwnersDefensively verifies owner
// defaults are applied to clones without mutating caller-owned specs.
func TestEnsureActionSpecOwners_FillsMissingOwnersDefensively(t *testing.T) {
	specs := []toolutil.ActionSpec{
		toolutil.NewActionSpec("missing", testCatalogActionRoute(), toolutil.ActionSpecOptions{}),
		toolutil.NewActionSpec("existing", testCatalogActionRoute(), toolutil.ActionSpecOptions{OwnerPackage: "custom"}),
	}

	got := ensureActionSpecOwners(specs, "fallback")
	if len(got) != 2 {
		t.Fatalf("ensureActionSpecOwners() returned %d specs, want 2", len(got))
	}
	if got[0].OwnerPackage != "fallback" || got[1].OwnerPackage != "custom" {
		t.Fatalf("owners = %q/%q, want fallback/custom", got[0].OwnerPackage, got[1].OwnerPackage)
	}
	got[0].OwnerPackage = "mutated"
	if specs[0].OwnerPackage != "" {
		t.Fatalf("input spec owner mutated to %q", specs[0].OwnerPackage)
	}
	if ensureActionSpecOwners(nil, "fallback") != nil {
		t.Fatal("ensureActionSpecOwners(nil) returned non-nil")
	}
}

// testCatalogActionRoute supports test catalog action route assertions in tools tests.
func testCatalogActionRoute(params ...string) toolutil.ActionRoute {
	properties := make(map[string]any, len(params))
	for _, param := range params {
		properties[param] = map[string]any{"type": "string"}
	}
	return toolutil.ActionRoute{
		Handler: func(context.Context, map[string]any) (any, error) {
			return map[string]any{}, nil
		},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": properties,
		},
	}
}

// mustBuildActionCatalog builds action catalog test fixtures and fails the test on error.
func mustBuildActionCatalog(t *testing.T, client *gitlabclient.Client, opts ActionCatalogOptions) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := BuildActionCatalog(client, opts)
	if err != nil {
		t.Fatalf("BuildActionCatalog(%+v) error = %v", opts, err)
	}
	return catalog
}

// newGitLabDotComClient constructs GitLab dot com client test fixtures.
func newGitLabDotComClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   "https://gitlab.com",
		GitLabToken: "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient(gitlab.com) error = %v", err)
	}
	return client
}

// assertCatalogHasAction checks catalog has action invariants for tests.
func assertCatalogHasAction(t *testing.T, catalog *actioncatalog.Catalog, actionID actioncatalog.ActionID) {
	t.Helper()
	if _, ok := catalog.Action(actionID); !ok {
		t.Fatalf("catalog missing action %s", actionID)
	}
}

// assertCatalogMissingAction checks catalog missing action invariants for tests.
func assertCatalogMissingAction(t *testing.T, catalog *actioncatalog.Catalog, actionID actioncatalog.ActionID) {
	t.Helper()
	if _, ok := catalog.Action(actionID); ok {
		t.Fatalf("catalog contains action %s", actionID)
	}
}

const (
	// expectedBaseDynamicCatalogActions identifies the expected base dynamic catalog actions constant used by this package.
	expectedBaseDynamicCatalogActions = 871
	// expectedEnterpriseDynamicCatalogActions identifies the expected enterprise dynamic catalog actions constant used by this package.
	expectedEnterpriseDynamicCatalogActions = 1031
	// expectedGitLabComEnterpriseCatalogActions identifies the expected GitLab com enterprise catalog actions constant used by this package.
	expectedGitLabComEnterpriseCatalogActions = 1037
)

// TestActionCatalog_BaselineCountsDoNotRegress covers ActionCatalog with table-driven subtests for baseline counts do not regress.
func TestActionCatalog_BaselineCountsDoNotRegress(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
		want       int
	}{
		{name: "base", want: expectedBaseDynamicCatalogActions},
		{name: "self-managed enterprise", enterprise: true, want: expectedEnterpriseDynamicCatalogActions},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true, want: expectedGitLabComEnterpriseCatalogActions},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := mustBuildDynamicActionCatalogForTest(t, tc.client, tc.enterprise)
			if got := catalog.CountActions(); got != tc.want {
				t.Fatalf("dynamic catalog action count = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestActionSpecCoverage_AllCatalogRoutesClassified verifies ActionSpecCoverage when all catalog routes classified.
func TestActionSpecCoverage_AllCatalogRoutesClassified(t *testing.T) {
	catalog := mustBuildDynamicActionCatalogForTest(t, newGitLabDotComClient(t), true)
	missing := make([]actioncatalog.ActionID, 0)
	for _, action := range catalog.Actions() {
		if action.SpecBacked {
			continue
		}
		missing = append(missing, action.ID)
	}
	if len(missing) > 0 {
		t.Fatalf("catalog actions must be spec-backed:\n%s", formatMissingActionSpecs(missing))
	}
}

// mustBuildDynamicActionCatalogForTest builds dynamic action catalog for test test fixtures and fails the test on error.
func mustBuildDynamicActionCatalogForTest(t *testing.T, client *gitlabclient.Client, enterprise bool) *actioncatalog.Catalog {
	t.Helper()
	catalog := mustBuildActionCatalog(t, client, ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	catalog, err := dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	return catalog
}

// formatMissingActionSpecs renders the result as a formatted string.
func formatMissingActionSpecs(ids []actioncatalog.ActionID) string {
	var builder strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&builder, "\t%s\n", id)
	}
	return builder.String()
}
