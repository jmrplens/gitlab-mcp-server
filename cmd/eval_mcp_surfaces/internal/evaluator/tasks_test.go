package evaluator

import (
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
	if !routeLooksEnterprise("gitlab_model_registry", "download") {
		t.Fatal("routeLooksEnterprise() missed CE-unavailable model registry download route")
	}
}

// TestLiveMergeStatusStillPreparing_CoversTransientStatuses verifies MR fixture
// readiness waits only for statuses GitLab can still advance asynchronously.
func TestLiveMergeStatusStillPreparing_CoversTransientStatuses(t *testing.T) {
	for _, status := range []string{"", "checking", "unchecked", "preparing", "ci_still_running", "approvals_syncing"} {
		if !liveMergeStatusStillPreparing(status) {
			t.Fatalf("liveMergeStatusStillPreparing(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"cannot_be_merged", "not_open", "not_approved"} {
		if liveMergeStatusStillPreparing(status) {
			t.Fatalf("liveMergeStatusStillPreparing(%q) = true, want false", status)
		}
	}
}

// TestStandaloneMetaToolForAction_ClassifiesInteractiveActions verifies the
// standalone meta-tool mapping returns the correct dispatcher for each
// supported action and rejects unknown actions.
func TestStandaloneMetaToolForAction_ClassifiesInteractiveActions(t *testing.T) {
	tests := []struct {
		action   string
		wantTool string
		wantOK   bool
	}{
		{action: actionDiscoverProjectResolve, wantTool: "gitlab_discover_project", wantOK: true},
		{action: "interactive.issue_create", wantTool: "gitlab_interactive_issue_create", wantOK: true},
		{action: "interactive.mr_create", wantTool: "gitlab_interactive_mr_create", wantOK: true},
		{action: "interactive.project_create", wantTool: "gitlab_interactive_project_create", wantOK: true},
		{action: "interactive.release_create", wantTool: "gitlab_interactive_release_create", wantOK: true},
		{action: "interactive.unknown_action", wantOK: false},
		{action: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			tool, ok := standaloneMetaToolForAction(tt.action)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if tool != tt.wantTool {
				t.Fatalf("tool = %q, want %q", tool, tt.wantTool)
			}
		})
	}
}

// TestMetaToolRouteForAction_MatchesKnownAndRejectsUnknown verifies the helper
// resolves a known domain.action pair against the routes map and rejects
// unknown or malformed action IDs.
func TestMetaToolRouteForAction_MatchesKnownAndRejectsUnknown(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": toolutil.ActionRoute{}},
	}

	tool, action, ok := metaToolRouteForAction("project.get", routes)
	if !ok || tool != "gitlab_project" || action != "get" {
		t.Fatalf("metaToolRouteForAction(project.get) = (%q, %q, %t), want gitlab_project/get/true", tool, action, ok)
	}

	if _, _, hasMatch := metaToolRouteForAction("project.missing", routes); hasMatch {
		t.Fatal("metaToolRouteForAction(missing) = true, want false")
	}
	if _, _, hasMatch := metaToolRouteForAction("malformed", routes); hasMatch {
		t.Fatal("metaToolRouteForAction(malformed) = true, want false")
	}
	if _, _, hasMatch := metaToolRouteForAction(".missing", routes); hasMatch {
		t.Fatal("metaToolRouteForAction(empty domain) = true, want false")
	}
	if _, _, hasMatch := metaToolRouteForAction("missing.", routes); hasMatch {
		t.Fatal("metaToolRouteForAction(empty action) = true, want false")
	}
}

// TestFilterTasksByEdition_ExcludesCEUnavailableRoutes verifies routes that are
// only available when Enterprise features are present stay out of CE case sets.
func TestFilterTasksByEdition_ExcludesCEUnavailableRoutes(t *testing.T) {
	tasks := []evalTask{
		{ID: "base", ExpectedTool: "gitlab_project", ExpectedAction: "get"},
		{ID: "ce-unavailable", ExpectedTool: "gitlab_model_registry", ExpectedAction: "download"},
		{ID: "typed-ce-route", ExpectedTool: "gitlab_model_registry", ExpectedAction: "download", Case: &EvalCase{Edition: EvalCaseEdition(editionCE)}},
	}

	ce, err := filterTasksByEdition(tasks, editionCE)
	if err != nil {
		t.Fatalf("filterTasksByEdition(ce) error = %v", err)
	}
	if got := taskIDs(ce); got != "base,typed-ce-route" {
		t.Fatalf("CE filtered IDs = %q, want base,typed-ce-route", got)
	}
	enterprise, err := filterTasksByEdition(tasks, editionEnterprise)
	if err != nil {
		t.Fatalf("filterTasksByEdition(enterprise) error = %v", err)
	}
	if got := taskIDs(enterprise); got != "ce-unavailable" {
		t.Fatalf("Enterprise filtered IDs = %q, want ce-unavailable", got)
	}
}

