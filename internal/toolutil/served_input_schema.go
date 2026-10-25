package toolutil

import (
	"slices"
)

// rootCombinatorKeywords are the keywords a tool's served input schema never
// carries at its root: the combinators and the conditionals.
//
// The Anthropic Messages API refuses a tool whose input_schema has oneOf,
// allOf or anyOf at the top level, with a 400 that names the tool by its index
// in the tools array ("tools.N.custom.input_schema: input_schema does not
// support oneOf, allOf, or anyOf at the top level"), so a request carrying one
// such tool fails as a whole, whatever the model was asked, and a client that
// sends its whole tool list with every request can make no call at all. Azure
// OpenAI and OpenAI's strict mode refuse a root anyOf as well, and the strict
// subsets those providers and Gemini accept have no not, if, then or else at
// all. A client may pass the schemas it lists to that API as they are, so the
// rule is the server's to keep, on every surface.
//
// The rules these keywords stated stay where they are read as data: in the
// catalog's input schema of the action, which find's required_params_any_of
// and required_params_exclusive, the gitlab://tools manifest and the site's
// tool reference read. What enforces them is the handler, before it sends
// anything, which is the one place every surface reaches.
var rootCombinatorKeywords = []string{"allOf", "anyOf", "oneOf", "not", "if", "then", "else"}

// RootCombinators returns, in the order [rootCombinatorKeywords] lists them,
// the combinators and conditionals schema carries at its root, and nil when
// it carries none. A schema that is not a JSON object carries none. It reads
// a map as it is and any other value, such as a compiled schema, through its
// JSON form, so it judges what a client is sent.
func RootCombinators(schema any) []string {
	root, ok := schema.(map[string]any)
	if !ok {
		root = schemaMapCopy(schema)
	}
	var found []string
	for _, keyword := range rootCombinatorKeywords {
		if _, present := root[keyword]; present {
			found = append(found, keyword)
		}
	}
	return found
}

// dropRootCombinators deletes from schema, which the caller owns, every
// keyword of [rootCombinatorKeywords] at its root, and leaves every nested
// one: a combinator inside a property or an item is no root of anything a
// client refuses.
func dropRootCombinators(schema map[string]any) {
	for keyword := range schema {
		if slices.Contains(rootCombinatorKeywords, keyword) {
			delete(schema, keyword)
		}
	}
}
