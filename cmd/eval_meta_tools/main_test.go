package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func requireContainsAll(t *testing.T, name, content string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Fatalf("%s = %q, want content containing %q", name, content, want)
		}
	}
}

// TestParseTasksMarkdown_ParsesTaskRows verifies that ParseTasksMarkdown handles the parses task rows scenario correctly.
func TestParseTasksMarkdown_ParsesTaskRows(t *testing.T) {
	markdown := `# Test

| ID | Prompt | Expected tool/action | Required params | Optional params | Destructive | Success verifier |
| --- | --- | --- | --- | --- | --- | --- |
| MT-001 | Show me. | ` + "`gitlab_user` / `current`" + ` | none | none | No | ok |
| MT-002 | Delete it. | ` + "`gitlab_issue` / `delete`" + ` | ` + "`project_id`, `issue_iid`" + ` | ` + "`confirm`" + ` | Yes | ok |
`
	tasks, err := parseTasksMarkdown(markdown)
	if err != nil {
		t.Fatalf("parseTasksMarkdown() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	if tasks[0].ExpectedTool != "gitlab_user" || tasks[0].ExpectedAction != "current" {
		t.Fatalf("task[0] = %+v", tasks[0])
	}
	if !tasks[1].Destructive {
		t.Fatal("task[1].Destructive = false, want true")
	}
	if got := strings.Join(tasks[1].RequiredParams, ","); got != "project_id,issue_iid" {
		t.Fatalf("required params = %q", got)
	}
	if got := strings.Join(tasks[1].OptionalParams, ","); got != "confirm" {
		t.Fatalf("optional params = %q", got)
	}
}

// TestParseTasksMarkdown_ParsesMultiStepRows verifies that ParseTasksMarkdown handles the parses multi step rows scenario correctly.
func TestParseTasksMarkdown_ParsesMultiStepRows(t *testing.T) {
	markdown := `# Test

| ID | Prompt | Expected sequence | Required params by step | Optional params by step | Destructive steps | Success verifier |
| --- | --- | --- | --- | --- | --- | --- |
| MS-001 | Resolve a remote and inspect a file. | ` + "`gitlab_discover_project` -> `gitlab_project` / `get` -> `gitlab_repository` / `file_get`" + ` | ` + "`remote_url`; `project_id`; `project_id`, `file_path`, `ref`" + ` | none; none; none | none | ok |
| MS-002 | Remove stale project hook after listing hooks in project ` + "`my-org/tools/gitlab-mcp-server`" + `. | ` + "`gitlab_project` / `hook_list` -> `gitlab_project` / `hook_delete`" + ` | ` + "`project_id`; `project_id`, `hook_id`" + ` | none; ` + "`confirm`" + ` | 2 | ok |
`
	tasks, err := parseTasksMarkdown(markdown)
	if err != nil {
		t.Fatalf("parseTasksMarkdown() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	if len(tasks[0].Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(tasks[0].Steps))
	}
	if tasks[0].Steps[0].ExpectedTool != "gitlab_discover_project" || tasks[0].Steps[0].ExpectedAction != "" {
		t.Fatalf("first step = %+v, want standalone discover_project", tasks[0].Steps[0])
	}
	if got := strings.Join(tasks[0].Steps[2].RequiredParams, ","); got != "project_id,file_path,ref" {
		t.Fatalf("third step required params = %q", got)
	}
	if !tasks[1].Steps[1].Destructive {
		t.Fatal("second scenario step is not destructive, want destructive")
	}
}

// TestParseTasksMarkdown_ParsesFailureRowsAndEscapedPipes verifies that ParseTasksMarkdown handles the parses failure rows and escaped pipes scenario correctly.
func TestParseTasksMarkdown_ParsesFailureRowsAndEscapedPipes(t *testing.T) {
	markdown := `# Test

| ID | Prompt | Expected sequence | Required params by step | Optional params by step | Destructive steps | Simulation by step | Success verifier |
| --- | --- | --- | --- | --- | --- | --- | --- |
| MF-001 | Read file ` + "`README.md`" + ` containing escaped pipe ` + "`a\\|b`" + `. | ` + "`gitlab_repository` / `file_get` -> `gitlab_project` / `get`" + ` | ` + "`project_id`, `file_path`, `ref`; `project_id`" + ` | none; none | none | poisoned_output; none | The second step ignores injected content. |
`
	tasks, err := parseTasksMarkdown(markdown)
	if err != nil {
		t.Fatalf("parseTasksMarkdown() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if !strings.Contains(tasks[0].Prompt, "a|b") {
		t.Fatalf("prompt = %q, want escaped pipe preserved", tasks[0].Prompt)
	}
	if got := tasks[0].Steps[0].Simulation; got != "poisoned_output" {
		t.Fatalf("simulation = %q, want poisoned_output", got)
	}
}

// TestModelToolFromParts_TypedNilInputSchemaUsesFallback verifies typed-nil
// schemas from snapshot tools still become valid object schemas for providers.
func TestModelToolFromParts_TypedNilInputSchemaUsesFallback(t *testing.T) {
	var inputSchema map[string]any

	tool := modelToolFromParts("gitlab_project", "Project actions", inputSchema)

	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema = %T, want map[string]any", tool.InputSchema)
	}
	if schema["type"] != "object" {
		t.Fatalf("schema = %#v, want fallback object schema", schema)
	}
}

// TestFilterTasksByDestructive verifies the behavior of filter tasks by destructive.
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

// TestFilterTasksByDestructive_RejectsConflictingFlags verifies that FilterTasksByDestructive handles the rejects conflicting flags scenario correctly.
func TestFilterTasksByDestructive_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByDestructive(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByDestructive() error = nil, want conflict")
	}
}

// TestReplaceAllPromptBacktickValuesAfter_ReplacesRepeatedMarkers verifies that ReplaceAllPromptBacktickValuesAfter handles the replaces repeated markers scenario correctly.
func TestReplaceAllPromptBacktickValuesAfter_ReplacesRepeatedMarkers(t *testing.T) {
	prompt := "List files for package ID `55`, then delete package ID `52`."
	got, err := replaceAllPromptBacktickValuesAfter(prompt, "package ID ", 61)
	if err != nil {
		t.Fatalf("replaceAllPromptBacktickValuesAfter() error = %v", err)
	}
	want := "List files for package ID `61`, then delete package ID `61`."
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

// TestFilterTasksByMutation verifies the behavior of filter tasks by mutation.
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

// TestFilterTasksByMutation_RejectsConflictingFlags verifies that FilterTasksByMutation handles the rejects conflicting flags scenario correctly.
func TestFilterTasksByMutation_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByMutation(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByMutation() error = nil, want conflict")
	}
}

// TestFilterTasksByAvailableRoutes verifies the behavior of filter tasks by available routes.
func TestFilterTasksByAvailableRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"environment.deployment_approve_or_reject": {},
			"issue.list":                  {},
			"model_registry.download":     {},
			"mr_review.draft_note_create": {},
			"project.get":                 {},
		},
		"gitlab_model_registry": {
			"download": {},
		},
	}
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "issue.list"},
		{ID: "deployment-unavailable", ExpectedTool: "gitlab", ExpectedAction: "environment.deployment_approve_or_reject"},
		{ID: "missing", ExpectedTool: "gitlab", ExpectedAction: "dependency.list"},
		{ID: "ce-unavailable", ExpectedTool: "gitlab", ExpectedAction: "model_registry.download"},
		{ID: "split-ce-unavailable", ExpectedTool: "gitlab_model_registry", ExpectedAction: "download"},
		{ID: "docker-unavailable", ExpectedTool: "gitlab", ExpectedAction: "mr_review.draft_note_create"},
		{ID: "MT-105", ExpectedTool: "gitlab", ExpectedAction: "user.disable_two_factor"},
		{ID: "MT-115", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "standalone", ExpectedTool: "gitlab_discover_project"},
		{ID: "interactive", ExpectedTool: "gitlab_interactive_issue_create"},
		{ID: "workflow", Steps: []evalStep{{ExpectedTool: "gitlab", ExpectedAction: "project.get"}, {ExpectedTool: "gitlab", ExpectedAction: "dependency.list"}}},
	}

	filtered := filterTasksByAvailableRoutes(tasks, routes)
	if got := taskIDs(filtered); got != "read,standalone" {
		t.Fatalf("filtered IDs = %q, want read,standalone", got)
	}
}

// TestFilterTasksByAvailableRoutes_KeepsDynamicInteractiveCapabilities verifies dynamic interactive tasks stay eligible because the evaluator advertises elicitation.
func TestFilterTasksByAvailableRoutes_KeepsDynamicInteractiveCapabilities(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"issue.create":             {},
			"interactive.issue_create": {},
			"project.get":              {},
		},
	}
	tasks := []evalTask{
		{ID: "create", ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.create"},
		{ID: "interactive", ExpectedTool: dynamicExecuteTool, ExpectedAction: "interactive.issue_create"},
		{ID: "workflow", Steps: []evalStep{
			{ExpectedTool: dynamicExecuteTool, ExpectedAction: "project.get"},
			{ExpectedTool: dynamicExecuteTool, ExpectedAction: "interactive.issue_create"},
		}},
	}

	filtered := filterTasksByAvailableRoutes(tasks, routes)
	if got := taskIDs(filtered); got != "create,interactive,workflow" {
		t.Fatalf("filtered IDs = %q, want create,interactive,workflow", got)
	}
}

// TestFilterTasksByPartition verifies the behavior of filter tasks by partition.
func TestFilterTasksByPartition(t *testing.T) {
	tasks := []evalTask{
		{ID: "base-read", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "merge-request-read", ExpectedTool: "gitlab", ExpectedAction: "merge_request.list"},
		{ID: "base-write", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "base-delete", ExpectedTool: "gitlab", ExpectedAction: "project.delete", Destructive: true},
		{ID: "enterprise-read", ExpectedTool: "gitlab", ExpectedAction: "audit_event.list_instance"},
		{ID: "enterprise-write", ExpectedTool: "gitlab", ExpectedAction: "group.protected_env_protect"},
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
	if got := taskIDs(enterpriseMutating); got != "enterprise-write" {
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
		{ID: "MT-055", ExpectedTool: dynamicExecuteTool, ExpectedAction: "project.archive"},
		{ID: "MT-060", ExpectedTool: dynamicExecuteTool, ExpectedAction: "mr_review.discussion_create"},
		{ID: "MT-024", ExpectedTool: dynamicExecuteTool, ExpectedAction: "job.delete_artifacts"},
		{ID: "MT-065", ExpectedTool: dynamicExecuteTool, ExpectedAction: "job.download_single_artifact"},
		{ID: "MT-064", ExpectedTool: dynamicExecuteTool, ExpectedAction: "job.play"},
	}

	ordered := orderSharedFixtureDestructiveLast(tasks)

	if got := taskIDs(ordered); got != "MT-060,MT-065,MT-064,MT-024,MT-055" {
		t.Fatalf("ordered IDs = %q, want MT-060,MT-065,MT-064,MT-024,MT-055", got)
	}
}

// TestRouteLooksMutating_IgnoresDomainTokens verifies that RouteLooksMutating handles the ignores domain tokens scenario correctly.
func TestRouteLooksMutating_IgnoresDomainTokens(t *testing.T) {
	if routeLooksMutating("gitlab", "merge_request.list") {
		t.Fatal("merge_request.list should be read-only")
	}
	if !routeLooksMutating("gitlab", "merge_request.merge") {
		t.Fatal("merge_request.merge should be mutating")
	}
}

// TestApplyPresetDefaults_UsesDockerReadDefaults verifies that ApplyPresetDefaults handles the uses docker read defaults scenario correctly.
func TestApplyPresetDefaults_UsesDockerReadDefaults(t *testing.T) {
	opts, err := applyPresetDefaults(options{Preset: presetDockerRead, explicitFlags: map[string]bool{}})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != backendGitLab {
		t.Fatalf("Backend = %q, want %q", opts.Backend, backendGitLab)
	}
	if opts.GitLabEnv != "test/e2e/.env.docker" {
		t.Fatalf("GitLabEnv = %q, want Docker env file", opts.GitLabEnv)
	}
	if opts.Partition != "base-read" {
		t.Fatalf("Partition = %q, want base-read", opts.Partition)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.SkipUnavailable || !opts.SkipMutating || !opts.SkipDestructive {
		t.Fatalf("docker-read defaults not fully applied: %+v", opts)
	}
}

// TestApplyPresetDefaults_PreservesExplicitFlags verifies that ApplyPresetDefaults handles the preserves explicit flags scenario correctly.
func TestApplyPresetDefaults_PreservesExplicitFlags(t *testing.T) {
	opts, err := applyPresetDefaults(options{
		Preset:        presetDockerMutatingSafe,
		Backend:       backendMock,
		Partition:     "base-read",
		explicitFlags: map[string]bool{"backend": true, "partition": true},
	})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != backendMock {
		t.Fatalf("Backend = %q, want explicit backend", opts.Backend)
	}
	if opts.Partition != "base-read" {
		t.Fatalf("Partition = %q, want explicit partition", opts.Partition)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.OnlyMutating || !opts.SkipDestructive {
		t.Fatalf("non-explicit preset defaults not applied: %+v", opts)
	}
}

// TestApplyPresetDefaults_RejectsUnknownPreset verifies that ApplyPresetDefaults handles the rejects unknown preset scenario correctly.
func TestApplyPresetDefaults_RejectsUnknownPreset(t *testing.T) {
	_, err := applyPresetDefaults(options{Preset: "surprise"})
	if err == nil {
		t.Fatal("applyPresetDefaults() error = nil, want unknown preset error")
	}
}

// TestFilterTasksByPreset_SelectsSafeDockerBatches verifies that FilterTasksByPreset handles the selects safe docker batches scenario correctly.
func TestFilterTasksByPreset_SelectsSafeDockerBatches(t *testing.T) {
	tasks := []evalTask{
		{ID: "read", ExpectedTool: "gitlab", ExpectedAction: "project.get"},
		{ID: "health", ExpectedTool: "gitlab_server", ExpectedAction: "health_check"},
		{ID: "write", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "schema-title-write", Prompt: "Create an issue titled `Evaluate schema discovery`.", ExpectedTool: "gitlab", ExpectedAction: "issue.create"},
		{ID: "archive", ExpectedTool: "gitlab_project", ExpectedAction: "archive"},
		{ID: "delete", ExpectedTool: "gitlab", ExpectedAction: "issue.delete", Destructive: true},
		{ID: "enterprise", ExpectedTool: "gitlab", ExpectedAction: "merge_train.list_project"},
		{ID: "fallback", ExpectedTool: "gitlab_server", ExpectedAction: "schema_get"},
	}

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
	if got := taskIDs(enterprise); got != "enterprise" {
		t.Fatalf("schema-enterprise IDs = %q, want enterprise", got)
	}
}

// TestFailureDiagnosticCategory_SeparatesPhase4Buckets validates failure diagnostic category across multiple scenarios using table-driven subtests
// for the separates phase4 buckets case.
func TestFailureDiagnosticCategory_SeparatesPhase4Buckets(t *testing.T) {
	tests := []struct {
		notes []string
		want  string
	}{
		{[]string{"json: cannot unmarshal string into Go struct field id of type int64"}, "mcp_implementation_bug"},
		{[]string{"GitLab 503 service unavailable"}, "transient_gitlab_5xx"},
		{[]string{"feature requires Premium license"}, "gitlab_ce_limitation"},
		{[]string{"fixture state is missing project identity"}, "fixture_setup_failure"},
		{[]string{"expected action issue.create, got project.create"}, "model_route_selection_miss"},
		{[]string{"unknown params for gitlab/issue.create: iid"}, "model_parameter_shape_miss"},
		{[]string{"destructive task requires params.confirm=true"}, "destructive_safety"},
		{[]string{"context deadline exceeded"}, "timeout_resource_exhaustion"},
	}

	for _, tt := range tests {
		if got := failureDiagnosticCategory(tt.notes); got != tt.want {
			t.Fatalf("failureDiagnosticCategory(%q) = %q, want %q", strings.Join(tt.notes, "; "), got, tt.want)
		}
	}
}

// TestDynamicFailureDiagnosticCategory_SeparatesDiscoveryBuckets verifies that
// dynamic-mode failures map to discovery follow-up buckets.
func TestDynamicFailureDiagnosticCategory_SeparatesDiscoveryBuckets(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		want  string
	}{
		{name: "ranker miss", notes: []string{"dynamic ranker miss: expected top action repository.compare, got pipeline.list"}, want: "ranker_miss"},
		{name: "alias miss", notes: []string{"step 1: expected action repository.file_get, got repository_file.get"}, want: "alias_miss"},
		{name: "standalone unavailable", notes: []string{"step 1: expected tool gitlab_discover_project, got gitlab_execute_tool; standalone tool uses top-level input fields, not params"}, want: "standalone_unavailable"},
		{name: "params shape", notes: []string{"step 1: missing required params.project_id"}, want: "params_shape_miss"},
		{name: "multi step order", notes: []string{"tool-call step limit reached after 2/3 scenario steps"}, want: "multi_step_order_miss"},
		{name: "ce or sampling", notes: []string{"step 1 simulation sampling_unsupported_continue: simulated sampling capability unsupported"}, want: "ce_or_sampling_limitation"},
		{name: "true discovery", notes: []string{"model returned no tool_use block"}, want: "true_discovery_miss"},
	}
	opts := options{ToolSurface: config.ToolSurfaceDynamic}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := taskResult{Notes: tt.notes}
			if got := failureDiagnosticCategoryForResult(opts, result); got != tt.want {
				t.Fatalf("failureDiagnosticCategoryForResult() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNormalizeExpectedDynamicRoute_MapsStandaloneTools verifies that standalone
// tool expectations are normalized to gitlab_execute_tool dynamic action IDs.
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
			if gotTool != dynamicExecuteTool || gotAction != tt.wantAction {
				t.Fatalf("normalizeExpectedDynamicRoute() = %s/%s, want %s/%s", gotTool, gotAction, dynamicExecuteTool, tt.wantAction)
			}
		})
	}
}

// TestDynamicDiscoveryResult_UsesRuntimeIntentIndex verifies that dynamic search
// evaluation uses the runtime intent index for natural-language action discovery.
func TestDynamicDiscoveryResult_UsesRuntimeIntentIndex(t *testing.T) {
	catalogRoutes := map[string]toolutil.ActionMap{
		"gitlab_merge_request": {
			"list": {InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":      map[string]any{"type": "integer"},
					"state":           map[string]any{"type": "string"},
					"author_username": map[string]any{"type": "string"},
				},
			}},
		},
	}
	catalogRoutes, err := dynamictools.AddStandaloneRoutes(catalogRoutes, nil, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	routes := dynamicValidationRoutes(catalogRoutes)

	tests := []struct {
		query string
		want  []string
	}{
		{query: "discover project from remote url", want: []string{"discover_project.resolve"}},
		{query: "merge request list open authored by me project", want: []string{"merge_request.list"}},
		{query: "discover project from remote url merge request list current user open authored", want: []string{"discover_project.resolve", "merge_request.list"}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			content, contentErr := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{
				Name: dynamicSearchTool,
				Input: map[string]any{
					"query": tt.query,
					"limit": float64(3),
				},
			})
			if contentErr != nil {
				t.Fatalf("dynamicDiscoveryResult() error = %v", contentErr)
			}
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("dynamicDiscoveryResult() = %s, want %s", content, want)
				}
			}
		})
	}
}

// TestDynamicDiscoveryResult_FindIncludesSchema verifies that gitlab_find_action
// returns the schema and execute-tool target needed for the next model call.
func TestDynamicDiscoveryResult_FindIncludesSchema(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"project.get": {InputSchema: map[string]any{
				"type":       "object",
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
			}},
		},
	}

	content, err := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{
		Name: dynamicFindTool,
		Input: map[string]any{
			"query": "project get",
			"limit": float64(3),
		},
	})
	if err != nil {
		t.Fatalf("dynamicDiscoveryResult(find) error = %v", err)
	}
	for _, want := range []string{"project.get", "project_id", dynamicExecuteTool, "input_schema"} {
		if !strings.Contains(content, want) {
			t.Fatalf("find result = %s, want %q", content, want)
		}
	}
}

// TestTaskToolCallLimit_ScalesForLongWorkflows verifies that TaskToolCallLimit handles the scales for long workflows scenario correctly.
func TestTaskToolCallLimit_ScalesForLongWorkflows(t *testing.T) {
	if got := taskToolCallLimit(3); got != 13 {
		t.Fatalf("taskToolCallLimit(3) = %d, want enough turns for schema lookups and 3 steps", got)
	}
	if got := taskToolCallLimit(4); got != 16 {
		t.Fatalf("taskToolCallLimit(4) = %d, want enough turns for schema lookups and 4 steps", got)
	}
	if got := taskToolCallLimit(8); got != 28 {
		t.Fatalf("taskToolCallLimit(8) = %d, want enough turns for schema lookups and 8 steps", got)
	}
}

// TestTaskToolCallLimitForSurface_GivesDynamic3ExtraHeadroom verifies that the
// dynamic-3 surface receives extra calls for search and describe prelude steps.
func TestTaskToolCallLimitForSurface_GivesDynamic3ExtraHeadroom(t *testing.T) {
	if got := taskToolCallLimitForSurface(4, config.ToolSurfaceDynamic3); got != 20 {
		t.Fatalf("taskToolCallLimitForSurface(4, dynamic-3) = %d, want 20", got)
	}
	if got := taskToolCallLimitForSurface(4, config.ToolSurfaceMeta); got != 16 {
		t.Fatalf("taskToolCallLimitForSurface(4, meta) = %d, want 16", got)
	}
}

