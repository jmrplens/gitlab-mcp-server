package actioncatalog

import (
	"slices"
	"strings"
)

// RequiredParamNames reads a JSON Schema "required" list. The list is a []any
// when the schema was decoded from JSON and a []string where an override wrote
// it in Go (toolutil.SchemaAnyOfRequired), and in either only a non-empty
// string names a parameter. Anything else names nothing. The answer is a new
// slice, so a caller may reorder it without reordering the schema.
func RequiredParamNames(raw any) []string {
	var names []string
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, isString := value.(string); isString && name != "" {
				names = append(names, name)
			}
		}
	case []string:
		for _, name := range values {
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// RequiredParams returns the parameters every call of an action needs: its
// input schema's root "required" list, sorted and without duplicates. A name
// an anyOf or oneOf branch requires is not one of them, since a call needs
// only one branch; [RequiredParamAlternatives] answers for those.
//
// These two functions are the reading of what an action requires that the
// gitlab://tools manifest, the dynamic surface and the site's tool reference
// share, because their readers have to agree on it and disagreeing fails in a
// model's first call: the manifest publishes it, find, describe and search
// publish it and build their example call from it, execute refuses a call
// that does not meet it, and the reference states it per action. find once
// folded every branch into its required list, so it told a model that
// security_attribute.update needed a name, a description and a color at once
// while the manifest said one of them (issue 1175). The meta surface's check
// of the root list is toolutil's own, since this package imports toolutil and
// cannot be imported back; it reads the same root list and no alternatives.
func RequiredParams(schema map[string]any) []string {
	names := RequiredParamNames(schema["required"])
	slices.Sort(names)
	return slices.Compact(names)
}

// RequiredParamAlternatives returns the alternative requirement groups of an
// action's input schema, of which a call needs every name of one group, not
// every group, and of exactly one where [RequiredParamAlternativesExclusive]
// says so. A schema with one of anyOf and oneOf at its root gives one
// group per branch, holding the names that branch requires beyond the root
// "required" list, in the order the branch declares them. A branch that is
// not an object is skipped.
//
// A schema with both asks for a branch of each, so each group is one
// combination: an anyOf group joined with a oneOf group, the anyOf ones
// varying slowest. No catalog action declares both today; listing the two
// keywords' groups side by side would have told a caller that one branch of
// either was enough.
//
// A keyword with a branch the root list already satisfies, or with a branch
// that requires nothing, asks nothing of a call, since every call satisfies
// that branch, and so constrains no group. Dropping that branch alone and
// keeping the others would publish as needed a name no call has to send.
func RequiredParamAlternatives(schema map[string]any) [][]string {
	root := RequiredParamNames(schema["required"])
	var groups [][]string
	for _, keyword := range []string{"anyOf", "oneOf"} {
		branches, _ := schema[keyword].([]any)
		groups = combineAlternatives(groups, keywordAlternatives(branches, root))
	}
	return groups
}

// RequiredParamAlternativesExclusive reports whether a call completes exactly
// one of the groups [RequiredParamAlternatives] returns rather than at least
// one: the groups come from a root oneOf, GitLab's exactly_one_of as
// toolutil.SchemaOneOfRequired writes it, which refuses a call completing two
// as it refuses one completing none, and from no anyOf beside it.
//
// An anyOf beside the oneOf makes each group a combination of the two, and a
// call may complete two such groups through two anyOf branches, so those
// groups are not exclusive and the answer is false. A keyword with nothing to
// ask contributes no group, so it neither makes the groups exclusive nor
// stops them being so.
func RequiredParamAlternativesExclusive(schema map[string]any) bool {
	root := RequiredParamNames(schema["required"])
	anyOf, _ := schema["anyOf"].([]any)
	oneOf, _ := schema["oneOf"].([]any)
	return len(keywordAlternatives(oneOf, root)) > 0 && len(keywordAlternatives(anyOf, root)) == 0
}

// UnmetAlternatives returns the alternative groups of an action's input
// schema, as [RequiredParamAlternatives] reads them, when params complete
// none of them, and nil when params complete one or the schema has none. A
// call completes a group when it carries every name of it. Every group is
// returned, not the nearest: a refusal names the choice find published, and a
// call that sent part of one group still has all of them open to it.
//
// It is what the two dispatchers that refuse such a call before its handler
// runs read: the dynamic execute, for every action, and the individual tool,
// for a destructive action, before it asks for the call to be confirmed. It
// counts a name by its presence, so it refuses no call that completes two
// groups of a oneOf: a branch of toolutil.SchemaOneOfRequired counts a name by
// its value, and a common name beside an empty filter is a call that branch
// serves. The handler refuses that call and every other this one lets
// through.
func UnmetAlternatives(schema, params map[string]any) [][]string {
	groups := RequiredParamAlternatives(schema)
	for _, group := range groups {
		if carriesEvery(params, group) {
			return nil
		}
	}
	return groups
}

// carriesEvery reports whether params carries every one of names.
func carriesEvery(params map[string]any, names []string) bool {
	for _, name := range names {
		if _, carried := params[name]; !carried {
			return false
		}
	}
	return true
}

// AlternativesPhrase renders alternative requirement groups as "one of a, b
// or c", or "exactly one of a, b or c" when they are exclusive, a group of
// several names joined with " + ", each name written by span. A single group
// is no choice, since a call needs all of it, so it is written as the group
// alone. It is empty when there are no groups. find, search and describe
// write the groups a result carries with it, and the dispatchers that refuse
// a call completing none of them name the choice with it.
func AlternativesPhrase(groups [][]string, exclusive bool, span func(string) string) string {
	rendered := make([]string, 0, len(groups))
	for _, group := range groups {
		names := make([]string, 0, len(group))
		for _, name := range group {
			names = append(names, span(name))
		}
		rendered = append(rendered, strings.Join(names, " + "))
	}
	last := len(rendered) - 1
	if last < 0 {
		return ""
	}
	if last == 0 {
		return rendered[0]
	}
	choice := "one of "
	if exclusive {
		choice = "exactly one of "
	}
	return choice + strings.Join(rendered[:last], ", ") + " or " + rendered[last]
}

// combineAlternatives returns the groups that meet the alternatives of two
// keywords at once: every group of first joined with every group of second,
// each name once. A keyword with no groups asks nothing, so the other one's
// groups answer alone.
func combineAlternatives(first, second [][]string) [][]string {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}
	var combined [][]string
	for _, left := range first {
		for _, right := range second {
			group := slices.Clone(left)
			for _, name := range right {
				if !slices.Contains(group, name) {
					group = append(group, name)
				}
			}
			combined = append(combined, group)
		}
	}
	return combined
}

// keywordAlternatives returns the groups one keyword's branches ask of a call
// beyond root, or none when one of them asks nothing beyond it.
func keywordAlternatives(branches []any, root []string) [][]string {
	var groups [][]string
	for _, raw := range branches {
		branch, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		var group []string
		for _, name := range RequiredParamNames(branch["required"]) {
			if !slices.Contains(root, name) {
				group = append(group, name)
			}
		}
		if len(group) == 0 {
			return nil
		}
		groups = append(groups, group)
	}
	return groups
}
