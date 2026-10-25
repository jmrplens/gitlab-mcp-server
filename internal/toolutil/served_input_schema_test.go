package toolutil

import (
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// TestRootCombinators_NamesWhatTheRootCarries verifies that RootCombinators
// names each combinator and conditional a schema carries at its root, in the
// order the rule lists them, whatever form the schema is held in, and none
// that a property or an item carries: a nested combinator is no root of a
// tool's input schema, which is all the Anthropic Messages API refuses.
func TestRootCombinators_NamesWhatTheRootCarries(t *testing.T) {
	branch := map[string]any{"required": []any{"name"}}
	cases := []struct {
		name   string
		schema any
		want   []string
	}{
		{
			name: "every keyword at the root, in the rule's order",
			schema: map[string]any{
				"then": branch, "else": branch, "if": branch, "not": branch,
				"oneOf": []any{branch}, "anyOf": []any{branch}, "allOf": []any{branch},
				"type": "object",
			},
			want: []string{"allOf", "anyOf", "oneOf", "not", "if", "then", "else"},
		},
		{
			name: "nested in a property and an item",
			schema: map[string]any{"type": "object", "properties": map[string]any{
				"value": map[string]any{"oneOf": []any{map[string]any{"type": "string"}}},
				"list":  map[string]any{"type": "array", "items": map[string]any{"anyOf": []any{branch}}},
			}},
		},
		{name: "a compiled schema, read through its JSON form", schema: &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{{Required: []string{"name"}}}}, want: []string{"anyOf"}},
		{name: "no schema", schema: nil},
		{name: "a value that is no JSON object", schema: []string{"anyOf"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RootCombinators(tc.schema); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RootCombinators() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDropRootCombinators_LeavesEverythingElse verifies that the served copy
// loses the combinators and conditionals at its root and nothing else: its
// type, properties, required list and closure stay, and so does a combinator
// inside a property.
func TestDropRootCombinators_LeavesEverythingElse(t *testing.T) {
	branch := map[string]any{"required": []any{"cn"}}
	scope := map[string]any{"anyOf": []any{map[string]any{"type": "string"}}}
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"cn": map[string]any{"type": "string"}, "scope": scope},
		"required":             []any{"group_id"},
		"additionalProperties": false,
		"allOf":                []any{branch}, "anyOf": []any{branch}, "oneOf": []any{branch},
		"not": branch, "if": branch, "then": branch, "else": branch,
	}

	dropRootCombinators(schema)

	want := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"cn": map[string]any{"type": "string"}, "scope": scope},
		"required":             []any{"group_id"},
		"additionalProperties": false,
	}
	if !reflect.DeepEqual(schema, want) {
		t.Errorf("dropRootCombinators() left %#v, want %#v", schema, want)
	}
}
