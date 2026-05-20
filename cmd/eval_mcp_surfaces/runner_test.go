package main

import "testing"

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
