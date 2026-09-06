package main

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// operationKeywords open a GraphQL document. A document may also open with a
// bare selection set, which is an anonymous query.
var operationKeywords = []string{"query", "mutation", "subscription", "fragment"}

// objectLiteral matches an opening brace whose first entry binds a name to a
// quoted string, which is a map written in JSON-ish shorthand and never
// GraphQL: the grammar allows a name after "alias:", never a string literal.
// The repository holds a few, such as the example bindings in the issues
// discovery metadata, and without this they read as broken documents.
var objectLiteral = regexp.MustCompile(`^\{\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*["']`)

// looksLikeDocument reports whether a constant string is a GraphQL document
// rather than ordinary Go text that happens to contain braces.
//
// A pre-filter is needed because every constant string in the repository
// passes through here and handing gqlparser an error message would report a
// parse failure about a string nobody ever sends. The filter is deliberately
// crude and deliberately generous: it asks for braces and an opening keyword,
// and everything that gets past it is judged by the schema, so a false
// positive costs a reviewable finding rather than a silent skip.
//
// The keyword is required at the very start because that is where every
// document in this repository has it, including the four assembled from a
// shared fragment: the fragment is spliced into the middle of a selection set,
// never in front of the operation.
func looksLikeDocument(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.Contains(trimmed, "{") || !strings.Contains(trimmed, "}") {
		return false
	}
	if rest, found := strings.CutPrefix(trimmed, "{"); found {
		return opensSelectionSet(rest) && !objectLiteral.MatchString(trimmed)
	}
	for _, keyword := range operationKeywords {
		rest, found := strings.CutPrefix(trimmed, keyword)
		if !found {
			continue
		}
		next, _ := utf8.DecodeRuneInString(rest)
		if next == '(' || next == '{' || unicode.IsSpace(next) {
			return true
		}
	}
	return false
}

// opensSelectionSet reports whether what follows an opening brace is a GraphQL
// selection rather than the body of a JSON object.
//
// A bare selection set is an anonymous query and GitLab accepts one, so the
// opening brace has to be allowed, and a JSON literal opens with the same
// character. They part company immediately after it: a selection names a field
// or spreads a fragment, while a JSON object holds a quoted key or nothing at
// all. Without this the repository's json.RawMessage constants and its mocked
// API responses are all read as broken documents.
func opensSelectionSet(rest string) bool {
	next, _ := utf8.DecodeRuneInString(strings.TrimSpace(rest))
	return next == '_' || next == '.' || unicode.IsLetter(next)
}
