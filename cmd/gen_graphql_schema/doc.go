// Command gen_graphql_schema pins a GitLab GraphQL schema into the repository.
//
// It introspects a live instance, converts the answer to SDL, and writes the
// compressed schema and a provenance record into internal/graphqlschema, where
// the test transport and cmd/audit_graphql_documents both read it. gitlab.com
// answers introspection to anyone, so no token is needed for the schema
// itself; a token is only read from GITLAB_TOKEN so the record can name the
// version the instance reports, which GitLab refuses to tell an anonymous
// caller.
//
// Generating needs the network, so it is not a CI gate. --check is: it loads
// the committed files from disk and fails when the schema does not parse or
// the record does not decode, which is what stops a truncated or half-written
// artifact from reaching a branch.
//
// Usage:
//
//	go run ./cmd/gen_graphql_schema/
//	go run ./cmd/gen_graphql_schema/ -url https://gitlab.example.com/api/graphql
//	go run ./cmd/gen_graphql_schema/ --check
package main
