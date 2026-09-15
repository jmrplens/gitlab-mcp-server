package main

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
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

// classifyDocument reports what a string literal or string constant asks the
// GitLab GraphQL API to do.
//
// The HTTP method cannot answer this: client-go sends every GraphQL request as
// a POST, so around twenty read-only actions legitimately POST and the verb
// separates nothing. The operation type in the document is the whole of the
// distinction, and it is visible in the source.
//
// Whether a string is a document at all is not decided here. This audit reads
// the shared inventory, and it used to re-read the text with a rule of its own
// that recognized shapes the inventory's did not, which is a narrowing it could
// not see: a document the inventory refuses never reaches this function, so the
// gate went on reporting that no read-only action reaches a mutation while
// having read fewer documents than its own rule described. Both questions are
// answered by [graphqldocs] now, from one rule, and what stays this audit's own
// is what the answer means.
//
// A bare selection set is an anonymous query. GitLab accepts one, and this
// project sends a few. It cannot express a mutation: the mutation keyword is
// mandatory for the write operation type, so a document that carries no
// mutation definition is a read by construction.
func classifyDocument(s string) documentKind {
	if !graphqldocs.LooksLikeDocument(s) {
		return notADocument
	}
	if graphqldocs.DefinesMutation(s) {
		return writeDocument
	}
	return readDocument
}
