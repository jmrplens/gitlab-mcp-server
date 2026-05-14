package tools

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

func TestBuildActionCatalog_DoesNotLeakCaptureState(t *testing.T) {
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if catalog.CountGroups() == 0 || catalog.CountActions() == 0 {
		t.Fatalf("catalog counts = groups %d actions %d, want non-zero", catalog.CountGroups(), catalog.CountActions())
	}

	if leaked := toolutil.CaptureMetaToolDefinitions(func() {}); len(leaked) != 0 {
		t.Fatalf("BuildActionCatalog() leaked captured meta-tool definitions: %v", leaked)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerProjectMeta(server, nil, false)
	names := toolNamesFromServer(t, server)
	if !slices.Contains(names, "gitlab_project") {
		t.Fatalf("registerProjectMeta() after BuildActionCatalog() registered tools %v, want gitlab_project", names)
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

	assertCatalogMissingAction(t, base, "orbit.status")
	assertCatalogMissingAction(t, enterprise, "orbit.status")
	assertCatalogHasAction(t, gitLabDotComEnterprise, "orbit.status")
}

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

func TestWithCatalogParameterGuidance_MergesRouteGuidance(t *testing.T) {
	route := toolutil.ActionRoute{
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "existing_scope",
				CommonConfusions: []string{"Keep the source project."},
			},
		},
	}

	got := withCatalogParameterGuidance("gitlab_job", "token_scope_remove_project", route)
	projectGuidance := got.ParameterGuidance["project_id"]
	if projectGuidance.SemanticRole != "existing_scope" {
		t.Fatalf("project_id SemanticRole = %q, want existing_scope", projectGuidance.SemanticRole)
	}
	if projectGuidance.ValueSource == "" || projectGuidance.ExampleBinding == "" {
		t.Fatalf("project_id guidance = %+v, want catalog value source and example", projectGuidance)
	}
	if !slices.Contains(projectGuidance.CommonConfusions, "Keep the source project.") || !slices.Contains(projectGuidance.CommonConfusions, "Do not use the project being removed as project_id.") {
		t.Fatalf("project_id CommonConfusions = %+v, want route and catalog guidance", projectGuidance.CommonConfusions)
	}
	if _, ok := got.ParameterGuidance["target_project_id"]; !ok {
		t.Fatalf("ParameterGuidance = %+v, want catalog-only target_project_id", got.ParameterGuidance)
	}
}

func TestBuildActionCatalog_UsesCanonicalActionSpecs(t *testing.T) {
	spec := toolutil.NewActionSpec("list", testCatalogActionRoute("search"), toolutil.ActionSpecOptions{
		Aliases:           []string{"project.search"},
		Tags:              []string{"Project"},
		Usage:             "Use to list projects with optional search filtering.",
		RelatedActions:    []string{"project.get"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{"search": {SemanticRole: "project_search_query"}},
		ReadOnly:          true,
		Idempotent:        true,
		OpenWorld:         true,
		OwnerPackage:      "projects",
		IndividualTool:    toolutil.IndividualToolSpec{Name: "gitlab_list_projects", Title: "List projects"},
	})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_project", Specs: []toolutil.ActionSpec{spec}}}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	action, ok := catalog.Action("project.list")
	if !ok {
		t.Fatal("catalog missing project.list")
	}
	if action.Usage != "Use to list projects with optional search filtering." || action.OwnerPackage != "projects" {
		t.Fatalf("action metadata = %+v, want spec metadata", action)
	}
	if !slices.Contains(action.Aliases, "project.search") || !slices.Contains(action.Tags, "project") || !slices.Contains(action.RelatedActions, "project.get") {
		t.Fatalf("action search metadata = aliases %+v tags %+v related %+v", action.Aliases, action.Tags, action.RelatedActions)
	}
	if action.Route.ParameterGuidance["search"].SemanticRole != "project_search_query" {
		t.Fatalf("route guidance = %+v, want spec guidance", action.Route.ParameterGuidance)
	}
}

func TestBuildActionCatalog_LegacyOverlayIsNotRequiredForSpecBackedActions(t *testing.T) {
	spec := toolutil.NewActionSpec("token_scope_remove_project", testCatalogActionRoute("project_id"), toolutil.ActionSpecOptions{
		ParameterGuidance: map[string]toolutil.ParameterGuidance{"project_id": {SemanticRole: "spec_scope_project"}},
	})

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{SpecGroups: []ActionSpecGroup{{ToolName: "gitlab_job", Specs: []toolutil.ActionSpec{spec}}}})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	action, ok := catalog.Action("job.token_scope_remove_project")
	if !ok {
		t.Fatal("catalog missing job.token_scope_remove_project")
	}
	guidance := action.Route.ParameterGuidance
	if guidance["project_id"].SemanticRole != "spec_scope_project" {
		t.Fatalf("project_id guidance = %+v, want spec guidance", guidance["project_id"])
	}
	if _, exists := guidance["target_project_id"]; exists {
		t.Fatalf("guidance = %+v, want no legacy target_project_id overlay for spec-backed action", guidance)
	}
}

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

func mustBuildActionCatalog(t *testing.T, client *gitlabclient.Client, opts ActionCatalogOptions) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := BuildActionCatalog(client, opts)
	if err != nil {
		t.Fatalf("BuildActionCatalog(%+v) error = %v", opts, err)
	}
	return catalog
}

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

func assertCatalogHasAction(t *testing.T, catalog *actioncatalog.Catalog, actionID actioncatalog.ActionID) {
	t.Helper()
	if _, ok := catalog.Action(actionID); !ok {
		t.Fatalf("catalog missing action %s", actionID)
	}
}

func assertCatalogMissingAction(t *testing.T, catalog *actioncatalog.Catalog, actionID actioncatalog.ActionID) {
	t.Helper()
	if _, ok := catalog.Action(actionID); ok {
		t.Fatalf("catalog contains action %s", actionID)
	}
}
