// pipelineinputs_test.go verifies BuildPipelineInputs, the shared conversion
// from JSON-decoded spec:inputs values to the SDK's PipelineInputsOption, and
// PipelineInputsSchema, the input-schema override that advertises exactly the
// value shapes the conversion accepts.
package toolutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildPipelineInputs_SupportedValues_MarshalUnchanged verifies every
// supported value type converts to an SDK input value that marshals back to
// the caller's JSON shape ([]any arrays normalized to string arrays).
func TestBuildPipelineInputs_SupportedValues_MarshalUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantJSON string
	}{
		{"string", "v", `"v"`},
		{"bool", true, `true`},
		{"float64", float64(1.5), `1.5`},
		{"int", 7, `7`},
		{"int64", int64(9), `9`},
		{"any array of strings", []any{"a", "b"}, `["a","b"]`},
		{"string array", []string{"c"}, `["c"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := BuildPipelineInputs(map[string]any{"k": tt.value})
			if err != nil {
				t.Fatalf("BuildPipelineInputs() error = %v", err)
			}
			got, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("marshal converted inputs: %v", err)
			}
			want := `{"k":` + tt.wantJSON + `}`
			if string(got) != want {
				t.Errorf("converted %s marshals to %s, want %s", tt.name, got, want)
			}
		})
	}
}

// TestBuildPipelineInputs_UnsupportedValues_ReturnError verifies each
// rejected value shape fails with an error naming the offending input.
func TestBuildPipelineInputs_UnsupportedValues_ReturnError(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		wantIn string
	}{
		{"array with non-string element", []any{"ok", 5}, "array elements must be strings"},
		{"nested object", map[string]any{"x": 1}, "unsupported value type"},
		{"nil", nil, "unsupported value type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPipelineInputs(map[string]any{"bad": tt.value})
			if err == nil {
				t.Fatal("BuildPipelineInputs() error = nil, want a failure")
			}
			if !strings.Contains(err.Error(), tt.wantIn) || !strings.Contains(err.Error(), `"bad"`) {
				t.Errorf("BuildPipelineInputs() error = %v, want it to mention %q and the input name", err, tt.wantIn)
			}
		})
	}
}

// TestPipelineInputsSchema_ConstrainsMapValues verifies the schema override
// replaces the open additionalProperties with the oneOf value constraint
// while leaving the rest of the generated schema intact.
func TestPipelineInputsSchema_ConstrainsMapValues(t *testing.T) {
	type input struct {
		ProjectID string         `json:"project_id" jsonschema:"Project,required"`
		Inputs    map[string]any `json:"inputs,omitempty" jsonschema:"Input values"`
	}
	schema := PipelineInputsSchema[input]("inputs")

	props, _ := schema["properties"].(map[string]any)
	if props["project_id"] == nil {
		t.Error("schema lost the sibling project_id property")
	}
	inputs, _ := props["inputs"].(map[string]any)
	ap, _ := inputs["additionalProperties"].(map[string]any)
	oneOf, _ := ap["oneOf"].([]any)
	if len(oneOf) != 4 {
		t.Fatalf("inputs.additionalProperties.oneOf has %d entries, want 4 (string, number, boolean, string array)", len(oneOf))
	}
}

// TestPipelineInputsSchema_AStructWithoutTheProperty_Panics covers the guard
// that turns a schema drifting away from its input struct into a startup
// failure.
//
// The override exists because the generic map field alone advertises
// additionalProperties:true, promising value shapes the converter rejects. If
// the property is renamed and this is not, the override silently stops applying
// and the tool starts accepting arguments it cannot convert — so failing at
// registration, loudly, is the point.
func TestPipelineInputsSchema_AStructWithoutTheProperty_Panics(t *testing.T) {
	t.Parallel()

	type withoutInputs struct {
		ProjectID string `json:"project_id"`
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("PipelineInputsSchema accepted a struct that has no such property")
		}
		if message, _ := recovered.(string); !strings.Contains(message, "inputs") {
			t.Errorf("panic = %v, want the missing property named", recovered)
		}
	}()

	_ = PipelineInputsSchema[withoutInputs]("inputs")
}
