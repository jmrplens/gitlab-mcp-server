// Command audit_graphql_documents fails when a raw GraphQL document in this
// repository is one the pinned GitLab schema refuses.
//
// The reading and the judging both live in cmd/internal/graphqldocs, which
// documents what the check can and cannot see. This command is the standalone
// gate over it: it renders the result as text, names the file and line of every
// refused document, and exits non-zero when there is one. The same result is
// one third of the R-PATH dimension of cmd/audit_1to1, where a document GitLab
// refuses is one of the three ways a registered action cannot reach the
// endpoint it names.
//
// Two commands rather than one because this gate answers a question a reader
// asks on its own ("does every document I ship still parse against the pin"),
// runs in a second, and predates the dimension that absorbed it. Keeping the
// name working keeps `make check-graphql-documents` and the CI step that calls
// it pointing at the same thing.
//
// # The live re-probe
//
// -live introspects an instance right now and judges the documents against the
// schema it serves. The pin can only report a document that was already broken
// on the day it was taken; this reports one GitLab has narrowed since, which is
// how every defect this gate was built for arose. The same run names every
// type, field and argument the pin and that instance disagree about under our
// own selection sets, so the pin's age is a number somebody sees. -schema is
// the same against an SDL file already on disk.
//
// Usage:
//
//	go run ./cmd/audit_graphql_documents/
//	go run ./cmd/audit_graphql_documents/ -v
//	go run ./cmd/audit_graphql_documents/ -live https://gitlab.com/api/graphql
//	go run ./cmd/audit_graphql_documents/ -schema /tmp/live/gitlab-schema.graphql
package main
