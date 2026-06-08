package toolutil

import (
	"fmt"
	"strings"
)

// SchemaPropertyOverride returns an input-schema override for a property path.
func SchemaPropertyOverride(propertyPath string, values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{PropertyPath: strings.TrimSpace(propertyPath), Values: cloneSchemaMap(values)}
}

// SchemaRootOverride returns an input-schema override applied at the schema root.
func SchemaRootOverride(values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{Values: cloneSchemaMap(values)}
}

// SchemaAnyOfRequired returns a root override that requires at least one of the
// supplied property names to be present.
func SchemaAnyOfRequired(propertyNames ...string) InputSchemaOverride {
	branches := make([]any, 0, len(propertyNames))
	for _, propertyName := range propertyNames {
		propertyName = strings.TrimSpace(propertyName)
		if propertyName == "" {
			continue
		}
		branches = append(branches, map[string]any{"required": []string{propertyName}})
	}
	if len(branches) == 0 {
		return InputSchemaOverride{}
	}
	return SchemaRootOverride(map[string]any{"anyOf": branches})
}

func applyInputSchemaOverrides(schema map[string]any, overrides []InputSchemaOverride) {
	if schema == nil || len(overrides) == 0 {
		return
	}
	for _, override := range overrides {
		target := schemaOverrideTarget(schema, strings.TrimSpace(override.PropertyPath))
		if target == nil {
			continue
		}
		for key, value := range override.Values {
			target[key] = cloneSchemaValue(value)
		}
	}
}

func validateInputSchemaOverrides(spec ActionSpec) error {
	if len(spec.InputSchemaOverrides) == 0 {
		return nil
	}
	if spec.Route.InputSchema == nil {
		return fmt.Errorf("action spec %q has input schema overrides without an input schema", spec.Name)
	}
	for _, override := range spec.InputSchemaOverrides {
		if len(override.Values) == 0 {
			return fmt.Errorf("action spec %q has empty input schema override", spec.Name)
		}
		propertyPath := strings.TrimSpace(override.PropertyPath)
		if propertyPath == "" {
			continue
		}
		if schemaOverrideTarget(spec.Route.InputSchema, propertyPath) == nil {
			return fmt.Errorf("action spec %q has input schema override for unknown property path %q", spec.Name, propertyPath)
		}
	}
	return nil
}

func schemaOverrideTarget(root map[string]any, propertyPath string) map[string]any {
	if propertyPath == "" {
		return root
	}
	parts := strings.Split(propertyPath, ".")
	return schemaOverrideTargetFrom(root, root, parts)
}

func schemaOverrideTargetFrom(root, schema map[string]any, parts []string) map[string]any {
	if len(parts) == 0 {
		return nil
	}
	schema = resolveSchemaRef(root, schema)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	child, ok := properties[parts[0]].(map[string]any)
	if !ok {
		return nil
	}
	child = resolveSchemaRef(root, child)
	if len(parts) == 1 {
		return child
	}
	if child == nil {
		return nil
	}
	if items, hasItems := child["items"].(map[string]any); hasItems {
		child = resolveSchemaRef(root, items)
	}
	return schemaOverrideTargetFrom(root, child, parts[1:])
}

func cloneInputSchemaOverrides(overrides []InputSchemaOverride) []InputSchemaOverride {
	if len(overrides) == 0 {
		return nil
	}
	out := make([]InputSchemaOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, InputSchemaOverride{
			PropertyPath: strings.TrimSpace(override.PropertyPath),
			Values:       cloneSchemaMap(override.Values),
		})
	}
	return out
}
