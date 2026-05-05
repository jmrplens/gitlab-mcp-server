package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

func TestFilterTasksByDestructive_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByDestructive(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByDestructive() error = nil, want conflict")
	}
}

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

func TestFilterTasksByMutation_RejectsConflictingFlags(t *testing.T) {
	_, err := filterTasksByMutation(nil, true, true)
	if err == nil {
		t.Fatal("filterTasksByMutation() error = nil, want conflict")
	}
}

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

func TestRouteLooksMutating_IgnoresDomainTokens(t *testing.T) {
	if routeLooksMutating("gitlab", "merge_request.list") {
		t.Fatal("merge_request.list should be read-only")
	}
	if !routeLooksMutating("gitlab", "merge_request.merge") {
		t.Fatal("merge_request.merge should be mutating")
	}
}

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

func TestApplyPresetDefaults_RejectsUnknownPreset(t *testing.T) {
	_, err := applyPresetDefaults(options{Preset: "surprise"})
	if err == nil {
		t.Fatal("applyPresetDefaults() error = nil, want unknown preset error")
	}
}

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

func TestTaskToolCallLimit_ScalesForLongWorkflows(t *testing.T) {
	if got := taskToolCallLimit(3); got != toolCallLimit {
		t.Fatalf("taskToolCallLimit(3) = %d, want baseline %d", got, toolCallLimit)
	}
	if got := taskToolCallLimit(8); got != 20 {
		t.Fatalf("taskToolCallLimit(8) = %d, want enough turns for schema lookups and 8 steps", got)
	}
}

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

func taskIDs(tasks []evalTask) string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return strings.Join(ids, ",")
}

func TestBuildCatalogSession_UsesClientEnterpriseMode(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, _, routes, err := buildCatalogSession(client)
	if err != nil {
		t.Fatalf("buildCatalogSession(enterprise=false) error = %v", err)
	}
	closeSession()
	if _, ok := routes["gitlab"]["merge_train.list_project"]; ok {
		t.Fatal("CE catalog registered enterprise-only merge_train.list_project route")
	}

	client = newEvalTestClient(t, true)
	_, closeSession, _, routes, err = buildCatalogSession(client)
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

func TestValidationRepairMessage_IncludesActionEnvelopeAndProjectHint(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}
	message := validationRepairMessage(step, validationResult{Message: "missing required params.project_id"})
	if !strings.Contains(message, `"action":"project.get"`) || !strings.Contains(message, "project_id") {
		t.Fatalf("message = %q, want action envelope example", message)
	}
	if !strings.Contains(message, "previous tool result") || !strings.Contains(message, "params.project_id") {
		t.Fatalf("message = %q, want previous-result project_id hint", message)
	}
}

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

func TestValidateStandaloneToolCall_AcceptsTopLevelInput(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"remote_url": "https://gitlab.example.com/my-org/project.git",
	})
	if !result.Valid {
		t.Fatalf("validateStepCall() Valid = false: %s", result.Message)
	}
}

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

func TestSchemaLookupResult_UnknownToolReturnsError(t *testing.T) {
	_, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_missing"}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("error = %v, want unknown tool", err)
	}
}

func TestSchemaLookupResult_MissingToolReturnsUsageExamples(t *testing.T) {
	payload, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_get", "params": map[string]any{}})
	if err != nil {
		t.Fatalf("schemaLookupResult() error = %v, want usage payload", err)
	}
	if !strings.Contains(payload, `"action":"schema_get"`) || !strings.Contains(payload, `"tool":"gitlab"`) || !strings.Contains(payload, "pipeline.get") {
		t.Fatalf("payload = %s, want schema_get usage examples", payload)
	}
}

func TestSuccessfulSimulatedToolContent_IncludesDiscoveredProject(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{}, modelContentBlock{
		Name:  "gitlab_discover_project",
		Input: map[string]any{"remote_url": "https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git"},
	}, 2, 3)
	if !strings.Contains(content, "my-org/tools/gitlab-mcp-server") || !strings.Contains(content, "default_branch") {
		t.Fatalf("successfulSimulatedToolContent() = %s, want project metadata", content)
	}
}

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
		"do not call file_get again after the update",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want repository file CRUD guidance containing %q", prompt, want)
		}
	}
}

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
		"Use the returned id as params.schedule_id",
		"Both schedule_delete_variable and schedule_delete are destructive and require params.confirm=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline schedule guidance containing %q", prompt, want)
		}
	}
}

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
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want project snippet CRUD guidance containing %q", prompt, want)
		}
	}
}

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

	system := systemPromptForTask(task)
	if !strings.Contains(system, "Return tool calls only") || strings.Contains(system, "runner.list_project") {
		t.Fatalf("systemPromptForTask() = %q, want compact exact-call system prompt", system)
	}
}

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
	system := systemPromptForTask(task)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline trigger delete system prompt without %q", system, unwanted)
		}
	}
}

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
	system := systemPromptForTask(task)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline schedule delete system prompt without %q", system, unwanted)
		}
	}
}

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

