package toolutil

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
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

// PipelineInputsSchema builds the JSON Schema for the typed input T and
// constrains the named map property's values to exactly the shapes
// BuildPipelineInputs accepts (string, number, boolean, array of strings).
// The generic map[string]any field alone would advertise
// additionalProperties:true, promising value shapes the converter rejects.
func PipelineInputsSchema[T any](property string) map[string]any {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("build input schema for %s: %v; check the input struct tags, unsupported field types, or circular schema references", property, err))
	}
	prop := schema.Properties[property]
	if prop == nil {
		panic(fmt.Sprintf("input schema has no %q property; keep the schema override in sync with the input struct", property))
	}
	prop.AdditionalProperties = &jsonschema.Schema{OneOf: []*jsonschema.Schema{
		{Type: "string"},
		{Type: "number"},
		{Type: "boolean"},
		{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
	}}
	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal input schema for %s: %v; check schema serialization for unsupported values", property, err))
	}
	var out map[string]any
	if unmarshalErr := json.Unmarshal(data, &out); unmarshalErr != nil {
		panic(fmt.Sprintf("unmarshal input schema for %s: %v; check generated schema JSON shape", property, unmarshalErr))
	}
	return out
}
