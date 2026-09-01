package tools

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestBuildActionCatalog_IncludesBaseEnterpriseAndMCPActions verifies BuildActionCatalog includes base enterprise and MCP actions.
func TestBuildActionCatalog_IncludesBaseEnterpriseAndMCPActions(t *testing.T) {
	t.Run("base", func(t *testing.T) {
		base := mustBuildActionCatalog(t, nil, ActionCatalogOptions{})
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
		withMCP := mustBuildActionCatalog(t, nil, ActionCatalogOptions{IncludeMCP: true})
		if _, ok := withMCP.Action("server.status"); !ok {
			t.Fatal("MCP catalog missing server.status")
		}
	})

	t.Run("enterprise", func(t *testing.T) {
		base := mustBuildActionCatalog(t, nil, ActionCatalogOptions{})
		enterprise := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})
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
		t.Run(forbidden, func(t *testing.T) {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("action_catalog.go contains %q; catalog construction must use ActionSpec groups directly", forbidden)
			}
		})
	}
}

// TestBuildActionCatalog_CapturesInlineAndDelegatedGroups verifies BuildActionCatalog captures inline and delegated groups.
func TestBuildActionCatalog_CapturesInlineAndDelegatedGroups(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

	for _, actionID := range []string{"project.list", "search.code", "runner.list"} {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(actionID)); !ok {
				t.Fatalf("catalog missing %s", actionID)
			}
		})
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
		t.Run(string(actionID), func(t *testing.T) {
			assertCatalogMissingAction(t, base, actionID)
			assertCatalogMissingAction(t, enterprise, actionID)
			assertCatalogHasAction(t, gitLabDotComEnterprise, actionID)
		})
	}
}

// TestBuildMCPActionGroup_IsReadOnlyDiagnostics pins what gitlab_server is.
//
// It is diagnostics and nothing else. The group used to gain check_update and
// apply_update when a self-updater was configured, and apply_update was this
// server's only destructive action that acted on the machine rather than on
// GitLab. Both are gone with the self-update subsystem, so a group that grew a
// mutating action again would be a surprise worth failing on.
func TestBuildMCPActionGroup_IsReadOnlyDiagnostics(t *testing.T) {
	group := BuildMCPActionGroup(nil)
	for _, want := range []string{"status", "health_check"} {
		t.Run(want, func(t *testing.T) {
			if _, ok := group.Actions[want]; !ok {
				t.Errorf("BuildMCPActionGroup is missing the %s action", want)
			}
		})
	}
	if len(group.Actions) != 2 {
		t.Errorf("gitlab_server carries %d actions, want only status and health_check: %v",
			len(group.Actions), group.Actions)
	}
	if !group.ReadOnly {
		t.Error("gitlab_server is not marked read-only; it reaches nothing that mutates")
	}
	if group.ToolName != "gitlab_server" || group.Description == "" || len(group.Icons) == 0 {
		t.Fatalf("BuildMCPActionGroup metadata = %+v, want tool name, description, and icons", group)
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
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{})
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

// TestBuildActionCatalog_CrossGroupDuplicateActionIDReturnsAddGroupContext
// verifies that when two distinct explicit spec groups (different
// ToolNames, so mergeActionSpecGroupOverrides never folds them together)
// resolve to the same canonical action ID, the second catalog.AddGroup
// call inside BuildActionCatalog's specGroups loop fails and the error is
// wrapped with "add catalog group %q" context naming the group that lost
// the race — distinct from the "build catalog group" context (a
// groupFromActionSpecGroup failure, covered by
// TestBuildActionCatalog_ActionSpecMapErrorReturnsContext) and the
// "add MCP action group" context (covered by
// TestBuildActionCatalog_DuplicateMCPGroupReturnsContext). Both probe
// groups share BaseDomain "zzz_probe_shared", so both project their "get"
// action to the identical ID "zzz_probe_shared.get" regardless of their
// own distinct ToolNames.
func TestBuildActionCatalog_CrossGroupDuplicateActionIDReturnsAddGroupContext(t *testing.T) {
	spec := toolutil.NewActionSpec("get", testCatalogActionRoute(), toolutil.ActionSpecOptions{ReadOnly: true, Idempotent: true})

	_, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{
		{ToolName: "gitlab_zzz_probe_a", BaseDomain: "zzz_probe_shared", Actions: []toolutil.ActionSpec{spec}},
		{ToolName: "gitlab_zzz_probe_b", BaseDomain: "zzz_probe_shared", Actions: []toolutil.ActionSpec{spec}},
	}})
	if err == nil {
		t.Fatal("BuildActionCatalog() error = nil, want cross-group duplicate action id error")
	}
	// mergeActionSpecGroupOverrides sorts merged groups by ToolName before
	// BuildActionCatalog's loop runs them through AddGroup in order, so
	// "gitlab_zzz_probe_a" is added first and "gitlab_zzz_probe_b" is the
	// one that collides.
	if !strings.Contains(err.Error(), `add catalog group "gitlab_zzz_probe_b"`) {
		t.Fatalf("BuildActionCatalog() error = %v, want add catalog group context for the colliding group", err)
	}
	if !strings.Contains(err.Error(), "duplicate action id") {
		t.Fatalf("BuildActionCatalog() error = %v, want it to mention duplicate action id", err)
	}
}

