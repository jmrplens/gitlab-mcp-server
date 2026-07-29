// schemas_test.go contains unit tests for the JSON Schema builders and
// response-content parsers shared by the synchronous Client path and the
// multi round-trip Flow path.
package elicitation

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// TestContentForAction_MapsActions verifies the action-to-outcome mapping
// shared by both elicitation mechanisms.
func TestContentForAction_MapsActions(t *testing.T) {
	content := map[string]any{"k": "v"}

	got, acceptErr := contentForAction("accept", content)
	if acceptErr != nil || got["k"] != "v" {
		t.Errorf("contentForAction(accept) = (%v, %v), want content back", got, acceptErr)
	}
	if _, declineErr := contentForAction("decline", nil); !errors.Is(declineErr, ErrDeclined) {
		t.Errorf("contentForAction(decline) error = %v, want ErrDeclined", declineErr)
	}
	if _, cancelErr := contentForAction("cancel", nil); !errors.Is(cancelErr, ErrCancelled) {
		t.Errorf("contentForAction(cancel) error = %v, want ErrCancelled", cancelErr)
	}
	if _, unknownErr := contentForAction("bogus", nil); unknownErr == nil || !strings.Contains(unknownErr.Error(), "unknown action") {
		t.Errorf("contentForAction(bogus) error = %v, want unknown action", unknownErr)
	}
}

// TestConfirmSchema_ShapeAndParse verifies the confirmation schema shape and
// that parseConfirmContent treats missing or non-boolean fields as false.
func TestConfirmSchema_ShapeAndParse(t *testing.T) {
	schema := confirmSchema("Sure?")
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("confirmSchema missing properties")
	}
	if _, hasConfirmed := props["confirmed"]; !hasConfirmed {
		t.Fatal("confirmSchema missing 'confirmed' property")
	}

	if !parseConfirmContent(map[string]any{"confirmed": true}) {
		t.Error("parseConfirmContent(true) = false")
	}
	if parseConfirmContent(map[string]any{"confirmed": "yes"}) {
		t.Error("parseConfirmContent(non-bool) = true, want false")
	}
	if parseConfirmContent(nil) {
		t.Error("parseConfirmContent(nil) = true, want false")
	}
}

// TestTextSchema_ParseTextContent verifies text schema construction and
// string extraction errors for non-string values.
func TestTextSchema_ParseTextContent(t *testing.T) {
	schema := textSchema("Enter name", "name")
	props := schema["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Fatal("textSchema missing field property")
	}

	got, err := parseTextContent(map[string]any{"name": "x"}, "name")
	if err != nil || got != "x" {
		t.Errorf("parseTextContent = (%q, %v), want ('x', nil)", got, err)
	}
	if _, typeErr := parseTextContent(map[string]any{"name": 5}, "name"); typeErr == nil {
		t.Error("parseTextContent(non-string) error = nil, want error")
	}
}

// TestSelectSchemas_ValidateOptions verifies enum construction and option
// validation across the single, multi, and integer select parsers.
func TestSelectSchemas_ValidateOptions(t *testing.T) {
	if _, err := parseSelectOneContent(map[string]any{"selection": "c"}, []string{"a", "b"}); err == nil {
		t.Error("parseSelectOneContent(out of enum) error = nil, want error")
	}
	if _, err := parseSelectMultiContent(map[string]any{"selections": []any{"a", 3}}, []string{"a"}); err == nil {
		t.Error("parseSelectMultiContent(non-string element) error = nil, want error")
	}
	if _, err := parseSelectOneIntContent(map[string]any{"selection": 2.5}, []int{1, 2}); err == nil {
		t.Error("parseSelectOneIntContent(non-integer) error = nil, want error")
	}
	if _, err := parseSelectOneIntContent(map[string]any{"selection": math.NaN()}, []int{1}); err == nil {
		t.Error("parseSelectOneIntContent(NaN) error = nil, want error")
	}
	got, err := parseSelectOneIntContent(map[string]any{"selection": float64(2)}, []int{1, 2})
	if err != nil || got != 2 {
		t.Errorf("parseSelectOneIntContent = (%d, %v), want (2, nil)", got, err)
	}
}

// TestNumberSchema_BoundsAndParse verifies numeric bounds inclusion and NaN
// rejection.
func TestNumberSchema_BoundsAndParse(t *testing.T) {
	schema := numberSchema("Rate", "rating", 0, 5)
	props := schema["properties"].(map[string]any)
	prop := props["rating"].(map[string]any)
	if prop["minimum"] != 0.0 || prop["maximum"] != 5.0 {
		t.Errorf("numberSchema bounds = (%v, %v), want (0, 5)", prop["minimum"], prop["maximum"])
	}

	unbounded := numberSchema("Any", "v", math.Inf(-1), math.Inf(1))
	uprop := unbounded["properties"].(map[string]any)["v"].(map[string]any)
	if _, hasMin := uprop["minimum"]; hasMin {
		t.Error("numberSchema(-Inf) still has minimum")
	}
	if _, hasMax := uprop["maximum"]; hasMax {
		t.Error("numberSchema(+Inf) still has maximum")
	}

	if _, err := parseNumberContent(map[string]any{"v": math.NaN()}, "v"); err == nil {
		t.Error("parseNumberContent(NaN) error = nil, want error")
	}
	if _, err := parseNumberContent(map[string]any{"v": "x"}, "v"); err == nil {
		t.Error("parseNumberContent(non-number) error = nil, want error")
	}
}
