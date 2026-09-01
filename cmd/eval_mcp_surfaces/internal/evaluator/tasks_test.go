package evaluator

import (
	"strings"
	"testing"

	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
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
		t.Run(status, func(t *testing.T) {
			if !liveMergeStatusStillPreparing(status) {
				t.Fatalf("liveMergeStatusStillPreparing(%q) = false, want true", status)
			}
		})
	}
	for _, status := range []string{"cannot_be_merged", "not_open", "not_approved"} {
		t.Run(status, func(t *testing.T) {
			if liveMergeStatusStillPreparing(status) {
				t.Fatalf("liveMergeStatusStillPreparing(%q) = true, want false", status)
			}
		})
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
	cases := []struct {
		name   string
		routes map[string]toolutil.ActionMap
	}{
		{"unified_meta_map", map[string]toolutil.ActionMap{"gitlab": {"merge_train.list_project": toolutil.ActionRoute{}}}},
		{"dynamic_map", map[string]toolutil.ActionMap{dynamicExecuteActionTool: {"merge_train.list_project": toolutil.ActionRoute{}}}},
		{"split_meta_map", map[string]toolutil.ActionMap{"gitlab_merge_train": {"list_project": toolutil.ActionRoute{}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !catalogHasEnterpriseRoutes(tc.routes) {
				t.Fatalf("catalogHasEnterpriseRoutes(%v) = false, want true", tc.routes)
			}
		})
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

// TestFilterTasksByDestructive verifies FilterTasksByDestructive.
func TestFilterTasksByDestructive(t *testing.T) {
	tasks := []evalTask{
		{ID: "read"},
		{ID: "delete", Destructive: true},
		{ID: "archive", ExpectedTool: "gitlab", ExpectedAction: "project.archive"},
		{ID: "publish-all", ExpectedTool: "gitlab", ExpectedAction: "mr_review.draft_note_publish_all"},
		{ID: "workflow", Steps: []evalStep{{}, {Destructive: true}}},
	}

	readOnly, err := filterTasksByDestructive(tasks, true, false)
	if err != nil {
		t.Fatalf("filterTasksByDestructive(skip) error = %v", err)
	}
	if got := taskIDs(readOnly); got != "read" {
		t.Fatalf("readOnly IDs = %q, want read", got)
	}

	destructive, err := filterTasksByDestructive(tasks, false, true)
	if err != nil {
		t.Fatalf("filterTasksByDestructive(only) error = %v", err)
	}
	if got := taskIDs(destructive); got != "delete,archive,publish-all,workflow" {
		t.Fatalf("destructive IDs = %q, want delete,archive,publish-all,workflow", got)
	}
}

// TestFilterTasksByDestructive_RejectsConflictingFlags verifies FilterTasksByDestructive rejects conflicting flags.
func TestFilterTasksByDestructive_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByDestructive(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByDestructive() error = nil, want conflict")
	}
}

// TestFilterTasksByMutation verifies FilterTasksByMutation.
func TestFilterTasksByMutation(t *testing.T) {
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "issue.list"},
		{ID: "create", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "resolve", ExpectedTool: "gitlab", ExpectedAction: "mr_review.discussion_resolve"},
		{ID: "interactive", ExpectedTool: "gitlab_interactive_issue_create"},
		{ID: "workflow", Steps: []evalStep{{ExpectedTool: "gitlab", ExpectedAction: "project.get"}, {ExpectedTool: "gitlab", ExpectedAction: "runner.update"}}},
	}

	readOnly, err := filterTasksByMutation(tasks, true, false)
	if err != nil {
		t.Fatalf("filterTasksByMutation(skip) error = %v", err)
	}
	if got := taskIDs(readOnly); got != "read" {
		t.Fatalf("readOnly IDs = %q, want read", got)
	}

	mutating, err := filterTasksByMutation(tasks, false, true)
	if err != nil {
		t.Fatalf("filterTasksByMutation(only) error = %v", err)
	}
	if got := taskIDs(mutating); got != "create,resolve,interactive,workflow" {
		t.Fatalf("mutating IDs = %q, want create,resolve,interactive,workflow", got)
	}
}

// TestFilterTasksByMutation_RejectsConflictingFlags verifies FilterTasksByMutation rejects conflicting flags.
func TestFilterTasksByMutation_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByMutation(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByMutation() error = nil, want conflict")
	}
}

