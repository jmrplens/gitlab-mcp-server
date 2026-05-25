package evaluator

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestEvalCreateMessageHandler_ReturnsDeterministicSamplingResult verifies the
// sampling bridge returns stable mock analysis content for evaluator tools.
func TestEvalCreateMessageHandler_ReturnsDeterministicSamplingResult(t *testing.T) {
	result, err := evalCreateMessageHandler(t.Context(), nil)
	if err != nil {
		t.Fatalf("evalCreateMessageHandler() error = %v", err)
	}
	content, ok := result.Content.(*mcp.TextContent)
	if !ok || !strings.Contains(content.Text, "Mock Analysis") {
		t.Fatalf("content = %#v, want mock analysis text", result.Content)
	}
	if result.Model != "eval-mcp-surfaces-sampling-mock" || result.Role != "assistant" {
		t.Fatalf("result = %+v, want evaluator mock assistant", result)
	}
}

// TestEvalElicitationHandler_DerivesNestedDefaults verifies elicitation schemas
// are auto-accepted with type-aware deterministic fixture values.
func TestEvalElicitationHandler_DerivesNestedDefaults(t *testing.T) {
	previousTag, _ := evalElicitationReleaseTag.Load().(string)
	setEvalElicitationReleaseTag("v-test")
	t.Cleanup(func() { setEvalElicitationReleaseTag(previousTag) })
	result, err := evalElicitationHandler(t.Context(), &mcp.ElicitRequest{Params: &mcp.ElicitParams{RequestedSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed":       map[string]any{"type": "boolean"},
			"selection":       map[string]any{"type": "string", "enum": []any{"private", "internal"}},
			"count":           map[string]any{"type": []any{"null", "integer"}},
			"ratio":           map[string]any{"type": "number"},
			"items":           map[string]any{"type": "array"},
			"tag_name":        map[string]any{"type": "string"},
			"nested":          map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string"}}},
			"ignored_non_map": "bad",
		},
	}}})
	if err != nil {
		t.Fatalf("evalElicitationHandler() error = %v", err)
	}
	if result.Action != "accept" {
		t.Fatalf("Action = %q, want accept", result.Action)
	}
	if result.Content["confirmed"] != true || result.Content["selection"] != "private" || result.Content["tag_name"] != "v-test" {
		t.Fatalf("content = %#v, want confirmed, private selection, and prepared tag", result.Content)
	}
	nested, ok := result.Content["nested"].(map[string]any)
	if !ok || nested["description"] != "Created by eval_mcp_surfaces elicitation handler" {
		t.Fatalf("nested content = %#v", result.Content["nested"])
	}
}

// TestFirstJSONSchemaType_SelectsFirstNonNullType verifies union schema types
// pick the first meaningful JSON schema type.
func TestFirstJSONSchemaType_SelectsFirstNonNullType(t *testing.T) {
	if got := firstJSONSchemaType([]any{"null", "boolean"}); got != "boolean" {
		t.Fatalf("firstJSONSchemaType() = %q, want boolean", got)
	}
	if got := firstJSONSchemaType(nil); got != "string" {
		t.Fatalf("firstJSONSchemaType(nil) = %q, want string", got)
	}
}

// TestEvalElicitationSelection_DefaultsWhenEnumMissing verifies selection fields
// always get a stable value even without usable enum metadata.
func TestEvalElicitationSelection_DefaultsWhenEnumMissing(t *testing.T) {
	if got := evalElicitationSelection(map[string]any{"enum": []any{""}}); got != "default" {
		t.Fatalf("evalElicitationSelection(empty enum) = %q, want default", got)
	}
}
