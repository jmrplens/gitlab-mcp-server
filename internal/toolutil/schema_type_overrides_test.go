package toolutil

import (
	"encoding/json"
	"reflect"
	"testing"
)

// stringOrIntProbe stands in for the shape every action with an ID parameter
// has: one StringOrInt beside an ordinary string.
type stringOrIntProbe struct {
	ProjectID StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string      `json:"name,omitempty"  jsonschema:"A plain string that must stay a plain string"`
}

// TestSchemaForOptions_StringOrInt_PublishesBothTypes verifies the published
// schema says what every code path already accepts.
//
// This is the individual surface's half of a divergence the e2e run found. On
// dynamic and meta the SDK validates gitlab_execute_action's own schema, which
// declares params as a plain object, so a numeric ID reaches UnmarshalParams
// and its coercion chain and works. On individual the action's schema is the
// tool's schema, so the SDK validates project_id itself and refused a number
// before any of that ran. Declaring both types is what makes the three
// surfaces answer the same call the same way.
func TestSchemaForOptions_StringOrInt_PublishesBothTypes(t *testing.T) {
	schema := buildSchemaForType(reflect.TypeFor[stringOrIntProbe]())

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("the schema carries no properties: %v", schema)
	}
	projectID, ok := properties["project_id"].(map[string]any)
	if !ok {
		t.Fatalf("the schema carries no project_id property: %v", properties)
	}
	types, ok := projectID["type"].([]any)
	if !ok || len(types) != 2 || types[0] != "string" || types[1] != "integer" {
		t.Errorf("project_id publishes type %v, want the pair [string integer]", projectID["type"])
	}
	// The description survives the type override, which is the part a
	// per-type schema replacement could silently drop.
	if projectID["description"] != "Project ID or URL-encoded path" {
		t.Errorf("project_id lost its description: %v", projectID["description"])
	}

	name, ok := properties["name"].(map[string]any)
	if !ok || name["type"] != "string" {
		t.Errorf("name publishes type %v, want a plain string: the override must reach StringOrInt and nothing else",
			name["type"])
	}
}

// TestUnmarshalParams_StringOrInt_AcceptsBothForms verifies the other half:
// that what the schema now admits is also what the handler receives.
//
// A schema that accepts a number and a decoder that refuses one would move the
// refusal rather than remove it, so both forms are decoded here and both have
// to arrive as the same string.
func TestUnmarshalParams_StringOrInt_AcceptsBothForms(t *testing.T) {
	cases := map[string]any{
		"a string":          "189",
		"a JSON number":     float64(189),
		"an integer":        189,
		"a path, not an ID": "group/project",
	}
	want := map[string]string{
		"a string":          "189",
		"a JSON number":     "189",
		"an integer":        "189",
		"a path, not an ID": "group/project",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			input, err := UnmarshalParams[stringOrIntProbe](map[string]any{"project_id": value})
			if err != nil {
				t.Fatalf("UnmarshalParams(%v) error = %v", value, err)
			}
			if got := input.ProjectID.String(); got != want[name] {
				t.Errorf("project_id decoded to %q, want %q", got, want[name])
			}
		})
	}
}

// TestSchemaForOptions_IsOneConfiguration verifies both reflection entry
// points share it, so a schema built through either says the same thing.
func TestSchemaForOptions_IsOneConfiguration(t *testing.T) {
	first, second := schemaForOptions(), schemaForOptions()
	if first != second {
		t.Error("schemaForOptions built a second configuration; the reflection options must be one value")
	}
	if _, ok := first.TypeSchemas[reflect.TypeFor[StringOrInt]()]; !ok {
		t.Error("the options carry no schema for StringOrInt")
	}
	data, err := json.Marshal(first.TypeSchemas[reflect.TypeFor[StringOrInt]()])
	if err != nil {
		t.Fatalf("marshaling the StringOrInt schema: %v", err)
	}
	if string(data) != `{"type":["string","integer"]}` {
		t.Errorf("the StringOrInt schema is %s, want the pair", data)
	}
}