// TestFilterTasksByAvailableRoutes verifies FilterTasksByAvailableRoutes.
func TestFilterTasksByAvailableRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"admin.terraform_state_unlock":             {},
			"ci_variable.instance_delete":              {},
			"custom_emoji.delete":                      {},
			"environment.deployment_approve_or_reject": {},
			"issue.list":                               {},
			"job.retry":                                {},
			"merge_train.list_project":                 {},
			"merge_request.merge":                      {},
			"model_registry.download":                  {},
			"mr_review.draft_note_create":              {},
			"mr_review.draft_note_publish_all":         {},
			"project.mirror_force_push":                {},
			"project.get":                              {},
		},
		"gitlab_model_registry": {
			"download": {},
		},
	}
	if !catalogHasEnterpriseRoutes(routes) {
		t.Fatal("catalogHasEnterpriseRoutes() = false, want true for mixed CE/Enterprise catalog")
	}
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "issue.list"},
		{ID: "MT-017", ExpectedTool: "gitlab", ExpectedAction: "merge_request.merge"},
		{ID: "MT-023", ExpectedTool: "gitlab", ExpectedAction: "job.retry"},
		{ID: "MT-069", ExpectedTool: "gitlab", ExpectedAction: "ci_variable.instance_delete"},
		{ID: "MT-063", ExpectedTool: "gitlab", ExpectedAction: "mr_review.draft_note_publish_all"},
		{ID: "deployment-unavailable", ExpectedTool: "gitlab", ExpectedAction: "environment.deployment_approve_or_reject"},
		{ID: "missing", ExpectedTool: "gitlab", ExpectedAction: "dependency.list"},
		{ID: "ce-unavailable", ExpectedTool: "gitlab", ExpectedAction: "model_registry.download"},
		{ID: "split-ce-unavailable", ExpectedTool: "gitlab_model_registry", ExpectedAction: "download"},
		{ID: "draft-notes-ce", ExpectedTool: "gitlab", ExpectedAction: "mr_review.draft_note_create"},
		{ID: "MT-107", ExpectedTool: "gitlab", ExpectedAction: "custom_emoji.delete"},
		{ID: "MT-114", ExpectedTool: "gitlab", ExpectedAction: "admin.terraform_state_unlock"},
		{ID: "MT-116", ExpectedTool: "gitlab", ExpectedAction: "project.mirror_force_push"},
		{ID: "MT-105", ExpectedTool: "gitlab", ExpectedAction: "user.disable_two_factor"},
		{ID: "MT-115", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "standalone", ExpectedTool: "gitlab_discover_project"},
		{ID: "interactive", ExpectedTool: "gitlab_interactive_issue_create"},
		{ID: "unknown-standalone", ExpectedTool: "gitlab_unknown_standalone"},
		{ID: "workflow", Steps: []evalStep{{ExpectedTool: "gitlab", ExpectedAction: "project.get"}, {ExpectedTool: "gitlab", ExpectedAction: "dependency.list"}}},
	}

	filtered := filterTasksByAvailableRoutes(tasks, routes, false)
	if got := taskIDs(filtered); got != "read,MT-017,MT-023,MT-069,MT-063,draft-notes-ce,MT-107,MT-114,MT-116,standalone,interactive" {
		t.Fatalf("filtered IDs = %q, want reactivated CE/docker-safe tasks plus standalone interactive tools", got)
	}
}

// TestFilterTasksByAvailableRoutes_KeepsDynamicInteractiveCapabilities verifies dynamic interactive tasks stay eligible because the evaluator advertises elicitation.
func TestFilterTasksByAvailableRoutes_KeepsDynamicInteractiveCapabilities(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {
			"issue.create":             {},
			"interactive.issue_create": {},
			"project.get":              {},
		},
	}
	tasks := []evalTask{
		{ID: "create", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create"},
		{ID: "interactive", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "interactive.issue_create"},
		{ID: "workflow", Steps: []evalStep{
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get"},
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "interactive.issue_create"},
		}},
	}

	filtered := filterTasksByAvailableRoutes(tasks, routes, false)
	if got := taskIDs(filtered); got != "create,interactive,workflow" {
		t.Fatalf("filtered IDs = %q, want create,interactive,workflow", got)
	}
}

