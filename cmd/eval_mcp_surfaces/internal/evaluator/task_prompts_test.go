package evaluator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestTaskPromptForSurface_DynamicRemoteURLGuidanceIsScoped verifies that
// the remote-URL discovery guidance is only emitted for tasks whose prompt
// actually contains a remote URL.
//
// The test renders a prompt for a project.get task that does not mention a
// remote URL and asserts the discovery guidance is absent and the literal
// discover_project.resolve action is not leaked. This protects the dynamic
// prompt builder from injecting unrelated guidance for tasks that already
// know the project path.
func TestTaskPromptForSurface_DynamicRemoteURLGuidanceIsScoped(t *testing.T) {
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	forbidden := []string{"first gitlab_find_action query for that discovery step must explicitly describe resolving the provided remote URL", "must use the project-discovery action with params.remote_url set to that exact URL"}
	for _, text := range forbidden {
		t.Run(text, func(t *testing.T) {
			if strings.Contains(got, text) {
				t.Fatalf("dynamic prompt unexpectedly included remote URL guidance %q:\n%s", text, got)
			}
		})
	}
	if strings.Contains(got, "discover_project.resolve") {
		t.Fatalf("dynamic prompt unexpectedly leaked exact discovery action:\n%s", got)
	}
}

// TestDynamicExampleParamValue_CompareRefsExtractsFromAndTo verifies that
// dynamicExampleParamValue pulls the correct ref values for repository.compare
// actions from a prompt that names both refs in
// backticks.
//
// The test invokes the helper with the from/to parameters for each action
// and asserts the extracted values match the prompt's refs. This protects
// the prompt builder from binding the wrong ref to the wrong action when a
// workflow depends on consistent from/to values.
func TestDynamicExampleParamValue_CompareRefsExtractsFromAndTo(t *testing.T) {
	prompt := "Prepare an LLM-assisted release summary for project `my-org/tools/gitlab-mcp-server`: inspect releases, compare refs `main` and `v0.0.0-eval-ms`, then generate release notes."
	if got := dynamicExampleParamValue("repository.compare", "from", prompt); got != "main" {
		t.Fatalf("dynamicExampleParamValue(from) = %v, want main", got)
	}
	if got := dynamicExampleParamValue("repository.compare", "to", prompt); got != "v0.0.0-eval-ms" {
		t.Fatalf("dynamicExampleParamValue(to) = %v, want v0.0.0-eval-ms", got)
	}
}

// TestTaskForSurface_RewritesToolDetailResourceIDs verifies capability tasks use
// detail resource IDs from the active surface instead of dynamic-only IDs.
func TestTaskForSurface_RewritesToolDetailResourceIDs(t *testing.T) {
	evalCase, ok := CaseByID("MS-040")
	if !ok {
		t.Fatal("CaseByID(MS-040) = false")
	}
	task := taskFromCase(evalCase)

	metaTask := taskForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(metaTask.Prompt, "`gitlab://tools/gitlab_project.get`") {
		t.Fatalf("meta prompt = %q, want meta project detail URI", metaTask.Prompt)
	}
	if strings.Contains(metaTask.Prompt, dynamicProjectGetToolDetailURI) {
		t.Fatalf("meta prompt kept dynamic project detail URI: %q", metaTask.Prompt)
	}

	dynamicTask := taskForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(dynamicTask.Prompt, "`"+dynamicProjectGetToolDetailURI+"`") {
		t.Fatalf("dynamic prompt = %q, want dynamic project detail URI", dynamicTask.Prompt)
	}
}

// TestJoinNonEmpty_TrimAndSkipBlanks verifies prompt fragments are composed
// without introducing empty paragraphs.
func TestJoinNonEmpty_TrimAndSkipBlanks(t *testing.T) {
	if got := joinNonEmpty("|", " first ", " ", "second"); got != "first|second" {
		t.Fatalf("joinNonEmpty() = %q, want first|second", got)
	}
}

// TestDynamicExampleParamValue_UsesPromptMarkers verifies exact-call guidance
// binds role-sensitive parameters from natural-language prompts.
func TestDynamicExampleParamValue_UsesPromptMarkers(t *testing.T) {
	if got := dynamicExampleParamValue("repository.file_create", "file_path", "create file `docs/eval.md`"); got != "docs/eval.md" {
		t.Fatalf("dynamicExampleParamValue(file_path) = %v, want docs/eval.md", got)
	}
	if got := dynamicExampleParamValue("pipeline.schedule_create", "active", "create inactive schedule `nightly`"); got != false {
		t.Fatalf("dynamicExampleParamValue(active) = %v, want false", got)
	}
}

// requireContainsAll returns contains all test data or fails the test.
func requireContainsAll(t *testing.T, name, content string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Fatalf("%s = %q, want content containing %q", name, content, want)
		}
	}
}

// TestDynamicTaskPrompt_UnresolvedRoleSensitiveParamsStayFindFirst verifies
// unresolved role-sensitive values keep Dynamic prompts on the find-first path.
func TestDynamicTaskPrompt_UnresolvedRoleSensitiveParamsStayFindFirst(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		absent []string
	}{
		{
			name: "missing target project",
			task: evalTask{ID: "MT-066", Prompt: "Remove a project from the CI job token allowlist of project `1`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_project_id":123`, "<target_project_id>"},
		},
		{
			name: "non numeric target project",
			task: evalTask{ID: "MT-066", Prompt: "Remove project ID `not-a-number` from the CI job token allowlist of project `1`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_project_id":123`},
		},
		{
			name: "missing target branch",
			task: evalTask{ID: "MT-MR", Prompt: "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` titled `Evaluation MR`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.create", RequiredParams: []string{"project_id", "source_branch", "target_branch", "title"}},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_branch":"main"`, "<target_branch>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic)
			for _, unwanted := range tt.absent {
				if strings.Contains(prompt, unwanted) {
					t.Fatalf("taskPromptForSurface() = %q, want no unsafe exact-call content %q", prompt, unwanted)
				}
			}
			if !strings.Contains(prompt, "Required parameters for action") && !strings.Contains(prompt, "gitlab_find_action") {
				t.Fatalf("taskPromptForSurface() = %q, want schema-first or dynamic discovery guidance", prompt)
			}
		})
	}
}

// TestTaskPrompt_GroupEpicIssueAssign_IsNoLongerHandedItsOwnCall is what the
// leak test for MT-140 became.
//
// It used to assert the opposite of this: that the prompt contained
// `"action":"group.epic_issue_assign"` with all four params marshaled, and the
// sentence "Exact required call". That is the answer the model was then scored
// on producing, so the case measured transcription rather than whether the
// meta surface describes itself. V05 deleted the builder that wrote it, and
// the assertion is inverted rather than dropped so the deletion stays deleted.
func TestTaskPrompt_GroupEpicIssueAssign_IsNoLongerHandedItsOwnCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-140",
		Prompt:         "Assign issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` to epic IID `12` in group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_issue_assign",
		RequiredParams: []string{"full_path", "epic_iid", "child_project_path", "child_iid"},
	}

	prompt := taskPrompt(task)
	for _, leaked := range []string{
		`"action":"group.epic_issue_assign"`,
		`"epic_iid":12`,
		`"child_project_path":"my-org/tools/gitlab-mcp-server"`,
		`"child_iid":99`,
		"Exact required call",
	} {
		t.Run(leaked, func(t *testing.T) {
			if strings.Contains(prompt, leaked) {
				t.Fatalf("taskPrompt() still hands the case its own call: found %q in\n%s", leaked, prompt)
			}
		})
	}
	// The user's own words stay: what V05 removes is the scaffolding around
	// them, never the request the case is asking a model to carry out.
	t.Run("keeps the user request", func(t *testing.T) {
		if !strings.Contains(prompt, task.Prompt) {
			t.Fatalf("taskPrompt() dropped the user request:\n%s", prompt)
		}
	})
}

