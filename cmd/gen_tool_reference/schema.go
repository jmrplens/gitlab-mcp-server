package main

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// requiredSuffix is the jsonschema tag remnant a reflected description can
// still carry. Every surface's listing trims it before a client sees the
// description (toolutil's schema lockdown), so a page trims it too.
const requiredSuffix = ",required"

// requiredRank places a required parameter before an optional one.
var requiredRank = map[bool]int{true: 0, false: 1}

// param is one parameter of an action's input schema.
type param struct {
	name        string
	typ         string
	required    bool
	tier        edition.Tier
	description string
}

// properties is the properties object of an input schema, or nil.
func properties(schema map[string]any) map[string]any {
	props, _ := schema["properties"].(map[string]any)
	return props
}

// stringList reads a JSON Schema list of names, which a reflected schema
// holds as []string and a decoded one as []any.
func stringList(value any) []string {
	if names, ok := value.([]string); ok {
		return names
	}
	items, _ := value.([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprint(item))
	}
	return names
}

// parameters reads the parameters of an input schema, the required ones first
// and each run in name order, and the alternative sets of which a call needs
// one beside them. Both are actioncatalog's reading, the one
// gitlab_find_action and gitlab://tools publish, so the page states the
// requirement a model is told at run time. tiers is the lowest tier at which
// each parameter is served.
func parameters(schema map[string]any, tiers map[string]edition.Tier) (params []param, oneOf [][]string) {
	required := map[string]bool{}
	for _, name := range actioncatalog.RequiredParams(schema) {
		required[name] = true
	}
	for name, raw := range properties(schema) {
		property, _ := raw.(map[string]any)
		description, _ := property["description"].(string)
		params = append(params, param{
			name:        name,
			typ:         schemaType(property),
			required:    required[name],
			tier:        tiers[name],
			description: strings.TrimSuffix(description, requiredSuffix),
		})
	}
	slices.SortFunc(params, func(a, b param) int {
		return cmp.Or(cmp.Compare(requiredRank[a.required], requiredRank[b.required]), cmp.Compare(a.name, b.name))
	})
	return params, actioncatalog.RequiredParamAlternatives(schema)
}

// schemaType renders the type of a property as Markdown: its JSON Schema type
// with null left out, an array as its item type followed by [], alternatives
// joined by a slash, and an enumeration's values after it, each a code span.
// Every part of it is a JSON Schema keyword or a value, so it reads the same
// in both languages.
func schemaType(property map[string]any) string {
	typ := typeName(property)
	values := property["enum"]
	if typ == "array" {
		items, _ := property["items"].(map[string]any)
		typ = typeName(items) + "[]"
		values = items["enum"]
	}
	enum, _ := values.([]any)
	if len(enum) == 0 {
		return "`" + typ + "`"
	}
	quoted := make([]string, 0, len(enum))
	for _, value := range enum {
		quoted = append(quoted, "`"+fmt.Sprint(value)+"`")
	}
	return "`" + typ + "` (" + strings.Join(quoted, ", ") + ")"
}

// typeName is the non-null type or types a schema names, from its type
// keyword or from the members of its oneOf or anyOf, and "any" when it names
// none.
func typeName(schema map[string]any) string {
	var names []string
	for _, name := range typeList(schema["type"]) {
		if name != "null" {
			names = append(names, name)
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		members, _ := schema[key].([]any)
		for _, raw := range members {
			member, _ := raw.(map[string]any)
			names = append(names, typeList(member["type"])...)
		}
	}
	if len(names) == 0 {
		return "any"
	}
	return strings.Join(names, "/")
}

// typeList reads a type keyword, which is one name or a list of them.
func typeList(value any) []string {
	if name, ok := value.(string); ok {
		return []string{name}
	}
	return stringList(value)
}

// seeAlsoIndex maps every name a "See also" clause may spell, an individual
// tool name or a canonical ID, to the canonical ID it stands for. It is the
// index gitlab://tools resolves the clause with on the dynamic surface: IDs in
// order, so a tool two actions share resolves to the later one, as it does
// there.
func seeAlsoIndex(actions map[string]*refAction) map[string]string {
	ids := make([]string, 0, len(actions))
	for id := range actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	index := make(map[string]string)
	for _, id := range ids {
		index[id] = id
		if name := actions[id].latest.IndividualTool.Name; name != "" {
			index[name] = id
		}
	}
	return index
}

// servedDescription is an action's description as the default dynamic
// surface serves it, through the one definition gitlab://tools serves it by
// ([actioncatalog.ServedDescription]): the individual tool's description with
// the names of its "See also" clause rewritten into canonical IDs and the
// names nothing resolves dropped, or the action's usage line as it is when it
// has no description. The names resolve through index, built over every build
// the reference covers, so a clause naming an action of a higher tier keeps
// it, as the dynamic surface of an instance at that tier does.
func servedDescription(action actioncatalog.Action, index map[string]string) string {
	return actioncatalog.ServedDescription(action, func(name string) (string, bool) {
		id, ok := index[name]
		return id, ok
	})
}
