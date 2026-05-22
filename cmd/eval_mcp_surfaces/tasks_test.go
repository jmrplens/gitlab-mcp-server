package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFilterTasks_SelectsCommaSeparatedIDs verifies explicit task filters keep
// requested task order from the corpus and ignore unknown IDs.
func TestFilterTasks_SelectsCommaSeparatedIDs(t *testing.T) {
	tasks := []evalTask{{ID: "A"}, {ID: "B"}, {ID: "C"}}
	filtered := filterTasks(tasks, " C, A, missing ")
	if got := taskIDs(filtered); got != "A,C" {
		t.Fatalf("filterTasks() IDs = %q, want A,C", got)
	}
}

// TestTaskRoutePredicates_ClassifyEnterpriseMutationAndDestruction verifies
// route string heuristics used for partitioning cover important token shapes.
func TestTaskRoutePredicates_ClassifyEnterpriseMutationAndDestruction(t *testing.T) {
	if !routeLooksDestructive("gitlab_project.archive") || !routeLooksDestructive("mr_review.draft_note_publish_all") {
		t.Fatal("routeLooksDestructive() missed archive or publish_all")
	}
	if !routeLooksMutating("gitlab_runner", "runner.update") || !routeLooksMutating("gitlab_project", "push_rule_edit") || routeLooksMutating("gitlab_project", "get") {
		t.Fatal("routeLooksMutating() did not classify update/read correctly")
	}
	if !routeLooksEnterprise("gitlab_merge_train", "list_project") || !routeLooksEnterprise("gitlab_environment", "protected_list") || !routeUnavailableOnCE("gitlab_environment", "deployment_approve_or_reject") {
		t.Fatal("enterprise/CE route predicates missed known routes")
	}
}

// TestFilterTasksByPreset_EnterpriseDockerUsesLiveFixtureRows verifies Docker
// Enterprise presets avoid schema-only Enterprise rows that do not have live fixtures.
func TestFilterTasksByPreset_EnterpriseDockerUsesLiveFixtureRows(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-137", ExpectedTool: "gitlab", ExpectedAction: "group.epic_create"},
		{ID: "MT-192", ExpectedTool: "gitlab_project", ExpectedAction: "push_rule_add"},
		{ID: "MT-196", ExpectedTool: "gitlab_project", ExpectedAction: "push_rule_delete", Destructive: true},
		{ID: "MS-045", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "push_rule_add"}, {ExpectedTool: "gitlab_project", ExpectedAction: "push_rule_delete", Destructive: true}}},
	}

	mutating, err := filterTasksByPreset(tasks, presetDockerEnterpriseMutatingSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-mutating-safe) error = %v", err)
	}
	if got := taskIDs(mutating); got != "MT-192" {
		t.Fatalf("docker-enterprise-mutating-safe IDs = %q, want MT-192", got)
	}
	destructive, err := filterTasksByPreset(tasks, presetDockerEnterpriseDestructiveSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-destructive-safe) error = %v", err)
	}
	if got := taskIDs(destructive); got != "MT-196,MS-045" {
		t.Fatalf("docker-enterprise-destructive-safe IDs = %q, want MT-196,MS-045", got)
	}
	schema, err := filterTasksByPreset(tasks, presetSchemaEnterprise)
	if err != nil {
		t.Fatalf("filterTasksByPreset(schema-enterprise) error = %v", err)
	}
	if got := taskIDs(schema); got != "MT-137,MT-192,MT-196,MS-045" {
		t.Fatalf("schema-enterprise IDs = %q, want all Enterprise rows", got)
	}
}

// TestTaskUsesCapabilityFallback_DetectsBridgeAndPromptOnlyTasks verifies the
// capability fallback partition only selects tasks that need MCP capability access.
func TestTaskUsesCapabilityFallback_DetectsBridgeAndPromptOnlyTasks(t *testing.T) {
	if !taskUsesCapabilityFallback(evalTask{Steps: []evalStep{{ExpectedTool: resourceReadTool}}}) {
		t.Fatal("taskUsesCapabilityFallback(bridge step) = false, want true")
	}
	if !taskUsesCapabilityFallback(evalTask{Prompt: "inspect schema fallback"}) {
		t.Fatal("taskUsesCapabilityFallback(prompt-only schema) = false, want true")
	}
	if taskUsesCapabilityFallback(evalTask{ExpectedTool: "gitlab_project", ExpectedAction: "get"}) {
		t.Fatal("taskUsesCapabilityFallback(route task) = true, want false")
	}
}

