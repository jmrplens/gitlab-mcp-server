package dynamic

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// exampleCatalog names one catalog a server can serve: a tier on a
// self-managed instance or on GitLab.com.
type exampleCatalog struct {
	name   string
	tier   edition.Tier
	dotcom bool
}

// servedCatalogs is every tier on both instance classes. The tier prunes
// fields from the input schemas and GitLab.com adds the Orbit group, so an
// example that satisfies its schema on one build can fail on another.
var servedCatalogs = []exampleCatalog{
	{name: "free", tier: edition.Free},
	{name: "premium", tier: edition.Premium},
	{name: "ultimate", tier: edition.Ultimate},
	{name: "gitlab.com free", tier: edition.Free, dotcom: true},
	{name: "gitlab.com premium", tier: edition.Premium, dotcom: true},
	{name: "gitlab.com ultimate", tier: edition.Ultimate, dotcom: true},
}

// buildServedCatalog builds the catalog the dynamic surface serves for one
// tier and instance class, standalone actions included.
func buildServedCatalog(t *testing.T, served exampleCatalog) *actioncatalog.Catalog {
	t.Helper()
	var client *gitlabclient.Client
	if served.dotcom {
		var err error
		client, err = gitlabclient.NewClientWithToken("https://gitlab.com", "test-token", false)
		if err != nil {
			t.Fatalf("NewClientWithToken(gitlab.com) error = %v", err)
		}
	}
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: served.tier, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(%s) error = %v", served.name, err)
	}
	catalog, err = AddStandaloneCatalog(catalog, client, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(%s) error = %v", served.name, err)
	}
	return catalog
}

// TestFind_EveryExampleSatisfiesItsInputSchema verifies that the execute call
// find and describe offer for an action is one its own input schema accepts,
// for every action of every catalog a server can serve. The example is what a
// model copies into its first call, so one its schema refuses teaches the
// model to send a call that fails before it reaches GitLab: a hex color filled
// with "value", every branch of an anyOf filled at once, a date written as
// its template. The schema is validated as published, and the formats the
// validator leaves to annotation (date, date-time, uri) are checked here. The
// example is also put to execute's own check of the params, which is what
// refuses a call before any handler runs.
//
// The same walk holds the two lists the example is built from: required_params
// names what every call carries and nothing an alternative names, and the
// example fills exactly those plus the first alternative group.
func TestFind_EveryExampleSatisfiesItsInputSchema(t *testing.T) {
	for _, served := range servedCatalogs {
		t.Run(served.name, func(t *testing.T) {
			registry := NewRegistryFromCatalog(buildServedCatalog(t, served))
			for _, entry := range registry.entries {
				checkPublishedExample(t, entry, registry.describeEntry(entry))
			}
		})
	}
}

// checkPublishedExample holds one described action to the four rules the
// catalog-wide test states: its example satisfies its input schema, execute's
// check of the params accepts it, its required params name nothing only an
// alternative requires, and its example fills exactly the required params and
// the first alternative group.
func checkPublishedExample(t *testing.T, entry actionEntry, description ActionDescription) {
	t.Helper()
	params, ok := description.Example.Arguments["params"].(map[string]any)
	if !ok {
		t.Errorf("%s example carries no params object: %#v", description.ID, description.Example.Arguments)
		return
	}
	if err := validateExampleParams(description.InputSchema, params); err != nil {
		t.Errorf("%s example params %v fail the action's input schema: %v", description.ID, params, err)
	}
	if refused := validateDynamicExecuteParams(entry, params); refused != nil {
		t.Errorf("%s example params %v are refused by execute: %s", description.ID, params, textContent(refused))
	}
	want := append([]string(nil), description.RequiredParams...)
	for _, group := range description.RequiredParamsAnyOf {
		if shared := slices.IndexFunc(group, func(name string) bool { return slices.Contains(description.RequiredParams, name) }); shared >= 0 {
			t.Errorf("%s required_params %v names %q, which only an alternative requires", description.ID, description.RequiredParams, group[shared])
		}
	}
	if len(description.RequiredParamsAnyOf) > 0 {
		want = append(want, description.RequiredParamsAnyOf[0]...)
	}
	if got := slices.Sorted(maps.Keys(params)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("%s example fills %v, want the required params and the first alternative %v", description.ID, got, want)
	}
}

