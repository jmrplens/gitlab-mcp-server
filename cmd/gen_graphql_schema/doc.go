// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
// -release pins a second schema beside the first: the schema of the GitLab
// release the REST record (docs/development/gitlab-api-live.json) is taken
// from, read from the dump GitLab's own CI keeps for the release's tag, or
// from a dump on disk with -from, and rendered by the same renderer, so the two
// pins differ only where GitLab does. A dump names no release of its own, so a
// pin read with -from records that it came from a file, and the release and
// revision it records are the REST record's, taken on its word rather than
// checked against the file. -from is refused beside --check, which reads only
// the committed pair and would report it sound with the file unread.
// cmd/audit_graphql_documents judges every
// document against it as well, because a field gitlab.com's pre-release adds
// is refused by every release. --check -release holds it to the REST record's
// release and revision instead of to an age, since a release's schema does not
// change once it is cut and which release is current is the REST record's to
// say, and holds the schema to the SHA-256 its record carries, so a hand edit
// or a half-written pair fails even where the type count agrees.
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
//	go run ./cmd/gen_graphql_schema/ -release
//	go run ./cmd/gen_graphql_schema/ -release -from tmp/tests/graphql/gitlab_schema.graphql
//	go run ./cmd/gen_graphql_schema/ --check -release
package main
