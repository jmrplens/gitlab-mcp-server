// action_schema_override_test.go tests the canonical parameter injections
// applied to every action input schema: the pagination bounds, the date/time
// and URI formats, and the instance-wide enums.
package toolutil

import (
	"maps"
	"reflect"
	"testing"
)

// TestApplyCanonicalParamRanges_TypeGuard verifies that the pagination bounds
// reach an integer per_page/page property and nothing else. The declared type
// is what decides: a domain that models per_page as a string (a cursor-style
// parameter, or a keyset token) must not acquire "minimum"/"maximum", because
// a numeric bound on a string property is a schema a strict client rejects
// outright.
func TestApplyCanonicalParamRanges_TypeGuard(t *testing.T) {
	cases := []struct {
		name      string
		property  map[string]any
		wantMin   any
		wantMax   any
		wantBound bool
	}{
		{
			name:      "integer_property_gets_the_documented_bounds",
			property:  map[string]any{"type": "integer"},
			wantMin:   1,
			wantMax:   100,
			wantBound: true,
		},
		{
			name:     "string_property_is_left_alone",
			property: map[string]any{"type": "string"},
		},
		{
			name:     "untyped_property_is_left_alone",
			property: map[string]any{"description": "how many"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			property := maps.Clone(tc.property)
			schema := map[string]any{"properties": map[string]any{"per_page": property}}
			applyCanonicalParamRanges(schema)

			minimum, hasMin := property["minimum"]
			maximum, hasMax := property["maximum"]
			if hasMin != tc.wantBound || hasMax != tc.wantBound {
				t.Fatalf("per_page bounds present = %v/%v, want %v", hasMin, hasMax, tc.wantBound)
			}
			if tc.wantBound && (minimum != tc.wantMin || maximum != tc.wantMax) {
				t.Errorf("per_page bounds = %v..%v, want %v..%v", minimum, maximum, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// TestApplyCanonicalParamRanges_ExistingBoundWins verifies a bound already
// declared per action is not overwritten, and that a property with no bound at
// all still receives the canonical one.
func TestApplyCanonicalParamRanges_ExistingBoundWins(t *testing.T) {
	page := map[string]any{"type": "integer", "minimum": 0}
	perPage := map[string]any{"type": "integer"}
	schema := map[string]any{"properties": map[string]any{"page": page, "per_page": perPage}}

	applyCanonicalParamRanges(schema)

	if page["minimum"] != 0 {
		t.Errorf("page.minimum = %v, want the per-action 0 to survive", page["minimum"])
	}
	if perPage["minimum"] != 1 || perPage["maximum"] != 100 {
		t.Errorf("per_page bounds = %v..%v, want 1..100", perPage["minimum"], perPage["maximum"])
	}
}

// TestApplyCanonicalParamFormats_TypeGuard verifies the canonical date/time
// and URI formats are injected into string properties only. GitLab spells a
// few of these names as integers (an expires_at sent as a Unix timestamp, a
// url that is an id), and JSON Schema "format" is defined for strings: adding
// it to an integer property describes a value that cannot occur.
func TestApplyCanonicalParamFormats_TypeGuard(t *testing.T) {
	cases := []struct {
		name       string
		property   map[string]any
		wantFormat any
	}{
		{
			name:       "string_property_gets_the_canonical_format",
			property:   map[string]any{"type": "string"},
			wantFormat: formatDate,
		},
		{
			name:     "integer_property_is_left_alone",
			property: map[string]any{"type": "integer"},
		},
		{
			name:       "declared_format_wins",
			property:   map[string]any{"type": "string", "format": formatDateTime},
			wantFormat: formatDateTime,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			property := maps.Clone(tc.property)
			schema := map[string]any{"properties": map[string]any{"expires_at": property}}
			applyCanonicalParamFormats(schema)

			if got := property["format"]; got != tc.wantFormat {
				t.Errorf("expires_at.format = %v, want %v", got, tc.wantFormat)
			}
		})
	}
}

// TestApplyCanonicalParamEnums_TypeGuard verifies the instance-wide enums are
// injected into string properties only, and never over an enum the action
// declared for itself. A "sort" modeled as an integer is a different
// parameter than GitLab's asc/desc one, and constraining it to those two
// literals would make every valid call invalid.
func TestApplyCanonicalParamEnums_TypeGuard(t *testing.T) {
	cases := []struct {
		name     string
		property map[string]any
		wantEnum any
	}{
		{
			name:     "string_property_gets_the_canonical_enum",
			property: map[string]any{"type": "string"},
			wantEnum: []any{"asc", "desc"},
		},
		{
			name:     "integer_property_is_left_alone",
			property: map[string]any{"type": "integer"},
		},
		{
			name:     "declared_enum_wins",
			property: map[string]any{"type": "string", "enum": []any{"ASC"}},
			wantEnum: []any{"ASC"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			property := maps.Clone(tc.property)
			schema := map[string]any{"properties": map[string]any{"sort": property}}
			applyCanonicalParamEnums(schema)

			got, has := property["enum"]
			if tc.wantEnum == nil {
				if has {
					t.Errorf("sort.enum = %v, want none", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.wantEnum) {
				t.Errorf("sort.enum = %v, want %v", got, tc.wantEnum)
			}
		})
	}
}

// TestApplyInputSchemaOverrides_AppliesAndSkips verifies an override reaches
// the property its path names, that an override for a path the schema does not
// carry is skipped instead of inventing one, and that a root override patches
// the schema itself.
func TestApplyInputSchemaOverrides_AppliesAndSkips(t *testing.T) {
	state := map[string]any{"type": "string"}
	schema := map[string]any{"properties": map[string]any{"state": state}}

	applyInputSchemaOverrides(schema, []InputSchemaOverride{
		SchemaEnumOverride("state", "opened", "closed"),
		SchemaFormatOverride("missing_property", formatURI),
		SchemaRootOverride(map[string]any{"additionalProperties": false}),
	})

	if !reflect.DeepEqual(state["enum"], []any{"opened", "closed"}) {
		t.Errorf("state.enum = %v, want [opened closed]", state["enum"])
	}
	if _, has := schema["properties"].(map[string]any)["missing_property"]; has {
		t.Errorf("override for an unknown path created a property: %v", schema["properties"])
	}
	if schema["additionalProperties"] != false {
		t.Errorf("root override not applied: %v", schema)
	}
}
