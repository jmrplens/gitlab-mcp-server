// Package graphqldocs reads every raw GraphQL document this repository writes
// out of the source and judges each one against a GitLab schema.
//
// The validating test transport catches a document the moment a test sends it,
// which covers most of them and cannot cover all of them: a document reachable
// by no test still ships, and a document reached only on an error branch is
// exercised by nobody. This package reads them out of the source instead, so
// the coverage of the check stops depending on the coverage of the tests.
//
// It loads the whole program with go/packages rather than matching the source
// with a regular expression, because four of this repository's documents are
// assembled by concatenating a shared fragment constant and only the type
// checker knows what the assembled value is. Constants are folded during type
// checking, so a document written as three pieces is judged as the one string
// GitLab would receive.
//
// A document that lives in a .graphql file rather than a Go constant is read
// straight off disk, because an embedded variable is not a constant and folds
// to nothing, so moving a long document into its own file would otherwise drop
// it out of the inventory without a word.
//
// What it cannot do is check variables: a document read out of the source has
// no request behind it, so nothing says which variables a handler will send or
// what they will hold. That half belongs to the test transport, which sees a
// real request, and to the recorded request inventory it writes.
//
// # What it reads, and what it does not
//
// It reads [DefaultPatterns], which holds every document this repository
// writes. It does not read client-go, which builds another 42 of its own for
// the achievements, work item, security attribute and terraform state services
// among others. Those reach GitLab through this server too, and the only thing
// judging them is the test transport, on whichever ones a test happens to
// drive. A count of documents from here is this repository's, not the server's
// whole GraphQL surface.
//
// # Who calls it
//
// Two callers, deliberately: cmd/audit_graphql_documents renders the result as
// the text of a standalone gate, and cmd/audit_1to1 folds the same result into
// the R-PATH dimension, where a document GitLab refuses is one of the three
// ways a registered action cannot reach the endpoint it names.
package graphqldocs
