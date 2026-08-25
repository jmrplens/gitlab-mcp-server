package toolutil

import (
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// BuildPipelineInputs converts a JSON-decoded inputs map into the SDK's
// type-safe gl.PipelineInputsOption. JSON numbers decode to float64 and JSON
// arrays to []any; both are normalized to the SDK's supported value types
// (string, float64, bool, []string). An unsupported value type returns an error.
//
// Shared by pipeline creation and manual-job play, which take the same
// spec:inputs value shapes.
func BuildPipelineInputs(raw map[string]any) (gl.PipelineInputsOption, error) {
	inputs := make(gl.PipelineInputsOption, len(raw))
	for name, value := range raw {
		switch v := value.(type) {
		case string:
			inputs[name] = gl.NewPipelineInputValue(v)
		case bool:
			inputs[name] = gl.NewPipelineInputValue(v)
		case float64:
			inputs[name] = gl.NewPipelineInputValue(v)
		case int:
			inputs[name] = gl.NewPipelineInputValue(v)
		case int64:
			inputs[name] = gl.NewPipelineInputValue(v)
		case []string:
			inputs[name] = gl.NewPipelineInputValue(v)
		case []any:
			strs := make([]string, 0, len(v))
			for _, e := range v {
				s, ok := e.(string)
				if !ok {
					return nil, fmt.Errorf("input %q: array elements must be strings", name)
				}
				strs = append(strs, s)
			}
			inputs[name] = gl.NewPipelineInputValue(strs)
		default:
			return nil, fmt.Errorf("input %q: unsupported value type %T", name, value)
		}
	}
	return inputs, nil
}
