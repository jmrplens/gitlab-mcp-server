package actioncatalog

import "slices"

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
// every group. A schema with one of anyOf and oneOf at its root gives one
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