// TestFind_AnAnyOfActionPublishesItsAlternativesApart verifies the case the
// defect was found on, through the three published answers: find, describe
// and the internal search. security_attribute.update requires attribute_id
// and at least one of name, description or color, so required_params names
// attribute_id alone, the three alternatives are their own groups, and the
// example sends one of them with a value its schema accepts.
func TestFind_AnAnyOfActionPublishesItsAlternativesApart(t *testing.T) {
	const id = "security_attribute.update"
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	wantRequired := []string{"attribute_id"}
	wantGroups := [][]string{{"name"}, {"description"}, {"color"}}

	_, found, err := registry.Find(t.Context(), nil, FindInput{Query: "update security attribute color", Limit: 50})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	index := slices.IndexFunc(found.Results, func(result FindResult) bool { return result.ID == id })
	if index < 0 {
		t.Fatalf("Find() did not return %s: %+v", id, found.Results)
	}
	result := found.Results[index]
	if !slices.Equal(result.RequiredParams, wantRequired) || !reflect.DeepEqual(result.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("find %s required_params = %v, required_params_any_of = %v, want %v and %v", id, result.RequiredParams, result.RequiredParamsAnyOf, wantRequired, wantGroups)
	}
	wantParams := map[string]any{"attribute_id": 123, "name": "value"}
	if got := result.Example.Arguments["params"]; !reflect.DeepEqual(got, wantParams) {
		t.Errorf("find %s example params = %#v, want %#v", id, got, wantParams)
	}

	_, described, err := registry.Describe(t.Context(), nil, DescribeInput{Action: id})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if got := described.Actions[0]; !slices.Equal(got.RequiredParams, wantRequired) || !reflect.DeepEqual(got.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("describe %s required_params = %v, required_params_any_of = %v, want %v and %v", id, got.RequiredParams, got.RequiredParamsAnyOf, wantRequired, wantGroups)
	}

	_, searched, err := registry.Search(t.Context(), nil, SearchInput{Query: "update security attribute color", Limit: 50})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	index = slices.IndexFunc(searched.Results, func(result SearchResult) bool { return result.ID == id })
	if index < 0 {
		t.Fatalf("Search() did not return %s: %+v", id, searched.Results)
	}
	if got := searched.Results[index]; !slices.Equal(got.RequiredParams, wantRequired) || !reflect.DeepEqual(got.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("search %s required_params = %v, required_params_any_of = %v, want %v and %v", id, got.RequiredParams, got.RequiredParamsAnyOf, wantRequired, wantGroups)
	}
}

// TestFind_PublishedAlternativesAreCopies verifies that a result's
// alternative groups are its own, on the two paths that publish them: the
// description find and describe answer with, and the internal search. The
// entry they come from is shared by every server of the process, so a caller
// that edits a group of one answer must not change the next answer anyone
// gets.
func TestFind_PublishedAlternativesAreCopies(t *testing.T) {
	const id = "security_attribute.update"
	wantGroups := [][]string{{"name"}, {"description"}, {"color"}}
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))

	t.Run("describe", func(t *testing.T) {
		entry, ok := registry.resolveAction(id)
		if !ok {
			t.Fatalf("resolveAction(%s) found nothing", id)
		}
		first := registry.describeEntry(entry)
		first.RequiredParamsAnyOf[0][0] = "edited"
		first.RequiredParamsAnyOf = append(first.RequiredParamsAnyOf, []string{"added"})
		if second := registry.describeEntry(entry); !reflect.DeepEqual(second.RequiredParamsAnyOf, wantGroups) {
			t.Errorf("a second answer carries %v after the first was edited, want the groups unchanged", second.RequiredParamsAnyOf)
		}
	})

	t.Run("search", func(t *testing.T) {
		search := func() SearchResult {
			t.Helper()
			_, searched, err := registry.Search(t.Context(), nil, SearchInput{Query: "update security attribute color", Limit: 50})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			index := slices.IndexFunc(searched.Results, func(result SearchResult) bool { return result.ID == id })
			if index < 0 {
				t.Fatalf("Search() did not return %s: %+v", id, searched.Results)
			}
			return searched.Results[index]
		}
		first := search()
		first.RequiredParamsAnyOf[0][0] = "edited"
		if second := search(); !reflect.DeepEqual(second.RequiredParamsAnyOf, wantGroups) {
			t.Errorf("a second search carries %v after the first was edited, want the groups unchanged", second.RequiredParamsAnyOf)
		}
	})
}