// TestTaskRoutesAvailable_HandlesStandaloneBridgeAndMissingRoutes verifies route
// availability accepts evaluator bridge tools and rejects unknown catalog routes.
func TestTaskRoutesAvailable_HandlesStandaloneBridgeAndMissingRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": toolutil.ActionRoute{}}}
	if !taskRoutesAvailable(evalTask{Steps: []evalStep{{ExpectedTool: resourceListTool}}}, routes, false) {
		t.Fatal("taskRoutesAvailable(resourceListTool) = false, want true")
	}
	if taskRoutesAvailable(evalTask{ExpectedTool: "gitlab_project", ExpectedAction: "missing"}, routes, false) {
		t.Fatal("taskRoutesAvailable(missing route) = true, want false")
	}
	if taskRoutesAvailable(evalTask{ID: "MT-105", ExpectedTool: "gitlab_project", ExpectedAction: "get"}, routes, false) {
		t.Fatal("taskRoutesAvailable(unavailable task) = true, want false")
	}
}

// TestCatalogHasEnterpriseRoutes_DetectsRouteMapShapes verifies Enterprise
// detection works for unified, dynamic, and split meta route maps.
func TestCatalogHasEnterpriseRoutes_DetectsRouteMapShapes(t *testing.T) {
	cases := []map[string]toolutil.ActionMap{
		{"gitlab": {"merge_train.list_project": toolutil.ActionRoute{}}},
		{dynamicExecuteTool: {"merge_train.list_project": toolutil.ActionRoute{}}},
		{"gitlab_merge_train": {"list_project": toolutil.ActionRoute{}}},
	}
	for _, routes := range cases {
		if !catalogHasEnterpriseRoutes(routes) {
			t.Fatalf("catalogHasEnterpriseRoutes(%v) = false, want true", routes)
		}
	}
	if catalogHasEnterpriseRoutes(map[string]toolutil.ActionMap{"gitlab_project": {"get": toolutil.ActionRoute{}}}) {
		t.Fatal("catalogHasEnterpriseRoutes(base route) = true, want false")
	}
}

// TestFilterTasksByAvailableRoutes_RespectsExplicitEdition verifies CE-only
// Docker runs do not inherit Enterprise availability from a mixed route map.
func TestFilterTasksByAvailableRoutes_RespectsExplicitEdition(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"issue.list":                               toolutil.ActionRoute{},
			"merge_train.list_project":                 toolutil.ActionRoute{},
			"model_registry.download":                  toolutil.ActionRoute{},
			"environment.deployment_approve_or_reject": toolutil.ActionRoute{},
		},
	}
	tasks := []evalTask{
		{ID: "base", ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.list"},
		{ID: "model-registry", ExpectedTool: dynamicExecuteTool, ExpectedAction: "model_registry.download"},
		{ID: "protected-deploy", ExpectedTool: dynamicExecuteTool, ExpectedAction: "environment.deployment_approve_or_reject"},
	}

	if got := taskIDs(filterTasksByAvailableRoutes(tasks, routes, false)); got != "base" {
		t.Fatalf("CE filtered IDs = %q, want base", got)
	}
	if got := taskIDs(filterTasksByAvailableRoutes(tasks, routes, true)); got != "base,model-registry,protected-deploy" {
		t.Fatalf("Enterprise filtered IDs = %q, want all tasks", got)
	}
}

