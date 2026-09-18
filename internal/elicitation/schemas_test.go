// schemas_test.go contains unit tests for the JSON Schema builders and
// response-content parsers shared by the synchronous Client path and the
// multi round-trip Flow path.
package elicitation

import (
	"errors"
	"fmt"
	"math"
	"strconv"
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

// TestConfirmSchema_ShapeAndParse verifies the confirmation schema shape, and
// that parseConfirmContent separates a refusal from an unreadable answer.
//
// Those two used to be one value. An answer with no boolean `confirmed` field
// read as false, and the caller reported that as "Operation canceled by user."
// — a decision nobody made, attributed to a person, because their client sent
// the wrong shape. Both still stop the action, which is the part that must not
// change; what differs is what the model is told, and a model told the user
// cancelled will not retry and will say so to the user.
func TestConfirmSchema_ShapeAndParse(t *testing.T) {
	schema := confirmSchema("Sure?")
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("confirmSchema missing properties")
	}
	if _, hasConfirmed := props["confirmed"]; !hasConfirmed {
		t.Fatal("confirmSchema missing 'confirmed' property")
	}

	tests := []struct {
		name           string
		content        map[string]any
		wantConfirmed  bool
		wantWellFormed bool
	}{
		{
			name:          "an explicit yes",
			content:       map[string]any{"confirmed": true},
			wantConfirmed: true, wantWellFormed: true,
		},
		{
			name:          "an explicit no is a decision, and well formed",
			content:       map[string]any{"confirmed": false},
			wantConfirmed: false, wantWellFormed: true,
		},
		{
			name:          "a non-boolean answered the wrong question",
			content:       map[string]any{"confirmed": "yes"},
			wantConfirmed: false, wantWellFormed: false,
		},
		{
			name:          "an unrelated field did not answer at all",
			content:       map[string]any{"title": "something"},
			wantConfirmed: false, wantWellFormed: false,
		},
		{
			name:          "no content",
			content:       nil,
			wantConfirmed: false, wantWellFormed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmed, wellFormed := parseConfirmContent(tt.content)
			if confirmed != tt.wantConfirmed {
				t.Errorf("confirmed = %v, want %v", confirmed, tt.wantConfirmed)
			}
			if wellFormed != tt.wantWellFormed {
				t.Errorf("wellFormed = %v, want %v", wellFormed, tt.wantWellFormed)
			}
		})
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

// TestSelectSchemas_ValidateOptions verifies enum construction, option
// validation, and cardinality enforcement across the single, multi, and
// integer select parsers.
func TestSelectSchemas_ValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{"select one rejects out-of-enum value", func() error {
			_, err := parseSelectOneContent(map[string]any{"selection": "c"}, []string{"a", "b"})
			return err
		}, true},
		// A client is free to answer with the wrong JSON type, so every
		// parser reads the field through a checked assertion. Without a case
		// per parser an unchecked one would panic on the first malformed
		// answer rather than returning the error the caller expects.
		{"select one rejects non-string selection", func() error {
			_, err := parseSelectOneContent(map[string]any{"selection": 3}, []string{"a", "b"})
			return err
		}, true},
		{"select multi rejects non-array selections", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": "a"}, []string{"a"}, 0, 0)
			return err
		}, true},
		{"select multi rejects non-string element", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": []any{"a", 3}}, []string{"a"}, 0, 0)
			return err
		}, true},
		{"select multi rejects out-of-enum element", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": []any{"c"}}, []string{"a", "b"}, 0, 0)
			return err
		}, true},
		{"select multi rejects fewer than minItems", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": []any{}}, []string{"a", "b"}, 1, 0)
			return err
		}, true},
		{"select multi rejects more than maxItems", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": []any{"a", "b"}}, []string{"a", "b"}, 0, 1)
			return err
		}, true},
		{"select multi accepts within bounds", func() error {
			_, err := parseSelectMultiContent(map[string]any{"selections": []any{"a"}}, []string{"a", "b"}, 1, 2)
			return err
		}, false},
		{"select int rejects non-integer", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": 2.5}, []int{1, 2})
			return err
		}, true},
		{"select int rejects NaN", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": math.NaN()}, []int{1})
			return err
		}, true},
		{"select int rejects infinity", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": math.Inf(1)}, []int{1})
			return err
		}, true},
		{"select int accepts allowed value", func() error {
			got, err := parseSelectOneIntContent(map[string]any{"selection": float64(2)}, []int{1, 2})
			if err == nil && got != 2 {
				return errors.New("wrong value")
			}
			return err
		}, false},
		{"select int accepts decimal string", func() error {
			got, err := parseSelectOneIntContent(map[string]any{"selection": "30"}, []int{10, 30})
			if err == nil && got != 30 {
				return errors.New("wrong value")
			}
			return err
		}, false},
		{"select int wraps strconv error", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": "thirty"}, []int{30})
			if !errors.Is(err, strconv.ErrSyntax) {
				return fmt.Errorf("error does not wrap strconv.ErrSyntax: %w", err)
			}
			return nil
		}, false},
		{"select int rejects float beyond int range", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": 1e300}, []int{1})
			return err
		}, true},
		{"select int rejects negative overflow", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": -1e300}, []int{1})
			return err
		}, true},
		{"select int rejects non-string non-number", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": true}, []int{1})
			return err
		}, true},
		{"select int rejects out-of-enum value", func() error {
			_, err := parseSelectOneIntContent(map[string]any{"selection": float64(9)}, []int{1, 2})
			return err
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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

	if _, err := parseNumberContent(map[string]any{"v": math.NaN()}, "v", math.Inf(-1), math.Inf(1)); err == nil {
		t.Error("parseNumberContent(NaN) error = nil, want error")
	}
	if _, err := parseNumberContent(map[string]any{"v": "x"}, "v", math.Inf(-1), math.Inf(1)); err == nil {
		t.Error("parseNumberContent(non-number) error = nil, want error")
	}
	if _, err := parseNumberContent(map[string]any{"v": -1.0}, "v", 0, 5); err == nil {
		t.Error("parseNumberContent(below minimum) error = nil, want error")
	}
	if _, err := parseNumberContent(map[string]any{"v": 6.0}, "v", 0, 5); err == nil {
		t.Error("parseNumberContent(above maximum) error = nil, want error")
	}
	if got, err := parseNumberContent(map[string]any{"v": 4.5}, "v", 0, 5); err != nil || got != 4.5 {
		t.Errorf("parseNumberContent(in range) = (%g, %v), want (4.5, nil)", got, err)
	}
}

// TestSelectMultiSchema_AdvertisesOnlyTheBoundsItWasGiven pins which
// cardinality keys the schema carries.
//
// Zero means "no bound" for both, so a zero must leave its key out rather than
// advertise a bound of zero: `maxItems: 0` would tell a client that no
// selection is allowed at all, and `minItems: 0` is noise a model pays tokens
// to read. Nothing asserted the resulting schema, so both keys could have
// appeared unconditionally, or stopped appearing altogether, without a test
// noticing.
func TestSelectMultiSchema_AdvertisesOnlyTheBoundsItWasGiven(t *testing.T) {
	tests := []struct {
		name     string
		minItems int
		maxItems int
		wantMin  any
		wantMax  any
	}{
		{name: "no bounds at all", minItems: 0, maxItems: 0},
		{name: "a lower bound only", minItems: 2, wantMin: 2},
		{name: "an upper bound only", maxItems: 3, wantMax: 3},
		{name: "both bounds", minItems: 1, maxItems: 4, wantMin: 1, wantMax: 4},
		{name: "a negative bound is no bound", minItems: -1, maxItems: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := selectMultiSchema("Pick some", []string{"a", "b"}, tt.minItems, tt.maxItems)
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("selectMultiSchema has no properties object")
			}
			array, ok := props["selections"].(map[string]any)
			if !ok {
				t.Fatal("selectMultiSchema has no 'selections' property")
			}
			if got := array["minItems"]; got != tt.wantMin {
				t.Errorf("minItems = %v, want %v", got, tt.wantMin)
			}
			if got := array["maxItems"]; got != tt.wantMax {
				t.Errorf("maxItems = %v, want %v", got, tt.wantMax)
			}
		})
	}
}