// TestBuildActionCatalog_CrossGroupAliasCollisionReturnsValidateContext
// verifies that BuildActionCatalog's final catalog.Validate() call (after
// every explicit spec group is individually added via AddGroup without
// conflict) still catches a cross-group alias collision and wraps it with
// "validate action catalog" context. AddGroup only checks action-ID and
// group-name uniqueness per group; it does not check aliases across
// groups, so two actions with different IDs (different BaseDomain, so no
// AddGroup conflict) that happen to share a plain Aliases entry pass
// AddGroup for both groups and are only rejected by the aggregate
// seenAliases check inside Catalog.Validate().
func TestBuildActionCatalog_CrossGroupAliasCollisionReturnsValidateContext(t *testing.T) {
	specA := toolutil.NewActionSpec("get", testCatalogActionRoute(), toolutil.ActionSpecOptions{
		ReadOnly: true, Idempotent: true, Aliases: []string{"zzz_shared_alias_probe"},
	})
	specB := toolutil.NewActionSpec("get", testCatalogActionRoute(), toolutil.ActionSpecOptions{
		ReadOnly: true, Idempotent: true, Aliases: []string{"zzz_shared_alias_probe"},
	})

	_, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{
		{ToolName: "gitlab_zzz_alias_probe_a", BaseDomain: "zzz_alias_probe_a", Actions: []toolutil.ActionSpec{specA}},
		{ToolName: "gitlab_zzz_alias_probe_b", BaseDomain: "zzz_alias_probe_b", Actions: []toolutil.ActionSpec{specB}},
	}})
	if err == nil {
		t.Fatal("BuildActionCatalog() error = nil, want cross-group alias collision error")
	}
	if !strings.Contains(err.Error(), "validate action catalog") {
		t.Fatalf("BuildActionCatalog() error = %v, want validate action catalog context", err)
	}
	if !strings.Contains(err.Error(), `alias "zzz_shared_alias_probe" maps to both`) {
		t.Fatalf("BuildActionCatalog() error = %v, want it to name the colliding alias", err)
	}
}