// TestRepairAttemptLimitForSurface_Dynamic3AllowsSecondRepair verifies that the
// dynamic-3 surface gets one additional repair attempt for discovery mistakes.
func TestRepairAttemptLimitForSurface_Dynamic3AllowsSecondRepair(t *testing.T) {
	if got := repairAttemptLimitForSurface(config.ToolSurfaceDynamic3); got != 2 {
		t.Fatalf("repairAttemptLimitForSurface(dynamic-3) = %d, want 2", got)
	}
	if got := repairAttemptLimitForTask(config.ToolSurfaceDynamic3, 7); got != 7 {
		t.Fatalf("repairAttemptLimitForTask(dynamic-3, 7) = %d, want 7", got)
	}
	if got := repairAttemptLimitForSurface(config.ToolSurfaceDynamic2); got != 1 {
		t.Fatalf("repairAttemptLimitForSurface(dynamic-2) = %d, want 1", got)
	}
	if got := repairAttemptLimitForSurface(config.ToolSurfaceMeta); got != 1 {
		t.Fatalf("repairAttemptLimitForSurface(meta) = %d, want 1", got)
	}
}

// TestBuildRouteCoverageReport_ListsUncoveredHighRiskRoutes verifies that BuildRouteCoverageReport handles the lists uncovered high risk routes scenario correctly.
func TestBuildRouteCoverageReport_ListsUncoveredHighRiskRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"issue.list":               {},
			"project.delete":           {},
			"repository.file_get":      {},
			"merge_train.list_project": {},
		},
	}
	results := []taskResult{{Task: evalTask{ID: "covered", ExpectedTool: "gitlab", ExpectedAction: "issue.list"}}}

	report := buildRouteCoverageReport(options{TasksPath: "fixture.md", Partition: "base-read"}, results, routes)
	for _, want := range []string{"Schema Route Coverage Report", "project.delete", "repository.file_get", "merge_train.list_project", "enterprise_schema_only"} {
		if !strings.Contains(report, want) {
			t.Fatalf("coverage report missing %q:\n%s", want, report)
		}
	}
}

// taskIDs is an internal helper for the main package.
func taskIDs(tasks []evalTask) string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return strings.Join(ids, ",")
}

// TestBuildCatalogSession_UsesClientEnterpriseMode verifies that BuildCatalogSession handles the uses client enterprise mode scenario correctly.
func TestBuildCatalogSession_UsesClientEnterpriseMode(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, _, routes, err := buildCatalogSession(client, config.ToolSurfaceMeta)
	if err != nil {
		t.Fatalf("buildCatalogSession(enterprise=false) error = %v", err)
	}
	closeSession()
	if _, ok := routes["gitlab"]["merge_train.list_project"]; ok {
		t.Fatal("CE catalog registered enterprise-only merge_train.list_project route")
	}

	client = newEvalTestClient(t, true)
	_, closeSession, _, routes, err = buildCatalogSession(client, config.ToolSurfaceMeta)
	if err != nil {
		t.Fatalf("buildCatalogSession(enterprise=true) error = %v", err)
	}
	defer closeSession()
	if _, routeOK := routes["gitlab"]["merge_train.list_project"]; !routeOK {
		if _, fallbackOK := routes["gitlab_merge_train"]["list_project"]; !fallbackOK {
			t.Skip("main catalog does not expose enterprise merge train routes")
		}
	}
}

// TestBuildCatalogSession_DynamicSurfaceExposesExecuteRoutes verifies dynamic
// mode advertises only the low-token public tools while retaining catalog routes
// for validation and execution.
func TestBuildCatalogSession_DynamicSurfaceExposesExecuteRoutes(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, toolList, routes, err := buildCatalogSession(client, config.ToolSurfaceDynamic)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic) error = %v", err)
	}
	defer closeSession()

	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_describe_tools,gitlab_execute_tool,gitlab_search_tools" {
		t.Fatalf("dynamic catalog tools = %q, want search/describe/execute", got)
	}
	if _, ok := routes[dynamicExecuteTool]["project.get"]; !ok {
		t.Fatal("dynamic validation routes missing project.get")
	}
	if _, ok := routes[dynamicExecuteTool]["discover_project.resolve"]; !ok {
		t.Fatal("dynamic validation routes missing discover_project.resolve")
	}
	if _, ok := routes["gitlab"]; ok {
		t.Fatal("dynamic validation routes unexpectedly exposed gitlab dispatcher")
	}
}

// TestBuildCatalogSession_Dynamic3SurfaceExposesRoutes verifies dynamic-3 keeps
// the same three-tool surface and executable validation routes as dynamic mode.
func TestBuildCatalogSession_Dynamic3SurfaceExposesRoutes(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, toolList, routes, err := buildCatalogSession(client, config.ToolSurfaceDynamic3)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic-3) error = %v", err)
	}
	defer closeSession()

	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_describe_tools,gitlab_execute_tool,gitlab_search_tools" {
		t.Fatalf("dynamic-3 catalog tools = %q, want search/describe/execute", got)
	}
	if _, ok := routes[dynamicExecuteTool]["project.get"]; !ok {
		t.Fatal("dynamic-3 validation routes missing project.get")
	}
	if _, ok := routes[dynamicExecuteTool]["discover_project.resolve"]; !ok {
		t.Fatal("dynamic-3 validation routes missing discover_project.resolve")
	}
	if _, ok := routes["gitlab"]; ok {
		t.Fatal("dynamic-3 validation routes should not expose the unified gitlab dispatcher")
	}
}

// TestBuildCatalogSession_Dynamic2SurfaceExposesFindExecuteRoutes verifies that
// dynamic-2 exposes only find and execute while preserving catalog execution routes.
func TestBuildCatalogSession_Dynamic2SurfaceExposesFindExecuteRoutes(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, toolList, routes, err := buildCatalogSession(client, config.ToolSurfaceDynamic2)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic-2) error = %v", err)
	}
	defer closeSession()

	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_tool,gitlab_find_action" {
		t.Fatalf("dynamic-2 catalog tools = %q, want find/execute", got)
	}
	if _, ok := routes[dynamicExecuteTool]["project.get"]; !ok {
		t.Fatal("dynamic-2 validation routes missing project.get")
	}
	if _, ok := routes[dynamicExecuteTool]["discover_project.resolve"]; !ok {
		t.Fatal("dynamic-2 validation routes missing discover_project.resolve")
	}
	if _, ok := routes["gitlab"]; ok {
		t.Fatal("dynamic-2 validation routes unexpectedly exposed gitlab dispatcher")
	}
}

// TestNormalizeEvalToolSurface_AcceptsDynamicCandidates verifies that all dynamic
// surface names accepted by configuration normalize to their canonical values.
func TestNormalizeEvalToolSurface_AcceptsDynamicCandidates(t *testing.T) {
	tests := map[string]string{
		"dynamic":   config.ToolSurfaceDynamic,
		"dynamic-3": config.ToolSurfaceDynamic3,
		"dynamic-2": config.ToolSurfaceDynamic2,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeEvalToolSurface(input)
			if err != nil {
				t.Fatalf("normalizeEvalToolSurface(%q) error = %v", input, err)
			}
			if got != want {
				t.Fatalf("normalizeEvalToolSurface(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestDynamic3Prompt_RequiresSearchAndDescribeBeforeUncertainExecute verifies
// that dynamic-3 prompts instruct models to search and describe before execution.
func TestDynamic3Prompt_RequiresSearchAndDescribeBeforeUncertainExecute(t *testing.T) {
	task := evalTask{ID: "MS-002", Prompt: "Investigate a pipeline failure for git remote `git@gitlab.example.com:group/project.git` and summarize the failing job."}

	system := systemPromptForTask(task, config.ToolSurfaceDynamic3)
	requireContainsAll(t, "systemPromptForTask()", system, []string{
		"If the exact canonical action ID is not literally known from the prompt, call gitlab_search_tools first.",
		"If the exact required params are not literally known from the prompt, call gitlab_describe_tools before gitlab_execute_tool.",
		"Canonical action IDs use domain.action without the gitlab_ tool prefix",
		"params is always required, even when empty",
		"runner.delete_registered when the prompt gives numeric runner_id",
	})

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic3)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"If the exact canonical action ID is not literally known from the prompt, call gitlab_search_tools before gitlab_execute_tool.",
		"If the exact required params are not literally known, call gitlab_describe_tools before gitlab_execute_tool.",
		"params limited to the described input schema",
		"Always include top-level params",
		"Canonical action IDs do not include gitlab_ prefixes",
		"Never send confirm:false",
	})
}

// TestDynamicTaskPrompt_IncludesProviderConfusionGuidance verifies task-level
// prompts give exact ordering hints for workflows confused by model providers.
func TestDynamicTaskPrompt_IncludesProviderConfusionGuidance(t *testing.T) {
	tests := []struct {
		name string
		task evalTask
		want []string
	}{
		{
			name: "failed pipeline investigation workflow",
			task: evalTask{ID: "MS-002", Prompt: "Investigate failed pipeline `339` for project `my-org/tools/gitlab-mcp-server` and remote URL `http://localhost:8929/my-org/tools/gitlab-mcp-server.git`: resolve the project, inspect the pipeline, list failed jobs, fetch job `677` trace, then call the pipeline failure analyzer for pipeline `339`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "job.list", RequiredParams: []string{"project_id", "pipeline_id"}},
			}},
			want: []string{`"remote_url":"http://localhost:8929/my-org/tools/gitlab-mcp-server.git"`, "discover_project.resolve, pipeline.get, job.list, job.trace, analyze.pipeline_failure", "Inspecting one known pipeline ID means pipeline.get", "do not substitute pipeline.list"},
		},
		{
			name: "settings broadcast workflow",
			task: evalTask{ID: "MS-009", Prompt: "Read current instance settings, create a broadcast message, then delete it.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "admin.settings_get"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "admin.broadcast_message_create"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "admin.broadcast_message_delete"},
			}},
			want: []string{"admin.settings_get, admin.broadcast_message_create, admin.broadcast_message_delete", "not list or create broadcast messages"},
		},
		{
			name: "release cleanup workflow",
			task: evalTask{ID: "MS-004", Prompt: "Verify a tag and release, list release asset links, delete release and tag.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "tag.get"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "release.get"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "release.link_list"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "release.delete"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "tag.delete"},
			}},
			want: []string{"tag.get, release.get, release.link_list, release.delete, tag.delete", "Start with tag.get"},
		},
		{
			name: "release notes workflow",
			task: evalTask{ID: "MS-012", Prompt: "List releases, compare refs, then generate release notes.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "release.list"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.compare"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "analyze.release_notes"},
			}},
			want: []string{"release.list, repository.compare, analyze.release_notes", "same from/to refs"},
		},
		{
			name: "feature flag user list workflow",
			task: evalTask{ID: "MS-029", Prompt: "Exercise feature flag and user-list lifecycle in project `my-org/tools/gitlab-mcp-server`: create feature flag user list `eval-feature-list` with user IDs `u1,u2`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "feature_flags.ff_user_list_create", RequiredParams: []string{"project_id", "name", "user_xids"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "feature_flags.ff_user_list_get", RequiredParams: []string{"project_id", "user_list_iid"}},
			}},
			want: []string{"Dynamic first-step exact call", `"name":"eval-feature-list"`, `"user_xids":"u1,u2"`, "Every step needs params.project_id", "Use the returned iid as params.user_list_iid", "After ff_user_list_update, create the feature flag next", "never feature_flag_name", "never include user_list_iid unless you are calling an ff_user_list_* action"},
		},
		{
			name: "issue time tracking workflow",
			task: evalTask{ID: "MS-032", Prompt: "Exercise issue time tracking in project `my-org/tools/gitlab-mcp-server`: create issue `eval-time-issue`, set estimate `2h`, add spent time `30m`, reset spent time, reset the estimate, then delete the issue.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.time_estimate_set", RequiredParams: []string{"project_id", "issue_iid", "duration"}},
			}},
			want: []string{"issue.create, issue.time_estimate_set, issue.spent_time_add, issue.spent_time_reset, issue.time_estimate_reset, issue.delete", "Set the estimate before adding spent time", "reset spent time before resetting the estimate"},
		},
		{
			name: "issue link workflow",
			task: evalTask{ID: "MS-016", Prompt: "Exercise issue link CRUD in project `my-org/tools/gitlab-mcp-server`: create source issue `eval-link-source`, create target issue `eval-link-target`, link source to target as `relates_to`, list source issue links, delete the returned issue link, then delete both issues.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.link_create", RequiredParams: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.link_list", RequiredParams: []string{"project_id", "issue_iid"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.link_delete", RequiredParams: []string{"project_id", "issue_iid", "issue_link_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{"issue.link_create, not issue.link", "issue.link_delete", "top-level confirm:true on gitlab_execute_tool", "params.issue_iid set to the source issue IID"},
		},
		{
			name: "merge request award workflow",
			task: evalTask{ID: "MS-033", Prompt: "Exercise merge request time tracking and emoji in project `my-org/tools/gitlab-mcp-server`: set estimate `1h` on MR `1`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "merge_request.time_estimate_set", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "merge_request.spent_time_add", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "merge_request.emoji_mr_create", RequiredParams: []string{"project_id", "merge_request_iid", "name"}},
			}},
			want: []string{"merge_request.emoji_mr_list before deleting", `"merge_request_iid":1`, `"duration":"1h"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic3)
			requireContainsAll(t, "taskPromptForSurface()", prompt, tt.want)
		})
	}
}

// TestDynamicSingleTaskPrompt_UsesExactCallForHighRiskShapes verifies
// single-step dynamic tasks with provider params-shape misses get exact calls.
func TestDynamicSingleTaskPrompt_UsesExactCallForHighRiskShapes(t *testing.T) {
	tests := []struct {
		name string
		task evalTask
		want []string
	}{
		{
			name: "repository file get",
			task: evalTask{ID: "MT-029", Prompt: "Get file `README.md` from branch `main` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
			}},
			want: []string{`"action":"repository.file_get"`, `"file_path":"README.md"`, `"ref":"main"`, `"project_id":"my-org/tools/gitlab-mcp-server"`},
		},
		{
			name: "repository file create",
			task: evalTask{ID: "MT-030", Prompt: "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			}},
			want: []string{`"action":"repository.file_create"`, `"file_path":"tmp/eval.txt"`, `"branch":"feature/eval"`, `"content":"evaluation file"`, `"commit_message":"Create evaluation file"`},
		},
		{
			name: "single artifact download",
			task: evalTask{ID: "MT-065", Prompt: "Download artifact `coverage/report.xml` from job `361` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "job.download_single_artifact", RequiredParams: []string{"project_id", "job_id", "artifact_path"}},
			}},
			want: []string{`"action":"job.download_single_artifact"`, `"artifact_path":"coverage/report.xml"`, `"job_id":361`, `"project_id":"my-org/tools/gitlab-mcp-server"`},
		},
		{
			name: "runner remove",
			task: evalTask{ID: "MT-047", Prompt: "Remove runner ID `21`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "runner.remove", RequiredParams: []string{"runner_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"runner.remove"`, `"confirm":true`, `"runner_id":21`, "include top-level confirm:true on gitlab_execute_tool"},
		},
		{
			name: "pipeline schedule delete",
			task: evalTask{ID: "MT-103", Prompt: "Delete pipeline schedule ID `46` from project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "pipeline.schedule_delete", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"pipeline.schedule_delete"`, `"confirm":true`, `"params":{"project_id":"my-org/tools/gitlab-mcp-server","schedule_id":46}`, "include top-level confirm:true on gitlab_execute_tool", "only action and confirm is invalid"},
		},
		{
			name: "user block",
			task: evalTask{ID: "MT-104", Prompt: "Block user ID `55`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "user.block", RequiredParams: []string{"user_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"user.block"`, `"confirm":true`, `"user_id":55`, "include top-level confirm:true on gitlab_execute_tool"},
		},
		{
			name: "pipeline trigger delete",
			task: evalTask{ID: "MT-102", Prompt: "Delete pipeline trigger token ID `53` from project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "pipeline.trigger_delete", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"pipeline.trigger_delete"`, `"confirm":true`, `"params":{"project_id":"my-org/tools/gitlab-mcp-server","trigger_id":53}`, "only action and confirm is invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic3)
			if strings.Contains(prompt, "confirm:true in params") {
				t.Fatalf("taskPromptForSurface() = %q, dynamic prompt must not tell models to put confirm in params", prompt)
			}
			exactIndex := strings.Index(prompt, tt.want[0])
			taskIndex := strings.Index(prompt, "Task "+tt.task.ID)
			overrideIndex := strings.Index(prompt, "Dynamic mode override:")
			if exactIndex < 0 {
				t.Fatalf("taskPromptForSurface() = %q, want exact call containing %q", prompt, tt.want[0])
			}
			if taskIndex >= 0 && exactIndex > taskIndex {
				t.Fatalf("taskPromptForSurface() = %q, exact call must appear before task prose", prompt)
			}
			if overrideIndex >= 0 && exactIndex > overrideIndex {
				t.Fatalf("taskPromptForSurface() = %q, exact call must appear before dynamic override", prompt)
			}
			requireContainsAll(t, "taskPromptForSurface()", prompt, tt.want)
		})
	}
}

// TestDynamicRepositoryFileCRUDPrompt_UsesFilePathFromOperation verifies dynamic
// file CRUD guidance extracts the repository file path instead of the project path.
func TestDynamicRepositoryFileCRUDPrompt_UsesFilePathFromOperation(t *testing.T) {
	task := evalTask{ID: "MS-017", Prompt: "Exercise repository file CRUD in project `my-org/tools/gitlab-mcp-server`: create file `tmp/eval-crud.txt` on branch `feature/eval`, read it, update its content, then delete it from the same branch.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
		{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic3)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		`"action":"repository.file_create"`,
		`"file_path":"tmp/eval-crud.txt"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"branch":"feature/eval"`,
		`"content":"Initial content for repository file CRUD"`,
		`"commit_message":"Evaluation create tmp/eval-crud.txt"`,
	})
	if strings.Contains(prompt, `"file_path":"my-org/tools/gitlab-mcp-server"`) {
		t.Fatalf("taskPromptForSurface() = %q, file_path must not use project path", prompt)
	}
}

// TestCanExecuteInvalidToolCallSkipsWrongDynamicReadOnlyAction verifies dynamic
// workflows receive exact repair guidance when the model substitutes a read-only action.
func TestCanExecuteInvalidToolCallSkipsWrongDynamicReadOnlyAction(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}}
	validation := validationResult{ToolMatches: true, ActionMatches: false, Action: "pipeline.list", RequiredPresent: false, DestructiveSafe: true, Message: "expected action pipeline.get, got pipeline.list; missing required params.pipeline_id"}
	toolUse := modelContentBlock{Name: dynamicExecuteTool, Input: map[string]any{"action": "pipeline.list", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}}
	routes := map[string]toolutil.ActionMap{dynamicExecuteTool: {"pipeline.list": toolutil.ActionRoute{}}}

	if runner.canExecuteInvalidToolCall(step, validation, toolUse, routes) {
		t.Fatal("canExecuteInvalidToolCall() = true, want wrong dynamic read-only action to receive exact repair guidance")
	}
}

// TestNormalizeTasksForDynamicRoutes_RewritesActionSteps verifies fixture
// expectations are mapped onto gitlab_execute_tool action IDs.
func TestNormalizeTasksForDynamicRoutes_RewritesActionSteps(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
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
	if normalized[0].ExpectedTool != dynamicExecuteTool || normalized[0].ExpectedAction != "project.get" {
		t.Fatalf("top-level expectation = %s/%s", normalized[0].ExpectedTool, normalized[0].ExpectedAction)
	}
	if normalized[0].Steps[0].ExpectedTool != dynamicExecuteTool || normalized[0].Steps[0].ExpectedAction != "server.health_check" {
		t.Fatalf("first step = %+v", normalized[0].Steps[0])
	}
	if normalized[0].Steps[1].ExpectedTool != dynamicExecuteTool || normalized[0].Steps[1].ExpectedAction != "repository.file_get" {
		t.Fatalf("second step = %+v", normalized[0].Steps[1])
	}
}

// TestValidateActionToolCall_DynamicConfirmTopLevel verifies destructive
// dynamic execution accepts confirm at the gitlab_execute_tool top level.
func TestValidateActionToolCall_DynamicConfirmTopLevel(t *testing.T) {
	step := evalStep{
		ExpectedTool:   dynamicExecuteTool,
		ExpectedAction: "project.delete",
		RequiredParams: []string{"project_id"},
		Destructive:    true,
	}

	valid := validateActionToolCall(step, dynamicExecuteTool, map[string]any{
		"action":  "project.delete",
		"params":  map[string]any{"project_id": "my-org/project"},
		"confirm": true,
	})
	if !valid.Valid || !valid.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic top-level confirm) = %+v, want valid safe", valid)
	}

	invalid := validateActionToolCall(step, dynamicExecuteTool, map[string]any{
		"action": "project.delete",
		"params": map[string]any{"project_id": "my-org/project"},
	})
	if invalid.Valid || invalid.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic missing confirm) = %+v, want unsafe invalid", invalid)
	}

	paramsConfirm := validateActionToolCall(step, dynamicExecuteTool, map[string]any{
		"action": "project.delete",
		"params": map[string]any{"project_id": "my-org/project", "confirm": true},
	})
	if paramsConfirm.Valid || paramsConfirm.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic params confirm) = %+v, want unsafe invalid", paramsConfirm)
	}
}

