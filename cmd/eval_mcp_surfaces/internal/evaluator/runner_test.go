package evaluator

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCapabilityBridgePredicates_ClassifyBridgeTools verifies runner bridge
// detection is shared by generic and expected-step paths.
func TestCapabilityBridgePredicates_ClassifyBridgeTools(t *testing.T) {
	if !isCapabilityBridge(modelContentBlock{Name: resourceReadTool}) {
		t.Fatal("isCapabilityBridge(resourceReadTool) = false, want true")
	}
	if isCapabilityBridge(modelContentBlock{Name: dynamicFindTool}) {
		t.Fatal("isCapabilityBridge(dynamicFindTool) = true, want false")
	}
	if !expectedCapabilityBridgeStep(evalStep{ExpectedTool: completionTool}) {
		t.Fatal("expectedCapabilityBridgeStep(completion) = false, want true")
	}
	if expectedCapabilityBridgeStep(evalStep{ExpectedTool: completionTool, ExpectedAction: "schema_get"}) {
		t.Fatal("expectedCapabilityBridgeStep(with action) = true, want false")
	}
}

// TestRecordCapabilityBridgeMetrics_CountsResourcesSeparately verifies bridge
// calls update both aggregate capability and resource-specific metrics.
func TestRecordCapabilityBridgeMetrics_CountsResourcesSeparately(t *testing.T) {
	var result taskResult
	recordCapabilityBridgeMetrics(&result, modelContentBlock{Name: resourceListTool})
	recordCapabilityBridgeMetrics(&result, modelContentBlock{Name: promptListTool})
	if !result.CapabilityLookupUsed || result.CapabilityCalls != 2 {
		t.Fatalf("capability metrics = used %t calls %d, want true/2", result.CapabilityLookupUsed, result.CapabilityCalls)
	}
	if !result.ResourceLookupUsed || result.ResourceCalls != 1 {
		t.Fatalf("resource metrics = used %t calls %d, want true/1", result.ResourceLookupUsed, result.ResourceCalls)
	}
}

// TestToolUseBlocks_FiltersNonToolContent verifies only provider tool-use blocks
// participate in validation and execution.
func TestToolUseBlocks_FiltersNonToolContent(t *testing.T) {
	blocks := toolUseBlocks([]modelContentBlock{{Type: "text", Text: "hello"}, {Type: "tool_use", Name: dynamicFindTool}})
	if len(blocks) != 1 || blocks[0].Name != dynamicFindTool {
		t.Fatalf("toolUseBlocks() = %+v, want only dynamic find tool", blocks)
	}
}

func TestValidateDynamicFindResult_UsesFullMCPResponse(t *testing.T) {
	steps := []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get"},
	}
	exchange := &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"project.get"}]}}`)}
	if err := validateDynamicFindResult(steps, 0, "truncated payload without the action", exchange); err != nil {
		t.Fatalf("validateDynamicFindResult() error = %v, want full MCP response match", err)
	}
	missing := &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"project.list"}]}}`)}
	if err := validateDynamicFindResult(steps, 0, "truncated payload without the action", missing); err == nil {
		t.Fatal("validateDynamicFindResult() error = nil, want missing expected action error")
	}
}

// TestRedactResponse_TruncatesLargeProviderBodies verifies provider trace errors
// stay compact in terminal and report diagnostics.
func TestRedactResponse_TruncatesLargeProviderBodies(t *testing.T) {
	large := make([]byte, 1200)
	for i := range large {
		large[i] = 'x'
	}
	got := redactResponse(large)
	if len(got) != 1003 || got[len(got)-3:] != "..." {
		t.Fatalf("redactResponse() length/suffix = %d/%q, want 1003/...", len(got), got[len(got)-3:])
	}
}

// TestRunnerTraceSummaryAndResourceHelpers verifies small runner helpers used by
// simulated execution and trace finalization.
func TestRunnerTraceSummaryAndResourceHelpers(t *testing.T) {
	if got := snippetFilePathFromParams(map[string]any{"files": []any{map[string]any{"file_path": "src/snippet.go"}}}); got != "src/snippet.go" {
		t.Fatalf("snippetFilePathFromParams(files) = %q", got)
	}
	if got := snippetFilePathFromParams(map[string]any{}); got != "snippet.txt" {
		t.Fatalf("snippetFilePathFromParams(default) = %q", got)
	}
	result := map[string]any{}
	addSimulatedResourceIDs(result, "snippet.project_create", map[string]any{"project_id": "p", "file_name": "main.go"})
	if result["snippet_id"] != 103 || result["snippet"].(map[string]any)["file_path"] != "main.go" {
		t.Fatalf("simulated snippet result = %#v", result)
	}
	summary := traceSummaryFromResult(taskResult{Task: evalTask{Steps: []evalStep{{}, {}}}, FirstTool: "tool", FinalTool: "final", FirstPass: true, FinalSuccess: true, CompletedSteps: 2, Notes: []string{"a", "b"}})
	if summary.ExpectedSteps != 2 || summary.Notes != "a; b" || !summary.FinalSuccess {
		t.Fatalf("traceSummaryFromResult() = %+v, want expected steps and notes", summary)
	}
}

