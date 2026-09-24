package testutil

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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

// CancelOnArrival is the fixture for the one thing a handler's cancellation
// test cannot learn from a context cancelled up front: whether the request the
// handler builds carries the caller's context at all.
//
// client-go takes that context only as a request option, gl.WithContext, and
// builds the request from context.Background() without it. A handler that
// forgets the option compiles, passes every other test and serves correct
// answers, while neither the action deadline nor an abandoned HTTP POST ever
// reaches the request it sends. A context that is already cancelled cannot
// tell the two apart in a handler that checks ctx.Err() before it builds
// anything, as the handlers this fixture was written for do: that handler
// returns before any request exists, whether or not it would have passed the
// context on.
//
// The context returned here is cancelled the moment the first request reaches
// the mock, and the mock holds every request until the client abandons it or
// five seconds pass, answering through respond only in the second case. A
// handler that passed the context therefore returns promptly with an error for
// which errors.Is(err, context.Canceled) holds; one that did not waits out the
// hold and returns whatever respond answered, which a test asserting the
// cancellation reports.
//
// The cancel is made through a [sync.Once] on the handler's own goroutine
// rather than by a watcher goroutine, since a CancelFunc may be called from
// anywhere and a goroutine that waited for the arrival would be one more thing
// a test that never sends a request could leak.
func CancelOnArrival(tb testing.TB, respond http.HandlerFunc) (context.Context, *gitlabclient.Client) {
	tb.Helper()
	// Five seconds is long enough that a request carrying the cancelled
	// context is always abandoned first, since that takes one round trip on
	// loopback, and short enough that a handler which dropped the context
	// fails its test in seconds rather than hanging it. It is written here
	// rather than as a constant so the one statement that states it is one a
	// test executes.
	return cancelOnArrival(tb, respond, 5*time.Second)
}

// cancelOnArrival is [CancelOnArrival] with the hold stated, so the helper's
// own test can reach the answer respond gives without waiting out the full
// hold. It is a parameter rather than a package variable a test overrides,
// because such a variable would be shared by every test in the package and a
// shortened hold would leak into a parallel one.
func cancelOnArrival(tb testing.TB, respond http.HandlerFunc, hold time.Duration) (context.Context, *gitlabclient.Client) {
	tb.Helper()
	ctx, cancel := context.WithCancel(tb.Context())
	tb.Cleanup(cancel)
	var arrived sync.Once
	client := NewTestClient(tb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived.Do(cancel)
		// The body is drained before the wait because the server only watches
		// the connection once the body has been read to its end: until then a
		// client that went away leaves the request's context live, an upload
		// would sit out the whole hold, and the test would pay for it in its
		// cleanup, where the server waits for this handler. It is put back
		// for respond, which may want to read it.
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		timer := time.NewTimer(hold)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
			respond(w, r)
		}
	}))
	return ctx, client
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
// Every GraphQL document sent through the returned client is validated against
// the pinned GitLab schema before handler sees it, so a mock can no longer
// answer what GitLab would refuse. The request proceeds either way and a
// refusal is reported with [testing.TB.Errorf]; [AllowInvalidGraphQL] is the
// opt-out for a test that sends a malformed document on purpose.
//
// That validation is the reason to build a test's client here rather than
// wiring [httptest.NewServer] to gitlabclient.NewClient by hand. A handful of
// tests still do the latter, none of them sending GraphQL today, and any
// GraphQL written against one of those seams would be judged by nobody, which
// is the state this helper exists to end. The same goes for the recording
// below: a client built by hand is a request nothing observes.
//
// Every request is also recorded, when [InventoryDirEnv] names a directory to
// record into, so the suite can answer what this server actually sends GitLab.
// Recording is off in an ordinary run.
//
// The returned client is safe for concurrent use; the httptest.Server that
// backs it is goroutine-safe by construction.
func NewTestClient(tb testing.TB, handler http.Handler) *gitlabclient.Client {
	tb.Helper()

	// The two wrappers read the request body separately, and each puts it
	// back, rather than sharing one read. They answer different questions and
	// one of them is a gate, so keeping them independent means neither can
	// break the other; a body a test sends is small enough that reading it
	// twice costs nothing worth this coupling.
	srv := httptest.NewServer(recordingHandler(tb, validatingHandler(tb, handler)))
	tb.Cleanup(srv.Close)

	cfg := &config.Config{
		GitLabURL:      srv.URL,
		GitLabToken:    "test-token",
		SkipTLSVerify:  false,
		DisableRetries: true,
	}

	client, err := newGitLabClient(cfg)
	if err != nil {
		tb.Fatalf("failed to create test gitlab client: %v", err)
	}

	return client
}

// newGitLabClient is the client constructor used by [NewTestClient]. It is a
// package variable so the construction-failure branch is testable: srv.URL is
// always a valid base URL, so [gitlabclient.NewClient] cannot be made to fail
// from outside.
var newGitLabClient = gitlabclient.NewClient

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

// ForbiddenHandler returns an [http.Handler] for mocks that must never be
// called. Each request is counted atomically and answered with a
// deterministic 500 so the client under test fails loudly, and a
// [testing.T.Cleanup] hook asserts on the test goroutine that no request
// arrived. This is the sanctioned replacement for `t.Fatal("should not be
// called")` inside handler literals, which the testing package forbids off
// the test goroutine (see .github/instructions/test-goroutines.instructions.md).
func ForbiddenHandler(t *testing.T) http.Handler {
	t.Helper()
	var hits atomic.Int64
	t.Cleanup(func() { reportForbiddenHits(t, hits.Load()) })
	return forbiddenHandlerCore(&hits)
}

// errorReporter is the subset of [testing.TB] that reportForbiddenHits needs,
// kept as an interface so the non-zero branch is testable with a recorder
// instead of failing the calling test.
type errorReporter interface {
	Errorf(format string, args ...any)
}

// reportForbiddenHits fails the test when a [ForbiddenHandler] mock received
// any request. Zero hits is the expected quiet path.
func reportForbiddenHits(r errorReporter, n int64) {
	if n != 0 {
		r.Errorf("mock API was called %d time(s); the test forbids any request", n)
	}
}

// forbiddenHandlerCore is the response half of [ForbiddenHandler], split out
// so tests can exercise the counting and the deterministic 500 without
// arming the cleanup assertion.
func forbiddenHandlerCore(hits *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "unexpected call: "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})
}