// TestDynamicExampleResolvers_PromptMarkers_ReturnTypedValues verifies each
// action-specific dynamic example resolver extracts the prompt's backticked
// value for the parameters it owns and declines every other parameter, so the
// exact-call prompt binds concrete values instead of placeholders.
func TestDynamicExampleResolvers_PromptMarkers_ReturnTypedValues(t *testing.T) {
	cases := []struct {
		name    string
		resolve func(action, param, prompt string) (any, bool)
		action  string
		param   string
		prompt  string
		want    any
		wantOK  bool
	}{
		{name: "file path from create marker", resolve: repositoryFileDynamicExample, action: "repository.file_create", param: "file_path", prompt: "please create file `docs/notes.md`", want: "docs/notes.md", wantOK: true},
		{name: "file path without marker", resolve: repositoryFileDynamicExample, action: "repository.file_create", param: "file_path", prompt: "no marker here"},
		{name: "create content default", resolve: repositoryFileDynamicExample, action: "repository.file_create", param: "content", prompt: "seed it", want: "Initial content for repository file CRUD", wantOK: true},
		{name: "update content default", resolve: repositoryFileDynamicExample, action: "repository.file_update", param: "content", prompt: "seed it", want: "Updated content for repository file CRUD", wantOK: true},
		{name: "content from marker", resolve: repositoryFileDynamicExample, action: "repository.file_update", param: "content", prompt: "with content `hello world`", want: "hello world", wantOK: true},
		{name: "commit message from marker", resolve: repositoryFileDynamicExample, action: "repository.file_delete", param: "commit_message", prompt: "use commit_message `chore: drop`", want: "chore: drop", wantOK: true},
		{name: "commit message from file path", resolve: repositoryFileDynamicExample, action: "repository.file_delete", param: "commit_message", prompt: "please delete file `tmp/x.txt`", want: "Evaluation delete tmp/x.txt", wantOK: true},
		{name: "commit message generic", resolve: repositoryFileDynamicExample, action: "repository.file_delete", param: "commit_message", prompt: "plain", want: "Evaluation delete repository file", wantOK: true},
		{name: "file action unrelated param", resolve: repositoryFileDynamicExample, action: "repository.file_get", param: "ref", prompt: "x"},
		{name: "non file action", resolve: repositoryFileDynamicExample, action: "project.get", param: "file_path", prompt: "create file `x`"},
		{name: "merge request iid", resolve: mergeRequestIIDDynamicExample, action: "merge_request.merge", param: "merge_request_iid", prompt: "Merge MR `12` now", want: 12, wantOK: true},
		{name: "merge request iid missing", resolve: mergeRequestIIDDynamicExample, action: "merge_request.merge", param: "merge_request_iid", prompt: "no iid"},
		{name: "merge request iid other action", resolve: mergeRequestIIDDynamicExample, action: "issue.get", param: "merge_request_iid", prompt: "MR `1`"},
		{name: "compare from", resolve: releaseDynamicExample, action: "repository.compare", param: "from", prompt: "compare refs `v1.0` and `v2.0`", want: "v1.0", wantOK: true},
		{name: "compare to", resolve: releaseDynamicExample, action: "repository.compare", param: "to", prompt: "compare refs `v1.0` and `v2.0`", want: "v2.0", wantOK: true},
		{name: "compare single ref", resolve: releaseDynamicExample, action: "repository.compare", param: "from", prompt: "compare refs `v1.0` only"},
		{name: "release tag", resolve: releaseDynamicExample, action: "release.create", param: "tag_name", prompt: "create release `v9.9.9` now", want: "v9.9.9", wantOK: true},
		{name: "release name", resolve: releaseDynamicExample, action: "release.create", param: "name", prompt: "release named `Big release`", want: "Big release", wantOK: true},
		{name: "release other param", resolve: releaseDynamicExample, action: "release.create", param: "ref", prompt: "x"},
		{name: "release other action", resolve: releaseDynamicExample, action: "release.get", param: "tag_name", prompt: "release `v1`"},
		{name: "time estimate", resolve: mergeRequestDynamicExample, action: "merge_request.time_estimate_set", param: "duration", prompt: "set estimate `2h`", want: "2h", wantOK: true},
		{name: "spent time", resolve: mergeRequestDynamicExample, action: "merge_request.spent_time_add", param: "duration", prompt: "add spent time `30m`", want: "30m", wantOK: true},
		{name: "award emoji", resolve: mergeRequestDynamicExample, action: "merge_request.emoji_mr_create", param: "name", prompt: "add award emoji `rocket`", want: "rocket", wantOK: true},
		{name: "award emoji missing", resolve: mergeRequestDynamicExample, action: "merge_request.emoji_mr_create", param: "name", prompt: "no emoji"},
		{name: "snippet file name", resolve: snippetDynamicExample, action: "snippet.project_create", param: "file_name", prompt: "create project snippet `notes`", want: "notes.md", wantOK: true},
		{name: "snippet files", resolve: snippetDynamicExample, action: "snippet.project_update", param: "files", prompt: "x", want: []map[string]any{{"action": "update", "file_path": "<returned_file_path>", "content": "Updated snippet content"}}, wantOK: true},
		{name: "snippet other", resolve: snippetDynamicExample, action: "snippet.project_get", param: "files", prompt: "x"},
		{name: "feature flag list name", resolve: featureFlagDynamicExample, action: "feature_flags.ff_user_list_create", param: "name", prompt: "create user list `beta-testers`", want: "beta-testers", wantOK: true},
		{name: "feature flag user xids", resolve: featureFlagDynamicExample, action: "feature_flags.ff_user_list_create", param: "user_xids", prompt: "with user IDs `u1,u2`", want: "u1,u2", wantOK: true},
		{name: "feature flag other action", resolve: featureFlagDynamicExample, action: "feature_flags.feature_flag_create", param: "name", prompt: "feature flag `x`"},
		{name: "issue title", resolve: issueDynamicExample, action: actionIssueCreate, param: "title", prompt: "please create issue `Crash on start`", want: "Crash on start", wantOK: true},
		{name: "issue title missing", resolve: issueDynamicExample, action: actionIssueCreate, param: "title", prompt: "no marker"},
		{name: "issue other action", resolve: issueDynamicExample, action: "issue.update", param: "title", prompt: "create issue `x`"},
		{name: "trigger description", resolve: pipelineDynamicExample, action: "pipeline.trigger_create", param: "description", prompt: "create trigger `nightly`", want: "nightly", wantOK: true},
		{name: "schedule description", resolve: pipelineDynamicExample, action: "pipeline.schedule_create", param: "description", prompt: "create an inactive schedule `nightly build`", want: "nightly build", wantOK: true},
		{name: "schedule inactive", resolve: pipelineDynamicExample, action: "pipeline.schedule_create", param: "active", prompt: "create an inactive schedule", want: false, wantOK: true},
		{name: "schedule active unresolved", resolve: pipelineDynamicExample, action: "pipeline.schedule_create", param: "active", prompt: "create an active schedule"},
		{name: "pipeline other action", resolve: pipelineDynamicExample, action: actionPipelineGet, param: "description", prompt: "x"},
		{name: "broadcast id", resolve: adminDynamicExample, action: "admin.broadcast_message_delete", param: "id", prompt: "delete broadcast message ID `7`", want: 7, wantOK: true},
		{name: "terraform state name", resolve: adminDynamicExample, action: "admin.terraform_state_unlock", param: "name", prompt: "unlock Terraform state `production`", want: "production", wantOK: true},
		{name: "admin other action", resolve: adminDynamicExample, action: "admin.settings_get", param: "id", prompt: "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.resolve(tc.action, tc.param, tc.prompt)
			if ok != tc.wantOK || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolver(%s, %s, %q) = %#v, %t; want %#v, %t", tc.action, tc.param, tc.prompt, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestDynamicExampleParamValue_NoResolverMatch_FallsBackToGenericExample
// verifies the dynamic resolver chain ends in the generic example table so a
// parameter no action-specific resolver claims still gets a concrete value.
func TestDynamicExampleParamValue_NoResolverMatch_FallsBackToGenericExample(t *testing.T) {
	if got := dynamicExampleParamValue(actionProjectGet, "ref", "no markers"); got != "main" {
		t.Fatalf("dynamicExampleParamValue(ref) = %#v, want main", got)
	}
}

// TestExampleParamValue_PromptHeuristics_ReturnTypedValues verifies the
// generic example table maps prompt wording onto typed parameter values
// (metric names, statuses, scopes, access levels, booleans and project IDs)
// and falls back to a bracketed placeholder for anything it cannot infer.
func TestExampleParamValue_PromptHeuristics_ReturnTypedValues(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		prompt string
		want   any
	}{
		{name: "metric lead time", param: "metric", prompt: "Show lead time for changes", want: "lead_time_for_changes"},
		{name: "metric unknown", param: "metric", prompt: "deployment frequency", want: "<metric>"},
		{name: "status passed", param: "status", prompt: "pipelines that passed", want: "passed"},
		{name: "scope failed jobs", param: "scope", prompt: "list failed jobs", want: "failed"},
		{name: "scopes read_api", param: "scopes", prompt: "token with read_api", want: []string{"read_api"}},
		{name: "scopes read_repository", param: "scopes", prompt: "token with read_repository", want: []string{"read_repository"}},
		{name: "scopes fallback", param: "scopes", prompt: "token", want: []string{"read_api"}},
		{name: "access level reporter", param: "access_level", prompt: "as reporter", want: 20},
		{name: "access level developer", param: "access_level", prompt: "as developer", want: 30},
		{name: "access level maintainer", param: "access_level", prompt: "as maintainer", want: 40},
		{name: "access level fallback", param: "access_level", prompt: "as guest", want: 30},
		{name: "paused true", param: "paused", prompt: "set paused=true", want: true},
		{name: "paused false", param: "paused", prompt: "set paused=false", want: false},
		{name: "paused fallback", param: "paused", prompt: "pause it", want: true},
		{name: "state event close", param: "state_event", prompt: "close the issue", want: "close"},
		{name: "state event unknown", param: "state_event", prompt: "noop", want: "<state_event>"},
		{name: "project id from prompt", param: "project_id", prompt: "list issues in project `my-org/app`", want: "my-org/app"},
		{name: "project id fallback", param: "project_id", prompt: "list issues", want: "<project_id>"},
		{name: "masked defaults false", param: "masked", prompt: "x", want: false},
		{name: "numeric marker wins", param: "pipeline_id", prompt: "inspect pipeline `77`", want: 77},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exampleParamValue(tc.param, tc.prompt); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("exampleParamValue(%s, %q) = %#v, want %#v", tc.param, tc.prompt, got, tc.want)
			}
		})
	}
}

