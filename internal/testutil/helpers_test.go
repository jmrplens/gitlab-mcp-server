// helpers_test.go validates the shared test utilities used across all domain
// tool tests. Each helper is exercised directly to ensure correct behavior
// in both success and failure scenarios. Tests use a sentinel [*testing.T]
// passed to assertion helpers so we can inspect [testing.T.Failed] without
// failing the surrounding test.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestCancelledCtx verifies that [CancelledCtx] returns a context whose
// [context.Context.Err] is already [context.Canceled]. The test makes no
// assertions on the cancel function (which is intentionally discarded) and
// focuses on the immediate-cancellation contract used by handler
// cancellation paths.
func TestCancelledCtx(t *testing.T) {
	ctx := CancelledCtx(t)
	if ctx.Err() != context.Canceled {
		t.Errorf("ctx.Err() = %v, want %v", ctx.Err(), context.Canceled)
	}
}

// TestCaptureSlog verifies that [CaptureSlog] installs a JSON [slog.Handler]
// whose output is captured in the returned [bytes.Buffer], then restores the
// original default logger on test exit.
//
// The test logs a single Info entry with a custom key and asserts the buffer
// contains the message, the custom key/value pair, and the INFO level
// marker. This protects the JSON-shape contract relied on by other tests
// that assert on structured log fields.
func TestCaptureSlog(t *testing.T) {
	buf := CaptureSlog(t)
	slog.Info("test message", "key", "val")
	out := buf.String()
	for _, want := range []string{`"msg":"test message"`, `"key":"val"`, `"level":"INFO"`} {
		if !strings.Contains(out, want) {
			t.Errorf("slog output missing %q, got: %s", want, out)
		}
	}
}

// TestAssertRequestMethod verifies [AssertRequestMethod] does not fail the
// test when the request method matches the expected value. The test uses a
// sentinel [*testing.T] and inspects [testing.T.Failed] directly so a real
// failure inside the helper would be observable.
func TestAssertRequestMethod(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test", nil)
	fakeT := &testing.T{}
	AssertRequestMethod(fakeT, r, http.MethodPost)
	if fakeT.Failed() {
		t.Error("AssertRequestMethod should not fail for matching method")
	}
}

// TestAssertRequestMethod_Mismatch verifies [AssertRequestMethod] marks the
// test as failed when the request method does not match the expected value.
// The test wires a GET request, asks the helper to expect POST, and asserts
// the sentinel [*testing.T] is now failed.
func TestAssertRequestMethod_Mismatch(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	fakeT := &testing.T{}
	AssertRequestMethod(fakeT, r, http.MethodPost)
	if !fakeT.Failed() {
		t.Error("AssertRequestMethod should fail for mismatched method")
	}
}

// TestAssertRequestPath verifies [AssertRequestPath] does not fail when the
// request URL path matches the expected value. The sentinel [*testing.T]
// pattern keeps the helper from failing the surrounding test, allowing us
// to inspect [testing.T.Failed] instead.
func TestAssertRequestPath(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v4/projects", nil)
	fakeT := &testing.T{}
	AssertRequestPath(fakeT, r, "/api/v4/projects")
	if fakeT.Failed() {
		t.Error("AssertRequestPath should not fail for matching path")
	}
}

// TestAssertRequestPath_Mismatch verifies [AssertRequestPath] marks the test
// as failed when the request URL path does not match the expected value.
// The test asks the helper to expect "/api/v4/issues" against a request
// that targets "/api/v4/projects".
func TestAssertRequestPath_Mismatch(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v4/projects", nil)
	fakeT := &testing.T{}
	AssertRequestPath(fakeT, r, "/api/v4/issues")
	if !fakeT.Failed() {
		t.Error("AssertRequestPath should fail for mismatched path")
	}
}