// TestNormalizeExpectedRoutes_RewritesMetaAndDynamicRoutes verifies task route
// normalization maps unified and dynamic catalogs to the executable route shape.
func TestNormalizeExpectedRoutes_RewritesMetaAndDynamicRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab":           {"project.get": toolutil.ActionRoute{}},
		dynamicExecuteTool: {"project.get": toolutil.ActionRoute{}},
	}
	if toolName, action := normalizeExpectedRoute("gitlab_project", "get", routes); toolName != "gitlab" || action != "project.get" {
		t.Fatalf("normalizeExpectedRoute() = %s/%s, want gitlab/project.get", toolName, action)
	}
	if toolName, action := normalizeExpectedDynamicRoute("gitlab_project", "get", routes); toolName != dynamicExecuteTool || action != "project.get" {
		t.Fatalf("normalizeExpectedDynamicRoute() = %s/%s, want execute/project.get", toolName, action)
	}
	if got := canonicalRouteID("gitlab_project", "get"); got != "project.get" {
		t.Fatalf("canonicalRouteID() = %q, want project.get", got)
	}
	if got := superDispatcherAction("gitlab_project", "get"); got != "project.get" {
		t.Fatalf("superDispatcherAction() = %q, want project.get", got)
	}
}

// TestNormalizeTasksForCatalog_RewritesTopLevelAndStepRoutes verifies catalog
// normalization clones nested steps while preserving the original fixture.
func TestNormalizeTasksForCatalog_RewritesTopLevelAndStepRoutes(t *testing.T) {
	tasks := []evalTask{{
		ID: "MT-1", ExpectedTool: "gitlab_project", ExpectedAction: "get",
		Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}, {ExpectedTool: "gitlab_discover_project"}},
	}}
	dynamicRoutes := map[string]toolutil.ActionMap{dynamicExecuteTool: {"project.get": toolutil.ActionRoute{}, actionDiscoverProjectResolve: toolutil.ActionRoute{}}}
	dynamic := normalizeTasksForCatalog(tasks, dynamicRoutes, "dynamic")
	if dynamic[0].ExpectedTool != dynamicExecuteTool || dynamic[0].ExpectedAction != "project.get" || dynamic[0].Steps[1].ExpectedAction != actionDiscoverProjectResolve {
		t.Fatalf("dynamic normalized = %+v", dynamic[0])
	}
	if tasks[0].Steps[0].ExpectedTool != "gitlab_project" {
		t.Fatalf("original task mutated = %+v", tasks[0])
	}
	metaRoutes := map[string]toolutil.ActionMap{"gitlab": {"project.get": toolutil.ActionRoute{}}}
	meta := normalizeTasksForCatalog(tasks, metaRoutes, "meta")
	if meta[0].ExpectedTool != "gitlab" || meta[0].ExpectedAction != "project.get" {
		t.Fatalf("meta normalized = %+v", meta[0])
	}
}

// TestParseTaskRow_ReportsColumnErrors verifies malformed task rows produce
// actionable parse errors instead of silently building partial scenarios.
func TestParseTaskRow_ReportsColumnErrors(t *testing.T) {
	_, err := parseTaskRow([]string{"MT-1", "Prompt", "`gitlab_project` / `get` -> `gitlab_issue` / `create`", "`project_id`", "none; none", "none", "ok"})
	if err == nil || !strings.Contains(err.Error(), "required params") {
		t.Fatalf("parseTaskRow() error = %v, want required params error", err)
	}
}

// TestParseExpectedToolAction_CoversStandaloneAndErrorForms verifies expected tool/action parsing edge cases.
func TestParseExpectedToolAction_CoversStandaloneAndErrorForms(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantTool   string
		wantAction string
		wantErr    string
	}{
		{name: "standalone", value: "`resource_list`", wantTool: "resource_list"},
		{name: "none action", value: "`gitlab_project` / `none`", wantTool: "gitlab_project"},
		{name: "dash action", value: "`gitlab_project` / `-`", wantTool: "gitlab_project"},
		{name: "empty standalone", value: "   ", wantErr: "empty tool"},
		{name: "too many separators", value: "gitlab / project / get", wantErr: "expected tool/action pair"},
		{name: "empty tool action", value: " / get", wantErr: "empty tool/action"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, action, err := parseExpectedToolAction(tt.value)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseExpectedToolAction(%q) error = %v, want substring %q", tt.value, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseExpectedToolAction(%q) error = %v", tt.value, err)
			}
			if tool != tt.wantTool || action != tt.wantAction {
				t.Fatalf("parseExpectedToolAction(%q) = %q/%q, want %q/%q", tt.value, tool, action, tt.wantTool, tt.wantAction)
			}
		})
	}
}