// TestParseSelectMultiContent_BoundsAreInclusive pins that the cardinality
// bounds admit their own endpoints.
//
// The bounds the schema advertises are `minItems` and `maxItems`, which JSON
// Schema defines inclusively, so an answer with exactly as many selections as
// the maximum allows is a valid answer and must not be refused. The lower
// endpoint was already held; the upper one was not, so a check that rejected
// the very count it advertises would have passed.
func TestParseSelectMultiContent_BoundsAreInclusive(t *testing.T) {
	options := []string{"a", "b", "c"}

	tests := []struct {
		name     string
		values   []any
		minItems int
		maxItems int
		wantLen  int
		wantErr  string
	}{
		{name: "exactly the minimum", values: []any{"a", "b"}, minItems: 2, maxItems: 3, wantLen: 2},
		{name: "exactly the maximum", values: []any{"a", "b", "c"}, minItems: 1, maxItems: 3, wantLen: 3},
		{name: "one under the minimum", values: []any{"a"}, minItems: 2, maxItems: 3, wantErr: "want at least 2"},
		{name: "one over the maximum", values: []any{"a", "b", "c"}, minItems: 1, maxItems: 2, wantErr: "want at most 2"},
		{name: "unbounded accepts an empty answer", values: []any{}, wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectMultiContent(map[string]any{"selections": tt.values}, options, tt.minItems, tt.maxItems)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSelectMultiContent() error = %v, want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSelectMultiContent() error = %v, want the answer accepted", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("parseSelectMultiContent() returned %d selections, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestParseSelectOneIntContent_ClassifiesWhatIsNotAnInteger pins which refusal
// a float that is not an integer earns.
//
// The two refusals say different things to a model: "is not an integer" means
// the answer was the wrong kind of number and a whole number would do, while
// "overflows int" means the value itself is out of range. An infinity is the
// first: it is not a whole number, and telling a model it is too large invites
// a retry with a smaller one, which will be refused for the same reason. The
// classification held only for a non-integral finite value, so the guard that
// catches the infinities could have been folded into the range check below it
// without a test noticing.
func TestParseSelectOneIntContent_ClassifiesWhatIsNotAnInteger(t *testing.T) {
	options := []int{1, 2, 3}

	tests := []struct {
		name  string
		value float64
	}{
		{name: "not a number", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "a finite fraction", value: 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSelectOneIntContent(map[string]any{"selection": tt.value}, options)
			if err == nil {
				t.Fatal("parseSelectOneIntContent() error = nil, want the value refused")
			}
			if !strings.Contains(err.Error(), "is not an integer") {
				t.Errorf("parseSelectOneIntContent() error = %v, want it reported as not an integer", err)
			}
		})
	}
}

// TestParseSelectOneIntContent_HoldsTheIntegerRange pins the two float64
// comparisons that decide whether a conversion to int is defined.
//
// The bounds are spelled asymmetrically on purpose: math.MinInt is -2^63 and is
// exactly representable as a float64, so `<` admits it, while math.MaxInt is
// 2^63-1 and rounds up to 2^63 as a float64, so `>=` is what rejects the value
// that is one too large. Nothing exercised either endpoint: the mutants that
// died there were killed by ordinary in-range values, so both comparisons could
// have been relaxed by one and the conversion would have been reached with a
// value Go leaves implementation-defined.
func TestParseSelectOneIntContent_HoldsTheIntegerRange(t *testing.T) {
	options := []int{math.MinInt, math.MaxInt, 0, 7}

	tests := []struct {
		name    string
		value   float64
		want    int
		wantErr bool
	}{
		{name: "the smallest int is representable and accepted", value: float64(math.MinInt), want: math.MinInt},
		{name: "one below the smallest int overflows", value: float64(math.MinInt) * 2, wantErr: true},
		{name: "the float that rounds past the largest int overflows", value: float64(math.MaxInt), wantErr: true},
		{name: "an ordinary value is unaffected", value: 7, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectOneIntContent(map[string]any{"selection": tt.value}, options)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSelectOneIntContent(%g) error = nil, want the value refused as out of range", tt.value)
				}
				if !strings.Contains(err.Error(), "overflows int") {
					t.Errorf("parseSelectOneIntContent(%g) error = %v, want it reported as an overflow", tt.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSelectOneIntContent(%g) error = %v, want it accepted", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseSelectOneIntContent(%g) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestParseNumberContent_BoundsAreInclusiveAndInfinityIsNoBound pins both ends
// of the numeric range check.
//
// The schema advertises `minimum` and `maximum`, which JSON Schema defines
// inclusively, so a value sitting exactly on a bound is inside the range the
// client was shown. Only values strictly inside and strictly outside were
// covered, so either comparison could have been tightened by one and refused
// the endpoint it advertises. The infinite bounds are the other half of the
// same contract: they mean the schema carried no bound at all, so no value can
// be outside one.
func TestParseNumberContent_BoundsAreInclusiveAndInfinityIsNoBound(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		minVal  float64
		maxVal  float64
		wantErr string
	}{
		{name: "exactly the minimum", value: 0, minVal: 0, maxVal: 5},
		{name: "exactly the maximum", value: 5, minVal: 0, maxVal: 5},
		{name: "just under the minimum", value: -0.5, minVal: 0, maxVal: 5, wantErr: "below the minimum"},
		{name: "just over the maximum", value: 5.5, minVal: 0, maxVal: 5, wantErr: "above the maximum"},
		{name: "an unbounded schema admits a huge value", value: 1e300, minVal: math.Inf(-1), maxVal: math.Inf(1)},
		{name: "an unbounded schema admits a tiny value", value: -1e300, minVal: math.Inf(-1), maxVal: math.Inf(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNumberContent(map[string]any{"v": tt.value}, "v", tt.minVal, tt.maxVal)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseNumberContent(%g) error = %v, want one naming %q", tt.value, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseNumberContent(%g) error = %v, want it accepted", tt.value, err)
			}
			if got != tt.value {
				t.Errorf("parseNumberContent(%g) = %g, want the value back unchanged", tt.value, got)
			}
		})
	}
}
