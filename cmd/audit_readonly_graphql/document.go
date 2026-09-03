package main

import (
	"regexp"
	"strings"
)

// documentKind is what one GraphQL document asks the server to do.
type documentKind int

const (
	// notADocument is a string that carries no GraphQL operation.
	notADocument documentKind = iota
	// readDocument is a query, a subscription, a bare selection set, or a
	// fragment: nothing in it changes server state.
	readDocument
	// writeDocument is a document carrying at least one mutation operation.
	writeDocument
)

// String names the kind for a report line.
func (k documentKind) String() string {
	switch k {
	case writeDocument:
		return "mutation"
	case readDocument:
		return "query"
	default:
		return "not-a-document"
	}
}

// mutationOperation matches a GraphQL mutation operation definition at the
// start of a line: "mutation(", "mutation {", "mutationName(", or
// "mutation Name {". The trailing punctuation is what separates an operation
// definition from English prose, which is why an error string such as
// "mutation errors: %s" does not match.
//
// The match is anchored per line rather than at the start of the string
// because documents in this repository are raw literals that open with a
// newline, are indented inside the literal, and are sometimes assembled from
// a shared fragment constant, so the operation keyword can sit on any line.
var mutationOperation = regexp.MustCompile(`(?m)^[ \t]*mutation[ \t]*(?:\(|\{|[A-Za-z_][A-Za-z0-9_]*[ \t]*[({])`)

// readOperation matches the read-side operation definitions in the same shape.
// A document is only classified at all if it looks like GraphQL, and this is
// what says so for the reads.
var readOperation = regexp.MustCompile(`(?m)^[ \t]*(?:query|subscription|fragment)[ \t]*(?:\(|\{|[A-Za-z_][A-Za-z0-9_]*\b)`)

// classifyDocument reports what a string literal or string constant asks the
// GitLab GraphQL API to do.
//
// The HTTP method cannot answer this: client-go sends every GraphQL request as
// a POST, so around twenty read-only actions legitimately POST and the verb
// separates nothing. The operation type in the document is the whole of the
// distinction, and it is visible in the source.
//
// A string has to look like a GraphQL document to be classified as one: it
// needs braces, and it needs an operation definition or a bare selection set.
// Everything else, including ordinary prose that happens to contain the word
// mutation, is notADocument.
func classifyDocument(s string) documentKind {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || !strings.Contains(trimmed, "{") || !strings.Contains(trimmed, "}") {
		return notADocument
	}
	if mutationOperation.MatchString(trimmed) {
		return writeDocument
	}
	if readOperation.MatchString(trimmed) {
		return readDocument
	}
	// A bare selection set is an anonymous query. GitLab accepts one, and this
	// project sends a few. It cannot express a mutation: the mutation keyword
	// is mandatory for the write operation type, so a document that omits the
	// keyword entirely is a read by construction.
	if strings.HasPrefix(trimmed, "{") {
		return readDocument
	}
	return notADocument
}
