// metatool_coverage_test.go targets the remaining uncovered branches in
// metatool.go that are not exercised by the higher-level tests in
// metatool_test.go. These tests focus on:
//
//   - SetMetaParamSchemaMode: the package-level mode setter (was 0%).
//   - resolveTopLevelRef: every fallback branch when $ref / $defs are
//     missing or malformed.
//   - compactParamsSchema: nil input, missing/non-object properties,
//     non-map property entries, and $ref resolution.
//   - buildMetaOneOf: per-action InputSchema = nil fallback.
//   - schemaForType: pointer dereference and cache hit paths.
//   - stripReservedKeys: presence of multiple reserved keys mixed with
//     real fields (covers the "out[k] = v" copy branch).
//   - UnmarshalParams: marshaling failure and double-failure recovery.
//   - enrichWithHints: data with no leading `{` and hints with non-text
//     content.

package toolutil

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSetMetaParamSchemaMode_ValidValues verifies that each documented mode
// is accepted and round-trips through currentMetaParamSchemaMode.
func TestSetMetaParamSchemaMode_ValidValues(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	for _, mode := range []string{MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull} {
		t.Run(mode, func(t *testing.T) {
			SetMetaParamSchemaMode(mode)
			if got := currentMetaParamSchemaMode(); got != mode {
				t.Errorf("currentMetaParamSchemaMode() = %q, want %q", got, mode)
			}
		})
	}
}

// TestSetMetaParamSchemaMode_InvalidCoercesToOpaque verifies that an unknown
// mode is silently coerced to "opaque" so misconfiguration cannot break the
// tools/list payload.
func TestSetMetaParamSchemaMode_InvalidCoercesToOpaque(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	SetMetaParamSchemaMode(MetaParamSchemaFull)
	SetMetaParamSchemaMode("nonsense")
	if got := currentMetaParamSchemaMode(); got != MetaParamSchemaOpaque {
		t.Errorf("invalid mode should coerce to opaque, got %q", got)
	}
}

// TestResolveTopLevelRef_NoRef returns the schema unchanged when no
// top-level $ref is present.
func TestResolveTopLevelRef_NoRef(t *testing.T) {
	s := map[string]any{"type": "object"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWithoutDefs returns the schema unchanged when
// $defs is absent; we cannot resolve the reference so the original wins.
func TestResolveTopLevelRef_RefWithoutDefs(t *testing.T) {
	s := map[string]any{"$ref": "#/$defs/Foo"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWrongPrefix returns the schema unchanged when
// the $ref does not match the supported "#/$defs/" prefix.
func TestResolveTopLevelRef_RefWrongPrefix(t *testing.T) {
	s := map[string]any{
		"$ref":  "https://example.com/schema.json",
		"$defs": map[string]any{"Foo": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("non-internal $ref should be returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefMissingTarget returns the schema unchanged when
// the referenced $defs entry does not exist.
func TestResolveTopLevelRef_RefMissingTarget(t *testing.T) {
	s := map[string]any{
		"$ref":  "#/$defs/Missing",
		"$defs": map[string]any{"Other": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("missing target should return original schema, got %v", got)
	}
}

// TestResolveTopLevelRef_ResolvesValidRef returns the referenced $defs
// entry when the reference is well-formed.
func TestResolveTopLevelRef_ResolvesValidRef(t *testing.T) {
	target := map[string]any{
		"type":       "object",
		"properties": map[string]any{"id": map[string]any{"type": "integer"}},
	}
	s := map[string]any{
		"$ref":  "#/$defs/Foo",
		"$defs": map[string]any{"Foo": target},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, target) {
		t.Errorf("expected ref to resolve to target, got %v", got)
	}
}

// TestCompactParamsSchema_Nil returns nil for a nil schema.
func TestCompactParamsSchema_Nil(t *testing.T) {
	if got := compactParamsSchema(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestCompactParamsSchema_NoProperties returns a permissive open object
// schema when the input has no `properties` field.
func TestCompactParamsSchema_NoProperties(t *testing.T) {
	got := compactParamsSchema(map[string]any{"type": "object"})
	if got["type"] != "object" {
		t.Errorf("type = %v, want object", got["type"])
	}
	if got["additionalProperties"] != true {
		t.Errorf("additionalProperties = %v, want true", got["additionalProperties"])
	}
	if _, hasProps := got["properties"]; hasProps {
		t.Errorf("expected no properties field, got %v", got["properties"])
	}
}

// TestCompactParamsSchema_NonObjectProperty replaces non-map property values
// (e.g. arrays, scalars) with empty schemas rather than panicking.
func TestCompactParamsSchema_NonObjectProperty(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"weird": "not-a-map",
			"id":    map[string]any{"type": "integer"},
		},
	})
	props, _ := got["properties"].(map[string]any)
	weird, _ := props["weird"].(map[string]any)
	if weird == nil {
		t.Fatalf("expected empty map for non-object property, got %v", props["weird"])
	}
	if len(weird) != 0 {
		t.Errorf("expected empty schema for non-object property, got %v", weird)
	}
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("id type = %v, want integer", id["type"])
	}
}

// TestCompactParamsSchema_PropertyWithEnum keeps only type and enum fields
// per property; description and other metadata are dropped.
func TestCompactParamsSchema_PropertyWithEnum(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"state": map[string]any{
				"type":        "string",
				"enum":        []any{"open", "closed"},
				"description": "should be stripped",
			},
		},
	})
	props, _ := got["properties"].(map[string]any)
	state, _ := props["state"].(map[string]any)
	if state["type"] != "string" {
		t.Errorf("type = %v, want string", state["type"])
	}
	enum, ok := state["enum"].([]any)
	if !ok || len(enum) != 2 {
		t.Errorf("enum = %v, want [open closed]", state["enum"])
	}
	if _, has := state["description"]; has {
		t.Errorf("expected description to be stripped, got %v", state["description"])
	}
}

// TestCompactParamsSchema_ResolvesTopLevelRef inlines a $ref before
// compacting so the final schema reflects the referenced definition.
func TestCompactParamsSchema_ResolvesTopLevelRef(t *testing.T) {
	s := map[string]any{
		"$ref": "#/$defs/Foo",
		"$defs": map[string]any{
			"Foo": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer", "description": "x"},
				},
			},
		},
	}
	got := compactParamsSchema(s)
	props, _ := got["properties"].(map[string]any)
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("expected $ref to resolve before compacting, got %v", got)
	}
	if _, has := got["$defs"]; has {
		t.Error("expected $defs to be dropped from compacted schema")
	}
}

// TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject substitutes a
// permissive object schema when a route does not declare an InputSchema.
func TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject(t *testing.T) {
	routes := ActionMap{
		"act": {Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}},
	}
	branches := buildMetaOneOf(routes, []string{"act"}, false)
	if len(branches) != 1 {
		t.Fatalf("len(branches) = %d, want 1", len(branches))
	}
	branch, _ := branches[0].(map[string]any)
	props, _ := branch["properties"].(map[string]any)
	params, _ := props["params"].(map[string]any)
	if params["type"] != "object" {
		t.Errorf("nil InputSchema should fall back to type:object, got %v", params)
	}
	if params["additionalProperties"] != true {
		t.Errorf("nil InputSchema should set additionalProperties:true, got %v", params)
	}
}