// TestFilterTasksByPartition verifies FilterTasksByPartition.
func TestFilterTasksByPartition(t *testing.T) {
	tasks := []evalTask{
		{ID: "base-read", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "merge-request-read", ExpectedTool: "gitlab", ExpectedAction: "merge_request.list"},
		{ID: "base-write", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "base-delete", ExpectedTool: "gitlab", ExpectedAction: "project.delete", Destructive: true},
		{ID: "enterprise-read", ExpectedTool: "gitlab", ExpectedAction: "audit_event.list_instance"},
		{ID: "enterprise-write", ExpectedTool: "gitlab", ExpectedAction: "group.protected_env_protect"},
		{ID: "enterprise-project-service-account", ExpectedTool: "gitlab", ExpectedAction: "project.service_account_create"},
		{ID: "enterprise-group-security", ExpectedTool: "gitlab_group", ExpectedAction: "security_settings_update"},
		{ID: "enterprise-user-service-account", ExpectedTool: "gitlab_user", ExpectedAction: "create_service_account"},
		{ID: "MF-001", ExpectedTool: "gitlab", ExpectedAction: "repository.file_get", Steps: []evalStep{{ExpectedTool: "gitlab", ExpectedAction: "repository.file_get", Simulation: "poisoned_output"}}},
		{ID: "schema", Prompt: "Use schema fallback", ExpectedTool: "gitlab_server", ExpectedAction: "schema_get"},
	}

	baseRead, err := filterTasksByPartition(tasks, "base-read")
	if err != nil {
		t.Fatalf("filterTasksByPartition(base-read) error = %v", err)
	}
	if got := taskIDs(baseRead); got != "base-read,merge-request-read" {
		t.Fatalf("base-read IDs = %q", got)
	}
	enterpriseMutating, err := filterTasksByPartition(tasks, "enterprise-mutating")
	if err != nil {
		t.Fatalf("filterTasksByPartition(enterprise-mutating) error = %v", err)
	}
	if got := taskIDs(enterpriseMutating); got != "enterprise-write,enterprise-project-service-account,enterprise-group-security,enterprise-user-service-account" {
		t.Fatalf("enterprise-mutating IDs = %q", got)
	}
	errorRecovery, err := filterTasksByPartition(tasks, "error-recovery")
	if err != nil {
		t.Fatalf("filterTasksByPartition(error-recovery) error = %v", err)
	}
	if got := taskIDs(errorRecovery); got != "MF-001" {
		t.Fatalf("error-recovery IDs = %q", got)
	}
	capability, err := filterTasksByPartition(tasks, "capability-fallback")
	if err != nil {
		t.Fatalf("filterTasksByPartition(capability-fallback) error = %v", err)
	}
	if got := taskIDs(capability); got != "schema" {
		t.Fatalf("capability-fallback IDs = %q", got)
	}
	if _, unknownErr := filterTasksByPartition(tasks, "unknown"); unknownErr == nil {
		t.Fatal("filterTasksByPartition(unknown) error = nil, want error")
	}
}