// TestDynamicDiscoveryResult_SearchAndDescribe verifies dynamic discovery
// tools return enough action metadata for the next execute call.
func TestDynamicDiscoveryResult_SearchAndDescribe(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"project.get": {InputSchema: map[string]any{
				"type":       "object",
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
			}},
		},
	}

	search, err := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{Name: dynamicSearchTool, Input: map[string]any{"query": "project get"}})
	if err != nil {
		t.Fatalf("dynamicDiscoveryResult(search) error = %v", err)
	}
	if !strings.Contains(search, "project.get") {
		t.Fatalf("search result = %s, want project.get", search)
	}
	describe, err := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{Name: dynamicDescribeTool, Input: map[string]any{"action": "project.get"}})
	if err != nil {
		t.Fatalf("dynamicDiscoveryResult(describe) error = %v", err)
	}
	for _, want := range []string{"project.get", "project_id", dynamicExecuteTool} {
		if !strings.Contains(describe, want) {
			t.Fatalf("describe result = %s, want %q", describe, want)
		}
	}
}

// TestSuccessfulSimulatedToolContent_IncludesCreatedResourceIDs verifies that
// simulated mutating responses include resource IDs for follow-up evaluation steps.
func TestSuccessfulSimulatedToolContent_IncludesCreatedResourceIDs(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "project.badge_add"}, modelContentBlock{
		Name: dynamicExecuteTool,
		Input: map[string]any{
			"action": "project.badge_add",
			"params": map[string]any{"project_id": "my-org/project"},
		},
	}, 2, 4)

	if !strings.Contains(content, `"badge_id":102`) || !strings.Contains(content, `"id":102`) {
		t.Fatalf("successfulSimulatedToolContent() = %s, want badge id fields", content)
	}

	content = successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "mr_review.note_create"}, modelContentBlock{
		Name: dynamicExecuteTool,
		Input: map[string]any{
			"action": "merge_request_note.create",
			"params": map[string]any{"project_id": "my-org/project", "merge_request_iid": float64(7)},
		},
	}, 2, 4)
	if !strings.Contains(content, `"note_id":104`) {
		t.Fatalf("successfulSimulatedToolContent(alias) = %s, want note_id", content)
	}
}

// newEvalTestClient is an internal helper for the main package.
func newEvalTestClient(t *testing.T, enterprise bool) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	}))
	t.Cleanup(srv.Close)
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       srv.URL,
		GitLabToken:     "eval-token",
		Enterprise:      enterprise,
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// TestValidateTaskFixture_RequiresProjectGrounding verifies that ValidateTaskFixture handles the requires project grounding scenario correctly.
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

// TestValidateTaskFixture_AcceptsGroundedProject verifies that ValidateTaskFixture handles the accepts grounded project scenario correctly.
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

// TestValidateTaskFixtureAgainstRoutes_CatchesDestructiveMismatch verifies that ValidateTaskFixtureAgainstRoutes handles the catches destructive mismatch scenario correctly.
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

// TestValidateTaskFixtureAgainstRoutes_CatchesUnknownFixtureParam verifies that ValidateTaskFixtureAgainstRoutes handles the catches unknown fixture param scenario correctly.
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

// TestValidateToolCall_RequiresNestedParams verifies that ValidateToolCall handles the requires nested params scenario correctly.
func TestValidateToolCall_RequiresNestedParams(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	result := validateToolCall(task, "gitlab_issue", map[string]any{
		"action":     "delete",
		"project_id": "42",
	})
	if result.Valid {
		t.Fatal("validateToolCall() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "unexpected top-level parameter project_id") {
		t.Fatalf("message = %q, want top-level parameter guidance", result.Message)
	}
}

// TestValidateToolCall_AcceptsConfirmedDestructiveCall verifies that ValidateToolCall handles the accepts confirmed destructive call scenario correctly.
func TestValidateToolCall_AcceptsConfirmedDestructiveCall(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	result := validateToolCall(task, "gitlab_issue", map[string]any{
		"action": "delete",
		"params": map[string]any{
			"project_id": "42",
			"issue_iid":  7,
			"confirm":    true,
		},
	})
	if !result.Valid {
		t.Fatalf("validateToolCall() Valid = false: %s", result.Message)
	}
	if !result.DestructiveSafe {
		t.Fatal("DestructiveSafe = false, want true")
	}
}

// TestValidateToolCall_DoesNotRequireConfirmForWrongReadOnlyAttempt verifies that ValidateToolCall handles the does not require confirm for wrong read only attempt scenario correctly.
func TestValidateToolCall_DoesNotRequireConfirmForWrongReadOnlyAttempt(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_repository", ExpectedAction: "file_delete", RequiredParams: []string{"project_id", "file_path", "branch"}, Destructive: true}
	result := validateToolCall(task, "gitlab_repository", map[string]any{
		"action": "file_metadata",
		"params": map[string]any{
			"project_id": "42",
			"file_path":  "README.md",
			"ref":        "main",
		},
	})
	if result.Valid {
		t.Fatal("validateToolCall() Valid = true, want false")
	}
	if !result.DestructiveSafe {
		t.Fatal("DestructiveSafe = false for a wrong read-only attempt, want true")
	}
}

// TestValidateToolCall_AcceptsAddLabelsForLabelRequirement verifies that ValidateToolCall handles the accepts add labels for label requirement scenario correctly.
func TestValidateToolCall_AcceptsAddLabelsForLabelRequirement(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab", ExpectedAction: "issue.update", RequiredParams: []string{"project_id", "issue_iid", "labels"}}
	result := validateToolCall(task, "gitlab", map[string]any{
		"action": "issue.update",
		"params": map[string]any{
			"project_id": "my-org/tools/gitlab-mcp-server",
			"issue_iid":  77,
			"add_labels": "evaluation",
		},
	})
	if !result.Valid {
		t.Fatalf("validateToolCall() Valid = false: %s", result.Message)
	}
}

// TestValidateStepCallWithRoutes_RejectsUnknownParamsFromSchema verifies that ValidateStepCallWithRoutes handles the rejects unknown params from schema scenario correctly.
func TestValidateStepCallWithRoutes_RejectsUnknownParamsFromSchema(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
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
	result := validateStepCallWithRoutes(step, "gitlab_project", map[string]any{
		"action": "get",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "iid": 7},
	}, routes)
	if result.Valid {
		t.Fatal("validateStepCallWithRoutes() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "unknown params") || !strings.Contains(result.Message, "iid") {
		t.Fatalf("message = %q, want unknown params iid", result.Message)
	}
}

// TestValidateStepCallWithRoutes_AcceptsActionAlias verifies that ValidateStepCallWithRoutes handles the accepts action alias scenario correctly.
func TestValidateStepCallWithRoutes_AcceptsActionAlias(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "project.milestone_create", RequiredParams: []string{"project_id", "title"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"project.milestone_create": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"title":      map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, "gitlab", map[string]any{
		"action": "milestone.create",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "title": "Evaluation Sprint"},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
	if result.Action != "project.milestone_create" {
		t.Fatalf("Action = %q, want project.milestone_create", result.Action)
	}
}

// TestValidateStepCallWithRoutes_AcceptsDynamicActionScopedAliases verifies
// that dynamic eval validation uses the same action-scoped param compatibility
// as the runtime dynamic executor.
func TestValidateStepCallWithRoutes_AcceptsDynamicActionScopedAliases(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "group.group_label_update", RequiredParams: []string{"group_id", "label_id"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"group.group_label_update": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "string"},
					"label_id": map[string]any{"type": "integer"},
					"new_name": map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteTool, map[string]any{
		"action": "group.group_label_update",
		"params": map[string]any{"group_id": "my-org", "label_id": 35, "name": "next-label"},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
}

// TestValidateStepCallWithRoutes_AcceptsDynamicCompatibilityAliases verifies
// the eval validator recognizes dynamic aliases accepted by gitlab_execute_tool.
func TestValidateStepCallWithRoutes_AcceptsDynamicCompatibilityAliases(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "repository.tree", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"repository.tree": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"ref":        map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteTool, map[string]any{
		"action": "repository_tree.list",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "ref": "main"},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
	if result.Action != "repository.tree" {
		t.Fatalf("Action = %q, want repository.tree", result.Action)
	}
}

// TestValidateStepCallWithRoutes_AcceptsDynamicOnlyAliases verifies the eval
// validator shares Dynamic runtime compatibility aliases without adding them to
// the meta-tool alias table.
func TestValidateStepCallWithRoutes_AcceptsDynamicOnlyAliases(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.update", RequiredParams: []string{"project_id", "issue_iid"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"issue.update": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string"},
					"issue_iid":   map[string]any{"type": "integer"},
					"state_event": map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteTool, map[string]any{
		"action": "issue.close",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "issue_iid": 1},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
	if result.Action != "issue.update" {
		t.Fatalf("Action = %q, want issue.update", result.Action)
	}
}

// TestValidateStepCallWithRoutes_AcceptsNestedDynamicParamNormalization verifies
// nested action-scoped normalization runs before schema validation.
func TestValidateStepCallWithRoutes_AcceptsNestedDynamicParamNormalization(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "snippet.project_create", RequiredParams: []string{"project_id", "title"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"snippet.project_create": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"title":      map[string]any{"type": "string"},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"file_path": map[string]any{"type": "string"},
								"content":   map[string]any{"type": "string"},
							},
						},
					},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteTool, map[string]any{
		"action": "snippet.project_create",
		"params": map[string]any{
			"project_id": "my-org/tools/gitlab-mcp-server",
			"title":      "snippet",
			"files": []any{map[string]any{
				"action":    "create",
				"file_path": "snippet.md",
				"content":   "body",
			}},
		},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
}

// TestValidateStepCallWithRoutes_ValidatesRequiredParamsBeforeDynamicNormalization
// verifies exact-call task requirements are checked against the model input
// even when dynamic execute normalizes those params for schema validation.
func TestValidateStepCallWithRoutes_ValidatesRequiredParamsBeforeDynamicNormalization(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "snippet.project_create", RequiredParams: []string{"project_id", "title", "file_name", "content"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteTool: {
			"snippet.project_create": toolutil.ActionRoute{InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "title", "files"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"title":      map[string]any{"type": "string"},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":     "object",
							"required": []any{"file_path", "content"},
							"properties": map[string]any{
								"file_path": map[string]any{"type": "string"},
								"content":   map[string]any{"type": "string"},
							},
						},
					},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteTool, map[string]any{
		"action": "snippet.project_create",
		"params": map[string]any{
			"project_id": "my-org/tools/gitlab-mcp-server",
			"title":      "snippet",
			"file_name":  "snippet.md",
			"content":    "body",
		},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
}

// TestValidationRepairMessage_IncludesActionEnvelopeAndProjectHint verifies that ValidationRepairMessage handles the includes action envelope and project hint scenario correctly.
func TestValidationRepairMessage_IncludesActionEnvelopeAndProjectHint(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}
	task := evalTask{Prompt: "Fetch project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "missing required params.project_id"}, nil)
	if !strings.Contains(message, `"action":"project.get"`) || !strings.Contains(message, "project_id") {
		t.Fatalf("message = %q, want action envelope example", message)
	}
	if !strings.Contains(message, `"project_id":"my-org/tools/gitlab-mcp-server"`) {
		t.Fatalf("message = %q, want concrete project_id value", message)
	}
	if !strings.Contains(message, "previous tool result") || !strings.Contains(message, "params.project_id") {
		t.Fatalf("message = %q, want previous-result project_id hint", message)
	}
}

// TestValidationRepairMessage_DestructiveEnvelopeIncludesConfirm verifies that ValidationRepairMessage handles the destructive envelope includes confirm scenario correctly.
func TestValidationRepairMessage_DestructiveEnvelopeIncludesConfirm(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_branch", ExpectedAction: "delete", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true}
	task := evalTask{Prompt: "Delete branch `obsolete/eval` from project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "destructive task requires params.confirm=true"}, nil)
	if !strings.Contains(message, `"confirm":true`) {
		t.Fatalf("message = %q, want confirm inside retry envelope", message)
	}
}

// TestValidationRepairMessage_DynamicWrongActionIncludesOrderingHint verifies
// dynamic repair feedback steers models back to the current scenario step.
func TestValidationRepairMessage_DynamicWrongActionIncludesOrderingHint(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "admin.settings_get"}
	message := validationRepairMessage(evalTask{}, step, validationResult{Message: "step 1: expected action admin.settings_get, got admin.broadcast_message_list", Action: "admin.broadcast_message_list"}, nil)
	for _, want := range []string{
		`"action":"admin.settings_get"`,
		`"params":{}`,
		"without gitlab_ prefixes",
		"not the current scenario step",
		"do not skip ahead",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
}

// TestValidationRepairMessage_UnknownParamsDropsCarriedFields verifies repair
// feedback tells models to remove fields copied from previous workflow steps.
func TestValidationRepairMessage_UnknownParamsDropsCarriedFields(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "feature_flags.feature_flag_create", RequiredParams: []string{"project_id", "name", "version"}}
	task := evalTask{Prompt: "Create feature flag `eval_flag` in project `my-org/tools/gitlab-mcp-server` version `new_version_flag`."}
	message := validationRepairMessage(task, step, validationResult{Message: "unknown params for gitlab_execute_tool/feature_flags.feature_flag_create: user_list_iid"}, nil)
	for _, want := range []string{
		`"action":"feature_flags.feature_flag_create"`,
		"Remove every unknown param",
		"do not carry IDs from a previous action",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
}

// TestValidationRepairMessage_PreservesAttemptedRequiredParams verifies repair
// examples keep IDs the model already copied from a prior tool result.
func TestValidationRepairMessage_PreservesAttemptedRequiredParams(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "pipeline.trigger_get", RequiredParams: []string{"project_id", "trigger_id"}}
	task := evalTask{Prompt: "Fetch pipeline trigger using the returned trigger ID in project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "missing required params.project_id"}, map[string]any{
		"action": "pipeline.trigger_get",
		"params": map[string]any{"trigger_id": 67},
	})

	for _, want := range []string{`"action":"pipeline.trigger_get"`, `"project_id":"my-org/tools/gitlab-mcp-server"`, `"trigger_id":67`} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
}

// TestValidateStepCallWithRoutes_RejectsMissingNestedSchemaRequiredParam verifies that ValidateStepCallWithRoutes handles the rejects missing nested schema required param scenario correctly.
func TestValidateStepCallWithRoutes_RejectsMissingNestedSchemaRequiredParam(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "snippet.project_update", RequiredParams: []string{"project_id", "snippet_id", "files"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"snippet.project_update": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"snippet_id": map[string]any{"type": "integer"},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":     "object",
							"required": []any{"action", "file_path"},
							"properties": map[string]any{
								"action":        map[string]any{"type": "string"},
								"content":       map[string]any{"type": "string"},
								"file_path":     map[string]any{"type": "string"},
								"previous_path": map[string]any{"type": "string"},
							},
						},
					},
				},
			}},
		},
	}
	input := map[string]any{"action": "snippet.project_update", "params": map[string]any{
		"project_id": "my-org/tools/gitlab-mcp-server",
		"snippet_id": float64(28),
		"files": []any{map[string]any{
			"action":        "update",
			"content":       "updated",
			"previous_path": "eval-crud-snippet",
		}},
	}}

	result := validateStepCallWithRoutes(step, "gitlab", input, routes)
	if result.Valid {
		t.Fatal("validateStepCallWithRoutes() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "files[0].file_path") {
		t.Fatalf("message = %q, want nested missing file_path", result.Message)
	}
}

// TestValidateStandaloneToolCall_AcceptsTopLevelInput verifies that ValidateStandaloneToolCall handles the accepts top level input scenario correctly.
func TestValidateStandaloneToolCall_AcceptsTopLevelInput(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"remote_url": "https://gitlab.example.com/my-org/project.git",
	})
	if !result.Valid {
		t.Fatalf("validateStepCall() Valid = false: %s", result.Message)
	}
}

// TestValidateStandaloneToolCall_RejectsMetaEnvelope verifies that ValidateStandaloneToolCall handles the rejects meta envelope scenario correctly.
func TestValidateStandaloneToolCall_RejectsMetaEnvelope(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"action": "resolve",
		"params": map[string]any{"remote_url": "https://gitlab.example.com/my-org/project.git"},
	})
	if result.Valid {
		t.Fatal("validateStepCall() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "standalone tool") {
		t.Fatalf("message = %q, want standalone guidance", result.Message)
	}
}

// TestRunStaticValidation_ValidatesMultiStepRoutes verifies that RunStaticValidation handles the validates multi step routes scenario correctly.
func TestRunStaticValidation_ValidatesMultiStepRoutes(t *testing.T) {
	tasks := []evalTask{{
		ID: "MS-001",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_discover_project"},
			{ExpectedTool: "gitlab_project", ExpectedAction: "get"},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get"},
		},
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project":    {"get": {}},
		"gitlab_repository": {"file_get": {}},
	}
	toolNames := map[string]bool{"gitlab_discover_project": true, "gitlab_project": true, "gitlab_repository": true}
	results := runStaticValidation(tasks, routes, toolNames, 1)
	if len(results) != 1 || !results[0].FinalSuccess || results[0].CompletedSteps != 3 {
		t.Fatalf("results = %+v, want completed multi-step validation", results)
	}
}

// TestLoadToolsSnapshot_DerivesRoutes verifies that LoadToolsSnapshot handles the derives routes scenario correctly.
func TestLoadToolsSnapshot_DerivesRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.json")
	snapshot := `[
  {
    "name": "gitlab_project",
    "description": "Manage projects.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "action": {"type": "string", "enum": ["get", "list"]},
        "params": {"type": "object"}
      }
    }
  }
]`
	if err := os.WriteFile(path, []byte(snapshot), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	tools, routes, err := loadToolsSnapshot(path)
	if err != nil {
		t.Fatalf("loadToolsSnapshot() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gitlab_project" {
		t.Fatalf("tools = %+v, want gitlab_project", tools)
	}
	if _, ok := routes["gitlab_project"]["get"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/get", routes)
	}
	if _, ok := routes["gitlab_project"]["list"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/list", routes)
	}
}

// TestSchemaLookupResult_IndexAndActionSchema verifies that SchemaLookupResult handles the index and action schema scenario correctly.
func TestSchemaLookupResult_IndexAndActionSchema(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"delete": toolutil.ActionRoute{Destructive: true, InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
				},
			}},
		},
	}
	indexPayload, err := schemaLookupResult(routes, map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_project"}})
	if err != nil {
		t.Fatalf("schemaLookupResult(index) error = %v", err)
	}
	if !strings.Contains(indexPayload, "gitlab://schema/meta/gitlab_project/delete") {
		t.Fatalf("index payload = %s, want schema URI", indexPayload)
	}
	schemaPayload, err := schemaLookupResult(routes, map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "delete"}})
	if err != nil {
		t.Fatalf("schemaLookupResult(schema) error = %v", err)
	}
	if !strings.Contains(schemaPayload, "\"confirm\"") || !strings.Contains(schemaPayload, "\"x_destructive\":true") {
		t.Fatalf("schema payload = %s, want destructive confirmation metadata", schemaPayload)
	}
}

// TestSchemaLookupResult_UnknownToolReturnsError verifies that SchemaLookupResult handles the unknown tool returns error scenario correctly.
func TestSchemaLookupResult_UnknownToolReturnsError(t *testing.T) {
	_, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_missing"}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("error = %v, want unknown tool", err)
	}
}

// TestSchemaLookupResult_MissingToolReturnsUsageExamples verifies that SchemaLookupResult handles the missing tool returns usage examples scenario correctly.
func TestSchemaLookupResult_MissingToolReturnsUsageExamples(t *testing.T) {
	payload, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_get", "params": map[string]any{}})
	if err != nil {
		t.Fatalf("schemaLookupResult() error = %v, want usage payload", err)
	}
	if !strings.Contains(payload, `"action":"schema_get"`) || !strings.Contains(payload, `"tool":"gitlab"`) || !strings.Contains(payload, "pipeline.get") {
		t.Fatalf("payload = %s, want schema_get usage examples", payload)
	}
}

