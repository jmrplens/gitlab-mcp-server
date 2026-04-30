// helpers_test.go validates the shared test utilities used across all domain
// tool tests. Each helper is exercised directly to ensure correct behavior
// in both success and failure scenarios.
package testutil

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCancelledCtx verifies that CancelledCtx returns a context that is
// already cancelled with context.Canceled error.
func TestCancelledCtx(t *testing.T) {
	ctx := CancelledCtx(t)
	if ctx.Err() != context.Canceled {
		t.Errorf("ctx.Err() = %v, want %v", ctx.Err(), context.Canceled)
	}
}

// TestCaptureSlog verifies that CaptureSlog captures slog output into a
// buffer and that the output contains the expected JSON fields.
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

// TestAssertRequestMethod verifies AssertRequestMethod does not fail the test
// when the expected method matches.
func TestAssertRequestMethod(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	fakeT := &testing.T{}
	AssertRequestMethod(fakeT, r, http.MethodPost)
	if fakeT.Failed() {
		t.Error("AssertRequestMethod should not fail for matching method")
	}
}

// TestAssertRequestMethod_Mismatch verifies AssertRequestMethod marks the
// test as failed when the method does not match.
func TestAssertRequestMethod_Mismatch(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	fakeT := &testing.T{}
	AssertRequestMethod(fakeT, r, http.MethodPost)
	if !fakeT.Failed() {
		t.Error("AssertRequestMethod should fail for mismatched method")
	}
}

// TestAssertRequestPath verifies AssertRequestPath does not fail the test
// when the expected path matches.
func TestAssertRequestPath(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v4/projects", nil)
	fakeT := &testing.T{}
	AssertRequestPath(fakeT, r, "/api/v4/projects")
	if fakeT.Failed() {
		t.Error("AssertRequestPath should not fail for matching path")
	}
}

// TestAssertRequestPath_Mismatch verifies AssertRequestPath marks the test
// as failed when the path does not match.
func TestAssertRequestPath_Mismatch(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v4/projects", nil)
	fakeT := &testing.T{}
	AssertRequestPath(fakeT, r, "/api/v4/issues")
	if !fakeT.Failed() {
		t.Error("AssertRequestPath should fail for mismatched path")
	}
}

// TestAssertQueryParam verifies AssertQueryParam does not fail when the
// query parameter matches.
func TestAssertQueryParam(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?page=2&per_page=20", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "2")
	if fakeT.Failed() {
		t.Error("AssertQueryParam should not fail for matching param")
	}
}

// TestAssertQueryParam_Mismatch verifies AssertQueryParam marks the test
// as failed when the parameter value does not match.
func TestAssertQueryParam_Mismatch(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?page=1", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "2")
	if !fakeT.Failed() {
		t.Error("AssertQueryParam should fail for mismatched value")
	}
}

// TestAssertQueryParam_Missing verifies AssertQueryParam fails when the
// parameter is not present in the URL.
func TestAssertQueryParam_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	fakeT := &testing.T{}
	AssertQueryParam(fakeT, r, "page", "1")
	if !fakeT.Failed() {
		t.Error("AssertQueryParam should fail for missing param")
	}
}

// TestNewTestClient verifies that NewTestClient creates a functional GitLab
// client connected to the mock server. The mock returns a canned response
// to validate the client can make API calls.
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

// TestRespondJSON verifies the JSON response writer sets correct headers,
// status code, and body content.
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

// TestRespondJSONWithPagination verifies that all pagination headers are set
// correctly on the response.
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

// TestRespondJSONWithPagination_PartialHeaders verifies that omitted
// pagination fields do not produce empty headers.
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