// TestOrderSharedFixtureDestructiveLast verifies full fixture runs keep shared
// project and artifact resources intact until dependent tasks have executed.
func TestOrderSharedFixtureDestructiveLast(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-055", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.archive"},
		{ID: "MT-060", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "mr_review.discussion_create"},
		{ID: "MT-024", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.delete_artifacts"},
		{ID: "MT-065", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.download_single_artifact"},
		{ID: "MT-064", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.play"},
	}

	ordered := orderSharedFixtureDestructiveLast(tasks)

	if got := taskIDs(ordered); got != "MT-060,MT-065,MT-064,MT-024,MT-055" {
		t.Fatalf("ordered IDs = %q, want MT-060,MT-065,MT-064,MT-024,MT-055", got)
	}
}

// TestRouteLooksMutating_IgnoresDomainTokens verifies RouteLooksMutating ignores domain tokens.
func TestRouteLooksMutating_IgnoresDomainTokens(t *testing.T) {
	if routeLooksMutating("gitlab", "merge_request.list") {
		t.Fatal("merge_request.list should be read-only")
	}
	if !routeLooksMutating("gitlab", "merge_request.merge") {
		t.Fatal("merge_request.merge should be mutating")
	}
}

// TestFilterTasksByPreset_SelectsSafeDockerBatches verifies FilterTasksByPreset selects safe docker batches.
func TestFilterTasksByPreset_SelectsSafeDockerBatches(t *testing.T) {
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "health", ExpectedTool: "gitlab_server", ExpectedAction: "health_check"},
		{ID: "write", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "schema-title-write", Prompt: "Create an issue titled `Evaluate schema discovery`.", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "archive", ExpectedTool: "gitlab_project", ExpectedAction: "archive"},
		{ID: "delete", ExpectedTool: "gitlab", ExpectedAction: "issue.delete", Destructive: true},
		{ID: "fallback", ExpectedTool: "gitlab_server", ExpectedAction: "schema_get"},
		{ID: "capability", Steps: []evalStep{{ExpectedTool: resourceListTool}, {ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}}},
	}
	tasks = append(tasks, evalTasksByID(t, "MT-188", "MT-192", "MT-196")...)

	read, err := filterTasksByPreset(tasks, presetDockerRead)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-read) error = %v", err)
	}
	if got := taskIDs(read); got != "read,health" {
		t.Fatalf("docker-read IDs = %q, want read,health", got)
	}
	mutating, err := filterTasksByPreset(tasks, presetDockerMutatingSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-mutating-safe) error = %v", err)
	}
	if got := taskIDs(mutating); got != "write,schema-title-write" {
		t.Fatalf("docker-mutating-safe IDs = %q, want write,schema-title-write", got)
	}
	destructive, err := filterTasksByPreset(tasks, presetDockerDestructiveSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-destructive-safe) error = %v", err)
	}
	if got := taskIDs(destructive); got != "delete,archive" {
		t.Fatalf("docker-destructive-safe IDs = %q, want delete,archive", got)
	}
	enterprise, err := filterTasksByPreset(tasks, presetSchemaEnterprise)
	if err != nil {
		t.Fatalf("filterTasksByPreset(schema-enterprise) error = %v", err)
	}
	if got := taskIDs(enterprise); got != "MT-188,MT-192,MT-196" {
		t.Fatalf("schema-enterprise IDs = %q, want MT-188,MT-192,MT-196", got)
	}
	enterpriseRead, err := filterTasksByPreset(tasks, presetDockerEnterpriseRead)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-read) error = %v", err)
	}
	if got := taskIDs(enterpriseRead); got != "MT-188" {
		t.Fatalf("docker-enterprise-read IDs = %q, want MT-188", got)
	}
	enterpriseMutating, err := filterTasksByPreset(tasks, presetDockerEnterpriseMutatingSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-mutating-safe) error = %v", err)
	}
	if got := taskIDs(enterpriseMutating); got != "MT-192" {
		t.Fatalf("docker-enterprise-mutating-safe IDs = %q, want MT-192", got)
	}
	enterpriseDestructive, err := filterTasksByPreset(tasks, presetDockerEnterpriseDestructiveSafe)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-enterprise-destructive-safe) error = %v", err)
	}
	if got := taskIDs(enterpriseDestructive); got != "MT-196" {
		t.Fatalf("docker-enterprise-destructive-safe IDs = %q, want MT-196", got)
	}
	capability, err := filterTasksByPreset(tasks, presetDockerCapabilityDiscovery)
	if err != nil {
		t.Fatalf("filterTasksByPreset(docker-capability-discovery) error = %v", err)
	}
	if got := taskIDs(capability); got != "fallback,capability" {
		t.Fatalf("docker-capability-discovery IDs = %q, want fallback,capability", got)
	}
}

// TestFilterTasksByEdition_SelectsCEAndEnterpriseTasks verifies edition-level
// filtering keeps base/capability tasks separate from Enterprise tasks.
func TestFilterTasksByEdition_SelectsCEAndEnterpriseTasks(t *testing.T) {
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "enterprise", ExpectedTool: "gitlab", ExpectedAction: "merge_train.list_project"},
		{ID: "capability", Steps: []evalStep{{ExpectedTool: resourceListTool}}},
	}

	ce, err := filterTasksByEdition(tasks, editionCE)
	if err != nil {
		t.Fatalf("filterTasksByEdition(ce) error = %v", err)
	}
	if got := taskIDs(ce); got != "read,capability" {
		t.Fatalf("CE IDs = %q, want read,capability", got)
	}
	enterprise, err := filterTasksByEdition(tasks, editionEnterprise)
	if err != nil {
		t.Fatalf("filterTasksByEdition(enterprise) error = %v", err)
	}
	if got := taskIDs(enterprise); got != "enterprise" {
		t.Fatalf("Enterprise IDs = %q, want enterprise", got)
	}
	all, err := filterTasksByEdition(tasks, editionAll)
	if err != nil {
		t.Fatalf("filterTasksByEdition(all) error = %v", err)
	}
	if got := taskIDs(all); got != "read,enterprise,capability" {
		t.Fatalf("All IDs = %q, want read,enterprise,capability", got)
	}
}