// TestFallbackExampleParamValue_KnownParams_ReturnPlaceholders verifies the
// last-resort example values: numeric IDs, booleans, cron and ref defaults,
// URL and key samples, and the bracketed placeholder for unknown params.
func TestFallbackExampleParamValue_KnownParams_ReturnPlaceholders(t *testing.T) {
	cases := []struct {
		param string
		want  any
	}{
		{param: "id", want: 123},
		{param: "note_id", want: 123},
		{param: "confirm", want: true},
		{param: "access_level", want: 30},
		{param: "cron", want: "0 2 * * 1"},
		{param: "ref", want: "main"},
		{param: "content_ref", want: "main"},
		{param: "link_url", want: "https://example.com/eval-crud-badge"},
		{param: "image_url", want: "https://example.com/eval-crud-badge.svg"},
		{param: "scopes", want: []string{"read_api"}},
		{param: "deploy_access_levels", want: []map[string]any{{"access_level": 40}}},
		{param: "approval_rules", want: []map[string]any{{"access_level": 40, "required_approvals": 1}}},
		{param: "unknown_param", want: "<unknown_param>"},
	}
	for _, tc := range cases {
		t.Run(tc.param, func(t *testing.T) {
			if got := fallbackExampleParamValue(tc.param); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("fallbackExampleParamValue(%s) = %#v, want %#v", tc.param, got, tc.want)
			}
		})
	}
	if key, ok := fallbackExampleParamValue("key").(string); !ok || !strings.HasPrefix(key, "ssh-ed25519 ") {
		t.Fatalf("fallbackExampleParamValue(key) = %#v, want ssh public key", fallbackExampleParamValue("key"))
	}
}

