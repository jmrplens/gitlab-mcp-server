package evaluator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestStringList_ImplementsFlagValue verifies repeated CLI flags preserve every
// supplied value and render as a comma-separated label.
func TestStringList_ImplementsFlagValue(t *testing.T) {
	var values stringList
	if err := values.Set("one"); err != nil {
		t.Fatalf("Set(one) error = %v", err)
	}
	_ = values.Set("two")
	if got := values.String(); got != "one,two" {
		t.Fatalf("String() = %q, want one,two", got)
	}
}

// TestModelContentBlockMarshalJSON_PreservesToolUseInputOnly verifies provider
// history serialization keeps Anthropic-required tool input without adding empty
// input objects to ordinary text blocks.
func TestModelContentBlockMarshalJSON_PreservesToolUseInputOnly(t *testing.T) {
	toolData, err := json.Marshal(modelContentBlock{Type: "tool_use", ID: "toolu", Name: capabilityListTool})
	if err != nil {
		t.Fatalf("Marshal(tool_use) error = %v", err)
	}
	if !strings.Contains(string(toolData), `"input":{}`) {
		t.Fatalf("tool JSON = %s, want empty input object", toolData)
	}
	textData, err := json.Marshal(modelContentBlock{Type: "text", Text: "hello"})
	if err != nil {
		t.Fatalf("Marshal(text) error = %v", err)
	}
	if strings.Contains(string(textData), "input") {
		t.Fatalf("text JSON = %s, want no input field", textData)
	}
}

// TestModelUsageAdd_AccumulatesAllTokenBuckets verifies usage aggregation covers
// prompt, completion, and cache token classes.
func TestModelUsageAdd_AccumulatesAllTokenBuckets(t *testing.T) {
	usage := modelUsage{InputTokens: 1, OutputTokens: 2, CacheCreationInputTokens: 3, CacheReadInputTokens: 4}
	usage.add(modelUsage{InputTokens: 10, OutputTokens: 20, CacheCreationInputTokens: 30, CacheReadInputTokens: 40})
	if usage != (modelUsage{InputTokens: 11, OutputTokens: 22, CacheCreationInputTokens: 33, CacheReadInputTokens: 44}) {
		t.Fatalf("usage = %+v, want summed buckets", usage)
	}
}

// TestModelProviderCallError_WrapsProviderTrace verifies provider failures keep
// both an ordinary error chain and trace metadata.
func TestModelProviderCallError_WrapsProviderTrace(t *testing.T) {
	base := errors.New("provider failed")
	err := &modelProviderCallError{err: base, Trace: &modelProviderTrace{ResponseStatus: 500}}
	if err.Error() != "provider failed" {
		t.Fatalf("Error() = %q, want provider failed", err.Error())
	}
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is(err, base) = false, unwrap %v", errors.Unwrap(err))
	}
	if err.Trace.ResponseStatus != 500 {
		t.Fatalf("trace status = %d, want 500", err.Trace.ResponseStatus)
	}
}

