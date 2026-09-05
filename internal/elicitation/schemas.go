// schemas.go holds the JSON Schema builders and
// response-content parsers shared by the synchronous [Client] path and the
// multi round-trip [Flow] path, so both mechanisms request and validate
// identical shapes.

package elicitation

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
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
				"type": "boolean",
				// A client that pre-populates from defaults opens this
				// dialog on "no", so an accidental Enter declines a
				// destructive action rather than approving one. The
				// specification offers defaults as a convenience; this is
				// the one schema here where the safe value is obvious, and
				// the others deliberately have none, since suggesting an
				// answer to an open question is not a convenience.
				"default":     false,
				"title":       "Confirm",
				"description": message,
			},
		},
		"required": []string{"confirmed"},
	}
}

// parseConfirmContent extracts the boolean confirmation value and reports
// whether the answer satisfied the schema it was asked against.
//
// The two are separate facts and used to be collapsed into one. An answer with
// no boolean `confirmed` field read as false, which the caller then reported as
// "Operation canceled by user." — a decision no user made, attributed to them,
// on the strength of a client-side bug. A destructive action must still not
// proceed, but what it reports has to be what happened.
func parseConfirmContent(content map[string]any) (confirmed, wellFormed bool) {
	confirmed, wellFormed = content["confirmed"].(bool)
	return confirmed, wellFormed
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

// parseSelectMultiContent extracts and validates the selected options,
// including the cardinality bounds advertised in the schema (0 = no bound).
func parseSelectMultiContent(content map[string]any, options []string, minItems, maxItems int) ([]string, error) {
	raw, ok := content["selections"].([]any)
	if !ok {
		return nil, errors.New("elicitation: response field 'selections' is not an array")
	}
	if minItems > 0 && len(raw) < minItems {
		return nil, fmt.Errorf("elicitation: %d selections returned, want at least %d", len(raw), minItems)
	}
	if maxItems > 0 && len(raw) > maxItems {
		return nil, fmt.Errorf("elicitation: %d selections returned, want at most %d", len(raw), maxItems)
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
	// The 2026-07-28 elicitation schema subset admits no integer enum:
	// PrimitiveSchemaDefinition is String | Number | Boolean | Enum, the
	// number variant carries no enum member, and every enum variant is
	// string-typed. So the options are offered as their decimal strings and
	// parsed back to integers on the way in.
	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = strconv.Itoa(o)
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

// parseSelectOneIntContent extracts and validates the selected integer. The
// schema offers the options as decimal strings (the elicitation subset has
// no integer enum), so the accepted value arrives as a string and is parsed
// back; a numeric answer from a lenient client is tolerated too.
func parseSelectOneIntContent(content map[string]any, options []int) (int, error) {
	var selected int
	switch v := content["selection"].(type) {
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("elicitation: response value %q is not an integer: %w", v, err)
		}
		selected = parsed
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) {
			return 0, fmt.Errorf("elicitation: response value %g is not an integer", v)
		}
		// Both bounds are exact float64 comparisons: MinInt is a power of
		// two and MaxInt rounds up to one, so >= rejects every value whose
		// int conversion would be implementation-defined.
		if v < math.MinInt || v >= math.MaxInt {
			return 0, fmt.Errorf("elicitation: response value %g overflows int", v)
		}
		selected = int(v)
	default:
		return 0, errors.New("elicitation: response field 'selection' is not a string or number")
	}
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

// parseNumberContent extracts the numeric value of fieldName and enforces
// the bounds advertised in the schema (infinities mean unbounded).
func parseNumberContent(content map[string]any, fieldName string, minVal, maxVal float64) (float64, error) {
	f, ok := content[fieldName].(float64)
	if !ok {
		return 0, fmt.Errorf("elicitation: response field %q is not a number", fieldName)
	}
	if math.IsNaN(f) {
		return 0, fmt.Errorf("elicitation: response field %q is NaN", fieldName)
	}
	if !isInf(minVal) && f < minVal {
		return 0, fmt.Errorf("elicitation: response value %g is below the minimum %g", f, minVal)
	}
	if !isInf(maxVal) && f > maxVal {
		return 0, fmt.Errorf("elicitation: response value %g is above the maximum %g", f, maxVal)
	}
	return f, nil
}