// TestFind_ExamplesUseValuesGitLabAccepts verifies the example values whose
// schema accepts more than GitLab does. A label's color is a free string in
// its schema and GitLab takes a hex color or a CSS color name, so "value"
// passed the schema and failed at GitLab, which is issue 1175's own case on
// another action; and an access level is an integer whose schema lists the
// levels in its description, where 1 is none of them. Developer, 30, is a
// level every one of these routes accepts, the member roles' 10 to 50
// included, and an enum that holds it is answered with it rather than with
// its first value, No access.
func TestFind_ExamplesUseValuesGitLabAccepts(t *testing.T) {
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	tests := []struct {
		action string
		param  string
		want   any
	}{
		{action: "project.label_create", param: "color", want: exampleHexColor},
		{action: "group.group_label_create", param: "color", want: exampleHexColor},
		{action: "access.invite_group", param: "access_level", want: 30},
		{action: "access.invite_project", param: "access_level", want: 30},
		{action: "group.saml_link_add", param: "access_level", want: 30},
		{action: "group.share_with_group", param: "group_access", want: 30},
		{action: "project.share_with_group", param: "group_access", want: 30},
		{action: "group.ldap_link_add", param: "group_access", want: 30},
		{action: "member_role.create_group", param: "base_access_level", want: 30},
		{action: "member_role.create_instance", param: "base_access_level", want: 30},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			entry, ok := registry.resolveAction(tc.action)
			if !ok {
				t.Fatalf("resolveAction(%s) found nothing", tc.action)
			}
			params, _ := registry.describeEntry(entry).Example.Arguments["params"].(map[string]any)
			if got := params[tc.param]; fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("%s example %s = %#v, want %#v", tc.action, tc.param, got, tc.want)
			}
		})
	}
}

