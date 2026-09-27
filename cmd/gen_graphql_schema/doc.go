// Command gen_graphql_schema pins a GitLab GraphQL schema into the repository.
//
// It introspects a live instance, converts the answer to SDL, and writes the
// schema as text and a provenance record into internal/graphqlschema, where
// the test transport and cmd/audit_graphql_documents both read it. gitlab.com
// answers introspection to anyone, so no token is needed for the schema
// itself; a token is only read from GITLAB_TOKEN so the record can name the
// version the instance reports, which GitLab refuses to tell an anonymous
// caller. The token is sent only when GITLAB_URL names the instance being
// introspected, so a re-pin sets both, GITLAB_URL=https://gitlab.com beside a
// gitlab.com token; a token set alone is withheld, the version is recorded as
// unknown, and --check refuses the pin.
//
// The version it records is gitlab.com's, which is always the next minor's
// pre-release and so one minor ahead of the released image the REST record
// is taken from. --check refuses a pin of any other instance, so the two name
// one minor only once the REST record is taken from the release this pin
// anticipates; the package documentation of
// [github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema] says why
// the pin is gitlab.com's and what the gap costs.
//
// Generating needs the network, so it is not a CI gate. --check is: it loads
// the committed files from disk and fails when the schema does not parse or
// the record does not decode, which is what stops a truncated or half-written
// artifact from reaching a branch. It also refuses a pin that is too old, on
// the window and the verdict
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance] holds for
// every record this repository pins; what the pin is a pin of — the instance,
// the version, the type floor and the schema beside it — is asked here.
//
// Usage:
//
//	go run ./cmd/gen_graphql_schema/
//	go run ./cmd/gen_graphql_schema/ -url https://gitlab.example.com/api/graphql
//	go run ./cmd/gen_graphql_schema/ --check
package main
