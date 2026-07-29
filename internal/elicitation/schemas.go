// Package elicitation: schemas.go holds the JSON Schema builders and
// response-content parsers shared by the synchronous [Client] path and the
// multi round-trip [Flow] path, so both mechanisms request and validate
// identical shapes.
package elicitation

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// contentForAction maps an elicitation action to its content or the
// corresponding sentinel error.
func contentForAction(action string, content map[string]any) (map[string]any, error) {
	switch action {
	case "accept":
		return content, nil
	case "decline":
		return nil, ErrDeclined
	case "cancel":
		return nil, ErrCancelled
	default:
		return nil, fmt.Errorf("elicitation: unknown action %q", action)
	}
}

// confirmSchema builds the schema for a yes/no confirmation prompt.
func confirmSchema(message string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed": map[string]any{
				"type":        "boolean",
				"title":       "Confirm",
				"description": message,
			},
		},
		"required": []string{"confirmed"},
	}
}

// parseConfirmContent extracts the boolean confirmation value; a missing or
// non-boolean field counts as not confirmed.
func parseConfirmContent(content map[string]any) bool {
	confirmed, ok := content["confirmed"].(bool)
	return ok && confirmed
}

// textSchema builds the schema for a free-form text prompt.
func textSchema(message, fieldName string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			fieldName: map[string]any{
				"type":        "string",
				"title":       fieldName,
				"description": message,
			},
		},
		"required": []string{fieldName},
	}
}

// parseTextContent extracts the string value of fieldName.
func parseTextContent(content map[string]any, fieldName string) (string, error) {
	text, ok := content[fieldName].(string)
	if !ok {
		return "", fmt.Errorf("elicitation: response field %q is not a string", fieldName)
	}
	return text, nil
}

// selectOneSchema builds the schema for a single-choice string selection.
func selectOneSchema(message string, options []string) map[string]any {
	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selection": map[string]any{
				"type":        "string",
				"title":       "Selection",
				"description": message,
				"enum":        enumValues,
			},
		},
		"required": []string{"selection"},
	}
}

// parseSelectOneContent extracts and validates the selected option.
func parseSelectOneContent(content map[string]any, options []string) (string, error) {
	selection, ok := content["selection"].(string)
	if !ok {
		return "", errors.New("elicitation: response field 'selection' is not a string")
	}
	// Validate against allowed options (defense in depth)
	if slices.Contains(options, selection) {
		return selection, nil
	}
	return "", fmt.Errorf("elicitation: selected value %q is not in the allowed options", selection)
}

// selectMultiSchema builds the schema for a multi-choice string selection.
func selectMultiSchema(message string, options []string, minItems, maxItems int) map[string]any {
	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}
	arraySchema := map[string]any{
		"type":        "array",
		"title":       "Selections",
		"description": message,
		"items": map[string]any{
			"type": "string",
			"enum": enumValues,
		},
	}
	if minItems > 0 {
		arraySchema["minItems"] = minItems
	}
	if maxItems > 0 {
		arraySchema["maxItems"] = maxItems
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selections": arraySchema,
		},
		"required": []string{"selections"},
	}
}

// parseSelectMultiContent extracts and validates the selected options.
func parseSelectMultiContent(content map[string]any, options []string) ([]string, error) {
	raw, ok := content["selections"].([]any)
	if !ok {
		return nil, errors.New("elicitation: response field 'selections' is not an array")
	}
	selections := make([]string, 0, len(raw))
	for _, v := range raw {
		s, isString := v.(string)
		if !isString {
			return nil, fmt.Errorf("elicitation: selection element is not a string: %v", v)
		}
		if !slices.Contains(options, s) {
			return nil, fmt.Errorf("elicitation: selected value %q is not in the allowed options", s)
		}
		selections = append(selections, s)
	}
	return selections, nil
}

// selectOneIntSchema builds the schema for a single-choice integer selection.
func selectOneIntSchema(message string, options []int) map[string]any {
	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selection": map[string]any{
				"type":        "integer",
				"title":       "Selection",
				"description": message,
				"enum":        enumValues,
			},
		},
		"required": []string{"selection"},
	}
}

// parseSelectOneIntContent extracts and validates the selected integer.
func parseSelectOneIntContent(content map[string]any, options []int) (int, error) {
	// JSON numbers are float64 by default
	f, ok := content["selection"].(float64)
	if !ok {
		return 0, errors.New("elicitation: response field 'selection' is not a number")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errors.New("elicitation: response field 'selection' is not a finite number")
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("elicitation: response value %g is not an integer", f)
	}
	selected := int(f)
	if !slices.Contains(options, selected) {
		return 0, fmt.Errorf("elicitation: selected value %d is not in the allowed options", selected)
	}
	return selected, nil
}

// numberSchema builds the schema for a bounded numeric prompt. Use
// math.Inf(-1) and math.Inf(1) to omit a bound.
func numberSchema(message, fieldName string, minVal, maxVal float64) map[string]any {
	prop := map[string]any{
		"type":        "number",
		"title":       fieldName,
		"description": message,
	}
	if !isInf(minVal) {
		prop["minimum"] = minVal
	}
	if !isInf(maxVal) {
		prop["maximum"] = maxVal
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			fieldName: prop,
		},
		"required": []string{fieldName},
	}
}

// parseNumberContent extracts the numeric value of fieldName.
func parseNumberContent(content map[string]any, fieldName string) (float64, error) {
	f, ok := content[fieldName].(float64)
	if !ok {
		return 0, fmt.Errorf("elicitation: response field %q is not a number", fieldName)
	}
	if math.IsNaN(f) {
		return 0, fmt.Errorf("elicitation: response field %q is NaN", fieldName)
	}
	return f, nil
}