// TestFilterTasksByPreset_EnterpriseDockerUsesLiveFixtureRows verifies Docker
// Enterprise presets avoid schema-only Enterprise rows that do not have live fixtures.
func TestFilterTasksByPreset_EnterpriseDockerUsesLiveFixtureRows(t *testing.T) {
	tasks := evalTasksByID(t, "MT-137", "MT-192", "MT-196", "MS-045")

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

func evalTasksByID(t *testing.T, ids ...string) []evalTask {
	t.Helper()
	tasks := make([]evalTask, 0, len(ids))
	for _, id := range ids {
		evalCase, ok := CaseByID(id)
		if !ok {
			t.Fatalf("CaseByID(%s) = false", id)
		}
		tasks = append(tasks, taskFromCase(evalCase))
	}
	return tasks
}

// TestFilterTasksByPreset_UsesTypedCaseMetadata verifies typed registry
// metadata, not ID heuristics, is the source of truth when present.
func TestFilterTasksByPreset_UsesTypedCaseMetadata(t *testing.T) {
	custom := evalTask{
		ID:             "typed-custom-enterprise",
		ExpectedTool:   "gitlab_project",
		ExpectedAction: "archive",
		Case: &EvalCase{
			ID:          "typed-custom-enterprise",
			Edition:     EvalCaseEdition(editionEnterprise),
			Partition:   EvalPartition(partitionEnterpriseRead),
			Presets:     []EvalPreset{EvalPreset(presetDockerEnterpriseRead)},
			Mutating:    false,
			Destructive: false,
		},
	}

	read, err := filterTasksByPreset([]evalTask{custom}, presetDockerEnterpriseRead)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-read) error = %v", err)
	}
	if got := taskIDs(read); got != "typed-custom-enterprise" {
		t.Fatalf("docker-enterprise-read IDs = %q, want typed-custom-enterprise", got)
	}
	if !taskIsEnterpriseDockerFixture(custom) {
		t.Fatal("taskIsEnterpriseDockerFixture(typed custom) = false, want true from typed presets")
	}
	partitioned, err := filterTasksByPartition([]evalTask{custom}, partitionEnterpriseRead)
	if err != nil {
		t.Fatalf("filterTasksByPartition(enterprise-read) error = %v", err)
	}
	if got := taskIDs(partitioned); got != "typed-custom-enterprise" {
		t.Fatalf("enterprise-read IDs = %q, want typed-custom-enterprise", got)
	}
	readOnly, err := filterTasksByMutation([]evalTask{custom}, true, false)
	if err != nil {
		t.Fatalf("filterTasksByMutation(skip) error = %v", err)
	}
	if got := taskIDs(readOnly); got != "typed-custom-enterprise" {
		t.Fatalf("skip-mutating IDs = %q, want typed-custom-enterprise", got)
	}
	nonDestructive, err := filterTasksByDestructive([]evalTask{custom}, true, false)
	if err != nil {
		t.Fatalf("filterTasksByDestructive(skip) error = %v", err)
	}
	if got := taskIDs(nonDestructive); got != "typed-custom-enterprise" {
		t.Fatalf("skip-destructive IDs = %q, want typed-custom-enterprise", got)
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
	enterpriseRoutes := map[string]toolutil.ActionMap{
		"gitlab_environment": {"deployment_approve_or_reject": toolutil.ActionRoute{}},
	}
	if !taskRoutesAvailable(evalTask{Steps: []evalStep{{ExpectedTool: resourceListTool}}}, routes, false) {
		t.Fatal("taskRoutesAvailable(resourceListTool) = false, want true")
	}
	if taskRoutesAvailable(evalTask{ExpectedTool: "gitlab_environment", ExpectedAction: "deployment_approve_or_reject"}, enterpriseRoutes, false) {
		t.Fatal("taskRoutesAvailable(enterprise route on CE catalog) = true, want false")
	}
	if !taskRoutesAvailable(evalTask{ExpectedTool: "gitlab_environment", ExpectedAction: "deployment_approve_or_reject"}, enterpriseRoutes, true) {
		t.Fatal("taskRoutesAvailable(enterprise route on Enterprise catalog) = false, want true")
	}
	if taskRoutesAvailable(evalTask{ExpectedTool: "gitlab_project", ExpectedAction: "missing"}, routes, false) {
		t.Fatal("taskRoutesAvailable(missing route) = true, want false")
	}
	if taskRoutesAvailable(evalTask{ID: "MT-105", ExpectedTool: "gitlab_project", ExpectedAction: "get"}, routes, false) {
		t.Fatal("taskRoutesAvailable(unavailable task) = true, want false")
	}
	if taskRoutesAvailable(evalTask{ID: "typed-skip", ExpectedTool: "gitlab_project", ExpectedAction: "get", Case: &EvalCase{SkipReasons: []string{"not available"}}}, routes, false) {
		t.Fatal("taskRoutesAvailable(typed skip) = true, want false")
	}
}

