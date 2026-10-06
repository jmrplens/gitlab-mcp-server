// Command gen_action_grants derives what every catalog action sends GitLab
// from its handlers and joins each request to what the live GitLab record
// says a fine-grained personal access token needs for it.
//
// The derivation reads the handlers the catalog names through
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests] and
// what each client-go method sends through
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes], and
// reads the statements of each handler as sequences, alternatives and
// optional requests, keeping them in the order the handler makes them. A
// //gitlab:request directive beside a request says what the syntax cannot,
// and a declaration in request_declarations.go answers a request the walk
// reaches and cannot read, or one GitLab answers with a redirect to another
// of its routes, which the client follows with the same token. The join matches each REST request to a route of
// docs/development/gitlab-api-live.json and judges each GraphQL document
// position by position against the authorization recorded for it, and a
// declaration in grant_declarations.go answers what the record cannot place.
//
// The same requests decide what a classic personal access token or an OAuth
// token needs: read_api for a REST GET or HEAD and a GraphQL query, api for
// anything else, with the routes GitLab grants otherwise declared in
// classic_declarations.go. Each action needs what the least demanding of its
// ways needs, and gates 4 and 5 ([classicFindings]) hold that value to the
// action's ways and to its read or write classification.
//
// It writes three artifacts: docs/development/action-requests.json, what
// each action sends; internal/tools/actiongrants/table_gen.go, the table the
// server reads, written as keyed literals of constants so that building it
// costs the server nothing at startup; and the same table for a person
// minting a token, a page of the documentation site in each of its
// languages: site/src/content/docs/reference/fine-grained-permissions.mdx
// and its Spanish twin under site/src/content/docs/es/. -check compares the
// three with what the tree derives now, both pages of the third included,
// and -check-derivation fails on every finding and on the five gates of
// [gateFindings] and [classicFindings] without writing anything.
//
// The command loads and builds itself with table_gen.go replaced by
// table_stub.go.txt through the go command's -overlay, which the Makefile
// targets pass, so a table a change to the finegrained types no longer
// compiles against cannot stop its own regeneration.
//
// Usage:
//
//	make gen-action-grants
//	make check-action-grants
//	make check-action-grants-derivation
package main