// TestDynamicCallBudgetForTask_ExactAndAmbiguousTasks_ShareOneDiscoveryBudget
// pins the fact that callBudgetForTask does not classify tasks at all: it
// hardcodes AllowedDiscoveryCalls to 0 and never sets SuppressDiscovery, so an
// exact task and an ambiguous one get the same discovery budget. Only the
// step-derived fields vary, and those are asserted here so a change to either
// behavior is caught.
func TestDynamicCallBudgetForTask_ExactAndAmbiguousTasks_ShareOneDiscoveryBudget(t *testing.T) {
	exactTask := evalTask{ID: "MT-066", Prompt: "Remove project ID `51` from the CI job token allowlist of project `1`.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
	}}
	exactBudget := callBudgetForTask(exactTask, config.ToolSurfaceDynamic)
	if exactBudget.ExpectedSteps != 1 || exactBudget.AllowedDiscoveryCalls != 0 || exactBudget.SuppressDiscovery {
		t.Fatalf("exact budget = %+v, want no discovery suppression", exactBudget)
	}

	ambiguousTask := evalTask{ID: "MT-AMB", Prompt: "Find the right project cleanup action.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.delete", RequiredParams: []string{"project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
	}}
	ambiguousBudget := callBudgetForTask(ambiguousTask, config.ToolSurfaceDynamic)
	if ambiguousBudget.AllowedDiscoveryCalls != 0 || ambiguousBudget.SuppressDiscovery {
		t.Fatalf("ambiguous budget = %+v, want default discovery budget", ambiguousBudget)
	}
	if ambiguousBudget.ExpectedSteps != exactBudget.ExpectedSteps {
		t.Fatalf("ExpectedSteps: ambiguous = %d, exact = %d; both tasks have one step",
			ambiguousBudget.ExpectedSteps, exactBudget.ExpectedSteps)
	}
	// MaxCalls is the only field a caller can act on, and it must leave room for
	// the repair attempts the surface allows on top of the expected steps. Both
	// budgets are checked: a task-dependent repair allowance would otherwise
	// undersize the ambiguous one without failing the equality check below.
	for _, tt := range []struct {
		name   string
		budget taskCallBudget
	}{
		{"exact", exactBudget},
		{"ambiguous", ambiguousBudget},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.budget.MaxCalls < tt.budget.ExpectedSteps+tt.budget.AllowedRepairCalls {
				t.Errorf("budget = %+v, want MaxCalls >= ExpectedSteps+AllowedRepairCalls", tt.budget)
			}
		})
	}
	if ambiguousBudget.MaxCalls != exactBudget.MaxCalls {
		t.Fatalf("MaxCalls: ambiguous = %d, exact = %d; the budget is not task-dependent",
			ambiguousBudget.MaxCalls, exactBudget.MaxCalls)
	}
}

// TestDynamicSingleTaskPrompt_UsesFindFirstForHighRiskShapes verifies
// single-step Dynamic tasks with historically brittle parameter shapes still use
// generic find-first guidance without leaking exact call envelopes.
func TestDynamicSingleTaskPrompt_UsesFindFirstForHighRiskShapes(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		absent []string
	}{
		{
			name: "repository file get",
			task: evalTask{ID: "MT-029", Prompt: "Get file `README.md` from branch `main` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
			}},
		},
		{
			name: "repository file create",
			task: evalTask{ID: "MT-030", Prompt: "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			}},
		},
		{
			name: "single artifact download",
			task: evalTask{ID: "MT-065", Prompt: "Download artifact `coverage/report.xml` from job `361` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.download_single_artifact", RequiredParams: []string{"project_id", "job_id", "artifact_path"}},
			}},
		},
		{
			name: "runner remove",
			task: evalTask{ID: "MT-047", Prompt: "Remove runner ID `21`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "runner.remove", RequiredParams: []string{"runner_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "pipeline schedule delete",
			task: evalTask{ID: "MT-103", Prompt: "Delete pipeline schedule ID `46` from project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.schedule_delete", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "user block",
			task: evalTask{ID: "MT-104", Prompt: "Block user ID `55`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "user.block", RequiredParams: []string{"user_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "pipeline trigger delete",
			task: evalTask{ID: "MT-102", Prompt: "Delete pipeline trigger token ID `53` from project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.trigger_delete", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "terraform state unlock",
			task: evalTask{ID: "MT-114", Prompt: "Unlock Terraform state `production` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.terraform_state_unlock", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "broadcast message delete",
			task: evalTask{ID: "MT-054", Prompt: "Delete broadcast message ID `9`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.broadcast_message_delete", RequiredParams: []string{"id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{`"id":123`},
		},
		{
			name: "project push rule add regex",
			task: evalTask{ID: "MT-192", Prompt: "Add a project push rule to project `my-org/tools/eval-push-rule` with commit message regex `^EVAL-`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.push_rule_add", RequiredParams: []string{"project_id"}, OptionalParams: []string{"commit_message_regex", "reject_unsigned_commits"}},
			}},
			absent: []string{`"commit_message_regex_enabled":`},
		},
		{
			name: "group service account PAT revoke",
			task: evalTask{ID: "MT-197", Prompt: "Revoke group service account PAT ID `23` for service account user ID `39` in group `my-org`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.service_account_pat_revoke", RequiredParams: []string{"group_id", "service_account_id", "token_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{`"action":"service_account_pat.revoke"`, `"personal_access_token_id":`, `"user_id":`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic)
			if strings.Contains(prompt, "confirm:true in params") {
				t.Fatalf("taskPromptForSurface() = %q, dynamic prompt must not tell models to put confirm in params", prompt)
			}
			requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
				"first call gitlab_find_action",
				"Use the returned result ID, input_schema, required_params, and example",
				"Do not use action IDs from memory",
			})
			for _, step := range taskSteps(tt.task) {
				if step.ExpectedAction != "" && strings.Contains(prompt, step.ExpectedAction) {
					t.Fatalf("taskPromptForSurface() leaked expected action %q in prompt %q", step.ExpectedAction, prompt)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(prompt, unwanted) {
					t.Fatalf("taskPromptForSurface() = %q, want no %q", prompt, unwanted)
				}
			}
		})
	}
}

// TestDynamicSingleTaskPrompt_TerraformStateUnlockExactCallAvoidsLegacyEnvelope
// verifies that the dynamic-surface task prompt for a single destructive
// Terraform state unlock points the model at gitlab_find_action first,
// includes the new schema-driven guidance, and never carries the legacy
// meta-tool terraform_state.unlock envelope from earlier iterations.
//
// The test asserts the rendered prompt contains the find-first guidance,
// top-level confirm:true, and the explicit warning to avoid action IDs from
// memory, and rejects three legacy envelope patterns. This protects the
// dynamic surface from regressions that would reintroduce the deprecated
// terraform state envelope.
func TestDynamicSingleTaskPrompt_TerraformStateUnlockExactCallAvoidsLegacyEnvelope(t *testing.T) {
	task := evalTask{ID: "MT-114", Prompt: "Unlock Terraform state `production` in project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.terraform_state_unlock", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"confirm"}, Destructive: true},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	required := []string{
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"top-level confirm:true",
		"Do not use action IDs from memory",
	}
	requireContainsAll(t, "taskPromptForSurface()", prompt, required)
	for _, unwanted := range []string{`"action":"terraform_state.unlock"`, `"terraform_state_name":`, `"action":"admin.terraform_state_unlock"`} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPromptForSurface() = %q, want no legacy terraform state envelope %q", prompt, unwanted)
		}
	}
}

