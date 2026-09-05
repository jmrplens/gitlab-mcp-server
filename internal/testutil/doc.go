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
package testutil