// TestGroupFromActionSpecGroup_DefaultsSurfaceKindToMetaGroup verifies the
// end-to-end contract that an explicit spec group left with a zero-value
// SurfaceKind ends up as actioncatalog.SurfaceKindMetaGroup in the built
// catalog, the same default NewGroup applies to ordinary domain-collected
// groups.
//
// Note: this does NOT exercise the `if specGroup.SurfaceKind == ""`
// assignment at action_catalog.go's groupFromActionSpecGroup (the
// SurfaceKindMetaGroup fallback next to the SurfaceKind check) — that
// branch is unreachable. groupFromActionSpecGroup's first statement clones
// specGroup through actioncatalog.CloneCatalogGroupSpec, which already
// defaults an empty SurfaceKind to SurfaceKindMetaGroup (group_spec.go);
// by the time groupFromActionSpecGroup's own check runs, SurfaceKind can
// never be empty. This mirrors the dead-branch pattern documented by
// TestCatalogGroupSpec_Validate_DeadBranches and
// TestActionsFromSpecs_SeenGuardIsUnreachable in the actioncatalog
// package: a defensive check made redundant by an earlier layer. We keep
// this test for the observable contract (the default reaches the built
// catalog) even though the specific redundant line cannot be covered.
func TestGroupFromActionSpecGroup_DefaultsSurfaceKindToMetaGroup(t *testing.T) {
	spec := toolutil.NewActionSpec("get", testCatalogActionRoute(), toolutil.ActionSpecOptions{ReadOnly: true, Idempotent: true})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{
		{ToolName: "gitlab_zzz_surfacekind_probe", BaseDomain: "zzz_surfacekind_probe", Actions: []toolutil.ActionSpec{spec}},
	}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	group, ok := catalog.Group("gitlab_zzz_surfacekind_probe")
	if !ok {
		t.Fatal("catalog missing gitlab_zzz_surfacekind_probe group")
	}
	if group.SurfaceKind != actioncatalog.SurfaceKindMetaGroup {
		t.Fatalf("SurfaceKind = %q, want default %q", group.SurfaceKind, actioncatalog.SurfaceKindMetaGroup)
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
	if client == nil && cacheableCatalogOptions(opts) {
		catalog, err := sharedActionCatalog(opts)
		if err != nil {
			t.Fatalf("BuildActionCatalog(%+v) error = %v", opts, err)
		}
		return catalog
	}
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
		GitLabURL:      "https://gitlab.com",
		GitLabToken:    "test-token",
		DisableRetries: true,
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
	// expectedBaseDynamicCatalogActions identifies the expected base (Free tier)
	// dynamic catalog actions. 851 = 865 −11 group webhooks −3 MR dependencies
	// gated to Premium (group_webhooks.md and merge request dependencies are
	// Premium/Ultimate). See cmd/audit_edition_tier.
	expectedBaseDynamicCatalogActions = 851
	// expectedEnterpriseDynamicCatalogActions identifies the expected enterprise dynamic catalog actions constant used by this package.
	// 1069 = 1066 + 3 security scan profile actions (attach/detach/list_project_statuses, Ultimate, client-go v2.45.0).
	expectedEnterpriseDynamicCatalogActions = 1069
	// expectedGitLabComEnterpriseCatalogActions identifies the expected GitLab com enterprise catalog actions constant used by this package.
	// 1075 = 1072 + 3 security scan profile actions (Ultimate, client-go v2.45.0).
	expectedGitLabComEnterpriseCatalogActions = 1075
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

// TestActionCatalog_PremiumTierHidesUltimate verifies the central tier filter
// gates Ultimate-only groups out of a Premium instance while keeping Premium
// groups. A Premium catalog must sit strictly between Free and Ultimate.
func TestActionCatalog_PremiumTierHidesUltimate(t *testing.T) {
	free := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	premium := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Premium, IncludeMCP: true})
	ultimate := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})

	if free.CountActions() >= premium.CountActions() || premium.CountActions() >= ultimate.CountActions() {
		t.Fatalf("expected free < premium < ultimate, got %d, %d, %d",
			free.CountActions(), premium.CountActions(), ultimate.CountActions())
	}
	// Ultimate-only groups are absent at Premium; Premium groups are present.
	if _, ok := premium.Group("gitlab_vulnerability"); ok {
		t.Error("gitlab_vulnerability (Ultimate) must not appear on a Premium instance")
	}
	if _, ok := ultimate.Group("gitlab_vulnerability"); !ok {
		t.Error("gitlab_vulnerability (Ultimate) must appear on an Ultimate instance")
	}
	if _, ok := premium.Group("gitlab_geo"); !ok {
		t.Error("gitlab_geo (Premium) must appear on a Premium instance")
	}
	if _, ok := free.Group("gitlab_geo"); ok {
		t.Error("gitlab_geo (Premium) must not appear on a Free instance")
	}
}