// TestAssertQueryParam verifies [AssertQueryParam] does not fail when the
// query parameter value matches the expected string. The sentinel
// [*testing.T] pattern lets the test observe [testing.T.Failed] directly
// while leaving the surrounding test green on success.
func TestAssertQueryParam(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?page=2&per_page=20", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "2")
	if fakeT.Failed() {
		t.Error("AssertQueryParam should not fail for matching param")
	}
}

// TestAssertQueryParam_Mismatch verifies [AssertQueryParam] marks the test
// as failed when the URL query parameter value disagrees with the expected
// string. The test posts a "page=1" request and asks the helper to expect
// "page=2".
func TestAssertQueryParam_Mismatch(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?page=1", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "2")
	if !fakeT.Failed() {
		t.Error("AssertQueryParam should fail for mismatched value")
	}
}

// TestAssertQueryParam_Missing verifies [AssertQueryParam] marks the test
// as failed when the requested query parameter is absent from the URL.
// The test posts a request with no query string and asks the helper to
// expect a value for "page".
func TestAssertQueryParam_Missing(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "1")
	if !fakeT.Failed() {
		t.Error("AssertQueryParam should fail for missing param")
	}
}

// TestNewTestClient verifies that [NewTestClient] returns a non-nil
// [gitlabclient.Client] backed by a live mock server.
//
// The mock handler responds to the GitLab version endpoint with a canned
// {"version":"17.0.0","revision":"abc"} payload so the helper has no reason
// to abort. The test asserts both the outer client and the underlying
// client-go handle ([gitlabclient.Client.GL]) are non-nil, protecting the
// factory contract relied on by every tool test.
func TestNewTestClient(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc"}`))
	})

	client := NewTestClient(t, handler)
	if client == nil {
		t.Fatal("NewTestClient returned nil")
	}
	if client.GL() == nil {
		t.Fatal("NewTestClient.GL() returned nil")
	}
}

// TestRespondJSON verifies that [RespondJSON] writes the supplied status
// code, sets Content-Type to "application/json", and writes the body
// verbatim.
//
// The test uses [httptest.NewRecorder] so it can assert on the captured
// status, headers, and body bytes. It guards the response-shape contract
// that every mock handler in the project depends on.
func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	RespondJSON(w, http.StatusCreated, `{"id":42}`)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if body := w.Body.String(); body != `{"id":42}` {
		t.Errorf("body = %q, want %q", body, `{"id":42}`)
	}
}

// TestRespondJSONWithPagination verifies that [RespondJSONWithPagination]
// sets every GitLab pagination header in the supplied [PaginationHeaders]
// struct.
//
// The test populates all six fields and asserts the response contains each
// X-Page, X-Per-Page, X-Total, X-Total-Pages, X-Next-Page, and
// X-Prev-Page header with the expected value. This protects the pagination
// contract that list-tool tests rely on when validating handler output.
func TestRespondJSONWithPagination(t *testing.T) {
	w := httptest.NewRecorder()
	p := PaginationHeaders{
		Page:       "2",
		PerPage:    "20",
		Total:      "100",
		TotalPages: "5",
		NextPage:   "3",
		PrevPage:   "1",
	}
	RespondJSONWithPagination(w, http.StatusOK, `[{"id":1}]`, p)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	checks := map[string]string{
		"X-Page":        "2",
		"X-Per-Page":    "20",
		"X-Total":       "100",
		"X-Total-Pages": "5",
		"X-Next-Page":   "3",
		"X-Prev-Page":   "1",
	}
	for header, want := range checks {
		got := w.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
}

// TestRespondJSONWithPagination_PartialHeaders verifies that
// [RespondJSONWithPagination] omits headers whose [PaginationHeaders]
// field is the empty string. The test sets only Page and PerPage, then
// asserts X-Page is set while X-Total, X-Total-Pages, X-Next-Page, and
// X-Prev-Page remain absent. This guards the contract that handlers can
// populate only the headers a given scenario exercises without leaking
// empty values into the response.
func TestRespondJSONWithPagination_PartialHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	p := PaginationHeaders{
		Page:    "1",
		PerPage: "20",
	}
	RespondJSONWithPagination(w, http.StatusOK, `[]`, p)

	if w.Header().Get("X-Page") != "1" {
		t.Errorf("X-Page = %q, want %q", w.Header().Get("X-Page"), "1")
	}
	// Omitted headers should not be set.
	for _, header := range []string{"X-Total", "X-Total-Pages", "X-Next-Page", "X-Prev-Page"} {
		if got := w.Header().Get(header); got != "" {
			t.Errorf("header %s = %q, want empty (not set)", header, got)
		}
	}
}

