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
// and each run in name order, and the alternative sets its anyOf requires at
// least one of. tiers is the lowest tier at which each parameter is served.
func parameters(schema map[string]any, tiers map[string]edition.Tier) (params []param, oneOf [][]string) {
	required := map[string]bool{}
	for _, name := range stringList(schema["required"]) {
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
	alternatives, _ := schema["anyOf"].([]any)
	for _, raw := range alternatives {
		alternative, _ := raw.(map[string]any)
		oneOf = append(oneOf, stringList(alternative["required"]))
	}
	return params, oneOf
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
// surface serves it: the individual tool's description, or the action's usage
// line when it has none, with the names of its "See also" clause rewritten
// into canonical IDs and the names nothing resolves dropped, a clause left
// empty removed whole.
func servedDescription(action actioncatalog.Action, index map[string]string) string {
	description := action.IndividualTool.Description
	if description == "" {
		description = action.Usage
	}
	rewritten := actioncatalog.SeeAlsoClause.ReplaceAllStringFunc(description, func(clause string) string {
		var kept []string
		for _, name := range seeAlsoNames(clause) {
			if id, ok := index[name]; ok {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			return ""
		}
		return "See also: " + strings.Join(kept, ", ") + "."
	})
	return strings.TrimRight(rewritten, " \n")
}

// seeAlsoNames is the list of names a "See also" clause spells.
func seeAlsoNames(clause string) []string {
	return strings.Split(strings.TrimSuffix(strings.TrimPrefix(clause, "See also: "), "."), ", ")
}
