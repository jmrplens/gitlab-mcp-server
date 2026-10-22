// action_schema_override_test.go tests the canonical parameter injections
// applied to every action input schema: the pagination bounds, the date/time
// and URI formats, and the instance-wide enums; and the root overrides that
// state a requirement across parameters.
package toolutil

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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

	// A declared ceiling survives the same way, beside a floor the property
	// did not declare and so still receives.
	capped := map[string]any{"type": "integer", "maximum": 50}
	applyCanonicalParamRanges(map[string]any{"properties": map[string]any{"per_page": capped}})
	if capped["minimum"] != 1 || capped["maximum"] != 50 {
		t.Errorf("capped per_page bounds = %v..%v, want 1..50", capped["minimum"], capped["maximum"])
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
		{
			// The issue lists' due_date is a keyword filter (overdue, week,
			// month), not a date. A date format on it told a model to send a
			// date, which GitLab matches against no keyword and answers with
			// the list unfiltered.
			name:     "enumerated_property_is_a_keyword_not_a_date",
			property: map[string]any{"type": "string", "enum": []any{"overdue", "week"}},
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

// TestSchemaApproverIDsOverride_StringItems_WidenedToIntegerOrString verifies the
// override an approver-IDs filter is registered with: the array keeps its
// place and its items admit both a numeric user ID and the Any/None literals,
// where the reflected schema admitted strings only and so refused every
// numeric ID a caller sends.
func TestSchemaApproverIDsOverride_StringItems_WidenedToIntegerOrString(t *testing.T) {
	approverIDs := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	schema := map[string]any{"properties": map[string]any{"approver_ids": approverIDs}}

	applyInputSchemaOverrides(schema, []InputSchemaOverride{SchemaApproverIDsOverride("approver_ids")})

	want := map[string]any{"type": []any{"integer", "string"}}
	if !reflect.DeepEqual(approverIDs["items"], want) {
		t.Errorf("approver_ids.items = %v, want %v", approverIDs["items"], want)
	}
	if approverIDs["type"] != "array" {
		t.Errorf("approver_ids.type = %v, want the array it was", approverIDs["type"])
	}
}

// TestFilterOverridesForSchema_NoOverrides_ReturnsWhatItWasGiven verifies that
// an action with no overrides keeps none, nil rather than an empty list, so the
// filter adds nothing to the specs of the many actions that declare none.
func TestFilterOverridesForSchema_NoOverrides_ReturnsWhatItWasGiven(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"state": map[string]any{"type": "string"}}}
	if got := FilterOverridesForSchema(schema, nil); got != nil {
		t.Errorf("FilterOverridesForSchema(schema, nil) = %#v, want nil", got)
	}
}

// TestApplyInputSchemaOverrides_NilSchema_SkipsEveryOverride verifies the
// contract an action with no input schema at all relies on: a nil map reaches
// this from specInputSchema, and every override, root or property, has to be
// skipped rather than written into a map that is not there. Writing to a nil
// map panics, so the only evidence that the path resolution refuses it is a
// call that returns.
func TestApplyInputSchemaOverrides_NilSchema_SkipsEveryOverride(t *testing.T) {
	var schema map[string]any

	applyInputSchemaOverrides(schema, []InputSchemaOverride{
		SchemaRootOverride(map[string]any{"additionalProperties": false}),
		SchemaEnumOverride("state", "opened", "closed"),
	})

	if len(schema) != 0 {
		t.Errorf("a nil schema gained content: %v", schema)
	}
}

// TestSchemaOneOfRequired_Names_OneUntypedBranchEach verifies the override
// GitLab's exactly_one_of is published with: one oneOf branch per name, each
// requiring that name alone with a value that is not empty, open to the
// call's other properties and declaring no type; names trimmed and blank ones
// left out; a repeated name kept once; and no override at all when no name is
// left, which [ActionSpec.Validate] refuses as empty.
//
// A repeated name is the case oneOf does not forgive where anyOf does: a call
// that sends the name satisfies both of its branches, and oneOf refuses a call
// that satisfies more than one, so the override would refuse every call that
// named it.
func TestSchemaOneOfRequired_Names_OneUntypedBranchEach(t *testing.T) {
	branches := func(names ...string) InputSchemaOverride {
		oneOf := make([]any, 0, len(names))
		for _, name := range names {
			oneOf = append(oneOf, map[string]any{
				"required":             []string{name},
				"properties":           map[string]any{name: map[string]any{"minLength": 1}},
				"additionalProperties": true,
			})
		}
		return InputSchemaOverride{Values: map[string]any{"oneOf": oneOf}}
	}
	cases := []struct {
		name  string
		names []string
		want  InputSchemaOverride
	}{
		{name: "two names give two branches", names: []string{"cn", "filter"}, want: branches("cn", "filter")},
		{name: "names are trimmed and blank ones left out", names: []string{" cn ", "", "  ", "filter"}, want: branches("cn", "filter")},
		{name: "a repeated name is one branch", names: []string{"cn", "filter", " cn"}, want: branches("cn", "filter")},
		{name: "no name left is no override", names: []string{"", " "}, want: InputSchemaOverride{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SchemaOneOfRequired(tc.names...); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SchemaOneOfRequired(%q) = %#v, want %#v", tc.names, got, tc.want)
			}
		})
	}
}

