package evaluator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