// TestOrderSharedFixtureDestructiveLast_UsesTypedFixtureDependencies verifies
// typed attempt-scoped fixtures and self-contained create/delete workflows are
// not delayed by shared-resource ordering.
func TestOrderSharedFixtureDestructiveLast_UsesTypedFixtureDependencies(t *testing.T) {
	tasks := []evalTask{
		{
			ID:             "MT-024",
			ExpectedTool:   dynamicExecuteActionTool,
			ExpectedAction: "job.delete_artifacts",
			Case: &EvalCase{
				Fixtures: []CaseFixtureSpec{FailedJobArtifactFixture},
			},
		},
		{
			ID:             "MS-043",
			ExpectedTool:   "gitlab_project",
			ExpectedAction: "service_account_delete",
			Steps: []evalStep{
				{ExpectedTool: "gitlab_project", ExpectedAction: "service_account_create"},
				{ExpectedTool: "gitlab_project", ExpectedAction: "service_account_delete"},
			},
			Case: &EvalCase{ID: "MS-043"},
		},
		{ID: "MT-065", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.download_single_artifact"},
	}

	ordered := orderSharedFixtureDestructiveLast(tasks)

	if got := taskIDs(ordered); got != "MT-024,MS-043,MT-065" {
		t.Fatalf("ordered IDs = %q, want MT-024,MS-043,MT-065", got)
	}
}

// TestCatalogHasEnterpriseRoutes_DetectsRouteMapShapes verifies Enterprise
// detection works for unified, dynamic, and split meta route maps.
func TestCatalogHasEnterpriseRoutes_DetectsRouteMapShapes(t *testing.T) {
	cases := []map[string]toolutil.ActionMap{
		{"gitlab": {"merge_train.list_project": toolutil.ActionRoute{}}},
		{dynamicExecuteActionTool: {"merge_train.list_project": toolutil.ActionRoute{}}},
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
		dynamicExecuteActionTool: {
			"issue.list":                               toolutil.ActionRoute{},
			"merge_train.list_project":                 toolutil.ActionRoute{},
			"model_registry.download":                  toolutil.ActionRoute{},
			"environment.deployment_approve_or_reject": toolutil.ActionRoute{},
		},
	}
	tasks := []evalTask{
		{ID: "base", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.list"},
		{ID: "model-registry", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "model_registry.download"},
		{ID: "protected-deploy", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "environment.deployment_approve_or_reject"},
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
		"gitlab":                 {"project.get": toolutil.ActionRoute{}},
		dynamicExecuteActionTool: {"project.get": toolutil.ActionRoute{}},
	}
	if toolName, action := normalizeExpectedRoute("gitlab_project", "get", routes); toolName != "gitlab" || action != "project.get" {
		t.Fatalf("normalizeExpectedRoute() = %s/%s, want gitlab/project.get", toolName, action)
	}
	if toolName, action := normalizeExpectedDynamicRoute("gitlab_project", "get", routes); toolName != dynamicExecuteActionTool || action != "project.get" {
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
	dynamicRoutes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {"project.get": toolutil.ActionRoute{}, actionDiscoverProjectResolve: toolutil.ActionRoute{}}}
	dynamic := normalizeTasksForCatalog(tasks, dynamicRoutes, "dynamic")
	if dynamic[0].ExpectedTool != dynamicFindTool || dynamic[0].ExpectedAction != "" || len(dynamic[0].Steps) != 4 || dynamic[0].Steps[1].ExpectedAction != "project.get" || dynamic[0].Steps[3].ExpectedAction != actionDiscoverProjectResolve {
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
