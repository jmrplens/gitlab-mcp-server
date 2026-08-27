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

// TestCompileToolSchemas_NilToolIsNoOp verifies CompileToolSchemas does
// nothing (and does not panic) when the tool pointer is nil, matching the
// documented early-return contract alongside the empty-key case already
// covered above.
func TestCompileToolSchemas_NilToolIsNoOp(t *testing.T) {
	CompileToolSchemas(nil, "some-key")
}

// TestCompiledSchema_MarshalErrorFallsBackToOriginalMap verifies that when
// the map cannot be marshaled to JSON (a channel value, which
// encoding/json rejects with an UnsupportedTypeError), compiledSchema falls
// back to returning the original map unchanged instead of caching a
// compiled schema, and that no cache entry is left behind for the key.
func TestCompiledSchema_MarshalErrorFallsBackToOriginalMap(t *testing.T) {
	key := "test|marshal-error|" + t.Name()
	schema := map[string]any{"type": "object", "bad": make(chan int)}

	got := compiledSchema(key, schema)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("compiledSchema() = %T, want the original map[string]any on marshal error", got)
	}
	if !reflect.DeepEqual(m, schema) {
		t.Errorf("compiledSchema() = %v, want unchanged original map %v", m, schema)
	}
	if _, cached := compiledSchemaCache.Load(key); cached {
		t.Error("compiledSchema() must not cache a schema when marshal fails")
	}
}

// TestCompiledSchema_UnmarshalErrorFallsBackToOriginalMap verifies that when
// the marshaled JSON is well-formed but does not fit the jsonschema.Schema
// struct shape (here, "required" holding a string instead of the expected
// []string), compiledSchema falls back to the original map instead of
// caching a partially-populated or invalid compiled schema.
func TestCompiledSchema_UnmarshalErrorFallsBackToOriginalMap(t *testing.T) {
	key := "test|unmarshal-error|" + t.Name()
	schema := map[string]any{"type": "object", "required": "not-an-array"}

	got := compiledSchema(key, schema)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("compiledSchema() = %T, want the original map[string]any on unmarshal error", got)
	}
	if !reflect.DeepEqual(m, schema) {
		t.Errorf("compiledSchema() = %v, want unchanged original map %v", m, schema)
	}
	if _, cached := compiledSchemaCache.Load(key); cached {
		t.Error("compiledSchema() must not cache a schema when unmarshal fails")
	}
}
