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
// What it cannot do is check variables: a document read out of the source has
// no request behind it, so nothing says which variables a handler will send or
// what they will hold. That half belongs to the test transport, which sees a
// real request.
//
// Usage:
//
//	go run ./cmd/audit_graphql_documents/
//	go run ./cmd/audit_graphql_documents/ -v
package main
