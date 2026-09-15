package evaluator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
