package main

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func TestProperties_SchemaShapes_ReturnTheObjectOrNil(t *testing.T) {
	props := map[string]any{"a": map[string]any{}}
	if got := properties(map[string]any{"properties": props}); len(got) != 1 {
		t.Errorf("properties() = %v, want the properties object", got)
	}
	if got := properties(map[string]any{"properties": "x"}); got != nil {
		t.Errorf("properties() of a non-object = %v, want nil", got)
	}
	if got := properties(nil); got != nil {
		t.Errorf("properties(nil) = %v, want nil", got)
	}
}

func TestStringList_EveryEncoding_ReadsTheNames(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{name: "reflected", value: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "decoded", value: []any{"a", "b"}, want: []string{"a", "b"}},
		{name: "absent", value: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringList(tt.value); !slices.Equal(got, tt.want) {
				t.Errorf("stringList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchemaType_EverySchemaShape_RendersItsType(t *testing.T) {
	tests := []struct {
		name     string
		property map[string]any
		want     string
	}{
		{name: "plain", property: map[string]any{"type": "string"}, want: "`string`"},
		{name: "nullable", property: map[string]any{"type": []any{"null", "boolean"}}, want: "`boolean`"},
		{name: "union", property: map[string]any{"type": []any{"string", "integer"}}, want: "`string/integer`"},
		{name: "one of", property: map[string]any{"oneOf": []any{map[string]any{"type": "boolean"}, map[string]any{"type": "string"}}}, want: "`boolean/string`"},
		{name: "any of", property: map[string]any{"anyOf": []any{map[string]any{"type": "integer"}}}, want: "`integer`"},
		{name: "untyped", property: map[string]any{}, want: "`any`"},
		{name: "array", property: map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "integer"}}, want: "`integer[]`"},
		{name: "array of untyped", property: map[string]any{"type": "array"}, want: "`any[]`"},
		{name: "enum", property: map[string]any{"type": "string", "enum": []any{"asc", "desc"}}, want: "`string` (`asc`, `desc`)"},
		{name: "array enum", property: map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []any{"a"}}}, want: "`string[]` (`a`)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaType(tt.property); got != tt.want {
				t.Errorf("schemaType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParameters_Schema_RequiredFirstThenByName(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"zeta":  map[string]any{"type": "string", "description": "Zeta,required"},
			"alpha": map[string]any{"type": "string", "description": "Alpha"},
			"mid":   map[string]any{"type": "string"},
			"beta":  map[string]any{"type": "string"},
		},
		"required": []any{"zeta", "mid"},
		"anyOf":    []any{map[string]any{"required": []any{"alpha"}}, map[string]any{"required": []any{"beta", "zeta"}}},
	}
	params, oneOf := parameters(schema, map[string]edition.Tier{"beta": edition.Premium})
	var order []string
	for _, p := range params {
		order = append(order, p.name)
	}
	if !slices.Equal(order, []string{"mid", "zeta", "alpha", "beta"}) {
		t.Errorf("parameter order = %v, want the required ones first, each run by name", order)
	}
	if params[1].description != "Zeta" || !params[1].required || params[2].required {
		t.Errorf("zeta = %+v, alpha = %+v", params[1], params[2])
	}
	if params[3].tier != edition.Premium || params[0].tier != edition.Free {
		t.Errorf("tiers = %v, %v", params[3].tier, params[0].tier)
	}
	if len(oneOf) != 2 || !slices.Equal(oneOf[1], []string{"beta", "zeta"}) {
		t.Errorf("oneOf = %v", oneOf)
	}
}

func TestParameters_NoAlternatives_HasNoOneOf(t *testing.T) {
	_, oneOf := parameters(schemaOf(nil, "a"), nil)
	if oneOf != nil {
		t.Errorf("oneOf = %v, want nil", oneOf)
	}
}

func TestSeeAlsoIndex_SharedToolName_ResolvesToTheLaterID(t *testing.T) {
	actions := map[string]*refAction{
		"a.first":  {latest: actioncatalog.Action{IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_shared"}}},
		"b.second": {latest: actioncatalog.Action{IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_shared"}}},
		"c.none":   {},
	}
	index := seeAlsoIndex(actions)
	if index["gitlab_shared"] != "b.second" {
		t.Errorf("gitlab_shared resolves to %q, want b.second", index["gitlab_shared"])
	}
	if index["c.none"] != "c.none" || index["a.first"] != "a.first" {
		t.Errorf("IDs resolve to %q and %q, want themselves", index["c.none"], index["a.first"])
	}
	if _, ok := index[""]; ok {
		t.Error("an action without an individual tool indexed the empty name")
	}
}

func TestServedDescription_SeeAlsoClause_IsRewrittenAsTheDynamicSurfaceServesIt(t *testing.T) {
	index := map[string]string{"gitlab_widget_get": "widget.get", "widget.list": "widget.list"}
	tests := []struct {
		name   string
		action actioncatalog.Action
		want   string
	}{
		{name: "names rewritten", action: describedAs("Get it. See also: gitlab_widget_get, widget.list."), want: "Get it. See also: widget.get, widget.list."},
		{name: "unknown dropped", action: describedAs("Get it. See also: gitlab_widget_get, gitlab_gone."), want: "Get it. See also: widget.get."},
		{name: "empty clause removed", action: describedAs("Get it.\n\nSee also: gitlab_gone."), want: "Get it."},
		{name: "usage when undescribed", action: actioncatalog.Action{Usage: "Use it."}, want: "Use it."},
		{name: "usage served as it is", action: actioncatalog.Action{Usage: "Use it. See also: gitlab_widget_get, gitlab_gone."}, want: "Use it. See also: gitlab_widget_get, gitlab_gone."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := servedDescription(tt.action, index); got != tt.want {
				t.Errorf("servedDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// describedAs is an action whose individual tool is described as text.
func describedAs(text string) actioncatalog.Action {
	return actioncatalog.Action{IndividualTool: toolutil.IndividualToolSpec{Description: text}}
}
