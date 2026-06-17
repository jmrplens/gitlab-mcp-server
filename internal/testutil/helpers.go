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

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// captureSlogMu serializes [CaptureSlog] calls so concurrent tests do not
// overwrite each other's [slog.Default] handler while capturing output.
var captureSlogMu sync.Mutex

// CancelledCtx returns a [context.Context] that is already cancelled. Use it
// to test handler cancellation paths without dealing with real timers.
//
// The returned context returns [context.Canceled] from [context.Context.Err]
// immediately. The associated cancel function is intentionally dropped —
// callers must not attempt to cancel it again.
func CancelledCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// CaptureSlog redirects [slog] output to an in-memory [bytes.Buffer] for the
// duration of the test. The original [slog.Default] logger is restored via
// [testing.T.Cleanup].
//
// CaptureSlog is NOT safe for [testing.T.Parallel]: it acquires
// [captureSlogMu] for the lifetime of the test and would deadlock with
// another parallel CaptureSlog caller.
func CaptureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	captureSlogMu.Lock()
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(original)
		captureSlogMu.Unlock()
	})
	return &buf
}

// MsgErrEmptyProjectID is the canonical assertion message for tests that
// expect an error when the GitLab tool input omits project_id.
const MsgErrEmptyProjectID = "expected error for empty project_id, got nil"

// AssertRequestMethod fails the test if r.Method does not equal expected.
// It calls [testing.T.Helper] so failure lines point at the caller.
func AssertRequestMethod(t *testing.T, r *http.Request, expected string) {
	t.Helper()
	if r.Method != expected {
		t.Errorf("HTTP method = %q, want %q (path: %s)", r.Method, expected, r.URL.Path)
	}
}

// AssertRequestPath fails the test if r.URL.Path does not equal expected.
// It calls [testing.T.Helper] so failure lines point at the caller.
func AssertRequestPath(t *testing.T, r *http.Request, expected string) {
	t.Helper()
	if r.URL.Path != expected {
		t.Errorf("URL path = %q, want %q", r.URL.Path, expected)
	}
}

// AssertQueryParam fails the test if the URL query parameter key does not
// equal expected. Missing parameters are reported as a mismatch with an
// empty actual value.
func AssertQueryParam(t *testing.T, r *http.Request, key, expected string) {
	t.Helper()
	got := r.URL.Query().Get(key)
	if got != expected {
		t.Errorf("query param %q = %q, want %q (path: %s)", key, got, expected, r.URL.Path)
	}
}

// NewTestClient creates a [gitlabclient.Client] pointed at a fresh
// [httptest.Server] backed by handler. The server is automatically torn down
// when the test finishes via [testing.T.Cleanup], so callers never need to
// manage its lifecycle.
//
// The client uses a static token ("test-token"), TLS verification disabled,
// and retries disabled — sufficient for most handler unit tests but not for
// exercising the transport retry policy. NewTestClient calls
// [testing.T.Helper] and [testing.T.Fatalf] if client construction fails.
//
// The returned client is safe for concurrent use; the httptest.Server that
// backs it is goroutine-safe by construction.
func NewTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		GitLabURL:      srv.URL,
		GitLabToken:    "test-token",
		SkipTLSVerify:  false,
		DisableRetries: true,
	}

	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test gitlab client: %v", err)
	}

	return client
}

// RespondJSON writes a JSON response with the given HTTP status and raw body.
// It sets Content-Type to "application/json". body is written verbatim —
// callers are responsible for producing valid JSON. Write errors are
// intentionally ignored because the only writer in tests is an
// [httptest.ResponseRecorder], which never fails.
func RespondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// PaginationHeaders is the set of GitLab pagination response headers
// returned with list endpoints. Empty fields are omitted from the response so
// handlers can populate only the headers a given scenario exercises.
type PaginationHeaders struct {
	Page       string // X-Page — current page number.
	PerPage    string // X-Per-Page — items per page in this response.
	Total      string // X-Total — total items across all pages.
	TotalPages string // X-Total-Pages — total page count.
	NextPage   string // X-Next-Page — next page number, if any.
	PrevPage   string // X-Prev-Page — previous page number, if any.
}

// RespondJSONWithPagination writes a JSON response with GitLab pagination
// headers attached. Headers whose [PaginationHeaders] field is empty are
// omitted, matching GitLab's behavior on pages without a next/previous
// pointer.
func RespondJSONWithPagination(w http.ResponseWriter, status int, body string, p PaginationHeaders) {
	w.Header().Set("Content-Type", "application/json")
	if p.Page != "" {
		w.Header().Set("X-Page", p.Page)
	}
	if p.PerPage != "" {
		w.Header().Set("X-Per-Page", p.PerPage)
	}
	if p.Total != "" {
		w.Header().Set("X-Total", p.Total)
	}
	if p.TotalPages != "" {
		w.Header().Set("X-Total-Pages", p.TotalPages)
	}
	if p.NextPage != "" {
		w.Header().Set("X-Next-Page", p.NextPage)
	}
	if p.PrevPage != "" {
		w.Header().Set("X-Prev-Page", p.PrevPage)
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