// TestCentralTierFilter_Invariants is a doc-grounded structural guardrail on the
// central tier filter. It asserts the filter's defining invariants directly from
// the catalog (no reliance on description text): a Free instance exposes no
// action carrying a paid Edition, a Premium instance exposes no Ultimate-only
// action, and a curated set of landmark actions land in exactly the right tiers.
func TestCentralTierFilter_Invariants(t *testing.T) {
	free := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	premium := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Premium, IncludeMCP: true})
	ultimate := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})

	// Invariant 1: a Free catalog contains no action with a paid Edition.
	for _, a := range free.Actions() {
		if edition.TierFromEdition(a.Edition) != edition.Free {
			t.Errorf("Free catalog leaks paid action %s (Edition=%q)", a.ID, a.Edition)
		}
	}
	// Invariant 2: a Premium catalog contains no Ultimate-only action.
	for _, a := range premium.Actions() {
		if edition.TierFromEdition(a.Edition) == edition.Ultimate {
			t.Errorf("Premium catalog leaks Ultimate action %s (Edition=%q)", a.ID, a.Edition)
		}
	}

	// Landmark actions: canonical IDs that must resolve to a specific tier. These
	// encode the audited Wave 1/2 decisions (see cmd/audit_edition_tier).
	landmarks := []struct {
		name      string
		id        actioncatalog.ActionID
		freeOK    bool // present on a Free instance
		premiumOK bool // present on a Premium instance
	}{
		{name: "core_action_on_every_tier", id: "issue.list", freeOK: true, premiumOK: true},
		{name: "basic_approval_state_is_free", id: "merge_request.approval_config", freeOK: true, premiumOK: true},
		{name: "service_accounts_are_free", id: "project.service_account_list", freeOK: true, premiumOK: true},
		{name: "epics_are_premium", id: "group.epic_list", freeOK: false, premiumOK: true},
		{name: "approval_rules_are_premium", id: "merge_request.approval_rule_create", freeOK: false, premiumOK: true},
		{name: "vulnerabilities_are_ultimate", id: "vulnerability.list", freeOK: false, premiumOK: false},
		{name: "dependencies_are_ultimate", id: "dependency.list", freeOK: false, premiumOK: false},
		{name: "security_findings_are_ultimate", id: "security_finding.list", freeOK: false, premiumOK: false},
	}
	for _, lm := range landmarks {
		t.Run(lm.name, func(t *testing.T) {
			assertLandmarkTiers(t, free, premium, ultimate, lm.id, lm.freeOK, lm.premiumOK)
		})
	}

	// Field-level (Phase 3): issue.create exists in all tiers, but its Premium
	// input fields (weight, epic_id) must be pruned from the Free schema and
	// present on Premium/Ultimate.
	for _, field := range []string{"weight", "epic_id"} {
		t.Run(field, func(t *testing.T) {
			if issueCreateHasInputProp(t, free, field) {
				t.Errorf("Free issue.create schema must not advertise Premium field %q", field)
			}
			if !issueCreateHasInputProp(t, premium, field) {
				t.Errorf("Premium issue.create schema must include field %q", field)
			}
		})
	}

	// reviewer_assignment_strategy (client-go v2.46.0) is Premium: pruned from
	// the Free project.create/project.update input schemas, present on Premium.
	for _, id := range []actioncatalog.ActionID{"project.create", "project.update"} {
		t.Run(string(id), func(t *testing.T) {
			if actionHasInputProp(t, free, id, "reviewer_assignment_strategy") {
				t.Errorf("Free %s schema must not advertise Premium field reviewer_assignment_strategy", id)
			}
			if !actionHasInputProp(t, premium, id, "reviewer_assignment_strategy") {
				t.Errorf("Premium %s schema must include field reviewer_assignment_strategy", id)
			}
		})
	}
}