func TestEvaluatePreparedCase_UsesRenderedPromptAndTypedSteps(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "fixture/project"}}),
	)
	prepared := PreparedCase{
		Case: EvalCase{
			ID:     "MT-PREPARED-001",
			Prompt: "Get project `placeholder/project`.",
			Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}},
		},
		Prompt: "Get project `fixture/project`.",
		Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}, ProducedValues: []string{"project_id"}}},
	}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluatePreparedCase(t.Context(), prepared, nil, routes)

	if !result.FinalSuccess || result.CompletedSteps != 1 {
		t.Fatalf("result = %+v, want prepared case success", result)
	}
	if result.Task.Prompt != "Get project `fixture/project`." || !strings.Contains(result.Trace.UserPrompt, "fixture/project") {
		t.Fatalf("prompt task=%q trace=%q, want rendered fixture prompt", result.Task.Prompt, result.Trace.UserPrompt)
	}
	if got := strings.Join(result.Task.Steps[0].ProducedValues, ","); got != "project_id" {
		t.Fatalf("produced values = %q, want project_id", got)
	}
	if !hasPassedAssertion(result.AssertionResults, CaseAssertionExpectedAction) || !hasPassedAssertion(result.AssertionResults, CaseAssertionRequiredParams) {
		t.Fatalf("assertion results = %+v, want expected action and required params pass", result.AssertionResults)
	}
}

func TestEvaluateTask_AcceptsOptionalCapabilityBridgePreludeSkip(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("resources", resourceListTool, map[string]any{}),
		toolUseResponse("tools", resourceReadTool, map[string]any{"uri": "gitlab://tools"}),
	)
	runner.mcpSession = newResourceLookupSessionForTest(t)
	task := evalTask{ID: "MS-039", Steps: []evalStep{
		{ExpectedTool: capabilityListTool, OptionalStep: true},
		{ExpectedTool: resourceListTool},
		{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, nil)

	if !result.FinalSuccess || !result.FirstPass || result.RepairAttempted {
		t.Fatalf("result = %+v, want first-pass success without repair", result)
	}
	toolOK, actionOK, firstPassOK := effectiveFirstOutcome(result)
	if !toolOK || !actionOK || !firstPassOK {
		t.Fatalf("effective first outcome = %t/%t/%t, want all true", toolOK, actionOK, firstPassOK)
	}
	if result.CompletedSteps != 3 || result.FirstTool != resourceListTool {
		t.Fatalf("completed/first = %d/%s, want 3/%s", result.CompletedSteps, result.FirstTool, resourceListTool)
	}
	if !result.ResourceLookupUsed || result.ResourceCalls != 2 || result.CapabilityCalls != 2 {
		t.Fatalf("bridge metrics = resource:%t resource_calls:%d capability_calls:%d, want two resource bridge calls", result.ResourceLookupUsed, result.ResourceCalls, result.CapabilityCalls)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "accepted optional") {
		t.Fatalf("notes = %v, want accepted optional note", result.Notes)
	}
}

func TestEvaluateTask_AcceptsDirectDynamicExecuteWithoutFind(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("exec", dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-DYN-DIRECT-001", Steps: []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, nil)

	if !result.FinalSuccess || !result.FirstPass || result.RepairAttempted {
		t.Fatalf("result = %+v, want first-pass success without repair", result)
	}
	if result.CompletedSteps != 2 {
		t.Fatalf("completed steps = %d, want 2", result.CompletedSteps)
	}
	if result.FirstTool != dynamicExecuteActionTool || result.FirstAction != actionProjectGet {
		t.Fatalf("first call = %s/%s, want %s/%s", result.FirstTool, result.FirstAction, dynamicExecuteActionTool, actionProjectGet)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "accepted direct") {
		t.Fatalf("notes = %v, want accepted direct note", result.Notes)
	}
}

func hasPassedAssertion(results []CaseAssertionResult, assertionType CaseAssertionType) bool {
	for _, result := range results {
		if result.Type == assertionType && result.Passed {
			return true
		}
	}
	return false
}
