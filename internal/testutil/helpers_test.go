// helpers_test.go validates the shared test utilities used across all domain
// tool tests. Each helper is exercised directly to ensure correct behavior
// in both success and failure scenarios. Tests use a sentinel [*testing.T]
// passed to assertion helpers so we can inspect [testing.T.Failed] without
// failing the surrounding test.
package testutil

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