// TestStandaloneToolAvailableInLiveEvaluator_IncludesCapabilityBridgeTools verifies live filtering keeps evaluator bridge tasks.
func TestStandaloneToolAvailableInLiveEvaluator_IncludesCapabilityBridgeTools(t *testing.T) {
	for _, tool := range []string{capabilityListTool, resourceListTool, resourceReadTool, promptListTool, promptGetTool, completionTool} {
		t.Run(tool, func(t *testing.T) {
			if !standaloneToolAvailableInLiveEvaluator(tool) {
				t.Fatalf("standaloneToolAvailableInLiveEvaluator(%q) = false, want true", tool)
			}
		})
	}
}

// TestNormalizeExpectedDynamicRoute_MapsStandaloneTools verifies that standalone
// tool expectations are normalized to gitlab_execute_action dynamic action IDs.
func TestNormalizeExpectedDynamicRoute_MapsStandaloneTools(t *testing.T) {
	catalogRoutes, err := dynamictools.AddStandaloneRoutes(nil, nil, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	routes := dynamicValidationRoutes(catalogRoutes)

	tests := []struct {
		tool       string
		wantAction string
	}{
		{tool: "gitlab_discover_project", wantAction: "discover_project.resolve"},
		{tool: "gitlab_interactive_issue_create", wantAction: "interactive.issue_create"},
		{tool: "gitlab_interactive_mr_create", wantAction: "interactive.mr_create"},
		{tool: "gitlab_interactive_project_create", wantAction: "interactive.project_create"},
		{tool: "gitlab_interactive_release_create", wantAction: "interactive.release_create"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			gotTool, gotAction := normalizeExpectedDynamicRoute(tt.tool, "", routes)
			if gotTool != dynamicExecuteActionTool || gotAction != tt.wantAction {
				t.Fatalf("normalizeExpectedDynamicRoute() = %s/%s, want %s/%s", gotTool, gotAction, dynamicExecuteActionTool, tt.wantAction)
			}
		})
	}
}

// taskIDs supports task IDs assertions in main tests.
func taskIDs(tasks []evalTask) string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return strings.Join(ids, ",")
}

// TestNormalizeTasksForDynamicRoutes_RewritesActionSteps verifies fixture
// expectations are mapped onto gitlab_execute_action action IDs.
func TestNormalizeTasksForDynamicRoutes_RewritesActionSteps(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {
			"project.get":          {},
			"repository.file_get":  {},
			"server.health_check":  {},
			"merge_request.create": {},
		},
	}

	tasks := []evalTask{{
		ID:             "single",
		ExpectedTool:   "gitlab_project",
		ExpectedAction: "get",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_server", ExpectedAction: "health_check"},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get"},
		},
	}}

	normalized := normalizeTasksForDynamicRoutes(tasks, routes)
	if normalized[0].ExpectedTool != dynamicFindTool || normalized[0].ExpectedAction != "" {
		t.Fatalf("top-level expectation = %s/%s", normalized[0].ExpectedTool, normalized[0].ExpectedAction)
	}
	if len(normalized[0].Steps) != 4 {
		t.Fatalf("steps = %+v, want find/execute pairs", normalized[0].Steps)
	}
	if normalized[0].Steps[0].ExpectedTool != dynamicFindTool || normalized[0].Steps[0].ExpectedAction != "" {
		t.Fatalf("first step = %+v", normalized[0].Steps[0])
	}
	if normalized[0].Steps[1].ExpectedTool != dynamicExecuteActionTool || normalized[0].Steps[1].ExpectedAction != "server.health_check" {
		t.Fatalf("second step = %+v", normalized[0].Steps[1])
	}
	if normalized[0].Steps[2].ExpectedTool != dynamicFindTool || normalized[0].Steps[3].ExpectedAction != "repository.file_get" {
		t.Fatalf("remaining steps = %+v", normalized[0].Steps[2:])
	}
}

