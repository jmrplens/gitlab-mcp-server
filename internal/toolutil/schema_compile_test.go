package toolutil

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCompileToolSchemas_RoundTripAndCache pins the three contracts of the
// compiled-schema cache: the conversion preserves the schema content
// (including non-standard keys such as x_destructive), the same cache key
// returns the identical *jsonschema.Schema pointer (what lets the SDK
// SchemaCache skip re-resolution), and an empty key leaves the maps
// untouched.
func TestCompileToolSchemas_RoundTripAndCache(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirm": map[string]any{"type": "boolean", "description": "d"},
		},
		"x_destructive": true,
	}
	output := map[string]any{"type": "object"}

	tool := &mcp.Tool{InputSchema: input, OutputSchema: output}
	CompileToolSchemas(tool, "test|roundtrip")

	compiled, ok := tool.InputSchema.(*jsonschema.Schema)
	if !ok {
		t.Fatalf("InputSchema = %T, want *jsonschema.Schema", tool.InputSchema)
	}
	if _, outOK := tool.OutputSchema.(*jsonschema.Schema); !outOK {
		t.Fatalf("OutputSchema = %T, want *jsonschema.Schema", tool.OutputSchema)
	}

	// Content equivalence: marshaling the compiled schema must yield the
	// same JSON object as the original map, x_destructive included.
	compiledJSON, err := json.Marshal(compiled)
	if err != nil {
		t.Fatalf("marshal compiled: %v", err)
	}
	var compiledMap map[string]any
	if unmarshalErr := json.Unmarshal(compiledJSON, &compiledMap); unmarshalErr != nil {
		t.Fatalf("unmarshal compiled: %v", unmarshalErr)
	}
	if !reflect.DeepEqual(compiledMap, input) {
		t.Errorf("compiled schema roundtrip = %v, want %v", compiledMap, input)
	}

	// Pointer stability: a second tool with the same key must receive the
	// exact same schema pointer.
	second := &mcp.Tool{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirm": map[string]any{"type": "boolean", "description": "d"},
		},
		"x_destructive": true,
	}}
	CompileToolSchemas(second, "test|roundtrip")
	if second.InputSchema != tool.InputSchema {
		t.Error("same cache key returned a different schema pointer")
	}

	// Empty key: no-op, maps stay maps.
	untouched := &mcp.Tool{InputSchema: map[string]any{"type": "object"}}
	CompileToolSchemas(untouched, "")
	if _, isMap := untouched.InputSchema.(map[string]any); !isMap {
		t.Errorf("empty key changed InputSchema to %T, want map", untouched.InputSchema)
	}
}