// TestDynamicSingleTaskPrompt_UsesFindFirstForOptionalOnlyList verifies optional-only list prompts stay find-first.
func TestDynamicSingleTaskPrompt_UsesFindFirstForOptionalOnlyList(t *testing.T) {
	task := evalTask{ID: "MT-003", Prompt: "List the 10 most recently updated projects I can access.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.list", OptionalParams: []string{"order_by", "sort", "per_page"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"Do not use action IDs from memory",
	})
	if strings.Contains(prompt, `"action":"project.list"`) || strings.Contains(prompt, "project.list") {
		t.Fatalf("taskPromptForSurface() = %q, want no exact project.list action", prompt)
	}
}

// TestDynamicSingleTaskPrompt_UsesFindFirstForSearchProjects verifies search prompts stay find-first.
func TestDynamicSingleTaskPrompt_UsesFindFirstForSearchProjects(t *testing.T) {
	task := evalTask{ID: "MT-033", Prompt: "Search all projects for `gitlab-mcp-server`.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "search.projects", RequiredParams: []string{"query"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"Do not use action IDs from memory",
	})
	if strings.Contains(prompt, `"action":"search.projects"`) || strings.Contains(prompt, "search.projects") {
		t.Fatalf("taskPromptForSurface() = %q, want no exact search.projects action", prompt)
	}
}

// TestDynamicSingleTaskPrompt_ExactProjectLookupPrefersProjectGet verifies exact project lookups
// steer models toward project.get instead of project.list.
func TestDynamicSingleTaskPrompt_ExactProjectLookupPrefersProjectGet(t *testing.T) {
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get", RequiredParams: []string{"project_id"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"requested catalog operation is project.get, not project.list",
		"first gitlab_find_action query should ask for project metadata for the exact namespace path",
		"follow-up gitlab_execute_action call must use project.get with params.project_id set to that exact path",
	})
	if strings.Contains(prompt, `"action":"project.list"`) {
		t.Fatalf("taskPromptForSurface() = %q, want no exact project.list action", prompt)
	}
}