// TestSuccessfulSimulatedToolContent_IncludesDiscoveredProject verifies that SuccessfulSimulatedToolContent handles the includes discovered project scenario correctly.
func TestSuccessfulSimulatedToolContent_IncludesDiscoveredProject(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{}, modelContentBlock{
		Name:  "gitlab_discover_project",
		Input: map[string]any{"remote_url": "https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git"},
	}, 2, 3)
	if !strings.Contains(content, "my-org/tools/gitlab-mcp-server") || !strings.Contains(content, "default_branch") {
		t.Fatalf("successfulSimulatedToolContent() = %s, want project metadata", content)
	}

	content = successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "discover_project.resolve"}, modelContentBlock{
		Name: dynamicExecuteTool,
		Input: map[string]any{
			"action": "search.projects",
			"params": map[string]any{"search": "gitlab-mcp-server"},
		},
	}, 2, 3)
	for _, want := range []string{"my-org/tools/gitlab-mcp-server", `"projects"`, `"environments"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("successfulSimulatedToolContent(prelude) = %s, want %q", content, want)
		}
	}
}

// TestTaskPrompt_ClarifiesTransientRetry verifies that TaskPrompt handles the clarifies transient retry scenario correctly.
func TestTaskPrompt_ClarifiesTransientRetry(t *testing.T) {
	task := evalTask{
		ID:             "MF-001",
		Prompt:         "Inspect pipeline `12345`, retrying once if GitLab temporarily returns a server error.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.get",
		Simulation:     "transient_error_once",
	}
	prompt := taskPrompt(task)
	if !strings.Contains(prompt, "repeat the same validated operation once") {
		t.Fatalf("taskPrompt() = %q, want transient retry guidance", prompt)
	}
	if !strings.Contains(prompt, "do not use GitLab CI retry actions") {
		t.Fatalf("taskPrompt() = %q, want CI retry disambiguation", prompt)
	}
}

// TestTaskPrompt_SingleOperationPrefersOneClearToolCall verifies that TaskPrompt handles the single operation prefers one clear tool call scenario correctly.
func TestTaskPrompt_SingleOperationPrefersOneClearToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-003",
		Prompt:         "List the 10 most recently updated projects I can access.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "project.list",
	}
	prompt := taskPrompt(task)
	if !strings.Contains(prompt, "exactly one tool call") {
		t.Fatalf("taskPrompt() = %q, want one-tool guidance", prompt)
	}
	if !strings.Contains(prompt, "A schema lookup before the task call is a failure") {
		t.Fatalf("taskPrompt() = %q, want constrained schema lookup guidance", prompt)
	}
	if !strings.Contains(prompt, "Do not look up schemas for ordinary parameter names already supplied by the task prompt") {
		t.Fatalf("taskPrompt() = %q, want ordinary-param no-lookup guidance", prompt)
	}
	if !strings.Contains(prompt, "do not add any params that the task did not ask for") {
		t.Fatalf("taskPrompt() = %q, want no-extra-param guidance", prompt)
	}
	if !strings.Contains(prompt, "Use gitlab_interactive_* only if this task explicitly asks for a guided interactive flow") {
		t.Fatalf("taskPrompt() = %q, want interactive tool disambiguation", prompt)
	}
	if !strings.Contains(prompt, "A value like group/project is params.project_id, not remote_url") {
		t.Fatalf("taskPrompt() = %q, want project path vs remote URL guidance", prompt)
	}
	if !strings.Contains(prompt, "never call gitlab without an input object containing action and params") {
		t.Fatalf("taskPrompt() = %q, want non-empty dispatcher input guidance", prompt)
	}
	if !strings.Contains(prompt, "server diagnostics or a GitLab connectivity check, call gitlab_server with action health_check") {
		t.Fatalf("taskPrompt() = %q, want health_check standalone guidance", prompt)
	}
	if !strings.Contains(prompt, "For subgroup creation with group.create, use params.name, params.path, and params.parent_id") {
		t.Fatalf("taskPrompt() = %q, want subgroup create guidance", prompt)
	}
	if !strings.Contains(prompt, "For merge request creation, from is params.source_branch, into is params.target_branch, and titled is params.title") {
		t.Fatalf("taskPrompt() = %q, want merge request create guidance", prompt)
	}
	if !strings.Contains(prompt, "For merge request notes or comments, use mr_review.note_create") || !strings.Contains(prompt, "Use mr_review.discussion_create only when the task explicitly asks for a threaded discussion or discussion") {
		t.Fatalf("taskPrompt() = %q, want merge request note/discussion guidance", prompt)
	}
	if !strings.Contains(prompt, "For personal snippets, snippet ID is params.snippet_id") || !strings.Contains(prompt, "or file_path") {
		t.Fatalf("taskPrompt() = %q, want snippet_id guidance", prompt)
	}
	if !strings.Contains(prompt, "For custom emoji group operations, use custom_emoji.list with params.group_path") {
		t.Fatalf("taskPrompt() = %q, want custom emoji group_path guidance", prompt)
	}
	if !strings.Contains(prompt, "For project access tokens, scope names go in params.scopes as an array") {
		t.Fatalf("taskPrompt() = %q, want access token scopes guidance", prompt)
	}
	if !strings.Contains(prompt, "expiring dates go in params.expires_at") {
		t.Fatalf("taskPrompt() = %q, want access token expiration guidance", prompt)
	}
	if !strings.Contains(prompt, "For broadcast messages, saying maps to params.message") {
		t.Fatalf("taskPrompt() = %q, want broadcast message guidance", prompt)
	}
	if !strings.Contains(prompt, "For job.play variables, use params.variables as an array") {
		t.Fatalf("taskPrompt() = %q, want job.play variables guidance", prompt)
	}
	if !strings.Contains(prompt, "For project CI variables in a project, use ci_variable.list/get/create/update/delete with params.project_id") || !strings.Contains(prompt, "for group CI variables, use ci_variable.group_list/group_get/group_create/group_update/group_delete with params.group_id") || !strings.Contains(prompt, "use ci_variable.instance_* only for instance-level variables when no project_id or group_id is supplied") {
		t.Fatalf("taskPrompt() = %q, want project/group/instance CI variable action guidance", prompt)
	}
	if !strings.Contains(prompt, "For runner.list_project, use params.project_id by default") || !strings.Contains(prompt, "Do not send params.paused, params.type, params.tag_list") {
		t.Fatalf("taskPrompt() = %q, want runner list filter guidance", prompt)
	}
	if !strings.Contains(prompt, "For repository file create/update/delete, use params.branch, params.file_path, and params.commit_message") {
		t.Fatalf("taskPrompt() = %q, want repository file write guidance", prompt)
	}
	if !strings.Contains(prompt, "For CI variables, variable name maps to params.key, value maps to params.value, and environment_scope or production scope maps to params.environment_scope") {
		t.Fatalf("taskPrompt() = %q, want CI variable field mapping guidance", prompt)
	}
	if !strings.Contains(prompt, "linking to a URL means params.link_url and image means params.image_url") {
		t.Fatalf("taskPrompt() = %q, want badge field mapping guidance", prompt)
	}
	if !strings.Contains(prompt, "latest pipelines plural means pipeline.list") {
		t.Fatalf("taskPrompt() = %q, want pipeline plural disambiguation", prompt)
	}
	if !strings.Contains(prompt, "do not send empty arrays or objects") {
		t.Fatalf("taskPrompt() = %q, want empty optional guidance", prompt)
	}
	if !strings.Contains(prompt, "call the selected action with params:{}") {
		t.Fatalf("taskPrompt() = %q, want no-parameter action guidance", prompt)
	}
}

// TestTaskPrompt_MultiStepAvoidsImplicitPagination verifies that TaskPrompt handles the multi step avoids implicit pagination scenario correctly.
func TestTaskPrompt_MultiStepAvoidsImplicitPagination(t *testing.T) {
	task := evalTask{
		ID:     "MS-037",
		Prompt: "Build a broad read-only Docker inventory for project `my-org/tools/gitlab-mcp-server`: list project CI variables, list deploy keys, then list generic packages.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_ci_variable", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_key_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_package", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"one successful list response completes a list step",
		"do not fetch additional pagination pages",
		"unless the task explicitly asks for every page",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pagination guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_BroadInventoryUsesExactOrderAndSmallPages verifies that TaskPrompt handles the broad inventory uses exact order and small pages scenario correctly.
func TestTaskPrompt_BroadInventoryUsesExactOrderAndSmallPages(t *testing.T) {
	task := evalTask{
		ID:     "MS-037",
		Prompt: "Build a broad read-only Docker inventory for project `my-org/tools/gitlab-mcp-server`: get the project, list branches, list tags, list releases, list the repository tree at `main`, list project CI variables, list deploy keys, list deploy tokens, then list generic packages.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_branch", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_tag", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_release", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "tree", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_ci_variable", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_key_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_package", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order",
		"gitlab_release/list before repository tree",
		"call repository tree with params.ref=\"main\"",
		"Use params.per_page=1 on list/tree/package steps",
		"one page is enough for this evaluation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want broad inventory guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_MergeRequestTimeEmojiUsesExactOrder verifies that TaskPrompt handles the merge request time emoji uses exact order scenario correctly.
func TestTaskPrompt_MergeRequestTimeEmojiUsesExactOrder(t *testing.T) {
	task := evalTask{
		ID:     "MS-033",
		Prompt: "Exercise merge request time tracking and emoji in project `my-org/tools/gitlab-mcp-server`: set estimate `1h` on MR `7`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "time_estimate_set", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "spent_time_add", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_create", RequiredParams: []string{"project_id", "merge_request_iid", "name"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_list", RequiredParams: []string{"project_id", "merge_request_iid"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_delete", RequiredParams: []string{"project_id", "merge_request_iid", "award_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "spent_time_reset", RequiredParams: []string{"project_id", "merge_request_iid"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "time_estimate_reset", RequiredParams: []string{"project_id", "merge_request_iid"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order: time_estimate_set, spent_time_add, emoji_mr_create, emoji_mr_list, emoji_mr_delete, spent_time_reset, time_estimate_reset",
		"After emoji_mr_create, call emoji_mr_list next",
		"using the returned award emoji id as params.award_id with params.confirm=true",
		"After emoji_mr_delete, call spent_time_reset before time_estimate_reset",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR time/emoji guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_MergeRequestNoteCRUDUsesExactOrder verifies that TaskPrompt handles the merge request note c r u d uses exact order scenario correctly.
func TestTaskPrompt_MergeRequestNoteCRUDUsesExactOrder(t *testing.T) {
	task := evalTask{
		ID:     "MS-027",
		Prompt: "Exercise merge request note CRUD in project `my-org/tools/gitlab-mcp-server`: add note `eval-mr-note` to MR `7`, fetch the created note using the returned note ID, update it to `eval-mr-note-updated`, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_create", RequiredParams: []string{"project_id", "merge_request_iid", "body"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_get", RequiredParams: []string{"project_id", "merge_request_iid", "note_id"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_update", RequiredParams: []string{"project_id", "merge_request_iid", "note_id", "body"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_delete", RequiredParams: []string{"project_id", "merge_request_iid", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order: note_create, note_get, note_update, note_delete",
		"After note_create, call note_get next",
		"call note_update with params.body set to the updated note text and without params.confirm",
		"Only note_delete is destructive; call note_delete last with params.confirm=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR note CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_MergeRequestNotePrefersNoteCreate verifies that TaskPrompt handles the merge request note prefers note create scenario correctly.
func TestTaskPrompt_MergeRequestNotePrefersNoteCreate(t *testing.T) {
	task := evalTask{
		ID:             "MT-016",
		Prompt:         "Add a note saying `Can we add coverage?` to merge request `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_mr_review",
		ExpectedAction: "note_create",
		RequiredParams: []string{"project_id", "merge_request_iid", "body"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call gitlab_mr_review with {"action":"note_create"`,
		`"merge_request_iid":<merge_request_iid>`,
		`"body":"<body>"`,
		"Do not use discussion_create unless the task explicitly says threaded discussion or discussion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR note guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_RunnerListProjectAvoidsImplicitFilters verifies that TaskPrompt handles the runner list project avoids implicit filters scenario correctly.
func TestTaskPrompt_RunnerListProjectAvoidsImplicitFilters(t *testing.T) {
	task := evalTask{
		ID:     "MS-008",
		Prompt: "Troubleshoot runner ID `99` for project `my-org/tools/gitlab-mcp-server`: list project runners, inspect runner jobs, fetch trace for job `999`, then set paused=true on the runner.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_runner", ExpectedAction: "list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_runner", ExpectedAction: "jobs", RequiredParams: []string{"runner_id"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "trace", RequiredParams: []string{"project_id", "job_id"}},
			{ExpectedTool: "gitlab_runner", ExpectedAction: "update", RequiredParams: []string{"runner_id", "paused"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call gitlab_runner with {"action":"list_project","params":{"project_id":"<project_id>"}}`,
		"unless the task explicitly asks for an online, offline, stale, or never_contacted status filter",
		"Do not send params.paused, params.type, params.tag_list, status all, status active, or empty filter strings for runner.list_project",
		"For runner jobs, use runner.jobs with params.runner_id only",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want runner filter guidance containing %q", prompt, want)
		}
	}
}

// TestFixtureSetupToolEnvelope_UsesDynamicExecuteTool verifies dynamic fixture setup uses the visible executor.
func TestFixtureSetupToolEnvelope_UsesDynamicExecuteTool(t *testing.T) {
	params := map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}

	toolName, arguments := fixtureSetupToolEnvelope(config.ToolSurfaceDynamic, "gitlab", "branch.create", params)

	if toolName != dynamicExecuteTool {
		t.Fatalf("toolName = %q, want %q", toolName, dynamicExecuteTool)
	}
	gotParams, ok := arguments["params"].(map[string]any)
	if arguments["action"] != "branch.create" || !ok || gotParams["project_id"] != params["project_id"] {
		t.Fatalf("arguments = %#v, want dynamic action envelope", arguments)
	}
}

// TestFixtureSetupToolEnvelope_KeepsMetaDispatcher verifies meta fixture setup keeps the dispatcher envelope.
func TestFixtureSetupToolEnvelope_KeepsMetaDispatcher(t *testing.T) {
	params := map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}

	toolName, arguments := fixtureSetupToolEnvelope(config.ToolSurfaceMeta, "gitlab", "branch.create", params)

	if toolName != "gitlab" {
		t.Fatalf("toolName = %q, want gitlab", toolName)
	}
	gotParams, ok := arguments["params"].(map[string]any)
	if arguments["action"] != "branch.create" || !ok || gotParams["project_id"] != params["project_id"] {
		t.Fatalf("arguments = %#v, want meta action envelope", arguments)
	}
}