// TestExampleFor_FillsTheRequiredParamsAndTheFirstAlternative verifies the
// shape of the example call: every parameter the schema's root requires, the
// first alternative group and none of the others, and top-level confirm only
// on a destructive action.
func TestExampleFor_FillsTheRequiredParamsAndTheFirstAlternative(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"attribute_id"},
		"anyOf": []any{
			map[string]any{"required": []any{"color"}},
			map[string]any{"required": []any{"name"}},
		},
		"properties": map[string]any{
			"attribute_id": map[string]any{"type": "integer"},
			"name":         map[string]any{"type": "string", "minLength": 1},
			"color":        map[string]any{"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"},
		},
	}

	t.Run("not destructive", func(t *testing.T) {
		got := exampleFor(actionEntry{ID: "security_attribute.update"}, schema)
		want := ActionExample{Tool: executeActionToolName, Arguments: map[string]any{
			"action": "security_attribute.update",
			"params": map[string]any{"attribute_id": 123, "color": exampleHexColor},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exampleFor() = %#v, want %#v", got, want)
		}
	})

	t.Run("destructive", func(t *testing.T) {
		got := exampleFor(actionEntry{ID: "security_attribute.delete", Destructive: true}, map[string]any{"type": "object"})
		want := ActionExample{Tool: executeActionToolName, Arguments: map[string]any{
			"action":  "security_attribute.delete",
			"params":  map[string]any{},
			"confirm": true,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exampleFor() = %#v, want %#v", got, want)
		}
	})
}

// TestExampleValue_SatisfiesEachConstraint verifies the value written for one
// parameter, constraint by constraint. Each case is a shape a required
// parameter of the catalog takes, and the value is the one the example
// publishes, so a change here changes what a model is taught to send.
func TestExampleValue_SatisfiesEachConstraint(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		schema map[string]any
		want   any
	}{
		{name: "a string enum takes its first value", param: "mode", schema: map[string]any{"type": "string", "enum": []any{"ADD", "REMOVE"}}, want: "ADD"},
		{name: "an integer enum takes its first value", param: "group_access", schema: map[string]any{"type": "integer", "enum": []any{0, 5}}, want: 0},
		{name: "an enum holding the placeholder takes it", param: "group_access", schema: map[string]any{"type": "integer", "enum": []any{0, 5, 10, 30, 40}}, want: 30},
		{name: "a decoded enum holding the placeholder keeps its own value", param: "access_level", schema: map[string]any{"type": "integer", "enum": []any{float64(0), float64(30)}}, want: float64(30)},
		{name: "a color with no pattern is still a color", param: "color", schema: map[string]any{"type": "string"}, want: exampleHexColor},
		{name: "an access level is Developer", param: "access_level", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "a group's access level is Developer", param: "group_access", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "a role's base access level is Developer", param: "base_access_level", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "an enum written in Go takes its first value", param: "mode", schema: map[string]any{"type": "string", "enum": []string{"ADD", "REMOVE"}}, want: "ADD"},
		{name: "an empty enum constrains nothing", param: "title", schema: map[string]any{"type": "string", "enum": []any{}}, want: "value"},
		{name: "an empty enum written in Go constrains nothing", param: "title", schema: map[string]any{"type": "string", "enum": []string{}}, want: "value"},
		{name: "a hex pattern is answered with a color", param: "color", schema: map[string]any{"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"}, want: exampleHexColor},
		{name: "a pattern the placeholder satisfies keeps it", param: "url", schema: map[string]any{"type": "string", "pattern": "^(https?|ftp)://"}, want: exampleURL},
		{name: "a pattern only the URL satisfies takes the URL", param: "target", schema: map[string]any{"type": "string", "pattern": "^https://"}, want: exampleURL},
		{name: "a pattern nothing satisfies keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "pattern": "^z+$"}, want: "value"},
		{name: "a pattern that does not compile keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "pattern": "("}, want: "value"},
		{name: "a date format takes a date", param: "expires_at", schema: map[string]any{"type": "string", "format": "date"}, want: exampleDate},
		{name: "a date-time format takes a timestamp", param: "expires_at", schema: map[string]any{"type": "string", "format": "date-time"}, want: exampleDateTime},
		{name: "a uri format takes a URL whatever the name", param: "bitbucket_server_url", schema: map[string]any{"type": "string", "format": "uri"}, want: exampleURL},
		{name: "an unknown format keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "format": "markdown"}, want: "value"},
		{name: "a string named like an identifier takes its digits", param: "user_id", schema: map[string]any{"type": "string"}, want: "123"},
		{name: "an integer named like an identifier", param: "attribute_id", schema: map[string]any{"type": "integer"}, want: 123},
		{name: "an integer whose placeholder is a path", param: "namespace_id", schema: map[string]any{"type": "integer"}, want: 123},
		{name: "an integer with no identifier name", param: "serial", schema: map[string]any{"type": "integer", "minimum": 0}, want: 1},
		{name: "a number", param: "weight", schema: map[string]any{"type": "number"}, want: 1},
		{name: "a project path where a string is allowed", param: "project_id", schema: map[string]any{"type": []any{"string", "integer"}}, want: "group/project"},
		{name: "an internal ID where an integer is allowed", param: "merge_request_iid", schema: map[string]any{"type": []any{"string", "integer"}}, want: 123},
		{name: "a boolean", param: "active", schema: map[string]any{"type": "boolean"}, want: true},
		{name: "a nullable array of identifiers", param: "user_achievement_ids", schema: map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "integer"}}, want: []any{123}},
		{name: "an array of strings", param: "scopes", schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, want: []any{"value"}},
		{
			name:  "an array of objects fills each item's required fields",
			param: "files",
			schema: map[string]any{"type": []any{"null", "array"}, "minItems": 1, "items": map[string]any{
				"type":     "object",
				"required": []any{"file_path", "content"},
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string"},
					"content":   map[string]any{"type": "string"},
					"encoding":  map[string]any{"type": "string"},
				},
			}},
			want: []any{map[string]any{"content": "value", "file_path": "path/to/file"}},
		},
		{
			name:  "an object fills its required fields only",
			param: "configuration",
			schema: map[string]any{"type": "object", "required": []any{"url", "access_token"}, "properties": map[string]any{
				"url":          map[string]any{"type": "string"},
				"access_token": map[string]any{"type": "string"},
				"note":         map[string]any{"type": "string"},
			}},
			want: map[string]any{"access_token": "value", "url": exampleURL},
		},
		{name: "an open object is empty", param: "settings", schema: map[string]any{"type": "object", "additionalProperties": true}, want: map[string]any{}},
		{name: "an untyped oneOf takes its first branch", param: "value", schema: map[string]any{"oneOf": []any{"ignored", map[string]any{"type": "boolean"}, map[string]any{"type": "string"}}}, want: true},
		{name: "an untyped anyOf takes its first branch", param: "value", schema: map[string]any{"anyOf": []any{map[string]any{"type": "integer"}}}, want: 1},
		{name: "a type that is only null keeps the placeholder", param: "title", schema: map[string]any{"type": "null"}, want: "value"},
		{name: "an unknown type keeps the placeholder", param: "title", schema: map[string]any{"type": "date"}, want: "value"},
		{name: "a type list naming nothing keeps the placeholder", param: "title", schema: map[string]any{"type": []any{"null", 42}}, want: "value"},
		{name: "no schema keeps the placeholder", param: "issue_iid", schema: nil, want: 123},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exampleValue(tc.param, tc.schema); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("exampleValue(%q, %v) = %#v, want %#v", tc.param, tc.schema, got, tc.want)
			}
		})
	}
}