// TestDynamicRepositoryFileCRUDPrompt_UsesFilePathFromOperation verifies dynamic
// file CRUD guidance extracts the repository file path instead of the project path.
func TestDynamicRepositoryFileCRUDPrompt_UsesFilePathFromOperation(t *testing.T) {
	task := evalTask{ID: "MS-017", Prompt: "Exercise repository file CRUD in project `my-org/tools/gitlab-mcp-server`: create file `tmp/eval-crud.txt` on branch `feature/eval`, read it, update its content, then delete it from the same branch.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"tmp/eval-crud.txt",
		"feature/eval",
		"my-org/tools/gitlab-mcp-server",
	})
	for _, unwanted := range []string{`"action":"repository.file_create"`, `"file_path":"my-org/tools/gitlab-mcp-server"`, "repository.file_create"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPromptForSurface() = %q, want no exact file CRUD content %q", prompt, unwanted)
		}
	}
}

// TestCanExecuteInvalidToolCallSkipsWrongDynamicReadOnlyAction verifies dynamic
// workflows receive exact repair guidance when the model substitutes a read-only action.
func TestCanExecuteInvalidToolCallSkipsWrongDynamicReadOnlyAction(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}}
	validation := validationResult{ToolMatches: true, ActionMatches: false, Action: "pipeline.list", RequiredPresent: false, DestructiveSafe: true, Message: "expected action pipeline.get, got pipeline.list; missing required params: pipeline_id"}
	toolUse := modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": "pipeline.list", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}}
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {"pipeline.list": toolutil.ActionRoute{}}}

	if runner.canExecuteInvalidToolCall(step, validation, toolUse, routes) {
		t.Fatal("canExecuteInvalidToolCall() = true, want wrong dynamic read-only action to receive exact repair guidance")
	}
}

// TestDynamicSingleTaskPrompt_ProjectPathUsesProjectID verifies that the
// dynamic-surface task prompt for an issue.update call instructs the model to
// pass the project path as params.project_id and explicitly warns against
// using full_path, path, or remote_url keys.
//
// The test asserts the rendered prompt contains the find-first guidance, the
// literal project path, the params.project_id mention, and the negative
// guidance about alternative keys, and rejects any exact "issue.update"
// action mention. This protects dynamic issue workflows from regressing to
// the wrong key name or hard-coding the action.
func TestDynamicSingleTaskPrompt_ProjectPathUsesProjectID(t *testing.T) {
	task := evalTask{ID: "MT-012", Prompt: "Close issue `10` in project `my-org/tools/gitlab-mcp-server` by setting `state_event` to `close`.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.update", RequiredParams: []string{"project_id", "issue_iid", "state_event"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	required := []string{
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"my-org/tools/gitlab-mcp-server",
		"params.project_id",
		"not params.full_path, params.path, or remote_url",
	}
	requireContainsAll(t, "taskPromptForSurface()", prompt, required)
	if strings.Contains(prompt, `"action":"issue.update"`) || strings.Contains(prompt, "issue.update") {
		t.Fatalf("taskPromptForSurface() = %q, want no exact issue.update action", prompt)
	}
}

// TestRunMCPSmokeRequiresGitLabBackend verifies RunMCPSmokeRequiresGitLabBackend.
func TestRunMCPSmokeRequiresGitLabBackend(t *testing.T) {
	err := runMCPSmoke(options{Backend: backendMock})
	if err == nil || !strings.Contains(err.Error(), "--backend=gitlab") {
		t.Fatalf("error = %v, want backend guard", err)
	}
}

// TestValidateExecutionOptionsRequiresDockerGuard verifies ValidateExecutionOptionsRequiresDockerGuard.
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

// TestCanExecuteInvalidToolCallSkipsUnexpectedMutations verifies CanExecuteInvalidToolCallSkipsUnexpectedMutations.
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

// TestCanExecuteInvalidToolCallSkipsUnknownParams verifies CanExecuteInvalidToolCallSkipsUnknownParams.
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
