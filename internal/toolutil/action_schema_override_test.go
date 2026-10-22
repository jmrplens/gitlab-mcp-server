// action_schema_override_test.go tests the canonical parameter injections
// applied to every action input schema: the pagination bounds, the date/time
// and URI formats, and the instance-wide enums; the root overrides that state
// a requirement across parameters; and the items overrides that state what
// each element of a list must carry.
package toolutil

import (
	"encoding/json"
	"fmt"
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

// itemsSchema returns an input schema holding one list of objects, rules,
// whose elements carry a role, a user and a group, each nullable as the SDK
// writes a pointer field, and nothing else.
func itemsSchema() map[string]any {
	nullableInteger := func() map[string]any { return map[string]any{"type": []any{"null", "integer"}} }
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"rules": map[string]any{
				"type": []any{"null", "array"},
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           nullableInteger(),
						"access_level": nullableInteger(),
						"user_id":      nullableInteger(),
						"group_id":     nullableInteger(),
						"_destroy":     map[string]any{"type": []any{"null", "boolean"}},
					},
				},
			},
		},
	}
}

// TestSchemaItemsOverride_PatchesTheElementsOfTheList verifies the override
// that states what each element of a list must be: its values land on the
// list's item schema and not on the list, a list reached through a nested
// object or through a $defs reference is reached as well, and its values are
// copied, so a caller that edits the map it passed afterwards changes no
// schema.
func TestSchemaItemsOverride_PatchesTheElementsOfTheList(t *testing.T) {
	t.Run("the item schema of a top-level list", func(t *testing.T) {
		schema := itemsSchema()
		values := map[string]any{"minProperties": 1}
		override := SchemaItemsOverride(" rules ", values)
		values["minProperties"] = 9

		applyInputSchemaOverrides(schema, []InputSchemaOverride{override})

		rules := schema["properties"].(map[string]any)["rules"].(map[string]any)
		if got := rules["items"].(map[string]any)["minProperties"]; got != 1 {
			t.Errorf("rules.items.minProperties = %v, want 1", got)
		}
		if _, has := rules["minProperties"]; has {
			t.Errorf("the list itself was patched: %v", rules)
		}
		if want := (InputSchemaOverride{PropertyPath: "rules", Items: true, Values: map[string]any{"minProperties": 1}}); !reflect.DeepEqual(override, want) {
			t.Errorf("SchemaItemsOverride() = %#v, want %#v", override, want)
		}
	})

	t.Run("a list inside an object", func(t *testing.T) {
		items := map[string]any{"type": "object"}
		schema := map[string]any{"properties": map[string]any{
			"policy": map[string]any{"type": "object", "properties": map[string]any{
				"rules": map[string]any{"type": "array", "items": items},
			}},
		}}
		applyInputSchemaOverrides(schema, []InputSchemaOverride{SchemaItemsOverride("policy.rules", map[string]any{"minProperties": 1})})
		if items["minProperties"] != 1 {
			t.Errorf("policy.rules.items = %v, want minProperties 1", items)
		}
	})

	t.Run("a list whose items are a $defs reference", func(t *testing.T) {
		rule := map[string]any{"type": "object"}
		schema := map[string]any{
			"$defs":      map[string]any{"Rule": rule},
			"properties": map[string]any{"rules": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Rule"}}},
		}
		applyInputSchemaOverrides(schema, []InputSchemaOverride{SchemaItemsOverride("rules", map[string]any{"minProperties": 1})})
		if rule["minProperties"] != 1 {
			t.Errorf("$defs.Rule = %v, want minProperties 1", rule)
		}
	})
}

// TestSchemaItemsOverride_NoListAtItsPath_PatchesNothing verifies that an
// items override names a list or nothing: at the root, at a property that is
// no list and at a path the schema does not carry, it is skipped rather than
// written onto the property or the root, which would constrain the call
// rather than an element of it.
func TestSchemaItemsOverride_NoListAtItsPath_PatchesNothing(t *testing.T) {
	for _, path := range []string{"", "name", "missing"} {
		t.Run(fmt.Sprintf("path %q", path), func(t *testing.T) {
			schema := itemsSchema()
			want := itemsSchema()
			applyInputSchemaOverrides(schema, []InputSchemaOverride{SchemaItemsOverride(path, map[string]any{"minProperties": 1})})
			if !reflect.DeepEqual(schema, want) {
				t.Errorf("an items override at %q changed the schema to %#v", path, schema)
			}
		})
	}
}

// TestValidateInputSchemaOverrides_ItemsOverride_NeedsAList verifies the check
// an action spec's overrides pass when the spec is validated: an items
// override is accepted at a list and refused at the root, at a property that
// is no list and at a path the schema does not carry, each with words that
// say an items override was the one refused.
func TestValidateInputSchemaOverrides_ItemsOverride_NeedsAList(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "a list", path: "rules"},
		{name: "the root", path: "", wantErr: `action spec "rules.set" has input schema override for the items of "", which is no list`},
		{name: "a property that is no list", path: "name", wantErr: `action spec "rules.set" has input schema override for the items of "name", which is no list`},
		{name: "a path the schema does not carry", path: "missing", wantErr: `action spec "rules.set" has input schema override for the items of "missing", which is no list`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := ActionSpec{
				Name:                 "rules.set",
				Route:                ActionRoute{InputSchema: itemsSchema()},
				InputSchemaOverrides: []InputSchemaOverride{SchemaItemsOverride(tc.path, map[string]any{"minProperties": 1})},
			}
			err := validateInputSchemaOverrides(spec)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateInputSchemaOverrides() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("validateInputSchemaOverrides() = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestFilterOverridesForSchema_ItemsOverride_FollowsItsList verifies that an
// items override is kept while its list is in the schema and dropped once the
// list is pruned, as a property override is, and that one at the root is
// dropped where a root override is kept: it targets the items of nothing.
func TestFilterOverridesForSchema_ItemsOverride_FollowsItsList(t *testing.T) {
	kept := SchemaItemsOverride("rules", map[string]any{"minProperties": 1})
	pruned := SchemaItemsOverride("approvals", map[string]any{"minProperties": 1})
	atTheRoot := SchemaItemsOverride("", map[string]any{"minProperties": 1})

	got := FilterOverridesForSchema(itemsSchema(), []InputSchemaOverride{kept, pruned, atTheRoot})
	if want := []InputSchemaOverride{kept}; !reflect.DeepEqual(got, want) {
		t.Errorf("FilterOverridesForSchema() = %#v, want %#v", got, want)
	}
}

// TestNewActionSpec_ItemsOverride_IsKeptAndKeyedApart verifies that an items
// override survives the copy every spec constructor makes of its overrides,
// and that the schema it derives is not the one a property override with the
// same path and values derives: both are memoized per process under a name
// built from the overrides, and a name that did not tell the two apart would
// hand one action the other's schema.
func TestNewActionSpec_ItemsOverride_IsKeptAndKeyedApart(t *testing.T) {
	shared := itemsSchema()
	values := map[string]any{"description": "Each rule"}
	onItems := NewActionSpec("rules.items", ActionRoute{InputSchema: shared}, ActionSpecOptions{
		InputSchemaOverrides: []InputSchemaOverride{SchemaItemsOverride("rules", values)},
	})
	onList := NewActionSpec("rules.list", ActionRoute{InputSchema: shared}, ActionSpecOptions{
		InputSchemaOverrides: []InputSchemaOverride{SchemaPropertyOverride("rules", values)},
	})

	if got := onItems.InputSchemaOverrides; len(got) != 1 || !got[0].Items {
		t.Fatalf("NewActionSpec() kept overrides %#v, want the one items override", got)
	}
	itemsRules := onItems.Route.InputSchema["properties"].(map[string]any)["rules"].(map[string]any)
	listRules := onList.Route.InputSchema["properties"].(map[string]any)["rules"].(map[string]any)
	if got := itemsRules["items"].(map[string]any)["description"]; got != "Each rule" {
		t.Errorf("items override: rules.items.description = %v, want %q", got, "Each rule")
	}
	if got, has := itemsRules["description"]; has {
		t.Errorf("items override: rules.description = %v, want none", got)
	}
	if got := listRules["description"]; got != "Each rule" {
		t.Errorf("property override: rules.description = %v, want %q", got, "Each rule")
	}
}

// nonNullBranchFor is the branch every items alternative is built of: the
// name present with a value that is not null, the element's other properties
// left open.
func nonNullBranchFor(names ...string) map[string]any {
	properties := make(map[string]any, len(names))
	for _, name := range names {
		properties[name] = map[string]any{"not": map[string]any{"type": "null"}}
	}
	return map[string]any{"required": names, "properties": properties, "additionalProperties": true}
}

// TestSchemaItemsAlternatives_Names_OneNonNullBranchEach verifies the four
// overrides that state what an element of a list has to name: one branch per
// name for exactly one and for at least one, a branch for the flag that
// exempts an element when it is true, and one branch per pair of names for at
// most one, each requiring its names with a value that is not null and
// declaring no type. Names are trimmed, a blank one left out and a repeated
// one kept once, and a list of names that constrains nothing is no override,
// which [ActionSpec.Validate] refuses as empty.
func TestSchemaItemsAlternatives_Names_OneNonNullBranchEach(t *testing.T) {
	onItems := func(values map[string]any) InputSchemaOverride {
		return InputSchemaOverride{PropertyPath: "rules", Items: true, Values: values}
	}
	trueFlag := map[string]any{
		"required":             []string{"_destroy"},
		"properties":           map[string]any{"_destroy": map[string]any{"const": true}},
		"additionalProperties": true,
	}
	cases := []struct {
		name string
		got  InputSchemaOverride
		want InputSchemaOverride
	}{
		{
			name: "exactly one",
			got:  SchemaItemsOneOfRequired("rules", " access_level ", "user_id", "", "group_id", "user_id"),
			want: onItems(map[string]any{"oneOf": []any{nonNullBranchFor("access_level"), nonNullBranchFor("user_id"), nonNullBranchFor("group_id")}}),
		},
		{name: "exactly one of no name", got: SchemaItemsOneOfRequired("rules", " "), want: InputSchemaOverride{}},
		{
			name: "at least one",
			got:  SchemaItemsAnyOfRequired("rules", "access_level", "user_id"),
			want: onItems(map[string]any{"anyOf": []any{nonNullBranchFor("access_level"), nonNullBranchFor("user_id")}}),
		},
		{name: "at least one of no name", got: SchemaItemsAnyOfRequired("rules"), want: InputSchemaOverride{}},
		{
			name: "at least one unless a flag is true",
			got:  SchemaItemsAnyOfRequiredUnlessTrue("rules", " _destroy ", "access_level", "user_id"),
			want: onItems(map[string]any{"anyOf": []any{nonNullBranchFor("access_level"), nonNullBranchFor("user_id"), trueFlag}}),
		},
		{
			name: "at least one with a blank flag",
			got:  SchemaItemsAnyOfRequiredUnlessTrue("rules", " ", "access_level"),
			want: onItems(map[string]any{"anyOf": []any{nonNullBranchFor("access_level")}}),
		},
		{name: "at least one of no name unless a flag is true", got: SchemaItemsAnyOfRequiredUnlessTrue("rules", "_destroy", ""), want: InputSchemaOverride{}},
		{
			name: "at most one",
			got:  SchemaItemsAtMostOneOf("rules", "access_level", "user_id", " group_id", "access_level"),
			want: onItems(map[string]any{"not": map[string]any{"anyOf": []any{
				nonNullBranchFor("access_level", "user_id"),
				nonNullBranchFor("access_level", "group_id"),
				nonNullBranchFor("user_id", "group_id"),
			}}}),
		},
		{name: "at most one of one name", got: SchemaItemsAtMostOneOf("rules", "access_level", " access_level"), want: InputSchemaOverride{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("override = %#v, want %#v", tc.got, tc.want)
			}
		})
	}
}

// TestSchemaItemsAlternatives_LockedDownSchema_HoldEachElement verifies the
// items overrides as a client receives them: applied to an input schema and
// locked down the way tools/list serves every tool, each holds every element
// of its list to its rule and leaves the rest of the call alone.
//
// A name counts only with a value that is not null. The handlers decode an
// element into pointers and send the fields that are not nil, so a null is
// a field the request leaves out: an element naming a role beside a null
// user names one grantee to GitLab, and the schema has to agree.
//
// The lockdown closes every object schema it reaches through anyOf or oneOf,
// so a branch that declared a type, or properties without leaving the
// element's other properties open, would refuse every property but its own.
// It does not descend into not, and the pairs at most one is built of leave
// the element open themselves, so the rule holds whether it does or not.
func TestSchemaItemsAlternatives_LockedDownSchema_HoldEachElement(t *testing.T) {
	grantees := []string{"access_level", "user_id", "group_id"}
	lists := []struct {
		name      string
		overrides []InputSchemaOverride
	}{
		{
			name:      "exactly one",
			overrides: []InputSchemaOverride{SchemaItemsOneOfRequired("rules", grantees...)},
		},
		{
			name:      "at least one unless destroyed",
			overrides: []InputSchemaOverride{SchemaItemsAnyOfRequiredUnlessTrue("rules", "_destroy", grantees...)},
		},
		{
			name: "an id or exactly one",
			overrides: []InputSchemaOverride{
				SchemaItemsAnyOfRequired("rules", "id", "access_level", "user_id", "group_id"),
				SchemaItemsAtMostOneOf("rules", grantees...),
			},
		},
	}
	elements := []struct {
		name    string
		element map[string]any
		valid   map[string]bool
	}{
		{name: "a role", element: map[string]any{"access_level": 30}, valid: map[string]bool{"exactly one": true, "at least one unless destroyed": true, "an id or exactly one": true}},
		{name: "a user", element: map[string]any{"user_id": 5}, valid: map[string]bool{"exactly one": true, "at least one unless destroyed": true, "an id or exactly one": true}},
		{name: "a group", element: map[string]any{"group_id": 7}, valid: map[string]bool{"exactly one": true, "at least one unless destroyed": true, "an id or exactly one": true}},
		{name: "a role beside a null user", element: map[string]any{"access_level": 30, "user_id": nil}, valid: map[string]bool{"exactly one": true, "at least one unless destroyed": true, "an id or exactly one": true}},
		{name: "a role and a user", element: map[string]any{"access_level": 30, "user_id": 5}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "a user and a group", element: map[string]any{"user_id": 5, "group_id": 7}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "a role and a group", element: map[string]any{"access_level": 30, "group_id": 7}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "all three", element: map[string]any{"access_level": 30, "user_id": 5, "group_id": 7}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "nothing", element: map[string]any{}, valid: map[string]bool{}},
		{name: "only nulls", element: map[string]any{"access_level": nil, "user_id": nil, "group_id": nil}, valid: map[string]bool{}},
		{name: "an id alone", element: map[string]any{"id": 11}, valid: map[string]bool{"an id or exactly one": true}},
		{name: "an id and a role", element: map[string]any{"id": 11, "access_level": 30}, valid: map[string]bool{"exactly one": true, "at least one unless destroyed": true, "an id or exactly one": true}},
		{name: "an id, a role and a user", element: map[string]any{"id": 11, "access_level": 30, "user_id": 5}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "a null id alone", element: map[string]any{"id": nil}, valid: map[string]bool{}},
		{name: "destroyed alone", element: map[string]any{"_destroy": true}, valid: map[string]bool{"at least one unless destroyed": true}},
		{name: "destroy false alone", element: map[string]any{"_destroy": false}, valid: map[string]bool{}},
		{name: "a property the element does not carry", element: map[string]any{"access_level": 30, "extra": 1}, valid: map[string]bool{}},
	}
	for _, list := range lists {
		t.Run(list.name, func(t *testing.T) {
			schema := itemsSchema()
			applyInputSchemaOverrides(schema, list.overrides)
			locked, isMap := lockedDownSchema(schema).(map[string]any)
			if !isMap {
				t.Fatalf("lockedDownSchema() = %T, want a map", lockedDownSchema(schema))
			}
			resolved := resolveSchemaForTest(t, locked)
			for _, element := range elements {
				t.Run(element.name, func(t *testing.T) {
					arguments := map[string]any{"name": "production", "rules": []any{element.element}}
					err := resolved.Validate(arguments)
					if want := element.valid[list.name]; (err == nil) != want {
						t.Errorf("Validate(%v) error = %v, want valid %t", arguments, err, want)
					}
				})
			}
		})
	}
}