// validateExampleParams validates params against a copy of schema, so
// resolving it cannot touch the schema every server shares, and then checks
// the string formats the validator does not.
func validateExampleParams(schema, params map[string]any) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	var copied jsonschema.Schema
	if err = json.Unmarshal(data, &copied); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	resolved, err := copied.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema: %w", err)
	}
	if err = resolved.Validate(params); err != nil {
		return err
	}
	return checkExampleFormats(schema, params, "params")
}

// checkExampleFormats walks a value beside the schema that describes it and
// refuses a string whose declared format it does not satisfy.
func checkExampleFormats(schema map[string]any, value any, path string) error {
	switch typed := value.(type) {
	case string:
		format, _ := schema["format"].(string)
		return checkExampleFormat(format, typed, path)
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for name, child := range typed {
			childSchema, _ := properties[name].(map[string]any)
			if err := checkExampleFormats(childSchema, child, path+"."+name); err != nil {
				return err
			}
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		for i, child := range typed {
			if err := checkExampleFormats(items, child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkExampleFormat checks one string against the formats the catalog
// declares.
func checkExampleFormat(format, value, path string) error {
	var err error
	switch format {
	case "date":
		_, err = time.Parse(time.DateOnly, value)
	case "date-time":
		_, err = time.Parse(time.RFC3339, value)
	case "uri":
		var parsed *url.URL
		parsed, err = url.Parse(value)
		if err == nil && (parsed.Scheme == "" || parsed.Host == "") {
			err = fmt.Errorf("%q is not an absolute URI", value)
		}
	}
	if err != nil {
		return fmt.Errorf("%s does not satisfy format %q: %w", path, format, err)
	}
	return nil
}
