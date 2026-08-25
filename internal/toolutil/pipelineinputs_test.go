// pipelineinputs_test.go verifies BuildPipelineInputs, the shared conversion
// from JSON-decoded spec:inputs values to the SDK's PipelineInputsOption.
package toolutil

import "testing"

// TestBuildPipelineInputs_AllTypes verifies BuildPipelineInputs accepts every
// supported value type and rejects non-string array elements.
func TestBuildPipelineInputs_AllTypes(t *testing.T) {
	in, err := BuildPipelineInputs(map[string]any{
		"s":      "v",
		"b":      true,
		"f":      float64(1.5),
		"i":      7,
		"i64":    int64(9),
		"arrAny": []any{"a", "b"},
		"arrStr": []string{"c"},
	})
	if err != nil {
		t.Fatalf("BuildPipelineInputs() unexpected error: %v", err)
	}
	if len(in) != 7 {
		t.Errorf("len = %d, want 7", len(in))
	}

	if _, badErr := BuildPipelineInputs(map[string]any{"arr": []any{"ok", 5}}); badErr == nil {
		t.Error("expected error for non-string array element")
	}
}

// TestDetailedStatusOutput_Illustration verifies the detailed_status illustration
// sub-object and the user created_at timestamp are mirrored.