// assertLandmarkTiers checks one landmark action against the three tier
// catalogs: present on Free and Premium exactly as declared, always on Ultimate.
func assertLandmarkTiers(t *testing.T, free, premium, ultimate *actioncatalog.Catalog, id actioncatalog.ActionID, freeOK, premiumOK bool) {
	t.Helper()
	if _, ok := free.Action(id); ok != freeOK {
		t.Errorf("Free: action %s present=%v, want %v", id, ok, freeOK)
	}
	if _, ok := premium.Action(id); ok != premiumOK {
		t.Errorf("Premium: action %s present=%v, want %v", id, ok, premiumOK)
	}
	if _, ok := ultimate.Action(id); !ok {
		t.Errorf("Ultimate: action %s must be present", id)
	}
}

// issueCreateHasInputProp reports whether the issue.create action's input schema
// in the catalog advertises the named property.
func issueCreateHasInputProp(t *testing.T, catalog *actioncatalog.Catalog, prop string) bool {
	t.Helper()
	return actionHasInputProp(t, catalog, "issue.create", prop)
}

// actionHasInputProp reports whether the named action's input schema in the
// catalog advertises the named property.
func actionHasInputProp(t *testing.T, catalog *actioncatalog.Catalog, id actioncatalog.ActionID, prop string) bool {
	t.Helper()
	action, ok := catalog.Action(id)
	if !ok {
		t.Fatalf("%s missing from catalog", id)
	}
	props, _ := action.Route.InputSchema["properties"].(map[string]any)
	_, has := props[prop]
	return has
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

// BenchmarkBuildActionCatalog_Ultimate measures the per-server cost of
// building the full Ultimate action catalog (paid per token+URL in HTTP mode).
func BenchmarkBuildActionCatalog_Ultimate(b *testing.B) {
	client := benchClient(b)
	b.ResetTimer()
	for range b.N {
		if _, err := BuildActionCatalog(client, ActionCatalogOptions{Enterprise: true, IncludeMCP: true}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestActionAnnotations_AdditiveActionsAreNotIdempotent is the gate for a hint
// that can only mislead when it is wrong.
//
// idempotentHint tells a model that repeating a call has no further effect.
// Nothing enforces it, so its entire purpose is to say whether a retry is safe,
// which makes an inaccurate one worse than an absent one.
//
// Logging spent time was the case that was wrong. GitLab's add_spent_time is
// additive — two calls of "1h" leave two hours — and the action was built
// through the update helper, which marks everything idempotent. A model
// retrying a call whose result it could not observe would have doubled a
// timesheet entry, quietly and in a place nobody checks for a while.
//
// The rule is checked by name because the name is the claim: an action called
// "add" that also says repeating it changes nothing is either misnamed or
// mis-annotated, and either way it should be looked at. Actions that genuinely
// set a value are named for it — set, update, reset — and keep the hint.
func TestActionAnnotations_AdditiveActionsAreNotIdempotent(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

	var checked int
	for _, action := range catalog.Actions() {
		name := string(action.ID)
		short := name
		if i := strings.LastIndex(name, "."); i >= 0 {
			short = name[i+1:]
		}
		if !strings.HasPrefix(short, "add_") && !strings.HasSuffix(short, "_add") && short != "add" {
			continue
		}
		checked++

		if action.Idempotent {
			t.Errorf("%s is annotated idempotent while its name says it accumulates; "+
				"a model may retry it and apply the effect twice", name)
		}
	}

	if checked == 0 {
		t.Fatal("no additive action was found, so this gate asserted nothing")
	}
	t.Logf("checked %d additive action(s)", checked)
}

// Shared catalog memoization for this package's tests: building the full
// catalog resolves and validates ~850 action schemas (~2s), and the result
// depends only on the projection options, so tests share one build per
// option set.

// sharedCatalogKey identifies one cacheable nil-client catalog projection.
type sharedCatalogKey struct {
	tier       edition.Tier
	enterprise bool
	includeMCP bool
}

var (
	sharedCatalogsMu sync.Mutex
	sharedCatalogs   = map[sharedCatalogKey]*actioncatalog.Catalog{}
)

// cacheableCatalogOptions reports whether opts only carries the fields the
// shared cache keys on: a client or spec-group override makes the
// build unique to its caller.
func cacheableCatalogOptions(opts ActionCatalogOptions) bool {
	return opts.SpecGroups == nil
}

// sharedActionCatalog returns a clone of the memoized nil-client catalog for
// opts, building it on first use.
func sharedActionCatalog(opts ActionCatalogOptions) (*actioncatalog.Catalog, error) {
	key := sharedCatalogKey{tier: opts.Tier, enterprise: opts.Enterprise, includeMCP: opts.IncludeMCP}
	sharedCatalogsMu.Lock()
	defer sharedCatalogsMu.Unlock()
	if cached, ok := sharedCatalogs[key]; ok {
		return cached.Clone(), nil
	}
	catalog, err := BuildActionCatalog(nil, opts)
	if err != nil {
		return nil, err
	}
	sharedCatalogs[key] = catalog
	return catalog.Clone(), nil
}

// Guard for the tool-name references embedded in the catalog's free-text
// surfaces: usage sentences, descriptions, aliases, and parameter guidance
// name other tools, and every such name must resolve on the surface that
// serves it.

// textNameToken matches anything that looks like a tool reference inside
// free text.
var textNameToken = regexp.MustCompile(`\bgitlab_[a-z0-9_]+\b`)

// textNameAllowlist holds gitlab_-prefixed tokens that are GitLab API
// vocabulary, not tool references: template-type enum values in the
// project templates schemas.
var textNameAllowlist = map[string]bool{
	"gitlab_ci_ymls":        true,
	"gitlab_ci_syntax_ymls": true,
}

// TestCatalogTextFields_NameReferencesResolve verifies every tool-name
// token in the catalog's text fields resolves against the catalog itself.
// The sweep that introduced this guard found a ghost alias and two
// archive descriptions whose "(read-only)" parenthetical described the
// archived project, not the action.
func TestCatalogTextFields_NameReferencesResolve(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	valid := map[string]bool{
		"gitlab_find_action":    true,
		"gitlab_execute_action": true,
	}
	for _, spec := range StandaloneSurfaceToolSpecs(nil) {
		if action, err := spec.ActionSpec(); err == nil {
			valid[action.IndividualTool.Name] = true
		}
	}
	// The catalog is the second source: it carries every action projected onto
	// the individual and meta surfaces, which the domain specs above do not.
	for _, action := range catalog.Actions() {
		if action.IndividualTool.Name != "" {
			valid[action.IndividualTool.Name] = true
		}
		if action.ToolName != "" {
			valid[action.ToolName] = true
		}
	}

	check := func(owner, field, text string) {
		t.Helper()
		for _, token := range textNameToken.FindAllString(text, -1) {
			if !valid[token] && !textNameAllowlist[token] {
				t.Errorf("%s (%s) references %q, which is not a tool or dispatcher in the catalog", owner, field, token)
			}
		}
	}
	for _, action := range catalog.Actions() {
		id := string(action.ID)
		check(id, "usage", action.Usage)
		check(id, "description", action.IndividualTool.Description)
		for _, alias := range action.Aliases {
			check(id, "alias", alias)
		}
		for param, guidance := range action.Route.ParameterGuidance {
			check(id, "guidance."+param, strings.Join(append([]string{guidance.ValueSource, guidance.ExampleBinding}, guidance.CommonConfusions...), " "))
		}
	}
}
