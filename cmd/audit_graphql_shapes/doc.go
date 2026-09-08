// Command audit_graphql_shapes fails when a struct this repository decodes a
// GraphQL response into cannot hold what the document it is sent with asks
// for.
//
// Every other GraphQL gate reads the request. The pinned schema judges the
// document (make check-graphql-documents) and the test transport judges the
// variables, and neither looks at the struct the answer is decoded into,
// because the fixture a test answers with is written to match that struct. So
// a decoder that disagrees with GitLab is invisible to all of them: the
// licensed run found startLine and endLine declared as String by the schema
// and decoded into int by two response structs, every located finding failed
// to decode, and every gate was green.
//
// This command pairs the two halves. It loads the program with go/packages,
// finds every call whose first argument is a GraphQLQuery, folds the document
// that call sends, resolves what the pointer it decodes into points at, and
// walks the validated selection set against that type through encoding/json's
// own rules: a field is matched by its json tag or, failing that, its name
// case-insensitively, embedded structs are flattened, a type that unmarshals
// itself is trusted, and a type parameter is read as whatever the caller bound
// it to.
//
// Three disagreements fail the gate: a field whose Go kind cannot hold what
// the schema says GitLab sends (a String decoded into an int, an object into a
// string, a list into a struct), a Go field the document never selects and so
// is always empty, and a scalar this audit has no serialization for. A field
// the document selects and no Go field reads is reported and does not fail:
// it is transfer, not truth. A document that reaches its call through a
// wrapper's parameter is paired at every call of that wrapper, where the
// document is named; a document built at run time, or one the sibling audit
// finds that no call this audit can see sends, is a failure rather than a
// silence, because a document nobody judges is the shape this gate exists to
// refuse.
//
// Usage:
//
//	go run ./cmd/audit_graphql_shapes/
//	go run ./cmd/audit_graphql_shapes/ -v
//	go run ./cmd/audit_graphql_shapes/ -schema /tmp/live/gitlab-schema.graphql
package main