// TestForbiddenHandler_NoCallsPasses verifies the zero-request path: arming
// the handler without driving any request must leave the test green when the
// cleanup assertion runs.
func TestForbiddenHandler_NoCallsPasses(t *testing.T) {
	_ = ForbiddenHandler(t)
}

// TestForbiddenHandlerCore_CountsAndRespondsWithError verifies the response
// half: each request increments the counter and receives a deterministic 500
// naming the unexpected method and path.
func TestForbiddenHandlerCore_CountsAndRespondsWithError(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(forbiddenHandlerCore(&hits))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/should/not/happen", http.NoBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(string(body), "GET /should/not/happen") {
		t.Errorf("body = %q, want the unexpected method and path named", body)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("hits = %d, want 1", n)
	}
}

// errorfRecorder records Errorf calls for asserting on failure-path helpers
// without failing the running test.
type errorfRecorder struct {
	messages []string
}

// Errorf implements the errorReporter seam by recording the formatted message.
func (r *errorfRecorder) Errorf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// TestReportForbiddenHits_NonZeroFails verifies the failure branch: a non-zero
// hit count must produce exactly one Errorf naming the count.
func TestReportForbiddenHits_NonZeroFails(t *testing.T) {
	rec := &errorfRecorder{}
	reportForbiddenHits(rec, 3)
	if len(rec.messages) != 1 {
		t.Fatalf("Errorf calls = %d, want 1", len(rec.messages))
	}
	if !strings.Contains(rec.messages[0], "3 time(s)") {
		t.Errorf("message = %q, want the hit count named", rec.messages[0])
	}
}

// TestReportForbiddenHits_ZeroIsQuiet verifies the expected path: zero hits
// must not report anything.
func TestReportForbiddenHits_ZeroIsQuiet(t *testing.T) {
	rec := &errorfRecorder{}
	reportForbiddenHits(rec, 0)
	if len(rec.messages) != 0 {
		t.Fatalf("Errorf calls = %d, want 0: %v", len(rec.messages), rec.messages)
	}
}

// fatalRecorder embeds a real testing.TB and overrides Fatalf so the
// construction-failure branch of NewTestClient can be observed instead of
// aborting the running test.
type fatalRecorder struct {
	testing.TB
	fatals []string
}

// Fatalf implements the override by recording the formatted message and
// returning, unlike the real implementation which never returns.
func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

// TestNewTestClient_ConstructionFailureFatals verifies that NewTestClient
// reports a fatal error when the underlying GitLab client constructor fails.
// The constructor seam is stubbed because a live httptest URL can never make
// the real constructor fail.
func TestNewTestClient_ConstructionFailureFatals(t *testing.T) {
	orig := newGitLabClient
	newGitLabClient = func(*config.Config) (*gitlabclient.Client, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newGitLabClient = orig })

	rec := &fatalRecorder{TB: t}
	client := NewTestClient(rec, http.NotFoundHandler())

	if len(rec.fatals) != 1 {
		t.Fatalf("Fatalf calls = %d, want 1: %v", len(rec.fatals), rec.fatals)
	}
	if !strings.Contains(rec.fatals[0], "boom") {
		t.Errorf("fatal message = %q, want the constructor error named", rec.fatals[0])
	}
	if client != nil {
		t.Errorf("client = %v, want nil after recorded fatal", client)
	}
}
