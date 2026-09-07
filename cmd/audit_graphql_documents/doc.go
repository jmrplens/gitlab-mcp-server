// Command audit_graphql_documents fails when a raw GraphQL document in this
// repository is one the pinned GitLab schema refuses.
//
// The validating test transport catches a document the moment a test sends it,
// which covers most of them and cannot cover all of them: a document reachable
// by no test still ships, and a document reached only on an error branch is
// exercised by nobody. This audit reads them out of the source instead, so the
// coverage of the gate stops depending on the coverage of the tests.
//
// It loads the whole program with go/packages rather than matching the source
// with a regular expression, because four of this repository's documents are
// assembled by concatenating a shared fragment constant and only the type
// checker knows what the assembled value is. Constants are folded during type
// checking, so a document written as three pieces is indexed as the one string
// GitLab would receive.
//
// A document that lives in a .graphql file rather than a Go constant is read
// straight off disk, because an embedded variable is not a constant and folds to
// nothing, so moving a long document into its own file would otherwise drop it
// out of the inventory without a word.
//
// What it cannot do is check variables: a document read out of the source has
// no request behind it, so nothing says which variables a handler will send or
// what they will hold. That half belongs to the test transport, which sees a
// real request.
//
// # What it reads, and what it does not
//
// It reads ./internal/..., which holds every document this repository writes.
// It does not read client-go, which builds another 42 of its own for the
// achievements, work item, security attribute and terraform state services
// among others. Those reach GitLab through this server too, and the only thing
// judging them is the test transport, on whichever ones a test happens to
// drive. The summary line counts this repository's documents, not the server's
// whole GraphQL surface.
//
// Usage:
//
//	go run ./cmd/audit_graphql_documents/
//	go run ./cmd/audit_graphql_documents/ -v
//	go run ./cmd/audit_graphql_documents/ -schema /tmp/live/gitlab-schema.graphql
package main