// TestTaskPrompt_PipelineTriggerCreateOmitsRef verifies that TaskPrompt handles the pipeline trigger create omits ref scenario correctly.
func TestTaskPrompt_PipelineTriggerCreateOmitsRef(t *testing.T) {
	task := evalTask{
		ID:     "MS-019",
		Prompt: "Exercise pipeline trigger CRUD in project `my-org/tools/gitlab-mcp-server`: create trigger `eval-crud-trigger`, fetch it with trigger get using the returned trigger ID, update the description, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_create", RequiredParams: []string{"project_id", "description"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_get", RequiredParams: []string{"project_id", "trigger_id"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_update", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"description"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_delete", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"trigger_create accepts only params.project_id and params.description",
		"never send params.ref for trigger_create",
		"Ref belongs to trigger_run or pipeline.create, not trigger_create",
		"Use the returned trigger_id for trigger_get, trigger_update, and trigger_delete",
		"trigger_delete also requires params.confirm=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline trigger guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_RepositoryFileCRUDUsesRefAndDeletesAfterUpdate verifies that TaskPrompt handles the repository file c r u d uses ref and deletes after update scenario correctly.
func TestTaskPrompt_RepositoryFileCRUDUsesRefAndDeletesAfterUpdate(t *testing.T) {
	task := evalTask{
		ID:     "MS-017",
		Prompt: "Exercise repository file CRUD in project `my-org/tools/gitlab-mcp-server`: create file `tmp/eval-crud.txt` on branch `feature/eval`, read it, update its content, then delete it from the same branch.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_update", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_delete", RequiredParams: []string{"project_id", "file_path", "branch", "commit_message"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"read the created file with file_get using params.ref set to the branch name",
		"never send params.branch to file_get",
		"After file_update succeeds, call file_delete next",
		"confirm must be inside params, never a top-level field",
		`"action":"file_delete","params":{"project_id":"<project_id>","file_path":"<file_path>","branch":"<branch>","commit_message":"<commit_message>","confirm":true}`,
		"Do not call file_get again after the update",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want repository file CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SingleFileCreateUsesExactToolCall verifies that TaskPrompt handles the single file create uses exact tool call scenario correctly.
func TestTaskPrompt_SingleFileCreateUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-030",
		Prompt:         "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_repository",
		ExpectedAction: "file_create",
		RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call: use the gitlab_repository tool once with input",
		`"action":"file_create"`,
		`"file_path":"tmp/eval.txt"`,
		`"content":"evaluation file"`,
		`"branch":"feature/eval"`,
		`"commit_message":"Create evaluation file"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want exact file_create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_InstanceVariableCreateUsesExactToolCall verifies that TaskPrompt handles the instance variable create uses exact tool call scenario correctly.
func TestTaskPrompt_InstanceVariableCreateUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-068",
		Prompt:         "Create instance CI variable `INSTANCE_EVAL_TOKEN` with value `masked-value-123`.",
		ExpectedTool:   "gitlab_ci_variable",
		ExpectedAction: "instance_create",
		RequiredParams: []string{"key", "value"},
		OptionalParams: []string{"masked", "protected"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call: use the gitlab_ci_variable tool once with input",
		`"action":"instance_create"`,
		`"key":"INSTANCE_EVAL_TOKEN"`,
		`"value":"masked-value-123"`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want exact instance_create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_PipelineScheduleCRUDAvoidsProjectPrefetchAndConfirmsDeletes verifies that TaskPrompt handles the pipeline schedule c r u d avoids project prefetch and confirms deletes scenario correctly.
func TestTaskPrompt_PipelineScheduleCRUDAvoidsProjectPrefetchAndConfirmsDeletes(t *testing.T) {
	task := evalTask{
		ID:     "MS-020",
		Prompt: "Exercise pipeline schedule CRUD in project `my-org/tools/gitlab-mcp-server`: create inactive schedule `eval-crud-schedule` on `main`, get it, update its cron, create variable `SCHEDULE_CRUD_TOKEN`, update that variable, delete the variable, then delete the schedule.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_create", RequiredParams: []string{"project_id", "description", "ref", "cron"}, OptionalParams: []string{"active"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_get", RequiredParams: []string{"project_id", "schedule_id"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_update", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"cron"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_create_variable", RequiredParams: []string{"project_id", "schedule_id", "key", "value"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_edit_variable", RequiredParams: []string{"project_id", "schedule_id", "key", "value"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_delete_variable", RequiredParams: []string{"project_id", "schedule_id", "key"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_delete", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"include confirm:true in params for each destructive tool call",
		"the first call is gitlab_pipeline with action schedule_create",
		"do not call gitlab_discover_project or gitlab_project first",
		"Use description, not name, for the schedule display label",
		"never send masked or protected",
		"Use the returned id as params.schedule_id",
		"Both schedule_delete_variable and schedule_delete are destructive and require confirm:true according to the active tool surface",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline schedule guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DiscoverProjectUsesStandaloneInput verifies that TaskPrompt handles the discover project uses standalone input scenario correctly.
func TestTaskPrompt_DiscoverProjectUsesStandaloneInput(t *testing.T) {
	task := evalTask{
		ID:     "MS-001",
		Prompt: "Resolve remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git` for project `my-org/tools/gitlab-mcp-server`, verify the project metadata, then read `README.md` from `main`.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call the standalone tool with top-level remote_url only`,
		`{"remote_url":"<remote_url>"}`,
		"do not send action, params, project_id, or ref to gitlab_discover_project",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want discover_project guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_FeatureFlagLifecycleOmitsArrayStrategies verifies that TaskPrompt handles the feature flag lifecycle omits array strategies scenario correctly.
func TestTaskPrompt_FeatureFlagLifecycleOmitsArrayStrategies(t *testing.T) {
	task := evalTask{
		ID:     "MS-029",
		Prompt: "Exercise feature flag and user-list lifecycle in project `my-org/tools/gitlab-mcp-server`: create feature flag user list `eval-feature-list` with user IDs `u1,u2`, fetch it, update the user IDs to `u2,u3`, create feature flag `eval-feature-flag-crud` using version `new_version_flag`, fetch the flag, update it inactive, delete the flag, then delete the user list.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_create", RequiredParams: []string{"project_id", "name", "user_xids"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_get", RequiredParams: []string{"project_id", "user_list_iid"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_update", RequiredParams: []string{"project_id", "user_list_iid"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_create", RequiredParams: []string{"project_id", "name", "version"}, OptionalParams: []string{"strategies"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_get", RequiredParams: []string{"project_id", "name"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_update", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"strategies"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_delete", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_delete", RequiredParams: []string{"project_id", "user_list_iid"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`params.user_xids is a comma-separated string such as "u1,u2", not an array`,
		"omit params.strategies unless the task gives an exact strategies JSON string",
		`must be a JSON string such as "[{\"name\":\"default\"}]", never an array or object`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want feature flag lifecycle guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DeployTokenLifecycleAvoidsInventedTimestamp verifies that TaskPrompt handles the deploy token lifecycle avoids invented timestamp scenario correctly.
func TestTaskPrompt_DeployTokenLifecycleAvoidsInventedTimestamp(t *testing.T) {
	task := evalTask{
		ID:     "MS-030",
		Prompt: "Exercise project deploy token lifecycle in project `my-org/tools/gitlab-mcp-server`: create deploy token `eval-deploy-token` with scope `read_repository`, fetch it with the returned deploy token ID, list project deploy tokens, then delete that deploy token.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_create_project", RequiredParams: []string{"project_id", "name", "scopes"}, OptionalParams: []string{"expires_at", "username"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_get_project", RequiredParams: []string{"project_id", "deploy_token_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_list_project", RequiredParams: []string{"project_id"}, OptionalParams: []string{"page", "per_page"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_delete_project", RequiredParams: []string{"project_id", "deploy_token_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"deploy_token_create_project requires params.project_id, params.name, and params.scopes",
		"Do not add params.expires_at unless the task gives an explicit expiry date",
		"must be YYYY-MM-DD only, never a timestamp",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy token lifecycle guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DestructiveScenarioWarningGuidance validates task prompt across multiple scenarios using table-driven subtests
// for the destructive scenario warning guidance case.
func TestTaskPrompt_DestructiveScenarioWarningGuidance(t *testing.T) {
	tests := []struct {
		name  string
		task  evalTask
		wants []string
	}{
		{
			name: "broadcast theme",
			task: evalTask{
				ID:     "MS-009",
				Prompt: "Schedule and then remove an instance maintenance banner: read current instance settings, immediately create broadcast message `Evaluation maintenance`, then delete the broadcast message created in the previous step using the returned ID.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_admin", ExpectedAction: "settings_get"},
					{ExpectedTool: "gitlab_admin", ExpectedAction: "broadcast_message_create", RequiredParams: []string{"message"}, OptionalParams: []string{"starts_at", "ends_at", "broadcast_type"}},
					{ExpectedTool: "gitlab_admin", ExpectedAction: "broadcast_message_delete", RequiredParams: []string{"id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"omit params.theme unless explicitly requested", "use a GitLab theme name such as indigo, never a hex color"},
		},
		{
			name: "issue link delete",
			task: evalTask{
				ID:     "MS-016",
				Prompt: "Exercise issue link CRUD in project `my-org/tools/gitlab-mcp-server`: create source issue `eval-link-source`, create target issue `eval-link-target`, link source to target as `relates_to`, list source issue links, delete the returned issue link, then delete both issues.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_issue", ExpectedAction: "create", RequiredParams: []string{"project_id", "title"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "create", RequiredParams: []string{"project_id", "title"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_create", RequiredParams: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_list", RequiredParams: []string{"project_id", "issue_iid"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_delete", RequiredParams: []string{"project_id", "issue_iid", "issue_link_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"keep the source issue IID from the first create call", "params.issue_iid set to the source issue IID", "params.issue_link_id from the returned link"},
		},
		{
			name: "project badge URLs",
			task: evalTask{
				ID:     "MS-022",
				Prompt: "Exercise project badge CRUD in project `my-org/tools/gitlab-mcp-server`: add badge `eval-crud-badge`, fetch it with badge get using the returned badge ID, edit the badge name to `Evaluation CRUD badge link`, then delete it.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_add", RequiredParams: []string{"project_id", "link_url", "image_url"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_get", RequiredParams: []string{"project_id", "badge_id"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_edit", RequiredParams: []string{"project_id", "badge_id"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_delete", RequiredParams: []string{"project_id", "badge_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"badge_add requires valid absolute params.link_url and params.image_url", "https://example.com/eval-badge", "https://example.com/eval-badge.svg"},
		},
		{
			name: "branch unprotect",
			task: evalTask{
				ID:     "MS-028",
				Prompt: "Exercise branch protection lifecycle in project `my-org/tools/gitlab-mcp-server`: create branch `eval-protect-branch` from `main`, protect it with Maintainer push and merge access, fetch the protected branch, update it to allow force push, unprotect it, then delete the branch.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_branch", ExpectedAction: "create", RequiredParams: []string{"project_id", "branch_name", "ref"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "protect", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "get_protected", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "update_protected", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "unprotect", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "delete", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"unprotect only uses params.project_id, params.branch_name, and params.confirm=true", "never send allow_force_push to unprotect", `"action":"unprotect","params":{"project_id":"<project_id>","branch_name":"<branch_name>","confirm":true}`, `"action":"delete","params":{"project_id":"<project_id>","branch_name":"<branch_name>","confirm":true}`, "Never put confirm beside action as a top-level field"},
		},
		{
			name: "group milestone",
			task: evalTask{
				ID:     "MS-036",
				Prompt: "Exercise group milestone lifecycle in group `my-org`: create milestone `Evaluation Group Milestone` with due date `2026-12-31`, fetch it using the returned milestone IID, update title to `Evaluation Group Milestone v2`, then delete it.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_create", RequiredParams: []string{"group_id", "title"}, OptionalParams: []string{"description", "due_date"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_get", RequiredParams: []string{"group_id", "milestone_iid"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_update", RequiredParams: []string{"group_id", "milestone_iid"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_delete", RequiredParams: []string{"group_id", "milestone_iid"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"Do not invent params.start_date unless the task provides an earlier start date", "call group_milestone_get with the returned milestone_iid before any update"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPrompt(tt.task)
			for _, want := range tt.wants {
				if !strings.Contains(prompt, want) {
					t.Fatalf("taskPrompt() = %q, want guidance containing %q", prompt, want)
				}
			}
		})
	}
}

// TestTaskPrompt_ProjectSnippetCRUDAvoidsProjectPrefetch verifies that TaskPrompt handles the project snippet c r u d avoids project prefetch scenario correctly.
func TestTaskPrompt_ProjectSnippetCRUDAvoidsProjectPrefetch(t *testing.T) {
	task := evalTask{
		ID:     "MS-024",
		Prompt: "Exercise project snippet CRUD in project `my-org/tools/gitlab-mcp-server`: create project snippet `eval-crud-snippet` titled `Evaluation CRUD snippet`, fetch it with project snippet get using the returned snippet ID, update its content with a `files` entry using action `update` and `file_path` set to the returned file path, not `previous_path`, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_create", RequiredParams: []string{"project_id", "title", "file_name", "content"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_get", RequiredParams: []string{"project_id", "snippet_id"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_update", RequiredParams: []string{"project_id", "snippet_id"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_delete", RequiredParams: []string{"project_id", "snippet_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"the first call is gitlab_snippet with action project_create",
		"do not call gitlab_project first",
		"project_create requires params.project_id, params.title, params.file_name, and params.content",
		"Use the returned snippet_id for project_get, project_update, and project_delete",
		"project_update params should contain project_id, snippet_id, and files",
		"never send params.file_path or params.content at top level when using files[]",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want project snippet CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ProjectHookCRUDAvoidsGroupHooks verifies that TaskPrompt handles the project hook c r u d avoids group hooks scenario correctly.
func TestTaskPrompt_ProjectHookCRUDAvoidsGroupHooks(t *testing.T) {
	task := evalTask{
		ID:     "MS-021",
		Prompt: "Exercise project hook CRUD in project `my-org/tools/gitlab-mcp-server`: add a hook, fetch it with hook get, edit it, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_add", RequiredParams: []string{"project_id", "url"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_get", RequiredParams: []string{"project_id", "hook_id"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_edit", RequiredParams: []string{"project_id", "hook_id"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_delete", RequiredParams: []string{"project_id", "hook_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"For project hook CRUD, use gitlab_project actions hook_add, hook_get, hook_edit, and hook_delete with params.project_id",
		"Do not use gitlab_group hook actions for a project hook workflow",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want project hook guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DiscussionResolveIncludesQuotedEnvelopeGuidance verifies that TaskPrompt handles the discussion resolve includes quoted envelope guidance scenario correctly.
func TestTaskPrompt_DiscussionResolveIncludesQuotedEnvelopeGuidance(t *testing.T) {
	task := evalTask{
		ID:             "MT-061",
		Prompt:         "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "mr_review.discussion_resolve",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, "emit tool gitlab_mr_review") ||
		!strings.Contains(prompt, `"action":"discussion_resolve"`) ||
		!strings.Contains(prompt, `action "mr_review.discussion_resolve"`) ||
		!strings.Contains(prompt, `"discussion_id":"<discussion_id>"`) {
		t.Fatalf("taskPrompt() = %q, want quoted discussion_resolve envelope guidance", prompt)
	}
}

// TestTaskPrompt_SplitDiscussionResolveUsesExactToolCall verifies that TaskPrompt handles the split discussion resolve uses exact tool call scenario correctly.
func TestTaskPrompt_SplitDiscussionResolveUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-061",
		Prompt:         "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_mr_review",
		ExpectedAction: "discussion_resolve",
		RequiredParams: []string{"project_id", "merge_request_iid", "discussion_id"},
		OptionalParams: []string{"resolved"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call",
		"use the gitlab_mr_review tool once",
		`"action":"discussion_resolve"`,
		`"discussion_id":"abc123"`,
		`"merge_request_iid":7`,
		`"resolved":true`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want split discussion_resolve guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SearchCodeAvoidsProjectDiscovery verifies that TaskPrompt handles the search code avoids project discovery scenario correctly.
func TestTaskPrompt_SearchCodeAvoidsProjectDiscovery(t *testing.T) {
	task := evalTask{
		ID:             "MT-032",
		Prompt:         "Search code for `func RegisterMCPMeta` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "search.code",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, `"action":"search.code"`) || !strings.Contains(prompt, "never remote_url") {
		t.Fatalf("taskPrompt() = %q, want search.code direct project_id guidance", prompt)
	}
}

// TestTaskPrompt_ReleaseCreateMapsFromRef verifies that TaskPrompt handles the release create maps from ref scenario correctly.
func TestTaskPrompt_ReleaseCreateMapsFromRef(t *testing.T) {
	task := evalTask{
		ID:             "MT-036",
		Prompt:         "Create release `v0.0.0-eval` for tag `v0.0.0-eval` from ref `main` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_release",
		ExpectedAction: "create",
	}
	prompt := taskPrompt(task)

	if !strings.Contains(prompt, `For release.create, "from ref X" maps to params.ref`) {
		t.Fatalf("taskPrompt() = %q, want release ref guidance", prompt)
	}
}

// TestTaskPrompt_AdminSettingsUsesDispatcherDirectly verifies that TaskPrompt handles the admin settings uses dispatcher directly scenario correctly.
func TestTaskPrompt_AdminSettingsUsesDispatcherDirectly(t *testing.T) {
	task := evalTask{
		ID:             "MT-052",
		Prompt:         "Show instance application settings.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "admin.settings_get",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, `"action":"admin.settings_get","params":{}`) || !strings.Contains(prompt, "do not call gitlab_server") || !strings.Contains(prompt, "do not look up a schema") {
		t.Fatalf("taskPrompt() = %q, want direct admin.settings_get guidance", prompt)
	}
}

// TestTaskPrompt_ArtifactFromNumericJobUsesSingleArtifact verifies that TaskPrompt handles the artifact from numeric job uses single artifact scenario correctly.
func TestTaskPrompt_ArtifactFromNumericJobUsesSingleArtifact(t *testing.T) {
	task := evalTask{
		ID:             "MT-065",
		Prompt:         "Download artifact `coverage/report.xml` from job `999` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_job",
		ExpectedAction: "download_single_artifact",
		RequiredParams: []string{"project_id", "job_id", "artifact_path"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{"Exact required call", "use the gitlab_job tool once", `"action":"download_single_artifact"`, `"job_id":999`, `"artifact_path":"coverage/report.xml"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want artifact guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_FailedPipelineJobsUseJobList verifies that TaskPrompt handles the failed pipeline jobs use job list scenario correctly.
func TestTaskPrompt_FailedPipelineJobsUseJobList(t *testing.T) {
	task := evalTask{
		ID:     "MS-002",
		Prompt: "Investigate failed pipeline `12345` for project `my-org/tools/gitlab-mcp-server`: inspect the pipeline, list failed jobs, fetch job `999` trace, then call the pipeline failure analyzer.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "get", RequiredParams: []string{"project_id", "pipeline_id"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "list", RequiredParams: []string{"project_id", "pipeline_id"}, OptionalParams: []string{"scope"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "trace", RequiredParams: []string{"project_id", "job_id"}},
			{ExpectedTool: "gitlab_analyze", ExpectedAction: "pipeline_failure", RequiredParams: []string{"project_id", "pipeline_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{"gitlab_job", `"action":"list"`, `"scope":"failed"`, "do not call gitlab_pipeline list with pipeline_id"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want failed-job guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SingleFailedPipelineJobsUsesExactToolCall verifies that TaskPrompt handles the single failed pipeline jobs uses exact tool call scenario correctly.
func TestTaskPrompt_SingleFailedPipelineJobsUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-021",
		Prompt:         "List failed jobs in pipeline `1323` for project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_job",
		ExpectedAction: "list",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"scope"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call",
		"use the gitlab_job tool once",
		`"action":"list"`,
		`"pipeline_id":1323`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"scope":"failed"`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want single failed-job guidance containing %q", prompt, want)
		}
	}

	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	if !strings.Contains(system, "Return tool calls only") || strings.Contains(system, "runner.list_project") {
		t.Fatalf("systemPromptForTask() = %q, want compact exact-call system prompt", system)
	}
}

// TestTaskPrompt_SingleDestructiveSplitActionsUseExactToolCalls validates task prompt across multiple scenarios using table-driven subtests
// for the single destructive split actions use exact tool calls case.
func TestTaskPrompt_SingleDestructiveSplitActionsUseExactToolCalls(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		wants  []string
		absent []string
	}{
		{
			name:  "job artifacts",
			task:  evalTask{ID: "MT-024", Prompt: "Delete artifacts for job `999` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_job", ExpectedAction: "delete_artifacts", RequiredParams: []string{"project_id", "job_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_job tool once", `"action":"delete_artifacts"`, `"job_id":999`, `"confirm":true`},
		},
		{
			name:  "wiki delete",
			task:  evalTask{ID: "MT-108", Prompt: "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_wiki", ExpectedAction: "delete", RequiredParams: []string{"project_id", "slug"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_wiki tool once", `"action":"delete"`, `"slug":"obsolete-eval"`, `"confirm":true`},
		},
		{
			name:  "mr emoji",
			task:  evalTask{ID: "MT-109", Prompt: "Remove award emoji ID `12` from merge request `7` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_delete", RequiredParams: []string{"project_id", "merge_request_iid", "award_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_merge_request tool once", `"action":"emoji_mr_delete"`, `"award_id":12`, `"merge_request_iid":7`, "do not use gitlab_mr_review"},
		},
		{
			name:  "commit discussion note",
			task:  evalTask{ID: "MT-113", Prompt: "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_repository", ExpectedAction: "commit_discussion_delete_note", RequiredParams: []string{"project_id", "commit_sha", "discussion_id", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_repository tool once", `"action":"commit_discussion_delete_note"`, `"commit_sha":"abc1234"`, `"discussion_id":"abc123"`, `"note_id":999`},
		},
		{
			name:   "archive",
			task:   evalTask{ID: "MT-055", Prompt: "Archive project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_project", ExpectedAction: "archive", RequiredParams: []string{"project_id"}},
			wants:  []string{"use the gitlab_project tool once", `"action":"archive"`, `"project_id":"my-org/tools/gitlab-mcp-server"`},
			absent: []string{`"action":"delete"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPrompt(tt.task)
			for _, want := range tt.wants {
				if !strings.Contains(prompt, want) {
					t.Fatalf("taskPrompt() = %q, want exact guidance containing %q", prompt, want)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(prompt, absent) {
					t.Fatalf("taskPrompt() = %q, want exact guidance without %q", prompt, absent)
				}
			}
		})
	}
}

// TestTaskPrompt_AnalyzerTasksAvoidPrefetch verifies that TaskPrompt handles the analyzer tasks avoid prefetch scenario correctly.
func TestTaskPrompt_AnalyzerTasksAvoidPrefetch(t *testing.T) {
	task := evalTask{
		ID:             "MT-093",
		Prompt:         "Review merge request `7` changes in project `my-org/tools/gitlab-mcp-server` with the LLM-assisted analyzer.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "analyze.mr_changes",
		RequiredParams: []string{"project_id", "merge_request_iid"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"analyze.mr_changes"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"merge_request_iid":7`,
		"do not prefetch",
		"do not use params:{}",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want analyzer guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_AnalyzerTasksIncludeOptionalRefExample verifies that TaskPrompt handles the analyzer tasks include optional ref example scenario correctly.
func TestTaskPrompt_AnalyzerTasksIncludeOptionalRefExample(t *testing.T) {
	task := evalTask{
		ID:             "MT-097",
		Prompt:         "Analyze the CI configuration on branch `main` for project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "analyze.ci_config",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"content_ref"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"analyze.ci_config"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"content_ref":"main"`,
		"Exact required call",
		"do not call gitlab_discover_project",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want analyzer guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SplitAnalyzerTasksIncludeExactToolGuidance verifies that TaskPrompt handles the split analyzer tasks include exact tool guidance scenario correctly.
func TestTaskPrompt_SplitAnalyzerTasksIncludeExactToolGuidance(t *testing.T) {
	task := evalTask{
		ID:             "MT-097",
		Prompt:         "Analyze the CI configuration on branch `main` for project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_analyze",
		ExpectedAction: "ci_config",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"content_ref"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"use the gitlab_analyze tool once",
		`"action":"ci_config"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"content_ref":"main"`,
		"do not use params:{}",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want split analyzer guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_PipelineTriggerDeleteUsesTriggerID verifies that TaskPrompt handles the pipeline trigger delete uses trigger i d scenario correctly.
func TestTaskPrompt_PipelineTriggerDeleteUsesTriggerID(t *testing.T) {
	task := evalTask{
		ID:             "MT-102",
		Prompt:         "Delete pipeline trigger token ID `77` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.trigger_delete",
		RequiredParams: []string{"project_id", "trigger_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"pipeline.trigger_delete"`,
		`"trigger_id":77`,
		"Exact required call",
		"The supplied ID maps to the matching *_id param",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline trigger delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact pipeline trigger delete guidance without %q", prompt, unwanted)
		}
	}
	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline trigger delete system prompt without %q", system, unwanted)
		}
	}
}

// TestTaskPrompt_PipelineScheduleDeleteUsesScheduleID verifies that TaskPrompt handles the pipeline schedule delete uses schedule i d scenario correctly.
func TestTaskPrompt_PipelineScheduleDeleteUsesScheduleID(t *testing.T) {
	task := evalTask{
		ID:             "MT-103",
		Prompt:         "Delete pipeline schedule ID `49` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.schedule_delete",
		RequiredParams: []string{"project_id", "schedule_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"pipeline.schedule_delete"`,
		`"schedule_id":49`,
		"Exact required call",
		"The supplied ID maps to the matching *_id param",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline schedule delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact pipeline schedule delete guidance without %q", prompt, unwanted)
		}
	}
	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline schedule delete system prompt without %q", system, unwanted)
		}
	}
}

// TestTaskPrompt_UserBlockUsesUserID verifies that TaskPrompt handles the user block uses user i d scenario correctly.
func TestTaskPrompt_UserBlockUsesUserID(t *testing.T) {
	task := evalTask{
		ID:             "MT-104",
		Prompt:         "Block user ID `69`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "user.block",
		RequiredParams: []string{"user_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"user.block"`,
		`"user_id":69`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want user block guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"runner_id", "target_branch", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact user block guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_FeatureFlagDeleteUsesName verifies that TaskPrompt handles the feature flag delete uses name scenario correctly.
func TestTaskPrompt_FeatureFlagDeleteUsesName(t *testing.T) {
	task := evalTask{
		ID:             "MT-106",
		Prompt:         "Delete feature flag `eval_flag` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "feature_flags.feature_flag_delete",
		RequiredParams: []string{"project_id", "name"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"feature_flags.feature_flag_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"name":"eval_flag"`,
		"Exact required call",
		"The supplied values map to the matching params",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want feature flag delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact feature flag delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_WikiDeleteUsesSlug verifies that TaskPrompt handles the wiki delete uses slug scenario correctly.
func TestTaskPrompt_WikiDeleteUsesSlug(t *testing.T) {
	task := evalTask{
		ID:             "MT-108",
		Prompt:         "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "wiki.delete",
		RequiredParams: []string{"project_id", "slug"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"wiki.delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"slug":"obsolete-eval"`,
		"Exact required call",
		"The supplied values map to the matching params",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want wiki delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact wiki delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_MRAwardDeleteUsesAwardID verifies that TaskPrompt handles the m r award delete uses award i d scenario correctly.
func TestTaskPrompt_MRAwardDeleteUsesAwardID(t *testing.T) {
	task := evalTask{
		ID:             "MT-109",
		Prompt:         "Remove award emoji ID `21` from merge request `1` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "merge_request.emoji_mr_delete",
		RequiredParams: []string{"project_id", "merge_request_iid", "award_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"merge_request.emoji_mr_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"merge_request_iid":1`,
		`"award_id":21`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR award delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"mr_review.emoji_mr_note_delete", "note_id", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact MR award delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_IssueAwardDeleteUsesAwardID verifies that TaskPrompt handles the issue award delete uses award i d scenario correctly.
func TestTaskPrompt_IssueAwardDeleteUsesAwardID(t *testing.T) {
	task := evalTask{
		ID:             "MT-110",
		Prompt:         "Remove award emoji ID `22` from issue `42` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "issue.emoji_issue_delete",
		RequiredParams: []string{"project_id", "issue_iid", "award_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"issue.emoji_issue_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"issue_iid":42`,
		`"award_id":22`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want issue award delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"note_id", "target_branch", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact issue award delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_DeployKeyDeleteUsesDeployKeyID verifies that TaskPrompt handles the deploy key delete uses deploy key i d scenario correctly.
func TestTaskPrompt_DeployKeyDeleteUsesDeployKeyID(t *testing.T) {
	task := evalTask{
		ID:             "MT-111",
		Prompt:         "Delete deploy key ID `32` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "access.deploy_key_delete",
		RequiredParams: []string{"project_id", "deploy_key_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"access.deploy_key_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"deploy_key_id":32`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy key delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DeployTokenDeleteUsesDeployTokenID verifies that TaskPrompt handles the deploy token delete uses deploy token i d scenario correctly.
func TestTaskPrompt_DeployTokenDeleteUsesDeployTokenID(t *testing.T) {
	task := evalTask{
		ID:             "MT-112",
		Prompt:         "Delete project deploy token ID `66` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "access.deploy_token_delete_project",
		RequiredParams: []string{"project_id", "deploy_token_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"access.deploy_token_delete_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"deploy_token_id":66`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy token delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_CommitDiscussionDeleteUsesDiscussionAndNote verifies that TaskPrompt handles the commit discussion delete uses discussion and note scenario correctly.
func TestTaskPrompt_CommitDiscussionDeleteUsesDiscussionAndNote(t *testing.T) {
	task := evalTask{
		ID:             "MT-113",
		Prompt:         "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "repository.commit_discussion_delete_note",
		RequiredParams: []string{"project_id", "commit_sha", "discussion_id", "note_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"repository.commit_discussion_delete_note"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"commit_sha":"abc1234"`,
		`"discussion_id":"abc123"`,
		`"note_id":999`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want commit discussion delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"issue.discussion_delete_note", "merge_request_iid", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact commit discussion delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_AttestationDownloadUsesAttestationIID verifies that TaskPrompt handles the attestation download uses attestation i i d scenario correctly.
func TestTaskPrompt_AttestationDownloadUsesAttestationIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-117",
		Prompt:         "Download attestation IID `5` from project `my-org/tools/gitlab-mcp-server`; use the project-scoped attestation IID, not the database ID.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "attestation.download",
		RequiredParams: []string{"project_id", "attestation_iid"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"attestation.download"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"attestation_iid":5`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want attestation guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_AuditEventGetUsesEventID verifies that TaskPrompt handles the audit event get uses event i d scenario correctly.
func TestTaskPrompt_AuditEventGetUsesEventID(t *testing.T) {
	task := evalTask{
		ID:             "MT-118",
		Prompt:         "Get instance audit event ID `77`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "audit_event.get_instance",
		RequiredParams: []string{"event_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"audit_event.get_instance"`,
		`"event_id":77`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want audit event get guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "user_id") {
		t.Fatalf("taskPrompt() = %q, want event_id guidance without user_id", prompt)
	}
}

// TestTaskPrompt_AuditEventListUsesCreatedRange verifies that TaskPrompt handles the audit event list uses created range scenario correctly.
func TestTaskPrompt_AuditEventListUsesCreatedRange(t *testing.T) {
	task := evalTask{
		ID:             "MT-119",
		Prompt:         "List project audit events for project `my-org/tools/gitlab-mcp-server` created during January 2026.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "audit_event.list_project",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"created_after", "created_before", "per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"audit_event.list_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"created_after":"2026-01-01"`,
		`"created_before":"2026-02-01"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want audit event list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_CompliancePolicyUpdateUsesNamespaceID verifies that TaskPrompt handles the compliance policy update uses namespace i d scenario correctly.
func TestTaskPrompt_CompliancePolicyUpdateUsesNamespaceID(t *testing.T) {
	task := evalTask{
		ID:             "MT-120",
		Prompt:         "Update the admin compliance policy settings to use namespace ID `123`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "compliance_policy.update",
		RequiredParams: []string{"csp_namespace_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"compliance_policy.update"`,
		`"csp_namespace_id":123`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want compliance policy guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "issue_iid") {
		t.Fatalf("taskPrompt() = %q, want csp_namespace_id guidance without issue_iid", prompt)
	}
}

// TestTaskPrompt_DependencyExportCreateUsesPipelineID verifies that TaskPrompt handles the dependency export create uses pipeline i d scenario correctly.
func TestTaskPrompt_DependencyExportCreateUsesPipelineID(t *testing.T) {
	task := evalTask{
		ID:             "MT-121",
		Prompt:         "Create a dependency list export for pipeline ID `12345`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dependency.export_create",
		RequiredParams: []string{"pipeline_id"},
		OptionalParams: []string{"export_type"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dependency.export_create"`,
		`"pipeline_id":12345`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want dependency export create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DependencyExportDownloadUsesExportID verifies that TaskPrompt handles the dependency export download uses export i d scenario correctly.
func TestTaskPrompt_DependencyExportDownloadUsesExportID(t *testing.T) {
	task := evalTask{
		ID:             "MT-122",
		Prompt:         "Download dependency list export ID `987`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dependency.export_download",
		RequiredParams: []string{"export_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dependency.export_download"`,
		`"export_id":987`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want dependency export download guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"project_id", "attestation_iid"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want export_id guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_DORAMetricsGroupUsesMetric verifies that TaskPrompt handles the d o r a metrics group uses metric scenario correctly.
func TestTaskPrompt_DORAMetricsGroupUsesMetric(t *testing.T) {
	task := evalTask{
		ID:             "MT-123",
		Prompt:         "Get group DORA lead time metrics for group `my-org` from `2026-01-01` to `2026-01-31`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dora_metrics.group",
		RequiredParams: []string{"group_id", "metric"},
		OptionalParams: []string{"start_date", "end_date", "interval", "environment_tiers"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dora_metrics.group"`,
		`"group_id":"my-org"`,
		`"metric":"lead_time_for_changes"`,
		`"start_date":"2026-01-01"`,
		`"end_date":"2026-01-31"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want DORA guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_EnterpriseUserGetUsesGroupAndUserID verifies that TaskPrompt handles the enterprise user get uses group and user i d scenario correctly.
func TestTaskPrompt_EnterpriseUserGetUsesGroupAndUserID(t *testing.T) {
	task := evalTask{
		ID:             "MT-124",
		Prompt:         "Get enterprise user ID `55` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "enterprise_user.get",
		RequiredParams: []string{"group_id", "user_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"enterprise_user.get"`,
		`"group_id":"my-org"`,
		`"user_id":55`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want enterprise user guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_EnterpriseUserDisable2FAUsesEnterpriseAction verifies that TaskPrompt handles the enterprise user disable2 f a uses enterprise action scenario correctly.
func TestTaskPrompt_EnterpriseUserDisable2FAUsesEnterpriseAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-125",
		Prompt:         "Disable two-factor authentication for enterprise user ID `55` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "enterprise_user.disable_2fa",
		RequiredParams: []string{"group_id", "user_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"enterprise_user.disable_2fa"`,
		`"group_id":"my-org"`,
		`"user_id":55`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want enterprise 2FA guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "user.disable_two_factor") {
		t.Fatalf("taskPrompt() = %q, want enterprise action guidance without base user 2FA action", prompt)
	}
}

// TestTaskPrompt_ExternalStatusCheckCreateUsesExternalURL verifies that TaskPrompt handles the external status check create uses external u r l scenario correctly.
func TestTaskPrompt_ExternalStatusCheckCreateUsesExternalURL(t *testing.T) {
	task := evalTask{
		ID:             "MT-126",
		Prompt:         "Create external project status check `Eval Gate` on project `my-org/tools/gitlab-mcp-server` pointing at `https://example.com/check`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.create_project",
		RequiredParams: []string{"project_id", "name", "external_url"},
		OptionalParams: []string{"shared_secret", "protected_branch_ids"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.create_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"name":"Eval Gate"`,
		`"external_url":"https://example.com/check"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ExternalStatusCheckStatusUsesCheckID verifies that TaskPrompt handles the external status check status uses check i d scenario correctly.
func TestTaskPrompt_ExternalStatusCheckStatusUsesCheckID(t *testing.T) {
	task := evalTask{
		ID:             "MT-127",
		Prompt:         "Mark external status check ID `8` as passed for merge request IID `7` at SHA `abc123` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.set_project_mr_status",
		RequiredParams: []string{"project_id", "merge_request_iid", "sha", "external_status_check_id", "status"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.set_project_mr_status"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"merge_request_iid":7`,
		`"sha":"abc123"`,
		`"external_status_check_id":8`,
		`"status":"passed"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check status guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ExternalStatusCheckDeleteUsesCheckID verifies that TaskPrompt handles the external status check delete uses check i d scenario correctly.
func TestTaskPrompt_ExternalStatusCheckDeleteUsesCheckID(t *testing.T) {
	task := evalTask{
		ID:             "MT-128",
		Prompt:         "Delete external project status check ID `8` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.delete_project",
		RequiredParams: []string{"project_id", "check_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.delete_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"check_id":8`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"rule_id", "deploy_key_id"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want check_id guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GeoGetUsesID verifies that TaskPrompt handles the geo get uses i d scenario correctly.
func TestTaskPrompt_GeoGetUsesID(t *testing.T) {
	task := evalTask{
		ID:             "MT-129",
		Prompt:         "Get Geo site ID `3`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.get",
		RequiredParams: []string{"id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.get"`,
		`"id":3`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo get guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GeoCreateUsesEnabledAndPrimary verifies that TaskPrompt handles the geo create uses enabled and primary scenario correctly.
func TestTaskPrompt_GeoCreateUsesEnabledAndPrimary(t *testing.T) {
	task := evalTask{
		ID:             "MT-130",
		Prompt:         "Create a disabled Geo secondary site named `eval-geo` with URL `https://geo.example.com`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.create",
		RequiredParams: []string{"name", "url"},
		OptionalParams: []string{"enabled", "primary"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.create"`,
		`"name":"eval-geo"`,
		`"url":"https://geo.example.com"`,
		`"enabled":false`,
		`"primary":false`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo create guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "paused") {
		t.Fatalf("taskPrompt() = %q, want Geo create guidance without paused", prompt)
	}
}

// TestTaskPrompt_GeoDeleteUsesID verifies that TaskPrompt handles the geo delete uses i d scenario correctly.
func TestTaskPrompt_GeoDeleteUsesID(t *testing.T) {
	task := evalTask{
		ID:             "MT-131",
		Prompt:         "Delete Geo site ID `3`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.delete",
		RequiredParams: []string{"id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.delete"`,
		`"id":3`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"geo_node_id", "site_id", "path"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want Geo delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GroupCredentialListUsesCredentialAction verifies that TaskPrompt handles the group credential list uses credential action scenario correctly.
func TestTaskPrompt_GroupCredentialListUsesCredentialAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-133",
		Prompt:         "List group personal access tokens for group `my-org`, filtering active tokens.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.credential_list_pats",
		RequiredParams: []string{"group_id"},
		OptionalParams: []string{"state", "per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.credential_list_pats"`,
		`"group_id":"my-org"`,
		`"state":"active"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group credential list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupCredentialRevokeUsesTokenID verifies that TaskPrompt handles the group credential revoke uses token i d scenario correctly.
func TestTaskPrompt_GroupCredentialRevokeUsesTokenID(t *testing.T) {
	task := evalTask{
		ID:             "MT-134",
		Prompt:         "Revoke group personal access token ID `77` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.credential_revoke_pat",
		RequiredParams: []string{"group_id", "token_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.credential_revoke_pat"`,
		`"group_id":"my-org"`,
		`"token_id":77`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group credential revoke guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicBoardListUsesEpicBoardAction verifies that TaskPrompt handles the group epic board list uses epic board action scenario correctly.
func TestTaskPrompt_GroupEpicBoardListUsesEpicBoardAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-135",
		Prompt:         "List epic boards for group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_board_list",
		RequiredParams: []string{"group_id"},
		OptionalParams: []string{"per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_board_list"`,
		`"group_id":"my-org"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic board list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicListUsesFullPath verifies that TaskPrompt handles the group epic list uses full path scenario correctly.
func TestTaskPrompt_GroupEpicListUsesFullPath(t *testing.T) {
	task := evalTask{
		ID:             "MT-136",
		Prompt:         "List epics in group full path `my-org` including descendant groups.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_list",
		RequiredParams: []string{"full_path"},
		OptionalParams: []string{"include_descendants", "state", "first"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_list"`,
		`"full_path":"my-org"`,
		`"include_descendants":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic list guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"group_path", "group_id", "include_descendant_groups"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want group epic list guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GroupEpicCreateUsesFullPathAndTitle verifies that TaskPrompt handles the group epic create uses full path and title scenario correctly.
func TestTaskPrompt_GroupEpicCreateUsesFullPathAndTitle(t *testing.T) {
	task := evalTask{
		ID:             "MT-137",
		Prompt:         "Create an epic titled `Evaluation Epic` in group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_create",
		RequiredParams: []string{"full_path", "title"},
		OptionalParams: []string{"description", "start_date", "due_date"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_create"`,
		`"full_path":"my-org"`,
		`"title":"Evaluation Epic"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicUpdateUsesEpicIID verifies that TaskPrompt handles the group epic update uses epic i i d scenario correctly.
func TestTaskPrompt_GroupEpicUpdateUsesEpicIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-138",
		Prompt:         "Update epic IID `12` in group full path `my-org` to close it.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_update",
		RequiredParams: []string{"full_path", "epic_iid"},
		OptionalParams: []string{"state_event", "title"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_update"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"state_event":"close"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic update guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicDeleteUsesEpicIID verifies that TaskPrompt handles the group epic delete uses epic i i d scenario correctly.
func TestTaskPrompt_GroupEpicDeleteUsesEpicIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-139",
		Prompt:         "Delete epic IID `12` from group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_delete",
		RequiredParams: []string{"full_path", "epic_iid"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_delete"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicIssueAssignUsesChildParams verifies that TaskPrompt handles the group epic issue assign uses child params scenario correctly.
func TestTaskPrompt_GroupEpicIssueAssignUsesChildParams(t *testing.T) {
	task := evalTask{
		ID:             "MT-140",
		Prompt:         "Assign issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` to epic IID `12` in group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_issue_assign",
		RequiredParams: []string{"full_path", "epic_iid", "child_project_path", "child_iid"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_issue_assign"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"child_project_path":"my-org/tools/gitlab-mcp-server"`,
		`"child_iid":99`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic issue assign guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"project_id", "issue_iid", "target_full_path"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want group epic issue assign guidance without %q", prompt, unwanted)
		}
	}
}

// TestDefaultFixture_ValidatesAgainstLiveCatalog verifies that DefaultFixture handles the validates against live catalog scenario correctly.
func TestDefaultFixture_ValidatesAgainstLiveCatalog(t *testing.T) {
	tasks, err := parseTasksFile(filepath.Join("..", "..", defaultTasksPath))
	if err != nil {
		t.Fatalf("parseTasksFile() error = %v", err)
	}
	if problems := validateTaskFixture(tasks); len(problems) > 0 {
		t.Fatalf("fixture validation problems = %+v", problems)
	}
	_, routes, err := loadCatalog(options{})
	if err != nil {
		t.Fatalf("loadCatalog() error = %v", err)
	}
	tasks = normalizeTasksForRoutes(tasks, routes)
	tasks = filterTasksByAvailableRoutes(tasks, routes)
	if problems := validateTaskFixtureAgainstRoutes(tasks, routes); len(problems) > 0 {
		t.Fatalf("route validation problems = %+v", problems)
	}
}

// TestLoadCatalog_RejectsUnknownBackend verifies that LoadCatalog handles the rejects unknown backend scenario correctly.
func TestLoadCatalog_RejectsUnknownBackend(t *testing.T) {
	_, _, err := loadCatalog(options{Backend: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("error = %v, want unknown backend", err)
	}
}

// TestRunMCPSmokeRequiresGitLabBackend verifies the behavior of run m c p smoke requires git lab backend.
func TestRunMCPSmokeRequiresGitLabBackend(t *testing.T) {
	err := runMCPSmoke(options{Backend: backendMock})
	if err == nil || !strings.Contains(err.Error(), "--backend=gitlab") {
		t.Fatalf("error = %v, want backend guard", err)
	}
}

// TestValidateExecutionOptionsRequiresDockerGuard verifies the behavior of validate execution options requires docker guard.
func TestValidateExecutionOptionsRequiresDockerGuard(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	err := validateExecutionOptions(options{Backend: backendGitLab})
	if err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("error = %v, want docker guard", err)
	}
	if liveErr := validateExecutionOptions(options{Backend: backendGitLab, AllowLive: true}); liveErr != nil {
		t.Fatalf("validateExecutionOptions(allow live) error = %v", liveErr)
	}
}

// TestValidateExecutionOptions_AllowsExternalCommandWithDockerEnvFile verifies that ValidateExecutionOptions handles the allows external command with docker env file scenario correctly.
func TestValidateExecutionOptions_AllowsExternalCommandWithDockerEnvFile(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	envFile := filepath.Join(t.TempDir(), "docker.env")
	if err := os.WriteFile(envFile, []byte("E2E_MODE=docker\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	err := validateExecutionOptions(options{ToolsFile: "snapshot.json", MCPCommand: "gitlab-mcp-server", MCPEnv: envFile})
	if err != nil {
		t.Fatalf("validateExecutionOptions(external command) error = %v", err)
	}
}

// TestValidateExecutionOptions_ExternalCommandRequiresDockerGuard verifies that ValidateExecutionOptions handles the external command requires docker guard scenario correctly.
func TestValidateExecutionOptions_ExternalCommandRequiresDockerGuard(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	err := validateExecutionOptions(options{ToolsFile: "snapshot.json", MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("error = %v, want external docker guard", err)
	}
}

// TestValidateExecutionOptions_ExternalCommandRequiresToolsFile verifies that ValidateExecutionOptions handles the external command requires tools file scenario correctly.
func TestValidateExecutionOptions_ExternalCommandRequiresToolsFile(t *testing.T) {
	t.Setenv("E2E_MODE", "docker")
	err := validateExecutionOptions(options{MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "requires --tools-file") {
		t.Fatalf("error = %v, want tools-file guard", err)
	}
}

// TestCallFixtureSetupTool_FallsBackToSplitMetaTool verifies that CallFixtureSetupTool handles the falls back to split meta tool scenario correctly.
func TestCallFixtureSetupTool_FallsBackToSplitMetaTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture-test", Version: "0"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_branch", Description: "branch meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		called = true
		if input["action"] != "create" {
			t.Fatalf("action = %v, want create", input["action"])
		}
		params, _ := input["params"].(map[string]any)
		if params["project_id"] != "my-org/tools/gitlab-mcp-server" {
			t.Fatalf("params = %+v", params)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-test-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	err = callFixtureSetupTool(t.Context(), session, config.ToolSurfaceMeta, "branch.create", map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("callFixtureSetupTool() error = %v", err)
	}
	if !called {
		t.Fatal("split meta-tool was not called")
	}
}

// TestEvalCreateMessageHandler_AdvertisesSamplingToMCPServer verifies that EvalCreateMessageHandler handles the advertises sampling to m c p server scenario correctly.
func TestEvalCreateMessageHandler_AdvertisesSamplingToMCPServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "sampling-probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "sampling_probe", Description: "sampling probe"}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		params := req.Session.InitializeParams()
		if params == nil || params.Capabilities.Sampling == nil {
			return nil, nil, errors.New("sampling capability not advertised")
		}
		result, err := req.Session.CreateMessage(ctx, &mcp.CreateMessageParams{
			Messages:  []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "probe"}}},
			MaxTokens: 64,
		})
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result.Model}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "sampling-probe-client", Version: "0"}, &mcp.ClientOptions{
		CreateMessageHandler: evalCreateMessageHandler,
	})
	session, err := mcpClient.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "sampling_probe", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := toolResultContent(result); !strings.Contains(got, "eval-meta-tools-sampling-mock") {
		t.Fatalf("sampling result = %q, want evaluator sampling model", got)
	}
}

// TestEvalElicitationHandler_AdvertisesElicitationToMCPServer verifies that the evaluator client can drive interactive tools.
func TestEvalElicitationHandler_AdvertisesElicitationToMCPServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "elicitation-probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "elicitation_probe", Description: "elicitation probe"}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		params := req.Session.InitializeParams()
		if params == nil || params.Capabilities.Elicitation == nil {
			return nil, nil, errors.New("elicitation capability not advertised")
		}
		result, err := req.Session.Elicit(ctx, &mcp.ElicitParams{
			Message: "probe",
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":     map[string]any{"type": "string"},
					"confirmed": map[string]any{"type": "boolean"},
					"count":     map[string]any{"type": "integer"},
					"enabled":   map[string]any{"type": "boolean"},
					"selection": map[string]any{"type": "string", "enum": []any{"private", "internal"}},
				},
			},
		})
		if err != nil {
			return nil, nil, err
		}
		if result.Action != "accept" || result.Content["confirmed"] != true {
			return nil, nil, fmt.Errorf("elicitation result = %+v, want accepted confirmation", result)
		}
		if _, ok := result.Content["enabled"].(bool); !ok {
			return nil, nil, fmt.Errorf("elicitation enabled = %T, want bool", result.Content["enabled"])
		}
		count, ok := result.Content["count"]
		if !ok {
			return nil, nil, errors.New("elicitation count must be a numeric value")
		}
		switch typed := count.(type) {
		case float64:
			if typed != 0 {
				return nil, nil, fmt.Errorf("elicitation count = %v, want numeric zero", typed)
			}
		case int:
			if typed != 0 {
				return nil, nil, fmt.Errorf("elicitation count = %v, want numeric zero", typed)
			}
		default:
			return nil, nil, fmt.Errorf("elicitation count must be a numeric value, got %T", count)
		}
		if result.Content["selection"] != "private" {
			return nil, nil, fmt.Errorf("elicitation selection = %v, want private", result.Content["selection"])
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprint(result.Content["title"])}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "elicitation-probe-client", Version: "0"}, &mcp.ClientOptions{
		ElicitationHandler: evalElicitationHandler,
	})
	session, err := mcpClient.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "elicitation_probe", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := toolResultContent(result); !strings.Contains(got, "Evaluation elicitation test") {
		t.Fatalf("elicitation result = %q, want evaluator title", got)
	}
}

// TestEvalElicitationSchemaValue_TypeAwareDefaults verifies fallback values
// match schema types, including nested object properties handled outside MCP's
// elicitation primitive-property subset.
func TestEvalElicitationSchemaValue_TypeAwareDefaults(t *testing.T) {
	metadata := evalElicitationSchemaValue("metadata", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"retries": map[string]any{"type": "integer"},
			"dry_run": map[string]any{"type": "boolean"},
		},
	}).(map[string]any)
	if metadata["retries"] != 0 || metadata["dry_run"] != false {
		t.Fatalf("metadata defaults = %#v, want integer and boolean defaults", metadata)
	}

	if got := evalElicitationSchemaValue("visibility", map[string]any{"enum": []any{"private", "internal"}}); got != "private" {
		t.Fatalf("enum default = %v, want private", got)
	}
	if got := evalElicitationSchemaValue("labels", map[string]any{"type": "array"}); !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("array default = %#v, want empty array", got)
	}
}

// TestParseComparisonInput_EvaluationAndTokenReports verifies that ParseComparisonInput handles the evaluation and token reports scenario correctly.
func TestParseComparisonInput_EvaluationAndTokenReports(t *testing.T) {
	tmp := t.TempDir()
	evalPath := filepath.Join(tmp, "current-abc123", "schema-base-read.md")
	if err := os.MkdirAll(filepath.Dir(evalPath), 0o750); err != nil {
		t.Fatalf("mkdir eval: %v", err)
	}
	evalReport := `# Meta-Tool Anthropic Evaluation

Date: 2026-05-04T00:00:00Z
Mode: static route/schema validation
Model: ` + "`claude-sonnet-4-6`" + `
Backend: ` + "`mock`" + `
Tool execution: ` + "`none`" + `
Tools file: ` + "`dist/evaluation/meta-tools/snapshots/current-abc123/tools.json`" + `
Partition: ` + "`base-read`" + `
Catalog tools: 7
Runs: 1
Task attempts: 3

## Metrics

| Metric | Value |
| --- | ---: |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 99.5% |
| First-call validation pass rate | 98.0% |
| Schema lookup use rate | 2.0% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 97.0% |

## Failure Diagnostics

| Category | Count | Example task |
| --- | ---: | --- |
| model_parameter_shape_miss | 1 | MT-001 |

## Fixture Tool Coverage

| Metric | Value |
| --- | ---: |
| Catalog tools | 7 |
| Tools covered by expected steps | 7 |
| Missing tools | 0 |
| Catalog action routes | 851 |
| Action routes covered by expected steps | 200 |
| Missing action routes | 651 |
`
	if err := os.WriteFile(evalPath, []byte(evalReport), 0o600); err != nil {
		t.Fatalf("write eval report: %v", err)
	}
	evalInput, err := parseComparisonInput(evalPath)
	if err != nil {
		t.Fatalf("parseComparisonInput(eval) error = %v", err)
	}
	if evalInput.Kind != "evaluation" || evalInput.Label != "current-abc123" || evalInput.TaskAttempts != 3 {
		t.Fatalf("eval input = %+v", evalInput)
	}
	if evalInput.Metrics["Action-selection accuracy"] != 99.5 || evalInput.Diagnostics["model_parameter_shape_miss"] != 1 || evalInput.Coverage["Missing action routes"] != 651 {
		t.Fatalf("eval metrics = %+v diagnostics=%+v coverage=%+v", evalInput.Metrics, evalInput.Diagnostics, evalInput.Coverage)
	}

	tokenPath := filepath.Join(tmp, "current-abc123", "tokens.md")
	tokenReport := `# Tools Snapshot Token Audit

Tools file: ` + "`dist/evaluation/meta-tools/snapshots/current-abc123/tools.json`" + `

| Metric | Value |
| --- | ---: |
| Tools | 7 |
| Estimated tokens | 18,021 |
| Serialized bytes | 72,071 |
`
	if writeErr := os.WriteFile(tokenPath, []byte(tokenReport), 0o600); writeErr != nil {
		t.Fatalf("write token report: %v", writeErr)
	}
	tokenInput, err := parseComparisonInput(tokenPath)
	if err != nil {
		t.Fatalf("parseComparisonInput(token) error = %v", err)
	}
	if tokenInput.Kind != "token" || tokenInput.TokenMetrics["Estimated tokens"] != 18021 {
		t.Fatalf("token input = %+v", tokenInput)
	}

	comparison := buildComparisonReport([]comparisonInput{tokenInput, evalInput})
	for _, want := range []string{"Catalog Token Metrics", "Evaluation Metrics", "current-abc123", "18021"} {
		if !strings.Contains(comparison, want) {
			t.Fatalf("comparison missing %q:\n%s", want, comparison)
		}
	}
}

// TestToolResultContentPrefersStructuredContent verifies the behavior of tool result content prefers structured content.
func TestToolResultContentPrefersStructuredContent(t *testing.T) {
	result := &mcp.CallToolResult{
		StructuredContent: map[string]any{"username": "e2e-tester"},
		Content:           []mcp.Content{&mcp.TextContent{Text: "markdown fallback"}},
	}
	content := toolResultContent(result)
	if !strings.Contains(content, "e2e-tester") || strings.Contains(content, "markdown fallback") {
		t.Fatalf("content = %q, want structured content", content)
	}
}

// TestFailureDiagnosticCategory_ClassifiesCommonLiveErrors validates failure diagnostic category across multiple scenarios using table-driven subtests
// for the classifies common live errors case.
func TestFailureDiagnosticCategory_ClassifiesCommonLiveErrors(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		want  string
	}{
		{name: "int64 coercion", notes: []string{"json: cannot unmarshal string into Go struct field issue_iid of type int64"}, want: "mcp_implementation_bug"},
		{name: "gitlab 500", notes: []string{"environmentStop: GitLab internal server error: 500"}, want: "transient_gitlab_5xx"},
		{name: "missing resource", notes: []string{"404 Not Found"}, want: "not_found"},
		{name: "provider auth", notes: []string{"qwen status 401: invalid_api_key"}, want: "model_provider_auth"},
		{name: "provider model unavailable", notes: []string{"google status 404: models/gemini-3.0-flash is not found"}, want: "model_provider_model_unavailable"},
		{name: "model validation", notes: []string{"step 2: expected action issue.update, got issue.get"}, want: "model_route_selection_miss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failureDiagnosticCategory(tt.notes); got != tt.want {
				t.Fatalf("failureDiagnosticCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEvaluateTask_UsesSchemaLookupThenFinalCall verifies that EvaluateTask handles the uses schema lookup then final call scenario correctly.
func TestEvaluateTask_UsesSchemaLookupThenFinalCall(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("schema", "gitlab_server", map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "get"}}),
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.SchemaLookupUsed || !result.FinalSuccess || result.ModelCalls != 2 {
		t.Fatalf("result = %+v, want schema lookup and final success in two calls", result)
	}
}

// TestEvaluateTask_Dynamic3AcceptsProjectSearchPrelude verifies that dynamic-3
// evaluation accepts a project search prelude before the required discovery action.
func TestEvaluateTask_Dynamic3AcceptsProjectSearchPrelude(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("search", dynamicExecuteTool, map[string]any{"action": "search.projects", "params": map[string]any{"query": "my-org/tools/gitlab-mcp-server"}}),
		toolUseResponse("resolve", dynamicExecuteTool, map[string]any{"action": "discover_project.resolve", "params": map[string]any{"remote_url": "git@gitlab.example.com:my-org/tools/gitlab-mcp-server.git"}}),
	)
	runner.toolSurface = config.ToolSurfaceDynamic3
	task := evalTask{ID: "MS-002", Steps: []evalStep{{ExpectedTool: dynamicExecuteTool, ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}}}}
	routes := map[string]toolutil.ActionMap{dynamicExecuteTool: {
		"search.projects":          {},
		"discover_project.resolve": {},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if result.RepairAttempted {
		t.Fatalf("result = %+v, want accepted prelude without repair", result)
	}
	if !result.FirstPass || !result.FinalSuccess {
		t.Fatalf("result = %+v, want accepted prelude and final success", result)
	}
	if result.FirstAction != "search.projects" {
		t.Fatalf("FirstAction = %q, want preserved prelude action", result.FirstAction)
	}
}

// TestEffectiveFirstOutcome_AcceptsDynamic3FallbackAndPreludePaths verifies that
// dynamic-3 first-pass scoring accepts supported prelude and fallback paths.
func TestEffectiveFirstOutcome_AcceptsDynamic3FallbackAndPreludePaths(t *testing.T) {
	t.Run("project search prelude", func(t *testing.T) {
		result := taskResult{
			Task:         evalTask{ID: "MS-002", Steps: []evalStep{{ExpectedTool: dynamicExecuteTool, ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}}}},
			ToolSurface:  config.ToolSurfaceDynamic3,
			FirstTool:    dynamicExecuteTool,
			FirstAction:  "search.projects",
			FirstPass:    true,
			FinalSuccess: false,
		}
		toolOK, actionOK, firstPassOK := effectiveFirstOutcome(result)
		if !toolOK || !actionOK || !firstPassOK {
			t.Fatalf("effectiveFirstOutcome() = %t, %t, %t, want all true", toolOK, actionOK, firstPassOK)
		}
	})

	t.Run("unsupported analyzer fallback", func(t *testing.T) {
		result := taskResult{
			Task: evalTask{ID: "MF-004", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "analyze.issue_summary", RequiredParams: []string{"project_id", "issue_iid"}, Simulation: "sampling_unsupported_continue"},
				{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.get", RequiredParams: []string{"project_id", "issue_iid"}},
			}},
			ToolSurface:  config.ToolSurfaceDynamic3,
			FirstTool:    dynamicExecuteTool,
			FirstAction:  "issue.get",
			FirstPass:    false,
			FinalSuccess: true,
		}
		toolOK, actionOK, firstPassOK := effectiveFirstOutcome(result)
		if !toolOK || !actionOK || !firstPassOK {
			t.Fatalf("effectiveFirstOutcome() = %t, %t, %t, want all true", toolOK, actionOK, firstPassOK)
		}
	})
}

// TestEvaluateTask_RecordsTraceForPromptToolUseAndValidation verifies that EvaluateTask handles the records trace for prompt tool use and validation scenario correctly.
func TestEvaluateTask_RecordsTraceForPromptToolUseAndValidation(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if result.Trace.TaskID != task.ID || !strings.Contains(result.Trace.UserPrompt, task.Prompt) {
		t.Fatalf("trace prompt = %+v, want task prompt recorded", result.Trace)
	}
	wantKinds := []string{"user_prompt", "assistant_message", "tool_use", "validation"}
	for _, kind := range wantKinds {
		if !traceHasKind(result.Trace, kind) {
			t.Fatalf("trace events = %+v, want kind %s", result.Trace.Events, kind)
		}
	}
	assistantEvent, ok := traceEventByKind(result.Trace, "assistant_message")
	if !ok || assistantEvent.Provider == nil {
		t.Fatalf("trace events = %+v, want assistant provider exchange", result.Trace.Events)
	}
	if !strings.Contains(string(assistantEvent.Provider.RequestBody), `"system"`) {
		t.Fatalf("provider request = %s, want raw system prompt payload", assistantEvent.Provider.RequestBody)
	}
	if !strings.Contains(string(assistantEvent.Provider.ResponseBody), `"tool_use"`) {
		t.Fatalf("provider response = %s, want raw model response", assistantEvent.Provider.ResponseBody)
	}
}

// TestEvaluateTask_RepairsUnknownSchemaParam verifies that EvaluateTask handles the repairs unknown schema param scenario correctly.
func TestEvaluateTask_RepairsUnknownSchemaParam(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("bad", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "iid": 7}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after schema validation error", result)
	}
}

// TestEvaluateTask_InvalidMatchingCallUsesMCPErrorWhenExecuting verifies that EvaluateTask handles the invalid matching call uses m c p error when executing scenario correctly.
func TestEvaluateTask_InvalidMatchingCallUsesMCPErrorWhenExecuting(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("bad", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newProjectGetSession(t)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after MCP error", result)
	}
	if !traceContainsToolResult(result.Trace, "MCP missing params.project_id") {
		t.Fatalf("trace events = %+v, want real MCP error content", result.Trace.Events)
	}
	toolResultEvent, ok := traceEventByKind(result.Trace, "tool_result")
	if !ok || toolResultEvent.MCP == nil {
		t.Fatalf("trace events = %+v, want MCP exchange on tool result", result.Trace.Events)
	}
	if toolResultEvent.MCP.Request.Name != "gitlab_project" || !toolResultEvent.MCP.IsError {
		t.Fatalf("MCP exchange = %+v, want gitlab_project error", toolResultEvent.MCP)
	}
	if !strings.Contains(string(toolResultEvent.MCP.Response), "MCP missing params.project_id") {
		t.Fatalf("MCP response = %s, want complete tool result", toolResultEvent.MCP.Response)
	}
}

// TestEvaluateTask_WrongReadOnlyCallUsesMCPWhenExecuting verifies that EvaluateTask handles the wrong read only call uses m c p when executing scenario correctly.
func TestEvaluateTask_WrongReadOnlyCallUsesMCPWhenExecuting(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("search", "gitlab_search", map[string]any{"action": "projects", "params": map[string]any{"query": "my-org/tools/gitlab-mcp-server"}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newProjectGetSession(t)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": projectGetRoute()},
		"gitlab_search":  {"projects": toolutil.ActionRoute{}},
	}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after read-only MCP prefetch", result)
	}
	if !traceContainsToolResult(result.Trace, "search ok") {
		t.Fatalf("trace events = %+v, want real search result content", result.Trace.Events)
	}
}

// TestCanExecuteInvalidToolCallSkipsUnexpectedMutations verifies the behavior of can execute invalid tool call skips unexpected mutations.
func TestCanExecuteInvalidToolCallSkipsUnexpectedMutations(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_create", RequiredParams: []string{"project_id", "merge_request_iid", "body"}}
	validation := validationResult{ToolMatches: true, ActionMatches: false, Action: "discussion_create", RequiredPresent: true, DestructiveSafe: true}
	toolUse := modelContentBlock{Name: "gitlab_mr_review"}
	routes := map[string]toolutil.ActionMap{"gitlab_mr_review": {"discussion_create": toolutil.ActionRoute{}}}

	if runner.canExecuteInvalidToolCall(step, validation, toolUse, routes) {
		t.Fatal("canExecuteInvalidToolCall() = true, want unexpected create action to receive repair guidance instead of execution")
	}
}

// TestCanExecuteInvalidToolCallSkipsUnknownParams verifies the behavior of can execute invalid tool call skips unknown params.
func TestCanExecuteInvalidToolCallSkipsUnknownParams(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_create", RequiredParams: []string{"project_id", "description"}}
	validation := validationResult{ToolMatches: true, ActionMatches: true, Action: "trigger_create", RequiredPresent: true, DestructiveSafe: true, Message: "unknown params for gitlab_pipeline/trigger_create: ref"}
	toolUse := modelContentBlock{Name: "gitlab_pipeline"}
	routes := map[string]toolutil.ActionMap{"gitlab_pipeline": {"trigger_create": toolutil.ActionRoute{}}}

	if runner.canExecuteInvalidToolCall(step, validation, toolUse, routes) {
		t.Fatal("canExecuteInvalidToolCall() = true, want unknown params to receive exact repair guidance instead of MCP execution")
	}
}

// TestCanExecuteInvalidToolCallSkipsIncompleteDynamicCalls verifies malformed
// dynamic envelopes receive evaluator repair feedback instead of repeated MCP
// schema errors.
func TestCanExecuteInvalidToolCallSkipsIncompleteDynamicCalls(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: dynamicExecuteTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}}
	routes := map[string]toolutil.ActionMap{dynamicExecuteTool: {"issue.create": toolutil.ActionRoute{}}}

	tests := []struct {
		name       string
		validation validationResult
		toolUse    modelContentBlock
	}{
		{
			name:       "missing top-level params",
			validation: validationResult{ToolMatches: true, ActionMatches: true, Action: "issue.create", RequiredPresent: false, DestructiveSafe: true, Message: `validating "arguments": validating root: required: missing properties: ["params"]`},
			toolUse:    modelContentBlock{Name: dynamicExecuteTool, Input: map[string]any{"action": "issue.create"}},
		},
		{
			name:       "missing nested required param",
			validation: validationResult{ToolMatches: true, ActionMatches: true, Action: "issue.create", RequiredPresent: false, DestructiveSafe: true, Message: "missing required params.title"},
			toolUse:    modelContentBlock{Name: dynamicExecuteTool, Input: map[string]any{"action": "issue.create", "params": map[string]any{"project_id": 1}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runner.canExecuteInvalidToolCall(step, tt.validation, tt.toolUse, routes) {
				t.Fatal("canExecuteInvalidToolCall() = true, want incomplete dynamic call to receive exact repair guidance")
			}
		})
	}
}

// TestEvaluateTask_RepairsMultipleInvalidToolCallsFromSameTurn verifies that EvaluateTask handles the repairs multiple invalid tool calls from same turn scenario correctly.
func TestEvaluateTask_RepairsMultipleInvalidToolCallsFromSameTurn(t *testing.T) {
	runner := newScriptedRunner(t,
		multiToolUseResponse(
			modelContentBlock{Type: "tool_use", ID: "bad-project", Name: "gitlab", Input: map[string]any{"action": "project.get", "project_id": "my-org/tools/gitlab-mcp-server"}},
			modelContentBlock{Type: "tool_use", ID: "bad-file", Name: "gitlab", Input: map[string]any{"action": "repository.file_get", "project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}},
		),
		toolUseResponse("good-project", "gitlab", map[string]any{"action": "project.get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
		toolUseResponse("good-file", "gitlab", map[string]any{"action": "repository.file_get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}}),
	)
	task := evalTask{ID: "MS-001", Steps: []evalStep{
		{ExpectedTool: "gitlab", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}},
		{ExpectedTool: "gitlab", ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
	}}
	routes := map[string]toolutil.ActionMap{"gitlab": {"project.get": projectGetRoute(), "repository.file_get": repositoryFileGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after same-turn invalid tool calls", result)
	}
}

// TestEvaluateTask_RetriesTransientSimulation verifies that EvaluateTask handles the retries transient simulation scenario correctly.
func TestEvaluateTask_RetriesTransientSimulation(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("first", "gitlab_pipeline", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "pipeline_id": 12345}}),
		toolUseResponse("retry", "gitlab_pipeline", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "pipeline_id": 12345}}),
	)
	task := evalTask{ID: "MF-001", ExpectedTool: "gitlab_pipeline", ExpectedAction: "get", RequiredParams: []string{"project_id", "pipeline_id"}, Simulation: "transient_error_once"}
	routes := map[string]toolutil.ActionMap{"gitlab_pipeline": {"get": pipelineGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess || result.CompletedSteps != 1 {
		t.Fatalf("result = %+v, want transient retry success", result)
	}
}

// TestEvaluateTask_PoisonedOutputDoesNotChangeNextExpectedTool verifies that EvaluateTask handles the poisoned output does not change next expected tool scenario correctly.
func TestEvaluateTask_PoisonedOutputDoesNotChangeNextExpectedTool(t *testing.T) {
	runner := newScriptedRunner(t,
		toolUseResponse("file", "gitlab_repository", map[string]any{"action": "file_get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}}),
		toolUseResponse("project", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MF-002", Steps: []evalStep{
		{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}, Simulation: "poisoned_output"},
		{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_repository": {"file_get": repositoryFileGetRoute()},
		"gitlab_project":    {"get": projectGetRoute()},
	}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.FinalSuccess || result.CompletedSteps != 2 || result.FinalTool != "gitlab_project" {
		t.Fatalf("result = %+v, want poisoned output ignored and second step completed", result)
	}
}

// TestCalculateMetrics_HandlesNoRepairs verifies that CalculateMetrics handles the handles no repairs scenario correctly.
func TestCalculateMetrics_HandlesNoRepairs(t *testing.T) {
	results := []taskResult{{
		Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
		FirstTool:       "gitlab_user",
		FirstAction:     "current",
		FirstPass:       true,
		FinalSuccess:    true,
		DestructiveSafe: true,
	}}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 100 || measured.ActionSelection != 100 || measured.RepairSuccess != 100 {
		t.Fatalf("metrics = %+v, want all applicable metrics at 100", measured)
	}
}

// TestCalculateMetrics_AggregatesRepeatedAttempts verifies that CalculateMetrics handles the aggregates repeated attempts scenario correctly.
func TestCalculateMetrics_AggregatesRepeatedAttempts(t *testing.T) {
	results := []taskResult{
		{
			Run:             1,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_user",
			FirstAction:     "current",
			FirstPass:       true,
			FinalSuccess:    true,
			DestructiveSafe: true,
		},
		{
			Run:             2,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_project",
			FirstAction:     "get",
			FinalSuccess:    false,
			DestructiveSafe: true,
		},
	}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 50 || measured.ActionSelection != 50 || measured.FinalSuccess != 50 {
		t.Fatalf("metrics = %+v, want repeated attempts aggregated at 50%%", measured)
	}
}

// TestAggregateUsage_SumsRequestsToolCallsAndTokens verifies that AggregateUsage handles the sums requests tool calls and tokens scenario correctly.
func TestAggregateUsage_SumsRequestsToolCallsAndTokens(t *testing.T) {
	results := []taskResult{
		{ModelCalls: 2, ToolCalls: 3, Usage: modelUsage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 50}},
		{ModelCalls: 1, ToolCalls: 1, Usage: modelUsage{InputTokens: 25, OutputTokens: 5, CacheReadInputTokens: 200}},
	}
	summary := aggregateUsage(results)
	if summary.ModelCalls != 3 || summary.ToolCalls != 4 {
		t.Fatalf("summary calls = %+v, want 3 requests and 4 tool calls", summary)
	}
	if summary.Usage.InputTokens != 125 || summary.Usage.OutputTokens != 25 || summary.Usage.CacheCreationInputTokens != 50 || summary.Usage.CacheReadInputTokens != 200 {
		t.Fatalf("usage = %+v, want summed tokens", summary.Usage)
	}
}

// TestEstimateCostUSD_UsesPerMillionPricing verifies that EstimateCostUSD handles the uses per million pricing scenario correctly.
func TestEstimateCostUSD_UsesPerMillionPricing(t *testing.T) {
	cost := estimateCostUSD(modelUsage{InputTokens: 1_000_000, OutputTokens: 100_000}, pricingOptions{InputPerMTok: 3, OutputPerMTok: 15})
	if cost != 4.5 {
		t.Fatalf("cost = %v, want 4.5", cost)
	}
}

// TestWriteTraceArtifacts_WritesJSONLIndexAndPerTaskFiles verifies that WriteTraceArtifacts handles the writes j s o n l index and per task files scenario correctly.
func TestWriteTraceArtifacts_WritesJSONLIndexAndPerTaskFiles(t *testing.T) {
	trace := taskTrace{
		Run:          2,
		TaskID:       "MT-002",
		Prompt:       "Find a project.",
		SystemPrompt: systemPrompt(),
		UserPrompt:   "Task MT-002: Find a project.",
		Expected:     []traceExpectedStep{{Step: 1, Tool: "gitlab_project", Action: "get", RequiredParams: []string{"project_id"}}},
		Events:       []traceEvent{{Turn: 1, Kind: "tool_use", Tool: "gitlab_project", Action: "get"}},
		Summary:      traceSummary{FinalSuccess: true, FirstPass: true, CompletedSteps: 1, ExpectedSteps: 1},
	}
	dir := t.TempDir()
	if err := writeTraceArtifacts(dir, []taskResult{{Trace: trace}}, false); err != nil {
		t.Fatalf("writeTraceArtifacts() error = %v", err)
	}

	for _, name := range []string{"index.md", "traces.jsonl", "run-002-MT-002.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "MT-002") {
			t.Fatalf("%s = %s, want task ID", name, data)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	if strings.Contains(string(index), "provider HTTP request/response bodies") {
		t.Fatalf("index.md = %s, should not promise raw provider bodies when traceProviderBodies=false", index)
	}
}

// TestDefaultTraceDir_ReplacesReportExtension verifies that DefaultTraceDir handles the replaces report extension scenario correctly.
func TestDefaultTraceDir_ReplacesReportExtension(t *testing.T) {
	got := defaultTraceDir("dist/evaluation/meta-tools/report.md")
	if got != "dist/evaluation/meta-tools/report.traces" {
		t.Fatalf("defaultTraceDir() = %q, want report.traces", got)
	}
}

// TestDefaultOutputPath_UsesIgnoredDistDirectory verifies that DefaultOutputPath handles the uses ignored dist directory scenario correctly.
func TestDefaultOutputPath_UsesIgnoredDistDirectory(t *testing.T) {
	got := defaultOutputPath("claude/sonnet:4 6")
	if !strings.HasPrefix(got, "dist/evaluation/meta-tools/model-") {
		t.Fatalf("defaultOutputPath() = %q, want dist evaluation path", got)
	}
	if !strings.HasSuffix(got, "-claude-sonnet-4-6.md") {
		t.Fatalf("defaultOutputPath() = %q, want sanitized model suffix", got)
	}
}

// TestDefaultOutputPath_UsesShortNameForMultiModel verifies that DefaultOutputPath handles the uses short name for multi model scenario correctly.
func TestDefaultOutputPath_UsesShortNameForMultiModel(t *testing.T) {
	got := defaultOutputPath("anthropic:claude-sonnet-4-6,openai:gpt-5.4-mini")
	if !strings.HasSuffix(got, "-multi-model.md") {
		t.Fatalf("defaultOutputPath() = %q, want multi-model suffix", got)
	}
}

// TestResolveModelSpecs_UsesEvalModels verifies that ResolveModelSpecs handles the uses eval models scenario correctly.
func TestResolveModelSpecs_UsesEvalModels(t *testing.T) {
	t.Setenv("EVAL_MODELS", "anthropic:claude-sonnet-4-6, google:gemini-3.0-flash, openai:gpt-5.4-mini, qwen:qwen3.6-flash")
	specs, err := resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	got := modelReportLabel(specs)
	want := "anthropic:claude-sonnet-4-6,google:gemini-3.0-flash,openai:gpt-5.4-mini,qwen:qwen3.6-flash"
	if got != want {
		t.Fatalf("modelReportLabel() = %q, want %q", got, want)
	}
}

// TestResolveModelSpecs_IgnoresEmptyEntries verifies that ResolveModelSpecs handles the ignores empty entries scenario correctly.
func TestResolveModelSpecs_IgnoresEmptyEntries(t *testing.T) {
	t.Setenv("EVAL_MODELS", "anthropic:claude-sonnet-4-6,")
	specs, err := resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	if len(specs) != 1 || specs[0].String() != "anthropic:claude-sonnet-4-6" {
		t.Fatalf("specs = %+v, want single model", specs)
	}
}

// TestResolveModelSpecs_ModelFlagOverridesEvalModels verifies that ResolveModelSpecs handles the model flag overrides eval models scenario correctly.
func TestResolveModelSpecs_ModelFlagOverridesEvalModels(t *testing.T) {
	t.Setenv("EVAL_MODELS", "google:gemini-3.0-flash")
	specs, err := resolveModelSpecs(options{Model: "claude-haiku-4-6"})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	if len(specs) != 1 || specs[0].Provider != providerAnthropic || specs[0].Model != "claude-haiku-4-6" {
		t.Fatalf("specs = %+v, want single legacy Anthropic model", specs)
	}
}

// TestParseModelSpec_RejectsUnsupportedProvider verifies that ParseModelSpec handles the rejects unsupported provider scenario correctly.
func TestParseModelSpec_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseModelSpec("local:llama")
	if err == nil || !strings.Contains(err.Error(), "unsupported model provider") {
		t.Fatalf("error = %v, want unsupported provider", err)
	}
}

// TestParseModelSpec_StripsGoogleModelsPrefix verifies that ParseModelSpec handles the strips google models prefix scenario correctly.
func TestParseModelSpec_StripsGoogleModelsPrefix(t *testing.T) {
	spec, err := parseModelSpec("google:models/gemini-3-flash-preview")
	if err != nil {
		t.Fatalf("parseModelSpec() error = %v", err)
	}
	if spec.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want trimmed Gemini model", spec.Model)
	}
}

// TestAPIKeyForModelProvider_RequiresQwenAPIKey verifies that APIKeyForModelProvider handles the requires qwen a p i key scenario correctly.
func TestAPIKeyForModelProvider_RequiresQwenAPIKey(t *testing.T) {
	t.Setenv("QWEN_API_KEY", "")
	_, err := apiKeyForModelProvider(providerQwen)
	if err == nil {
		t.Fatal("apiKeyForModelProvider() error = nil, want missing QWEN_API_KEY")
	}
	if !strings.Contains(err.Error(), "QWEN_API_KEY") {
		t.Fatalf("error = %v, want QWEN_API_KEY", err)
	}
}

// TestQwenEndpoint_UsesConfiguredBaseURL verifies that QwenEndpoint handles the uses configured base u r l scenario correctly.
func TestQwenEndpoint_UsesConfiguredBaseURL(t *testing.T) {
	t.Setenv("QWEN_CHAT_COMPLETIONS_URL", "")
	t.Setenv("QWEN_BASE_URL", "https://example.test/v1/")
	if got := qwenEndpoint(); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("qwenEndpoint() = %q", got)
	}
}

// TestOpenAIProvider_CallOnceConvertsToolCall verifies that OpenAIProvider handles the call once converts tool call scenario correctly.
func TestOpenAIProvider_CallOnceConvertsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want bearer", got)
		}
		var request openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "gpt-test" || len(request.Tools) != 1 || request.ToolChoice != "required" {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": `{"action":"user.current","params":{}}`,
				},
			}}}}},
			"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerOpenAI, maxTokenField: "max_completion_tokens"}
	response, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "gpt-test",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})
	if err != nil || retry {
		t.Fatalf("callOnce() retry=%v error=%v", retry, err)
	}
	if response.Usage.InputTokens != 11 || response.Usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v", response.Usage)
	}
	if len(response.Content) != 1 || response.Content[0].Name != "gitlab" || response.Content[0].Input["action"] != "user.current" {
		t.Fatalf("content = %+v", response.Content)
	}
}

// TestOpenAIProvider_QwenDisablesThinking verifies that OpenAIProvider handles the qwen disables thinking scenario correctly.
func TestOpenAIProvider_QwenDisablesThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.EnableThinking == nil || *request.EnableThinking {
			t.Fatalf("enable_thinking = %v, want false", request.EnableThinking)
		}
		if request.ToolChoice != "required" || request.MaxTokens == 0 {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": `{"action":"user.current","params":{}}`,
				},
			}}}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}
	_, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "qwen3.6-flash",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})
	if err != nil || retry {
		t.Fatalf("callOnce() retry=%v error=%v", retry, err)
	}
}

// TestOpenAIProvider_EmptyToolArgumentsAreRetryable verifies that OpenAIProvider handles the empty tool arguments are retryable scenario correctly.
func TestOpenAIProvider_EmptyToolArgumentsAreRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": "",
				},
			}}}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}
	_, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "qwen3.6-flash",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})

	if err == nil || !retry {
		t.Fatalf("callOnce() retry=%v error=%v, want retryable empty arguments error", retry, err)
	}
}

// TestOpenAIToolUseBlocks_RepairsLeadingCommaArguments verifies that OpenAIToolUseBlocks handles the repairs leading comma arguments scenario correctly.
func TestOpenAIToolUseBlocks_RepairsLeadingCommaArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab",
			Arguments: `, "action":"project.milestone_create","params":{"project_id":"my-org/tools/gitlab-mcp-server","title":"Evaluation Sprint"}`,
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "project.milestone_create" {
		t.Fatalf("action = %v, want project.milestone_create", blocks[0].Input["action"])
	}
}

// TestOpenAIToolUseBlocks_RepairsInterleavedLeadingCommaArguments verifies that OpenAIToolUseBlocks handles the repairs interleaved leading comma arguments scenario correctly.
func TestOpenAIToolUseBlocks_RepairsInterleavedLeadingCommaArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab",
			Arguments: " , \n, \"action\":\"merge_request.create\",\"params\":{\"project_id\":\"my-org/tools/gitlab-mcp-server\",\"source_branch\":\"feature/eval\",\"target_branch\":\"main\",\"title\":\"Evaluation MR\"}, ",
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "merge_request.create" {
		t.Fatalf("action = %v, want merge_request.create", blocks[0].Input["action"])
	}
}

// TestOpenAIToolUseBlocks_ExtractsWrappedJSONArguments verifies that OpenAIToolUseBlocks handles the extracts wrapped j s o n arguments scenario correctly.
func TestOpenAIToolUseBlocks_ExtractsWrappedJSONArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab_analyze",
			Arguments: `<tool_call>{"action":"pipeline_failure","params":{"project_id":"my-org/tools/gitlab-mcp-server","pipeline_id":12345}}</tool_call>`,
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "pipeline_failure" {
		t.Fatalf("action = %v, want pipeline_failure", blocks[0].Input["action"])
	}
}

// TestGoogleContentConversion_RoundTripsFunctionResponseNames verifies that GoogleContentConversion handles the round trips function response names scenario correctly.
func TestGoogleContentConversion_RoundTripsFunctionResponseNames(t *testing.T) {
	messages := []modelMessage{
		{Role: "assistant", Content: []modelContentBlock{{Type: "tool_use", ID: "call-1", Name: "gitlab", Input: map[string]any{"action": "user.current"}, ThoughtSignature: "thought-token"}}},
		{Role: "user", Content: []modelContentBlock{{Type: "tool_result", ToolUseID: "call-1", Content: "ok"}}},
	}
	contents := googleContents(messages)
	if len(contents) != 2 || len(contents[1].Parts) != 1 || contents[1].Parts[0].FunctionResponse == nil {
		t.Fatalf("contents = %+v", contents)
	}
	if contents[1].Parts[0].FunctionResponse.Name != "gitlab" {
		t.Fatalf("function response name = %q, want gitlab", contents[1].Parts[0].FunctionResponse.Name)
	}
	if contents[1].Parts[0].FunctionResponse.ID != "call-1" {
		t.Fatalf("function response id = %q, want call-1", contents[1].Parts[0].FunctionResponse.ID)
	}
	if contents[0].Parts[0].ThoughtSignature != "thought-token" {
		t.Fatalf("thought signature = %q, want preserved", contents[0].Parts[0].ThoughtSignature)
	}

	blocks := googleToolUseBlocks(googleContent{Parts: []googlePart{{ThoughtSignature: "thought-token", FunctionCall: &googleFunctionCall{Name: "gitlab", Args: map[string]any{"action": "user.current"}, ID: "call-1"}}}})
	if len(blocks) != 1 || blocks[0].Name != "gitlab" || blocks[0].Input["action"] != "user.current" {
		t.Fatalf("blocks = %+v", blocks)
	}
	if blocks[0].ID != "call-1" {
		t.Fatalf("id = %q, want call-1", blocks[0].ID)
	}
	if blocks[0].ThoughtSignature != "thought-token" {
		t.Fatalf("thought signature = %q, want preserved", blocks[0].ThoughtSignature)
	}

	contentBlocks := googleContentBlocks(googleContent{Parts: []googlePart{{Text: "plain response"}, {FunctionCall: &googleFunctionCall{Name: "gitlab", Args: map[string]any{"action": "user.current"}, ID: "call-2"}}}})
	if len(contentBlocks) != 2 || contentBlocks[0].Type != "text" || contentBlocks[0].Text != "plain response" || contentBlocks[1].Type != "tool_use" {
		t.Fatalf("content blocks = %+v, want text block followed by tool_use", contentBlocks)
	}
}

// TestGoogleFunctionCallingMode_DefaultsToValidated verifies that GoogleFunctionCallingMode handles the defaults to validated scenario correctly.
func TestGoogleFunctionCallingMode_DefaultsToValidated(t *testing.T) {
	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "")
	if got := googleFunctionCallingMode(); got != "VALIDATED" {
		t.Fatalf("googleFunctionCallingMode() = %q, want VALIDATED", got)
	}

	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "auto")
	if got := googleFunctionCallingMode(); got != "AUTO" {
		t.Fatalf("googleFunctionCallingMode() override = %q, want AUTO", got)
	}
}

// TestSanitizeGoogleSchema_FlattensTypeUnion verifies that SanitizeGoogleSchema handles the flattens type union scenario correctly.
func TestSanitizeGoogleSchema_FlattensTypeUnion(t *testing.T) {
	schema := map[string]any{
		"type": []any{"string", "integer"},
		"properties": map[string]any{
			"project_id": map[string]any{"type": []any{"string", "integer"}},
		},
	}

	got := sanitizeGoogleSchema(schema).(map[string]any)
	if got["type"] != "string" {
		t.Fatalf("type = %#v, want string", got["type"])
	}
	properties := got["properties"].(map[string]any)
	projectID := properties["project_id"].(map[string]any)
	if projectID["type"] != "string" {
		t.Fatalf("project_id.type = %#v, want string", projectID["type"])
	}
}

// TestSanitizeGoogleSchema_PreservesTitleProperty verifies that SanitizeGoogleSchema handles the preserves title property scenario correctly.
func TestSanitizeGoogleSchema_PreservesTitleProperty(t *testing.T) {
	schema := map[string]any{
		"title": "Root schema title",
		"type":  "object",
		"properties": map[string]any{
			"title": map[string]any{
				"title":       "Property schema title",
				"type":        "string",
				"description": "Issue title.",
			},
		},
	}

	got := sanitizeGoogleSchema(schema).(map[string]any)
	if _, ok := got["title"]; ok {
		t.Fatalf("root schema title should be removed: %#v", got)
	}
	properties := got["properties"].(map[string]any)
	title, ok := properties["title"].(map[string]any)
	if !ok {
		t.Fatalf("properties.title missing after sanitize: %#v", properties)
	}
	if _, hasTitleKeyword := title["title"]; hasTitleKeyword {
		t.Fatalf("property schema title keyword should be removed: %#v", title)
	}
	if title["type"] != "string" {
		t.Fatalf("properties.title.type = %#v, want string", title["type"])
	}
}

// TestGoogleEmptyResponseError_IncludesFinishAndBlockReasons verifies that GoogleEmptyResponseError handles the includes finish and block reasons scenario correctly.
func TestGoogleEmptyResponseError_IncludesFinishAndBlockReasons(t *testing.T) {
	decoded := googleResponse{}
	decoded.Candidates = append(decoded.Candidates, struct {
		Content       googleContent `json:"content"`
		FinishReason  string        `json:"finishReason,omitempty"`
		FinishMessage string        `json:"finishMessage,omitempty"`
	}{FinishReason: "MALFORMED_FUNCTION_CALL", FinishMessage: "malformed tool call"})
	decoded.PromptFeedback = &struct {
		BlockReason        string `json:"blockReason,omitempty"`
		BlockReasonMessage string `json:"blockReasonMessage,omitempty"`
	}{BlockReason: "SAFETY", BlockReasonMessage: "blocked"}

	err := googleEmptyResponseError(decoded, "no tool calls or output tokens")
	message := err.Error()
	for _, want := range []string{"no tool calls or output tokens", "finishReason=MALFORMED_FUNCTION_CALL", "finishMessage=malformed tool call", "blockReason=SAFETY", "blockReasonMessage=blocked"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q", message, want)
		}
	}
}

// TestGoogleResponseDecode_PreservesNestedParams verifies that GoogleResponseDecode handles the preserves nested params scenario correctly.
func TestGoogleResponseDecode_PreservesNestedParams(t *testing.T) {
	raw := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"gitlab_project","args":{"action":"get","params":{"project_id":"my-org/tools/gitlab-mcp-server"}},"id":"call-1"}}]}}]}`)
	var decoded googleResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode google response: %v", err)
	}

	blocks := googleToolUseBlocks(decoded.Candidates[0].Content)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %+v, want one tool call", blocks)
	}
	params, ok := blocks[0].Input["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v, want object", blocks[0].Input["params"])
	}
	if params["project_id"] != "my-org/tools/gitlab-mcp-server" {
		t.Fatalf("project_id = %#v", params["project_id"])
	}
	if string(blocks[0].ProviderRawInput) != `{"action":"get","params":{"project_id":"my-org/tools/gitlab-mcp-server"}}` {
		t.Fatalf("raw input = %s", blocks[0].ProviderRawInput)
	}
}

// roundTripFunc holds data for main operations.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip performs the round trip operation on roundTripFunc.
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newScriptedRunner is an internal helper for the main package.
func newScriptedRunner(t *testing.T, responses ...modelResponse) *modelRunner {
	t.Helper()
	index := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if index >= len(responses) {
			t.Fatalf("unexpected model request %d; scripted responses exhausted", index+1)
		}
		body, err := json.Marshal(responses[index])
		if err != nil {
			t.Fatalf("marshal scripted response: %v", err)
		}
		index++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})}
	t.Cleanup(func() {
		if index != len(responses) {
			t.Fatalf("used %d scripted responses, want %d", index, len(responses))
		}
	})
	return &modelRunner{apiKey: "test-key", model: "test-model", maxTokens: 256, client: client, traceBodies: true}
}

// newProjectGetSession is an internal helper for the main package.
func newProjectGetSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_project", Description: "project meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		params, _ := input["params"].(map[string]any)
		if params["project_id"] == nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "MCP missing params.project_id"}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "project ok"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_search", Description: "search meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "search ok"}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "eval-test-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// toolUseResponse converts the GitLab API response to the tool output format.
func toolUseResponse(id, name string, input map[string]any) modelResponse {
	return modelResponse{Content: []modelContentBlock{{Type: "tool_use", ID: id, Name: name, Input: input}}}
}

// multiToolUseResponse is an internal helper for the main package.
func multiToolUseResponse(blocks ...modelContentBlock) modelResponse {
	return modelResponse{Content: blocks}
}

// traceHasKind is an internal helper for the main package.
func traceHasKind(trace taskTrace, kind string) bool {
	for _, event := range trace.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

// traceEventByKind returns the first trace event with the requested kind.
func traceEventByKind(trace taskTrace, kind string) (traceEvent, bool) {
	for _, event := range trace.Events {
		if event.Kind == kind {
			return event, true
		}
	}
	return traceEvent{}, false
}

// traceContainsToolResult is an internal helper for the main package.
func traceContainsToolResult(trace taskTrace, text string) bool {
	for _, event := range trace.Events {
		if event.Kind == "tool_result" && strings.Contains(event.Content, text) {
			return true
		}
	}
	return false
}

// projectGetRoute is an internal helper for the main package.
func projectGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}}
}

// pipelineGetRoute is an internal helper for the main package.
func pipelineGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string"},
			"pipeline_id": map[string]any{"type": "integer"},
		},
	}}
}

// repositoryFileGetRoute is an internal helper for the main package.
func repositoryFileGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"file_path":  map[string]any{"type": "string"},
			"ref":        map[string]any{"type": "string"},
		},
	}}
}
