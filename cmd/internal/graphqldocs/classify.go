package graphqldocs

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ignoredText is what may sit between two parts of a definition without
// changing what the definition is: whitespace, and a comment, which runs to the
// end of its line. The name is the specification's, which calls this class
// Ignored.
//
// It is spelled once because leaving it out anywhere narrows the rule against
// legal GraphQL rather than against prose. A document may write a directive
// straight after the operation type, and may carry a comment between the
// keyword and the selection set, between the operation name and its variable
// definitions, or between a fragment's name and its "on". Each of those parses,
// and a rule that admitted only a space would drop them out of the inventory
// with nothing reporting the loss, which is the narrowing this file exists to
// end.
//
// The specification counts a comma as Ignored too and this does not, which is
// deliberate: no document writes "mutation , {", so admitting it would buy
// nothing, and a comma is the one member of the class that reads naturally in
// English prose, where every extra thing this rule accepts is a false document
// in the inventory.
const ignoredText = `(?:\s|#[^\n]*)`

// definitionOf builds the rule that recognizes an operation definition written
// with one of the given keywords.
//
// What follows the keyword is what separates a definition from English prose:
// the selection set the operation opens with, its variable definitions, a
// directive, or an operation name followed by one of those three. "mutation
// errors: %s" therefore defines nothing, since a name followed by a colon is
// none of them, and neither does an identifier such as queryBuilder, whose
// keyword is not a word of its own.
//
// The keyword is looked for at the start of any line rather than at the start
// of the document, because the operation is not always the first thing in the
// string: a document assembled around a template hole or a format verb carries
// the hole in front of it, and so does one written under a header line that is
// not a "#" comment. Anchoring at the start of the document is what used to let
// a document in either shape leave the inventory and be judged by nothing.
//
// What sits between the keyword and what follows it may be a newline, and may
// be a comment: see [ignoredText]. A document is free to put its selection set
// on the line below its keyword and GitLab reads it the same way, so a rule
// that demanded a space would refuse a document GitLab accepts.
func definitionOf(keywords string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*(?:` + keywords + `)\b` + ignoredText + `*(?:[({@]|[A-Za-z_][A-Za-z0-9_]*` + ignoredText + `*[({@])`)
}

// mutationDefinition matches the one operation type that changes server state.
var mutationDefinition = definitionOf("mutation")

// readDefinition matches the operation types that do not.
var readDefinition = definitionOf("query|subscription")

// fragmentDefinition matches a fragment definition, which declares no operation
// of its own and is still a document: this repository writes fragments as
// constants and splices them into the operations that use them. The "on" is
// what separates one from prose that opens with the word fragment.
var fragmentDefinition = regexp.MustCompile(`(?m)^[ \t]*fragment\b` + ignoredText + `+[A-Za-z_][A-Za-z0-9_]*` + ignoredText + `+on\b`)

// objectLiteral matches an opening brace whose first entry binds a name to a
// quoted string, which is a map written in JSON-ish shorthand and never
// GraphQL: the grammar allows a name after "alias:", never a string literal.
// The repository holds a few, such as the example bindings in the issues
// discovery metadata, and without this they read as broken documents.
var objectLiteral = regexp.MustCompile(`^\{\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*["']`)

// LooksLikeDocument reports whether a constant string is a GraphQL document
// rather than ordinary Go text that happens to contain braces.
//
// This is the one definition of what counts as a document, and every caller
// asks it rather than carrying a rule of its own. The inventory is built on it,
// so a second rule anywhere is a second inventory: cmd/audit_readonly_graphql
// used to classify the text it read with a rule that recognized shapes this one
// refused, which left it printing "no read-only action reaches a mutation"
// while having read fewer documents than its own rule describes. A gate that
// narrows silently is the failure this package exists to remove, so the
// read-against-write question that audit does own now starts here, in
// [DefinesMutation].
//
// A pre-filter is needed because every constant string in the repository passes
// through here and handing gqlparser an error message would report a parse
// failure about a string nobody ever sends. The filter is deliberately crude
// and deliberately generous: it asks for braces and an operation definition or
// a selection set, and everything that gets past it is judged by the schema, so
// a false positive costs a reviewable finding rather than a silent skip.
func LooksLikeDocument(value string) bool {
	trimmed := skipComments(value)
	if !strings.Contains(trimmed, "{") || !strings.Contains(trimmed, "}") {
		return false
	}
	return opensBareSelectionSet(trimmed) || definesOperation(trimmed)
}

// opensBareSelectionSet reports whether the document is an anonymous query
// written as a selection set with no keyword in front of it.
func opensBareSelectionSet(trimmed string) bool {
	rest, found := strings.CutPrefix(trimmed, "{")
	return found &&
		opensSelectionSet(rest) &&
		!objectLiteral.MatchString(trimmed) &&
		!isUnitAnnotation(trimmed)
}

// definesOperation reports whether any line of the document opens a definition.
//
// A string that opens with a brace and is not a selection set still reaches
// here, because the first line is not a veto: a document assembled around a
// template action opens with the action's own braces and carries its operation
// below them.
func definesOperation(trimmed string) bool {
	return mutationDefinition.MatchString(trimmed) ||
		readDefinition.MatchString(trimmed) ||
		fragmentDefinition.MatchString(trimmed)
}

// DefinesMutation reports whether a document carries a mutation operation,
// which is the half of the question cmd/audit_readonly_graphql asks of every
// document it reads.
//
// It lives here rather than there because the two answers have to be made of
// the same rule. What a mutation looks like is part of what a document looks
// like, and the moment the two are written twice they drift: a shape
// [LooksLikeDocument] admits and a separate mutation rule does not match is a
// mutation classified as a read, which is exactly the silence that audit gates
// against. What stays that audit's own is what the answer means, which is that
// an action classified ReadOnly must not be able to reach one.
//
// The comments are not skipped first: the keyword has to open a line, and a
// comment line opens with "#".
func DefinesMutation(document string) bool {
	return mutationDefinition.MatchString(document)
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
// while a UCUM annotation never is.
var unitAnnotation = regexp.MustCompile(`^\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// isUnitAnnotation reports whether a one-word brace is a unit rather than a
// selection set of one field.
//
// The reserved prefix is what rescues the spaceless document the whitespace
// rule would otherwise lose: GraphQL reserves names beginning with "__" for
// introspection, so `{__typename}` is a document in any instrument's units and
// a unit in none. A one-word selection of an ordinary field, `{id}`, stays out,
// because nothing in the text tells it from a unit; what that costs is bounded,
// since a bare selection set is a read by construction and no mutation can be
// written without its keyword.
func isUnitAnnotation(trimmed string) bool {
	return unitAnnotation.MatchString(trimmed) && !strings.HasPrefix(trimmed, "{__")
}