func TestLoadCatalog_RejectsUnknownBackend(t *testing.T) {
	_, _, err := loadCatalog(options{Backend: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("error = %v, want unknown backend", err)
	}
}

func TestRunMCPSmokeRequiresGitLabBackend(t *testing.T) {
	err := runMCPSmoke(options{Backend: backendMock})
	if err == nil || !strings.Contains(err.Error(), "--backend=gitlab") {
		t.Fatalf("error = %v, want backend guard", err)
	}
}

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

func TestValidateExecutionOptions_ExternalCommandRequiresDockerGuard(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	err := validateExecutionOptions(options{ToolsFile: "snapshot.json", MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("error = %v, want external docker guard", err)
	}
}

func TestValidateExecutionOptions_ExternalCommandRequiresToolsFile(t *testing.T) {
	t.Setenv("E2E_MODE", "docker")
	err := validateExecutionOptions(options{MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "requires --tools-file") {
		t.Fatalf("error = %v, want tools-file guard", err)
	}
}

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

	err = callFixtureSetupTool(t.Context(), session, "branch.create", map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("callFixtureSetupTool() error = %v", err)
	}
	if !called {
		t.Fatal("split meta-tool was not called")
	}
}

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
}

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
}

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

func TestEstimateCostUSD_UsesPerMillionPricing(t *testing.T) {
	cost := estimateCostUSD(modelUsage{InputTokens: 1_000_000, OutputTokens: 100_000}, pricingOptions{InputPerMTok: 3, OutputPerMTok: 15})
	if cost != 4.5 {
		t.Fatalf("cost = %v, want 4.5", cost)
	}
}

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
	if err := writeTraceArtifacts(dir, []taskResult{{Trace: trace}}); err != nil {
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
}

func TestDefaultTraceDir_ReplacesReportExtension(t *testing.T) {
	got := defaultTraceDir("dist/evaluation/meta-tools/report.md")
	if got != "dist/evaluation/meta-tools/report.traces" {
		t.Fatalf("defaultTraceDir() = %q, want report.traces", got)
	}
}

func TestDefaultOutputPath_UsesIgnoredDistDirectory(t *testing.T) {
	got := defaultOutputPath("claude/sonnet:4 6")
	if !strings.HasPrefix(got, "dist/evaluation/meta-tools/model-") {
		t.Fatalf("defaultOutputPath() = %q, want dist evaluation path", got)
	}
	if !strings.HasSuffix(got, "-claude-sonnet-4-6.md") {
		t.Fatalf("defaultOutputPath() = %q, want sanitized model suffix", got)
	}
}

func TestDefaultOutputPath_UsesShortNameForMultiModel(t *testing.T) {
	got := defaultOutputPath("anthropic:claude-sonnet-4-6,openai:gpt-5.4-mini")
	if !strings.HasSuffix(got, "-multi-model.md") {
		t.Fatalf("defaultOutputPath() = %q, want multi-model suffix", got)
	}
}

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

func TestParseModelSpec_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseModelSpec("local:llama")
	if err == nil || !strings.Contains(err.Error(), "unsupported model provider") {
		t.Fatalf("error = %v, want unsupported provider", err)
	}
}

func TestParseModelSpec_StripsGoogleModelsPrefix(t *testing.T) {
	spec, err := parseModelSpec("google:models/gemini-3-flash-preview")
	if err != nil {
		t.Fatalf("parseModelSpec() error = %v", err)
	}
	if spec.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want trimmed Gemini model", spec.Model)
	}
}

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

func TestQwenEndpoint_UsesConfiguredBaseURL(t *testing.T) {
	t.Setenv("QWEN_CHAT_COMPLETIONS_URL", "")
	t.Setenv("QWEN_BASE_URL", "https://example.test/v1/")
	if got := qwenEndpoint(); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("qwenEndpoint() = %q", got)
	}
}

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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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
	return &modelRunner{apiKey: "test-key", model: "test-model", maxTokens: 256, client: client}
}

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

func toolUseResponse(id, name string, input map[string]any) modelResponse {
	return modelResponse{Content: []modelContentBlock{{Type: "tool_use", ID: id, Name: name, Input: input}}}
}

func multiToolUseResponse(blocks ...modelContentBlock) modelResponse {
	return modelResponse{Content: blocks}
}

func traceHasKind(trace taskTrace, kind string) bool {
	for _, event := range trace.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func traceContainsToolResult(trace taskTrace, text string) bool {
	for _, event := range trace.Events {
		if event.Kind == "tool_result" && strings.Contains(event.Content, text) {
			return true
		}
	}
	return false
}

func projectGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}}
}

func pipelineGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string"},
			"pipeline_id": map[string]any{"type": "integer"},
		},
	}}
}

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