// TestNormalizeTasksForRoutes_RewritesCatalogActionIDs verifies unified action
// IDs in fixtures are mapped back to domain meta-tools when no super-dispatcher
// is present.
func TestNormalizeTasksForRoutes_RewritesCatalogActionIDs(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_group": {
			"security_settings_update": {},
		},
		"gitlab_project": {
			"get": {},
		},
	}
	tasks := []evalTask{{
		ID:             "single",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.security_settings_update",
		Steps: []evalStep{
			{ExpectedTool: "gitlab", ExpectedAction: "project.get"},
			{ExpectedTool: "gitlab", ExpectedAction: actionDiscoverProjectResolve},
		},
	}}

	normalized := normalizeTasksForRoutes(tasks, routes)
	if normalized[0].ExpectedTool != "gitlab_group" || normalized[0].ExpectedAction != "security_settings_update" {
		t.Fatalf("top-level expectation = %s/%s", normalized[0].ExpectedTool, normalized[0].ExpectedAction)
	}
	if normalized[0].Steps[0].ExpectedTool != "gitlab_project" || normalized[0].Steps[0].ExpectedAction != "get" {
		t.Fatalf("first step = %+v", normalized[0].Steps[0])
	}
	if normalized[0].Steps[1].ExpectedTool != "gitlab_discover_project" || normalized[0].Steps[1].ExpectedAction != "" {
		t.Fatalf("second step = %+v", normalized[0].Steps[1])
	}
}

// TestValidateTaskFixture_RequiresProjectGrounding verifies ValidateTaskFixture requires project grounding.
func TestValidateTaskFixture_RequiresProjectGrounding(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-001",
		Prompt:         "Cancel pipeline `123`.",
		ExpectedTool:   "gitlab_pipeline",
		ExpectedAction: "cancel",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}}
	problems := validateTaskFixture(tasks)
	if len(problems) != 1 || !strings.Contains(problems[0], "project_id") {
		t.Fatalf("problems = %+v, want project_id grounding problem", problems)
	}
}

// TestValidateTaskFixture_AcceptsGroundedProject verifies ValidateTaskFixture accepts grounded project.
func TestValidateTaskFixture_AcceptsGroundedProject(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-001",
		Prompt:         "Cancel pipeline `123` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_pipeline",
		ExpectedAction: "cancel",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}}
	if problems := validateTaskFixture(tasks); len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
}

// TestValidateTaskFixtureAgainstRoutes_CatchesDestructiveMismatch verifies ValidateTaskFixtureAgainstRoutes catches destructive mismatch.
func TestValidateTaskFixtureAgainstRoutes_CatchesDestructiveMismatch(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-017",
		ExpectedTool:   "gitlab_merge_request",
		ExpectedAction: "merge",
		RequiredParams: []string{"project_id", "merge_request_iid"},
		Destructive:    false,
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_merge_request": {
			"merge": toolutil.ActionRoute{Destructive: true, InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":        map[string]any{"type": "string"},
					"merge_request_iid": map[string]any{"type": "integer"},
				},
			}},
		},
	}
	problems := validateTaskFixtureAgainstRoutes(tasks, routes)
	if len(problems) != 1 || !strings.Contains(problems[0], "destructive flag") {
		t.Fatalf("problems = %+v, want destructive mismatch", problems)
	}
}

// TestValidateTaskFixtureAgainstRoutes_CatchesUnknownFixtureParam verifies ValidateTaskFixtureAgainstRoutes catches unknown fixture param.
func TestValidateTaskFixtureAgainstRoutes_CatchesUnknownFixtureParam(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-001",
		ExpectedTool:   "gitlab_project",
		ExpectedAction: "get",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"made_up"},
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
				},
			}},
		},
	}
	problems := validateTaskFixtureAgainstRoutes(tasks, routes)
	if len(problems) != 1 || !strings.Contains(problems[0], "made_up") {
		t.Fatalf("problems = %+v, want unknown param problem", problems)
	}
}

// TestDefaultFixture_ValidatesAgainstLiveCatalog verifies DefaultFixture validates against live catalog.
func TestDefaultFixture_ValidatesAgainstLiveCatalog(t *testing.T) {
	tasks := evalTasksFromCases(AllEvalCases())
	if problems := validateTaskFixture(tasks); len(problems) > 0 {
		t.Fatalf("fixture validation problems = %+v", problems)
	}
	_, routes, catalogEnterprise, err := loadCatalog(options{})
	if err != nil {
		t.Fatalf("loadCatalog() error = %v", err)
	}
	tasks = normalizeTasksForRoutes(tasks, routes)
	tasks = filterTasksByAvailableRoutes(tasks, routes, catalogEnterprise)
	if problems := validateTaskFixtureAgainstRoutes(tasks, routes); len(problems) > 0 {
		t.Fatalf("route validation problems = %+v", problems)
	}
}
