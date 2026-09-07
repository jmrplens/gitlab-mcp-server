// Package testutil provides test helpers for gitlab-mcp-server.
//
// It wraps [net/http/httptest], the official
// [gitlab.com/gitlab-org/api/client-go] client, and shared assertion helpers
// so every domain test can stand up an isolated MCP server in a few lines.
// The package is consumed by every tool test under [internal/tools] and is
// the only sanctioned way to construct a [gitlabclient.Client] in tests.
//
// # Helper categories
//
//   - Client factory: [NewTestClient] spins up an [httptest.Server], wires it
//     into [gitlabclient.NewClient], and tears the server down on test exit.
//   - Response writers: [RespondJSON], [RespondJSONWithPagination],
//     [RespondGraphQL], and [RespondGraphQLError] keep response shapes
//     consistent across packages.
//   - Request assertions: [AssertRequestMethod], [AssertRequestPath], and
//     [AssertQueryParam] validate inbound HTTP calls in mock handlers.
//   - Context and logging: [CancelledCtx] returns a pre-cancelled context;
//     [CaptureSlog] captures [log/slog] output to a buffer for assertions.
//   - Embedded resources: [AssertEmbeddedResource] toggles the embedded
//     resource global flag and checks MCP call results.
//   - GraphQL helpers: [GraphQLHandler] and [ParseGraphQLVariables] simplify
//     mocking [POST /api/graphql] requests.
//
// # Typical usage
//
//	func TestListBranches(t *testing.T) {
//	    client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"main"}]`)
//	    }))
//	    // ... call the domain handler with client ...
//	}
//
// # Coverage
//
// This package does not reach the repository's 100% statement rule, and the
// residue is named here rather than left for the next contributor to
// rediscover: the [testing.T.Fatalf] branches of [AssertEmbeddedResource],
// [IsolateTempDir] and the legacy elicitation client. Every one of them exists
// to abort the caller's test, so reaching it means arranging for a helper to
// fail while the test that called it keeps running, and the only way to do
// that is to route the abort through a package variable a test can replace.
// That would cost these helpers the guarantee they are used for, since a
// Fatalf that no longer aborts leaves the code after it running on the value
// it was refusing, and it would have to be done at every call site to be
// worth anything. The recording and shape files added for the request
// inventory are at 100%, and the seams they use ([createShard],
// [recorderPackage], the requestReporter interface) are the pattern to follow
// if these are ever closed.
package testutil