// TestSchemaAnyOfRequired_RepeatedName_IsOneBranch verifies that the anyOf
// override reads its names the way the oneOf one does, through the same
// helper: a repeated name is one branch rather than two, which anyOf would
// have tolerated and which published the same alternative twice.
func TestSchemaAnyOfRequired_RepeatedName_IsOneBranch(t *testing.T) {
	want := InputSchemaOverride{Values: map[string]any{"anyOf": []any{
		map[string]any{"required": []string{"name"}},
		map[string]any{"required": []string{"path"}},
	}}}
	if got := SchemaAnyOfRequired("name", "path", "name "); !reflect.DeepEqual(got, want) {
		t.Errorf("SchemaAnyOfRequired(name, path, name) = %#v, want %#v", got, want)
	}
}

// TestSchemaOneOfRequired_LockedDownSchema_AcceptsExactlyOneName verifies the
// override as a client receives it: applied to an input schema and locked
// down the way tools/list serves every tool, the schema accepts a call that
// names one of the two and refuses a call that names both or neither.
//
// The lockdown is why a branch declares no type and leaves the call's other
// properties open. It closes every object schema it reaches through oneOf,
// one with properties and no type included, and a closed branch knows only
// the name it requires, so it refuses every other property a call sends: the
// call naming the common name alone would be refused for carrying the group
// beside it, and no call at all would pass.
//
// A name counts only with a value that is not empty, which is how the handler
// reads it: it leaves an empty selector out of the request, so a call with an
// empty filter beside a common name is served as the common name alone, and
// one whose only selector is empty names none.
func TestSchemaOneOfRequired_LockedDownSchema_AcceptsExactlyOneName(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"group_id"},
		"properties": map[string]any{
			"group_id": map[string]any{"type": "string"},
			"cn":       map[string]any{"type": "string"},
			"filter":   map[string]any{"type": "string"},
		},
	}
	applyInputSchemaOverrides(schema, []InputSchemaOverride{SchemaOneOfRequired("cn", "filter")})
	locked, isMap := lockedDownSchema(schema).(map[string]any)
	if !isMap {
		t.Fatalf("lockedDownSchema() = %T, want a map", lockedDownSchema(schema))
	}
	resolved := resolveSchemaForTest(t, locked)

	cases := []struct {
		name      string
		arguments map[string]any
		wantValid bool
	}{
		{name: "the common name alone", arguments: map[string]any{"group_id": "g", "cn": "engineers"}, wantValid: true},
		{name: "the filter alone", arguments: map[string]any{"group_id": "g", "filter": "(dept=eng)"}, wantValid: true},
		{name: "both", arguments: map[string]any{"group_id": "g", "cn": "engineers", "filter": "(dept=eng)"}},
		{name: "neither", arguments: map[string]any{"group_id": "g"}},
		{name: "the common name beside an empty filter", arguments: map[string]any{"group_id": "g", "cn": "engineers", "filter": ""}, wantValid: true},
		{name: "the filter beside an empty common name", arguments: map[string]any{"group_id": "g", "cn": "", "filter": "(dept=eng)"}, wantValid: true},
		{name: "an empty common name alone", arguments: map[string]any{"group_id": "g", "cn": ""}},
		{name: "both empty", arguments: map[string]any{"group_id": "g", "cn": "", "filter": ""}},
		{name: "a property the schema does not name", arguments: map[string]any{"group_id": "g", "cn": "engineers", "extra": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := resolved.Validate(tc.arguments)
			if (err == nil) != tc.wantValid {
				t.Errorf("Validate(%v) error = %v, want valid %t", tc.arguments, err, tc.wantValid)
			}
		})
	}
}

// resolveSchemaForTest decodes a schema map into a resolved jsonschema-go
// schema, the validator the SDK puts a tool's arguments to.
func resolveSchemaForTest(t *testing.T, schema map[string]any) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var decoded jsonschema.Schema
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	resolved, err := decoded.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return resolved
}

// TestSchemaOverrideTarget_NestedObjectProperty verifies an override path
// reaches a property of a nested object directly, not only one inside an
// array's items: the step into a child with no items stays on the child.
func TestSchemaOverrideTarget_NestedObjectProperty(t *testing.T) {
	name := map[string]any{"type": "string"}
	schema := map[string]any{"properties": map[string]any{
		"owner": map[string]any{"type": "object", "properties": map[string]any{"name": name}},
	}}
	if got := schemaOverrideTarget(schema, "owner.name"); !reflect.DeepEqual(got, name) {
		t.Errorf("schemaOverrideTarget(owner.name) = %#v, want %#v", got, name)
	}
}