// TestExampleOptionalParamValue_PromptHints_ReturnTypedValues verifies the
// optional-parameter resolvers: regex markers, month ranges, explicit dates,
// environment scopes, protected-environment shapes, state words, access
// levels, boolean cues and sort hints, plus the unresolved default.
func TestExampleOptionalParamValue_PromptHints_ReturnTypedValues(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		prompt string
		want   any
		wantOK bool
	}{
		{name: "commit message regex", param: "commit_message_regex", prompt: "require commit message regex `^feat`", want: "^feat", wantOK: true},
		{name: "created after month", param: "created_after", prompt: "events in March 2026", want: "2026-03-01", wantOK: true},
		{name: "created before month", param: "created_before", prompt: "events in March 2026", want: "2026-04-01", wantOK: true},
		{name: "created after without month", param: "created_after", prompt: "events"},
		{name: "start date", param: "start_date", prompt: "range from `2026-01-01` to `2026-02-01`", want: "2026-01-01", wantOK: true},
		{name: "end date", param: "end_date", prompt: "from `2026-01-01` to `2026-02-01`", want: "2026-02-01", wantOK: true},
		{name: "environment scope", param: "environment_scope", prompt: "with production scope", want: "production", wantOK: true},
		{name: "deploy access levels", param: "deploy_access_levels", prompt: "protect environment staging", want: []map[string]any{{"access_level": 40}}, wantOK: true},
		{name: "approval rules", param: "approval_rules", prompt: "require one approval", want: []map[string]any{{"access_level": 40, "required_approvals": 1}}, wantOK: true},
		{name: "state active", param: "state", prompt: "only active runners", want: "active", wantOK: true},
		{name: "state event reopen", param: "state_event", prompt: "reopen the issue", want: "reopen", wantOK: true},
		{name: "push access maintainer", param: "push_access_level", prompt: "maintainer push and merge", want: 40, wantOK: true},
		{name: "push access developer", param: "push_access_level", prompt: "developer push", want: 30, wantOK: true},
		{name: "merge access maintainer", param: "merge_access_level", prompt: "maintainer merge", want: 40, wantOK: true},
		{name: "merge access developer", param: "merge_access_level", prompt: "developer push and merge", want: 30, wantOK: true},
		{name: "reject unsigned commits", param: "reject_unsigned_commits", prompt: "reject unsigned commits", want: true, wantOK: true},
		{name: "include descendants", param: "include_descendants", prompt: "including descendant groups", want: true, wantOK: true},
		{name: "enabled disabled", param: "enabled", prompt: "leave it disabled", want: false, wantOK: true},
		{name: "active inactive", param: "active", prompt: "an inactive schedule", want: false, wantOK: true},
		{name: "primary secondary", param: "primary", prompt: "a secondary site", want: false, wantOK: true},
		{name: "order by updated", param: "order_by", prompt: "recently updated projects", want: "updated_at", wantOK: true},
		{name: "sort latest", param: "sort", prompt: "the latest pipelines", want: "desc", wantOK: true},
		{name: "per page ten", param: "per_page", prompt: "the 10 most recently updated projects", want: 10, wantOK: true},
		{name: "unknown param", param: "unknown_param", prompt: "anything"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := exampleOptionalParamValue(tc.param, tc.prompt)
			if ok != tc.wantOK || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("exampleOptionalParamValue(%s, %q) = %#v, %t; want %#v, %t", tc.param, tc.prompt, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestNumericExampleValue_ParsesDigitsOrFallsBack verifies numeric prompt
// values parse to ints and non-numeric text yields the stable fallback ID.
func TestNumericExampleValue_ParsesDigitsOrFallsBack(t *testing.T) {
	cases := []struct {
		value string
		want  any
	}{
		{value: "42", want: 42},
		{value: "abc", want: 123},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := numericExampleValue(tc.value); got != tc.want {
				t.Fatalf("numericExampleValue(%q) = %#v, want %#v", tc.value, got, tc.want)
			}
		})
	}
}

// TestParamSemanticRole_MapsParamsToRoles verifies role-sensitive parameter
// names map to their semantic roles and everything else echoes the name.
func TestParamSemanticRole_MapsParamsToRoles(t *testing.T) {
	cases := []struct {
		param string
		want  string
	}{
		{param: "project_id", want: "scope_owner_project"},
		{param: "target_project_id", want: "target_project"},
		{param: "group_id", want: "group_scope"},
		{param: "full_path", want: "group_scope"},
		{param: "target_group_id", want: "target_group"},
		{param: "issue_iid", want: "source_issue"},
		{param: "child_iid", want: "source_issue"},
		{param: "target_issue_iid", want: "target_issue"},
		{param: "source_branch", want: "source_branch"},
		{param: "target_branch", want: "target_branch"},
		{param: "child_project_path", want: "child_project_path"},
		{param: "title", want: "title"},
	}
	for _, tc := range cases {
		t.Run(tc.param, func(t *testing.T) {
			if got := paramSemanticRole(tc.param); got != tc.want {
				t.Fatalf("paramSemanticRole(%s) = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

// TestExactParamValueIsPlaceholder_DetectsPlaceholderShapes verifies nil,
// blank, ellipsis and angle-bracket strings count as placeholders, including
// when nested in slices and maps, while concrete values do not.
func TestExactParamValueIsPlaceholder_DetectsPlaceholderShapes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", value: nil, want: true},
		{name: "blank", value: "  ", want: true},
		{name: "ellipsis", value: "...", want: true},
		{name: "angle brackets", value: "<project_id>", want: true},
		{name: "concrete string", value: "my-org/app", want: false},
		{name: "map slice with placeholder", value: []map[string]any{{"url": "<url>"}}, want: true},
		{name: "map slice concrete", value: []map[string]any{{"url": "https://example.com"}}, want: false},
		{name: "any slice with placeholder", value: []any{"ok", "<x>"}, want: true},
		{name: "map with placeholder", value: map[string]any{"k": "<v>"}, want: true},
		{name: "map concrete", value: map[string]any{"k": "v"}, want: false},
		{name: "integer", value: 5, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exactParamValueIsPlaceholder(tc.value); got != tc.want {
				t.Fatalf("exactParamValueIsPlaceholder(%#v) = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

// TestFallbackParamProvenance_ReportsZeroConfidenceFallback verifies a
// fallback provenance carries the generic example value, the fallback marker
// and zero confidence so exact-call safety checks can reject it.
func TestFallbackParamProvenance_ReportsZeroConfidenceFallback(t *testing.T) {
	got := fallbackParamProvenance("ref")
	if got.ParamName != "ref" || got.Value != "main" || got.SourceMarker != "fallback" || got.Confidence != 0 || got.SemanticRole != "ref" || got.SourceText != "main" {
		t.Fatalf("fallbackParamProvenance(ref) = %+v, want fallback provenance for main", got)
	}
}

// TestRoleParamProvenance_ResolvesRoleSensitiveParams verifies role-sensitive
// parameters bind to the backticked prompt value after their role marker
// (numeric where the API expects an ID), that the allowlist and target-issue
// contexts change which markers apply, and that unrelated params are declined.
func TestRoleParamProvenance_ResolvesRoleSensitiveParams(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		all    map[string]bool
		prompt string
		want   any
		wantOK bool
	}{
		{name: "project with allowlist marker", param: "project_id", all: map[string]bool{"target_project_id": true}, prompt: "Add to the allowlist of project `12` the target project ID `34`.", want: 12, wantOK: true},
		{name: "project plain marker", param: "project_id", prompt: "List issues in project `my-org/app`.", want: "my-org/app", wantOK: true},
		{name: "project no marker", param: "project_id", prompt: "List issues."},
		{name: "target project id", param: "target_project_id", prompt: "add target project ID `34`", want: 34, wantOK: true},
		{name: "target project non numeric", param: "target_project_id", prompt: "add target project ID `abc`"},
		{name: "target group id", param: "target_group_id", prompt: "share with target group ID `5`", want: 5, wantOK: true},
		{name: "source issue with target", param: "issue_iid", all: map[string]bool{"target_issue_iid": true}, prompt: "Link source issue IID `3` to target issue IID `4`.", want: 3, wantOK: true},
		{name: "issue without target context", param: "issue_iid", prompt: "issue IID `3`"},
		{name: "target issue", param: "target_issue_iid", prompt: "to target issue IID `4`", want: 4, wantOK: true},
		{name: "child iid", param: "child_iid", prompt: "child issue IID `9`", want: 9, wantOK: true},
		{name: "source branch", param: "source_branch", prompt: "Create MR from `feat/x` into `main`", want: "feat/x", wantOK: true},
		{name: "target branch", param: "target_branch", prompt: "Create MR from `feat/x` into `main`", want: "main", wantOK: true},
		{name: "full path", param: "full_path", prompt: "for group path `my-org`", want: "my-org", wantOK: true},
		{name: "child project path", param: "child_project_path", prompt: "child project path `my-org/app`", want: "my-org/app", wantOK: true},
		{name: "parent id", param: "parent_id", prompt: "under group ID `9`", want: 9, wantOK: true},
		{name: "unrelated param", param: "title", prompt: "titled `x`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := roleParamProvenance(tc.param, tc.prompt, tc.all)
			if ok != tc.wantOK {
				t.Fatalf("roleParamProvenance(%s) ok = %t, want %t", tc.param, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !reflect.DeepEqual(got.Value, tc.want) || got.Confidence != 1 || got.ParamName != tc.param {
				t.Fatalf("roleParamProvenance(%s) = %+v, want value %#v with confidence 1", tc.param, got, tc.want)
			}
		})
	}
}

// TestExactCallParamsAreSafe_RejectsPlaceholdersAndUnresolvedRoles verifies
// an exact call is only advertised when no value is a placeholder and every
// role-sensitive parameter was bound with confidence.
func TestExactCallParamsAreSafe_RejectsPlaceholdersAndUnresolvedRoles(t *testing.T) {
	cases := []struct {
		name        string
		provenances []paramProvenance
		want        bool
	}{
		{name: "placeholder value", provenances: []paramProvenance{{ParamName: "title", Value: "<title>", Confidence: 0.7}}, want: false},
		{name: "unresolved target role", provenances: []paramProvenance{{ParamName: "target_project_id", Value: 123, Confidence: 0}}, want: false},
		{name: "project with unresolved target context", provenances: []paramProvenance{{ParamName: "project_id", Value: "a/b", Confidence: 0}, {ParamName: "target_project_id", Value: 2, Confidence: 1}}, want: false},
		{name: "project alone needs no role", provenances: []paramProvenance{{ParamName: "project_id", Value: "a/b", Confidence: 0}}, want: true},
		{name: "all resolved", provenances: []paramProvenance{{ParamName: "source_branch", Value: "feat", Confidence: 1}, {ParamName: "target_branch", Value: "main", Confidence: 1}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exactCallParamsAreSafe(tc.provenances); got != tc.want {
				t.Fatalf("exactCallParamsAreSafe() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestTaskWithRenderedCasePrompt_ResolvesPromptSources verifies the task
// prompt is kept when set, taken from the typed case prompt, rendered from a
// variable-free template, and left empty when a template needs fixture data.
func TestTaskWithRenderedCasePrompt_ResolvesPromptSources(t *testing.T) {
	cases := []struct {
		name string
		task evalTask
		want string
	}{
		{name: "explicit prompt wins", task: evalTask{Prompt: "explicit", Case: &EvalCase{Prompt: "case"}}, want: "explicit"},
		{name: "no case", task: evalTask{}, want: ""},
		{name: "case prompt", task: evalTask{Case: &EvalCase{Prompt: "case prompt"}}, want: "case prompt"},
		{name: "empty template", task: evalTask{Case: &EvalCase{}}, want: ""},
		{name: "template without variables", task: evalTask{Case: &EvalCase{ID: "MT-T", PromptTemplate: CasePromptTemplate{Text: "static text"}}}, want: "static text"},
		{name: "template needing fixture data", task: evalTask{Case: &EvalCase{ID: "MT-T", PromptTemplate: CasePromptTemplate{Text: "Get {{ .Project.Path }}"}}}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskWithRenderedCasePrompt(tc.task).Prompt; got != tc.want {
				t.Fatalf("taskWithRenderedCasePrompt().Prompt = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestProjectGetToolDetailURIForSurface_MapsSurfaces verifies the tool detail
// resource URI used in prompts follows the surface's tool naming.
func TestProjectGetToolDetailURIForSurface_MapsSurfaces(t *testing.T) {
	cases := []struct {
		surface string
		want    string
	}{
		{surface: config.ToolSurfaceMeta, want: "gitlab://tools/gitlab_project.get"},
		{surface: config.ToolSurfaceIndividual, want: "gitlab://tools/gitlab_get_project"},
		{surface: config.ToolSurfaceDynamic, want: dynamicProjectGetToolDetailURI},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			if got := projectGetToolDetailURIForSurface(tc.surface); got != tc.want {
				t.Fatalf("projectGetToolDetailURIForSurface(%s) = %q, want %q", tc.surface, got, tc.want)
			}
		})
	}
}

// TestTaskHasAnyActionOrStep_MatchesActionIDsAndToolPairs verifies the helper
// matches on canonical action IDs, on tool/action pairs, and reports false
// when neither list matches.
func TestTaskHasAnyActionOrStep_MatchesActionIDsAndToolPairs(t *testing.T) {
	steps := []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.list"}}
	cases := []struct {
		name    string
		actions []string
		pairs   [][2]string
		want    bool
	}{
		{name: "action id", actions: []string{"issue.list"}, want: true},
		{name: "tool pair", pairs: [][2]string{{"gitlab_project", "get"}}, want: true},
		{name: "no match", actions: []string{"issue.create"}, pairs: [][2]string{{"gitlab_project", "list"}}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskHasAnyActionOrStep(steps, tc.actions, tc.pairs); got != tc.want {
				t.Fatalf("taskHasAnyActionOrStep() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestMonthRangeFromPrompt_ParsesMonthAndYear verifies a "Month YYYY" phrase
// yields the first day of that month and of the next one, tolerates trailing
// punctuation, and reports false without a year.
func TestMonthRangeFromPrompt_ParsesMonthAndYear(t *testing.T) {
	cases := []struct {
		name      string
		prompt    string
		wantStart string
		wantEnd   string
		wantOK    bool
	}{
		{name: "month with year", prompt: "audit events in March 2026", wantStart: "2026-03-01", wantEnd: "2026-04-01", wantOK: true},
		{name: "december rolls year", prompt: "in December 2025.", wantStart: "2025-12-01", wantEnd: "2026-01-01", wantOK: true},
		{name: "month without year", prompt: "in March next year", wantOK: false},
		{name: "no month", prompt: "recent events", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := monthRangeFromPrompt(tc.prompt)
			if ok != tc.wantOK || start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("monthRangeFromPrompt(%q) = %q, %q, %t; want %q, %q, %t", tc.prompt, start, end, ok, tc.wantStart, tc.wantEnd, tc.wantOK)
			}
		})
	}
}

// TestNumericBacktickValueAfter_SkipsNonNumericMatches verifies the numeric
// marker scan skips non-numeric backticked values, reports the first numeric
// one, and fails cleanly for missing or unterminated markers.
func TestNumericBacktickValueAfter_SkipsNonNumericMatches(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		want   int
		wantOK bool
	}{
		{name: "second match numeric", text: "job `abc` then job `5`", want: 5, wantOK: true},
		{name: "only non numeric", text: "job `abc`"},
		{name: "missing marker", text: "pipeline `5`"},
		{name: "unterminated", text: "job `5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := numericBacktickValueAfter(tc.text, "job ")
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("numericBacktickValueAfter(%q) = %d, %t; want %d, %t", tc.text, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestCompareRefsFromToPromptValues_RequiresTwoRefs verifies the compare
// marker needs two backticked refs after it before reporting a pair.
func TestCompareRefsFromToPromptValues_RequiresTwoRefs(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		wantOK bool
	}{
		{name: "no marker", prompt: "diff `a` and `b`"},
		{name: "one ref", prompt: "compare refs `a`"},
		{name: "two refs", prompt: "compare refs `a` with `b`", wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := compareRefsFromToPromptValues(tc.prompt); ok != tc.wantOK {
				t.Fatalf("compareRefsFromToPromptValues(%q) ok = %t, want %t", tc.prompt, ok, tc.wantOK)
			}
		})
	}
}

// TestTaskPromptForSurface_RewritesToolDetailURIForMeta verifies the
// meta-surface prompt path rewrites the dynamic tool detail URI to the
// meta-tool form while keeping the rest of the prompt intact.
func TestTaskPromptForSurface_RewritesToolDetailURIForMeta(t *testing.T) {
	task := evalTask{ID: "MT-URI", Prompt: "Read " + dynamicProjectGetToolDetailURI + " first.", Steps: []evalStep{{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}}}
	prompt := taskPromptForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(prompt, "gitlab://tools/gitlab_project.get") || strings.Contains(prompt, dynamicProjectGetToolDetailURI) {
		t.Fatalf("prompt = %q, want meta tool detail URI", prompt)
	}
}

// exactCallParamFixtures are the thirty-eight task fixtures the per-case prompt
// tests V05 deleted carried, with the parameter values each of them pinned.
//
// It sits at package level because the table is the data and the loop over it
// is four lines; inlining thirty-eight fixtures inside the test function makes
// the function itself unreadable by any measure, including the linter's.
var exactCallParamFixtures = []struct {
	name string
	task evalTask
	want map[string]any
}{
	{
		name: "ArtifactFromNumericJobUsesSingleArtifact",
		task: evalTask{
			ID:             "MT-065",
			Prompt:         "Download artifact `coverage/report.xml` from job `999` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab_job",
			ExpectedAction: "download_single_artifact",
			RequiredParams: []string{"project_id", "job_id", "artifact_path"},
		},
		want: map[string]any{"job_id": 999, "artifact_path": "coverage/report.xml"},
	},
	{
		name: "AttestationDownloadUsesAttestationIID",
		task: evalTask{
			ID:             "MT-117",
			Prompt:         "Download attestation IID `5` from project `my-org/tools/gitlab-mcp-server`; use the project-scoped attestation IID, not the database ID.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "attestation.download",
			RequiredParams: []string{"project_id", "attestation_iid"},
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "attestation_iid": 5},
	},
	{
		name: "AuditEventGetUsesEventID",
		task: evalTask{
			ID:             "MT-118",
			Prompt:         "Get instance audit event ID `77`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "audit_event.get_instance",
			RequiredParams: []string{"event_id"},
		},
		want: map[string]any{"event_id": 77},
	},
	{
		name: "AuditEventListUsesCreatedRange",
		task: evalTask{
			ID:             "MT-119",
			Prompt:         "List project audit events for project `my-org/tools/gitlab-mcp-server` created during January 2026.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "audit_event.list_project",
			RequiredParams: []string{"project_id"},
			OptionalParams: []string{"created_after", "created_before", "per_page"},
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "created_after": "2026-01-01", "created_before": "2026-02-01"},
	},
	{
		name: "CommitDiscussionDeleteUsesDiscussionAndNote",
		task: evalTask{
			ID:             "MT-113",
			Prompt:         "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "repository.commit_discussion_delete_note",
			RequiredParams: []string{"project_id", "commit_sha", "discussion_id", "note_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "commit_sha": "abc1234", "discussion_id": "abc123", "note_id": 999},
	},
	{
		name: "CompliancePolicyUpdateUsesNamespaceID",
		task: evalTask{
			ID:             "MT-120",
			Prompt:         "Update the admin compliance policy settings to use namespace ID `123`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "compliance_policy.update",
			RequiredParams: []string{"csp_namespace_id"},
		},
		want: map[string]any{"csp_namespace_id": 123},
	},
	{
		name: "DependencyExportCreateUsesPipelineID",
		task: evalTask{
			ID:             "MT-121",
			Prompt:         "Create a dependency list export for pipeline ID `12345`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "dependency.export_create",
			RequiredParams: []string{"pipeline_id"},
			OptionalParams: []string{"export_type"},
		},
		want: map[string]any{"pipeline_id": 12345},
	},
	{
		name: "DependencyExportDownloadUsesExportID",
		task: evalTask{
			ID:             "MT-122",
			Prompt:         "Download dependency list export ID `987`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "dependency.export_download",
			RequiredParams: []string{"export_id"},
		},
		want: map[string]any{"export_id": 987},
	},
	{
		name: "DeployKeyDeleteUsesDeployKeyID",
		task: evalTask{
			ID:             "MT-111",
			Prompt:         "Delete deploy key ID `32` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "access.deploy_key_delete",
			RequiredParams: []string{"project_id", "deploy_key_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "deploy_key_id": 32},
	},
	{
		name: "DeployTokenDeleteUsesDeployTokenID",
		task: evalTask{
			ID:             "MT-112",
			Prompt:         "Delete project deploy token ID `66` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "access.deploy_token_delete_project",
			RequiredParams: []string{"project_id", "deploy_token_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "deploy_token_id": 66},
	},
	{
		name: "DORAMetricsGroupUsesMetric",
		task: evalTask{
			ID:             "MT-123",
			Prompt:         "Get group DORA lead time metrics for group `my-org` from `2026-01-01` to `2026-01-31`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "dora_metrics.group",
			RequiredParams: []string{"group_id", "metric"},
			OptionalParams: []string{"start_date", "end_date", "interval", "environment_tiers"},
		},
		want: map[string]any{"group_id": "my-org", "metric": "lead_time_for_changes", "start_date": "2026-01-01", "end_date": "2026-01-31"},
	},
	{
		name: "EnterpriseUserDisable2FAUsesEnterpriseAction",
		task: evalTask{
			ID:             "MT-125",
			Prompt:         "Disable two-factor authentication for enterprise user ID `55` in group `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "enterprise_user.disable_2fa",
			RequiredParams: []string{"group_id", "user_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"group_id": "my-org", "user_id": 55},
	},
	{
		name: "EnterpriseUserGetUsesGroupAndUserID",
		task: evalTask{
			ID:             "MT-124",
			Prompt:         "Get enterprise user ID `55` in group `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "enterprise_user.get",
			RequiredParams: []string{"group_id", "user_id"},
		},
		want: map[string]any{"group_id": "my-org", "user_id": 55},
	},
	{
		name: "ExternalStatusCheckCreateUsesExternalURL",
		task: evalTask{
			ID:             "MT-126",
			Prompt:         "Create external project status check `Eval Gate` on project `my-org/tools/gitlab-mcp-server` pointing at `https://example.com/check`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "external_status_check.create_project",
			RequiredParams: []string{"project_id", "name", "external_url"},
			OptionalParams: []string{"shared_secret", "protected_branch_ids"},
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "name": "Eval Gate", "external_url": "https://example.com/check"},
	},
	{
		name: "ExternalStatusCheckDeleteUsesCheckID",
		task: evalTask{
			ID:             "MT-128",
			Prompt:         "Delete external project status check ID `8` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "external_status_check.delete_project",
			RequiredParams: []string{"project_id", "check_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "check_id": 8},
	},
	{
		name: "ExternalStatusCheckStatusUsesCheckID",
		task: evalTask{
			ID:             "MT-127",
			Prompt:         "Mark external status check ID `8` as passed for merge request IID `7` at SHA `abc123` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "external_status_check.set_project_mr_status",
			RequiredParams: []string{"project_id", "merge_request_iid", "sha", "external_status_check_id", "status"},
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "merge_request_iid": 7, "sha": "abc123", "external_status_check_id": 8, "status": "passed"},
	},
	{
		name: "FeatureFlagDeleteUsesName",
		task: evalTask{
			ID:             "MT-106",
			Prompt:         "Delete feature flag `eval_flag` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "feature_flags.feature_flag_delete",
			RequiredParams: []string{"project_id", "name"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "name": "eval_flag"},
	},
	{
		name: "GeoCreateUsesEnabledAndPrimary",
		task: evalTask{
			ID:             "MT-130",
			Prompt:         "Create a disabled Geo secondary site named `eval-geo` with URL `https://geo.example.com`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "geo.create",
			RequiredParams: []string{"name", "url"},
			OptionalParams: []string{"enabled", "primary"},
		},
		want: map[string]any{"name": "eval-geo", "url": "https://geo.example.com", "enabled": false, "primary": false},
	},
	{
		name: "GeoDeleteUsesID",
		task: evalTask{
			ID:             "MT-131",
			Prompt:         "Delete Geo site ID `3`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "geo.delete",
			RequiredParams: []string{"id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"id": 3},
	},
	{
		name: "GeoGetUsesID",
		task: evalTask{
			ID:             "MT-129",
			Prompt:         "Get Geo site ID `3`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "geo.get",
			RequiredParams: []string{"id"},
		},
		want: map[string]any{"id": 3},
	},
	{
		name: "GroupCredentialListUsesCredentialAction",
		task: evalTask{
			ID:             "MT-133",
			Prompt:         "List group personal access tokens for group `my-org`, filtering active tokens.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.credential_list_pats",
			RequiredParams: []string{"group_id"},
			OptionalParams: []string{"state", "per_page"},
		},
		want: map[string]any{"group_id": "my-org", "state": "active"},
	},
	{
		name: "GroupCredentialRevokeUsesTokenID",
		task: evalTask{
			ID:             "MT-134",
			Prompt:         "Revoke group personal access token ID `77` in group `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.credential_revoke_pat",
			RequiredParams: []string{"group_id", "token_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"group_id": "my-org", "token_id": 77},
	},
	{
		name: "GroupEpicBoardListUsesEpicBoardAction",
		task: evalTask{
			ID:             "MT-135",
			Prompt:         "List epic boards for group `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.epic_board_list",
			RequiredParams: []string{"group_id"},
			OptionalParams: []string{"per_page"},
		},
		want: map[string]any{"group_id": "my-org"},
	},
	{
		name: "GroupEpicCreateUsesFullPathAndTitle",
		task: evalTask{
			ID:             "MT-137",
			Prompt:         "Create an epic titled `Evaluation Epic` in group full path `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.epic_create",
			RequiredParams: []string{"full_path", "title"},
			OptionalParams: []string{"description", "start_date", "due_date"},
		},
		want: map[string]any{"full_path": "my-org", "title": "Evaluation Epic"},
	},
	{
		name: "GroupEpicDeleteUsesEpicIID",
		task: evalTask{
			ID:             "MT-139",
			Prompt:         "Delete epic IID `12` from group full path `my-org`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.epic_delete",
			RequiredParams: []string{"full_path", "epic_iid"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"full_path": "my-org", "epic_iid": 12},
	},
	{
		name: "GroupEpicListUsesFullPath",
		task: evalTask{
			ID:             "MT-136",
			Prompt:         "List epics in group full path `my-org` including descendant groups.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.epic_list",
			RequiredParams: []string{"full_path"},
			OptionalParams: []string{"include_descendants", "state", "first"},
		},
		want: map[string]any{"full_path": "my-org", "include_descendants": true},
	},
	{
		name: "GroupEpicUpdateUsesEpicIID",
		task: evalTask{
			ID:             "MT-138",
			Prompt:         "Update epic IID `12` in group full path `my-org` to close it.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "group.epic_update",
			RequiredParams: []string{"full_path", "epic_iid"},
			OptionalParams: []string{"state_event", "title"},
		},
		want: map[string]any{"full_path": "my-org", "epic_iid": 12, "state_event": "close"},
	},
	{
		name: "InstanceVariableCreateUsesExactToolCall",
		task: evalTask{
			ID:             "MT-068",
			Prompt:         "Create instance CI variable `INSTANCE_EVAL_TOKEN` with value `masked-value-123`.",
			ExpectedTool:   "gitlab_ci_variable",
			ExpectedAction: "instance_create",
			RequiredParams: []string{"key", "value"},
			OptionalParams: []string{"masked", "protected"},
		},
		want: map[string]any{"key": "INSTANCE_EVAL_TOKEN", "value": "masked-value-123"},
	},
	{
		name: "IssueAwardDeleteUsesAwardID",
		task: evalTask{
			ID:             "MT-110",
			Prompt:         "Remove award emoji ID `22` from issue `42` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "issue.emoji_issue_delete",
			RequiredParams: []string{"project_id", "issue_iid", "award_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "issue_iid": 42, "award_id": 22},
	},
	{
		name: "MRAwardDeleteUsesAwardID",
		task: evalTask{
			ID:             "MT-109",
			Prompt:         "Remove award emoji ID `21` from merge request `1` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "merge_request.emoji_mr_delete",
			RequiredParams: []string{"project_id", "merge_request_iid", "award_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "merge_request_iid": 1, "award_id": 21},
	},
	{
		name: "PipelineScheduleDeleteUsesScheduleID",
		task: evalTask{
			ID:             "MT-103",
			Prompt:         "Delete pipeline schedule ID `49` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "pipeline.schedule_delete",
			RequiredParams: []string{"project_id", "schedule_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"schedule_id": 49},
	},
	{
		name: "PipelineTriggerDeleteUsesTriggerID",
		task: evalTask{
			ID:             "MT-102",
			Prompt:         "Delete pipeline trigger token ID `77` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "pipeline.trigger_delete",
			RequiredParams: []string{"project_id", "trigger_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"trigger_id": 77},
	},
	{
		name: "ProjectGetUsesExactToolCall",
		task: evalTask{
			ID:             "MT-002",
			Prompt:         "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.",
			ExpectedTool:   "gitlab_project",
			ExpectedAction: "get",
			RequiredParams: []string{"project_id"},
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"},
	},
	{
		name: "SingleFailedPipelineJobsUsesExactToolCall",
		task: evalTask{
			ID:             "MT-021",
			Prompt:         "List failed jobs in pipeline `1323` for project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab_job",
			ExpectedAction: "list",
			RequiredParams: []string{"project_id", "pipeline_id"},
			OptionalParams: []string{"scope"},
		},
		want: map[string]any{"pipeline_id": 1323, "project_id": "my-org/tools/gitlab-mcp-server", "scope": "failed"},
	},
	{
		name: "SingleFileCreateUsesExactToolCall",
		task: evalTask{
			ID:             "MT-030",
			Prompt:         "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab_repository",
			ExpectedAction: "file_create",
			RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"},
		},
		want: map[string]any{"file_path": "tmp/eval.txt", "content": "evaluation file", "branch": "feature/eval", "commit_message": "Create evaluation file"},
	},
	{
		name: "SplitDiscussionResolveUsesExactToolCall",
		task: evalTask{
			ID:             "MT-061",
			Prompt:         "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab_mr_review",
			ExpectedAction: "discussion_resolve",
			RequiredParams: []string{"project_id", "merge_request_iid", "discussion_id"},
			OptionalParams: []string{"resolved"},
		},
		want: map[string]any{"discussion_id": "abc123", "merge_request_iid": 7, "resolved": true},
	},
	{
		name: "UserBlockUsesUserID",
		task: evalTask{
			ID:             "MT-104",
			Prompt:         "Block user ID `69`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "user.block",
			RequiredParams: []string{"user_id"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"user_id": 69},
	},
	{
		name: "WikiDeleteUsesSlug",
		task: evalTask{
			ID:             "MT-108",
			Prompt:         "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.",
			ExpectedTool:   "gitlab",
			ExpectedAction: "wiki.delete",
			RequiredParams: []string{"project_id", "slug"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		},
		want: map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "slug": "obsolete-eval"},
	},
}

// TestExactCallParams_ResolvesEveryRecordedFixture is what those thirty-eight
// per-case prompt tests became.
//
// Each of them built one task fixture, rendered its prompt through the
// exact-call builder, and asserted the marshaled envelope contained the
// resolved values. The envelope is the leak V05 removes and the assertion on it
// went with the builder; the resolution underneath it did not. exactCallParams
// and resolveExactParamProvenance are kept for V09, they are still reached from
// runner.go and validation.go, and those prompt tests were their only coverage,
// so deleting them outright would have left two live functions almost untested
// and the loss would have surfaced a gate later as a coverage number.
//
// What is asserted is therefore what those tests were really worth: that a
// value written in a user's sentence is recovered as the parameter it belongs
// to. Containment rather than map equality, which is what the old assertions
// did against the marshaled JSON, so an optional parameter no fixture pins
// stays out of the comparison.
func TestExactCallParams_ResolvesEveryRecordedFixture(t *testing.T) {
	for _, tc := range exactCallParamFixtures {
		t.Run(tc.name, func(t *testing.T) {
			steps := taskSteps(tc.task)
			if len(steps) != 1 {
				t.Fatalf("taskSteps() = %d steps, want exactly one", len(steps))
			}
			got, _ := exactCallParams(steps[0], tc.task.Prompt, true)
			assertResolvedParams(t, got, tc.want)
		})
	}
}

// assertResolvedParams fails for every expected parameter the resolver did not
// recover, or recovered as something else.
func assertResolvedParams(t *testing.T, got, want map[string]any) {
	t.Helper()
	for param, expected := range want {
		value, ok := got[param]
		if !ok {
			t.Errorf("param %s not resolved; got %#v", param, got)
			continue
		}
		// DeepEqual rather than a string comparison: the claim is that a value
		// is recovered as the parameter it belongs to *and* as the type that
		// parameter takes, and rendering both sides through fmt.Sprint would
		// make 7 and "7" the same answer, which is exactly the resolver bug
		// worth catching.
		if !reflect.DeepEqual(value, expected) {
			t.Errorf("param %s = %#v, want %#v", param, value, expected)
		}
	}
}

// answerKeyFields are the two fields of evalStep that hold the answer a case is
// scored against: which tool the model was supposed to call and which action.
//
// A prompt builder that reads either of them is writing the answer into the
// question. That is not a style matter: a score produced from such a prompt
// says how well a model transcribes, and the whole point of this evaluator is
// to say how well the *surface* describes itself.
var answerKeyFields = []string{"ExpectedTool", "ExpectedAction"}

// answerKeyExemptFunctions may read those fields despite being reachable from a
// prompt builder, each for a stated reason.
//
// The plan asked for exactly one, and one is not reachable. The second is
// structural rather than a concession: taskSteps is what *builds* the
// []evalStep every other function receives, so it necessarily touches every
// field of a step, and anything that needs any field at all drags it in. Today
// that is taskHasSimulationMode, which reads step.Simulation to find out
// whether the fixture is simulating a transient error — a property of the
// environment the run is in, not of the answer. The number could be forced to
// one by giving the simulation check its own path to the steps, and that would
// be a second way to build them for the sake of a count.
//
//   - taskHasDestructiveStep asks a different question. Whether a task is
//     destructive is a property of what the user asked for, not of the answer:
//     someone who says "delete the branch" has already said it, and the prompt
//     may repeat that without revealing which action performs it.
//   - taskSteps constructs the steps. Without that read there is nothing to
//     leak and nothing to score.
//
// An exemption covers what the excused function calls, since both of these
// return a verdict and write no prompt text; see reachableFrom.
var answerKeyExemptFunctions = []string{"taskHasDestructiveStep", "taskSteps"}

// promptEntryPoints are the four builders that write the scaffolding around a
// case's own words.
//
// Not taskPromptForSurface, which a run actually calls, and the difference is
// the whole scope of this gate. That function first replaces the task's prompt
// with the case's rendered text, and rendering reaches the fixture machinery,
// which mentions the steps for reasons that have nothing to do with prompts.
// Following it made the walk report taskSteps and demand an exemption for the
// function that *builds* the steps, which would have been an exemption for the
// scope being wrong rather than for anything the prompts do.
//
// The claim this gate makes is narrower and is the one worth making: the text
// this package writes around the user's words names no answer. The user's own
// words are the audit's `case` site, which it reports and says plainly that no
// change to this package can take away. Verified rather than assumed:
// RenderCasePrompt fills its template from FixtureOutput, the values a fixture
// produced, and consults no expected step.
var promptEntryPoints = []string{"taskPrompt", "dynamicTaskPrompt", "systemPrompt", "dynamicSystemPrompt"}

// TestPromptBuilders_NeverReadTheAnswerKey is the gate that keeps V06's
// deletion deleted.
//
// The leak it removes did not arrive in one commit. It grew one helpful clause
// at a time, each of them defensible on its own, which is how nineteen rules
// came to switch on the expected action and how a case ended up being told the
// tool it was about to be graded on choosing. Deleting the clauses without a
// gate would leave exactly the conditions that produced them.
//
// It reads the package with go/ast rather than reflection because the property
// is about the source: a field read is visible there whether or not any test
// happens to drive the branch containing it.
func TestPromptBuilders_NeverReadTheAnswerKey(t *testing.T) {
	functions, err := packageFunctions(".")
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	for _, entry := range promptEntryPoints {
		if _, ok := functions[entry]; !ok {
			t.Fatalf("entry point %s is not defined in this package: the gate would pass by reaching nothing", entry)
		}
	}

	var offenders []string
	for _, name := range sortedNamesOf(reachableFrom(functions, promptEntryPoints)) {
		if slices.Contains(answerKeyExemptFunctions, name) {
			continue
		}
		if fields := answerKeyFieldsRead(functions[name]); len(fields) > 0 {
			offenders = append(offenders, name+" reads "+strings.Join(fields, " and "))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("prompt builders reading the answer key:\n  %s\n\nA prompt that names the case's own expected tool or action measures transcription, not the surface. Remove the clause, or state why it is a property of the request rather than of the answer and declare it in answerKeyExemptFunctions.",
			strings.Join(offenders, "\n  "))
	}
}

// TestPromptAnswerKeyGate_ExemptionsDescribeTheTree fails when an exemption
// stops matching anything, on the terms every declaration table in this
// repository is held to: a list that excuses nothing is a list a reader learns
// to skip.
func TestPromptAnswerKeyGate_ExemptionsDescribeTheTree(t *testing.T) {
	functions, err := packageFunctions(".")
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	reachable := reachableFrom(functions, promptEntryPoints)
	for _, name := range answerKeyExemptFunctions {
		declarations, ok := functions[name]
		if !ok {
			t.Errorf("exempt function %s is not defined in this package", name)
			continue
		}
		if !reachable[name] {
			t.Errorf("exempt function %s is no longer reachable from a prompt builder, so the exemption excuses nothing", name)
			continue
		}
		if len(answerKeyFieldsRead(declarations)) == 0 {
			t.Errorf("exempt function %s no longer reads the answer key, so the exemption excuses nothing", name)
		}
	}
}

// packageFunctions parses every non-test Go file in dir and returns every
// declaration of each name.
//
// Every declaration, not the last one seen. Methods are keyed by their bare
// name here, so `String` on one type and `String` on another share a key, and
// a map of one declaration per name would keep whichever file was read last
// and silently drop the rest. A gate that can drop a declaration can miss the
// read it exists to find, and it would miss it quietly, which is worse than
// not having the gate.
func packageFunctions(dir string) (map[string][]*ast.FuncDecl, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	functions := map[string][]*ast.FuncDecl{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Body != nil {
				functions[function.Name.Name] = append(functions[function.Name.Name], function)
			}
		}
	}
	return functions, nil
}

// reachableFrom walks the call graph from the named entry points.
//
// A call is any identifier used as a function value, not only one in call
// position, so a rule passed to a dispatcher as `appendGroupGuidance` counts
// as reached. That is exactly how taskRetryGuidance applies its rules, and a
// walk that only followed call expressions would see none of them.
func reachableFrom(functions map[string][]*ast.FuncDecl, entryPoints []string) map[string]bool {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		declarations, ok := functions[name]
		if !ok {
			return
		}
		seen[name] = true
		// An exemption covers what the excused function calls, not only the
		// function itself. taskHasDestructiveStep reads the steps through
		// taskSteps, which builds them; walking into it would demand a second
		// exemption for the machinery the first one needs, and taskSteps would
		// then be excused everywhere rather than under the one caller the
		// exemption is about. What makes this safe is that an excused function
		// returns a verdict and writes no prompt text.
		if slices.Contains(answerKeyExemptFunctions, name) {
			return
		}
		for _, declaration := range declarations {
			ast.Inspect(declaration.Body, func(node ast.Node) bool {
				if identifier, isIdent := node.(*ast.Ident); isIdent {
					if _, defined := functions[identifier.Name]; defined {
						visit(identifier.Name)
					}
				}
				return true
			})
		}
	}
	for _, entry := range entryPoints {
		visit(entry)
	}
	return seen
}

// answerKeyFieldsRead names the answer-key fields any declaration of one name
// selects, so a name shared by several declarations is judged by all of them.
func answerKeyFieldsRead(declarations []*ast.FuncDecl) []string {
	found := map[string]bool{}
	for _, declaration := range declarations {
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && slices.Contains(answerKeyFields, selector.Sel.Name) {
				found[selector.Sel.Name] = true
			}
			return true
		})
	}
	return sortedNamesOf(found)
}

// sortedNamesOf renders a name set in a stable order, so a failure lists the
// same thing twice in a row.
func sortedNamesOf(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestPromptContract_StatesTheSurfaceAndNothingElse pins what a prompt is
// allowed to say, now that everything per-case has gone.
//
// The claim is two-sided and both sides matter. A prompt must still carry the
// user's words and whether what they asked for is destructive, or a model is
// being asked to guess; and it must state how this surface is called, or the
// score measures the model's prior rather than the surface. What it must not do
// is name the case's own action, tool or parameters, which the answer-key gate
// and the prompt audit check across the whole corpus.
func TestPromptContract_StatesTheSurfaceAndNothingElse(t *testing.T) {
	task := evalTask{
		ID:     "MS-contract",
		Prompt: "Close issue `7` in project `my-org/tools/gitlab-mcp-server`.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_issue", ExpectedAction: "update", RequiredParams: []string{"project_id", "issue_iid"}},
			{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", Destructive: true},
		},
	}
	surfaces := []struct {
		name    string
		surface string
		states  []string
	}{
		{
			name:    "meta states the envelope",
			surface: config.ToolSurfaceMeta,
			states:  []string{`{"action":"...","params":{...}}`},
		},
		{
			name:    "dynamic states find-then-execute",
			surface: config.ToolSurfaceDynamic,
			states:  []string{"gitlab_find_action", "gitlab_execute_action"},
		},
	}
	for _, tc := range surfaces {
		t.Run(tc.name, func(t *testing.T) {
			stimulus := systemPromptForTask(task, tc.surface) + "\n" + taskPromptForSurface(task, tc.surface)
			for _, want := range tc.states {
				if !strings.Contains(stimulus, want) {
					t.Errorf("the stimulus does not state %q:\n%s", want, stimulus)
				}
			}
			if !strings.Contains(stimulus, task.Prompt) {
				t.Errorf("the stimulus dropped the user's own words:\n%s", stimulus)
			}
			if !strings.Contains(stimulus, "Destructive: Yes") {
				t.Errorf("the stimulus does not say the task is destructive:\n%s", stimulus)
			}
			for _, leaked := range []string{"issue.update", "issue.delete", "gitlab_issue", "issue_iid", "confirm"} {
				if strings.Contains(stimulus, leaked) {
					t.Errorf("the stimulus names %q, which is this case's own answer:\n%s", leaked, stimulus)
				}
			}
		})
	}
}