// TestSchemaForType_PointerType dereferences pointer types so *T and T
// produce the same cached schema.
func TestSchemaForType_PointerType(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
	}
	direct := schemaForType(reflect.TypeFor[sample]())
	pointer := schemaForType(reflect.TypeFor[*sample]())
	if direct == nil || pointer == nil {
		t.Fatal("expected non-nil schemas for both T and *T")
	}
	if !reflect.DeepEqual(direct, pointer) {
		t.Errorf("schemaForType(T) and schemaForType(*T) should be equal,\n  T  = %v\n  *T = %v", direct, pointer)
	}
}

// TestSchemaForType_CacheHit returns the cached schema for repeated calls
// with the same reflect.Type.
func TestSchemaForType_CacheHit(t *testing.T) {
	type cached struct {
		ID int `json:"id"`
	}
	first := schemaForType(reflect.TypeFor[cached]())
	second := schemaForType(reflect.TypeFor[cached]())
	if first == nil || second == nil {
		t.Fatal("expected non-nil schemas")
	}
	// Cache hit should return the same map pointer.
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("expected cached schema map to be reused on second call")
	}
}

// TestStripReservedKeys_MultipleKeys verifies that real keys are preserved
// when reserved keys are also present (covers the "copy" branch).
func TestStripReservedKeys_MultipleKeys(t *testing.T) {
	in := map[string]any{
		"confirm": true,
		"name":    "proj",
		"id":      42,
	}
	out := stripReservedKeys(in)
	if _, has := out["confirm"]; has {
		t.Error("expected confirm to be stripped")
	}
	if out["name"] != "proj" {
		t.Errorf("name = %v, want proj", out["name"])
	}
	if out["id"] != 42 {
		t.Errorf("id = %v, want 42", out["id"])
	}
	// Original map must not be mutated.
	if _, has := in["confirm"]; !has {
		t.Error("stripReservedKeys mutated the input map")
	}
}

// TestEnrichWithHints_NonObjectJSONFromArray returns the result unchanged
// when the marshaled JSON is an array (does not start with `{`).
func TestEnrichWithHints_NonObjectJSONFromArray(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Some output\n\n💡 **Next steps:**\n- do thing\n"},
		},
	}
	result := []string{"a", "b"}
	got := enrichWithHints(result, callResult)
	// Array inputs must be returned unchanged because we only enrich JSON
	// objects to keep the {next_steps, ...} contract well-defined.
	gotSlice, ok := got.([]string)
	if !ok || len(gotSlice) != 2 {
		t.Errorf("expected array result returned unchanged, got %v (%T)", got, got)
	}
}

// TestEnrichWithHints_NonTextContentSkipped iterates past non-text content
// blocks when looking for hints; only TextContent contributes to extraction.
func TestEnrichWithHints_NonTextContentSkipped(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			// Non-text content (e.g. resource link) must not panic and must
			// not be inspected for hints.
			&mcp.ResourceLink{URI: "gitlab://resource"},
		},
	}
	result := map[string]any{"ok": true}
	got := enrichWithHints(result, callResult)
	// No hints found → input must be returned unchanged.
	if !reflect.DeepEqual(got, result) {
		t.Errorf("expected result unchanged when no text content, got %v", got)
	}
}

// TestUnmarshalParams_DoubleFailureReturnsOriginalError confirms that when
// neither the strict pass nor the numeric-string-coerced retry succeed, the
// original error message is preserved (rather than the retry's).
func TestUnmarshalParams_DoubleFailureReturnsOriginalError(t *testing.T) {
	type strictInput struct {
		ID int `json:"id"`
	}
	// "id" cannot be coerced from a non-numeric string to int even after
	// numeric-string coercion, so both passes fail.
	_, err := UnmarshalParams[strictInput](map[string]any{"id": "not-a-number"})
	if err == nil {
		t.Fatal("expected error from double-failure path")
	}
	if !strings.Contains(err.Error(), "invalid params for this action") {
		t.Errorf("expected wrapped error message, got %q", err.Error())
	}
}
