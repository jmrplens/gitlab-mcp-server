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
	trimmed := skipComments(value)
	if !strings.Contains(trimmed, "{") || !strings.Contains(trimmed, "}") {
		return false
	}
	if unitAnnotation.MatchString(trimmed) {
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

// skipComments trims leading whitespace and any GraphQL comment lines in front
// of a document.
//
// A "#" comment is legal at the top of a document and gqlparser accepts one, so
// without this a maintainer who writes a perfectly ordinary explanatory line
// above an operation drops that document out of the inventory. The audit would
// then report one fewer document and exit 0, which is the silence this whole
// command exists to remove.
func skipComments(value string) string {
	trimmed := strings.TrimSpace(value)
	for strings.HasPrefix(trimmed, "#") {
		_, rest, found := strings.Cut(trimmed, "\n")
		if !found {
			return ""
		}
		trimmed = strings.TrimSpace(rest)
	}
	return trimmed
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

// unitAnnotation matches one word wrapped in braces with no space anywhere, the
// shape OpenTelemetry writes a unit in.
//
// `metric.WithUnit("{entry}")` and its two neighbors were read as anonymous
// queries and reported as broken documents in a package that sends no GraphQL at
// all. The discriminator is the whitespace rather than the single field, because
// `{ __typename }` is a legal document of one field and is written with spaces,
// while a UCUM annotation never is. A document written as `{__typename}` would
// now be skipped, which trades a false alarm on every metric unit for a silence
// on a shape this repository does not use and gqlparser would still judge
// through any test that drives it.
var unitAnnotation = regexp.MustCompile(`^\{[A-Za-z_][A-Za-z0-9_]*\}$`)
