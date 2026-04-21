// Package testutil provides shared test utilities for MCP tool tests.
// It includes a test GitLab client factory, JSON response helpers, and
// pagination header utilities used across all domain tool test files.
package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// CancelledCtx returns a pre-cancelled context for testing cancellation handling.
func CancelledCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// MsgErrEmptyProjectID is the shared assertion message for tests that expect
// an error when project_id is empty.
const MsgErrEmptyProjectID = "expected error for empty project_id, got nil"

// AssertRequestMethod fails the test if the HTTP method does not match expected.
func AssertRequestMethod(t *testing.T, r *http.Request, expected string) {
	t.Helper()
	if r.Method != expected {
		t.Errorf("HTTP method = %q, want %q (path: %s)", r.Method, expected, r.URL.Path)
	}
}

// AssertRequestPath fails the test if the URL path does not match expected.
func AssertRequestPath(t *testing.T, r *http.Request, expected string) {
	t.Helper()
	if r.URL.Path != expected {
		t.Errorf("URL path = %q, want %q", r.URL.Path, expected)
	}
}

// AssertQueryParam fails the test if query parameter key does not equal expected.
func AssertQueryParam(t *testing.T, r *http.Request, key, expected string) {
	t.Helper()
	got := r.URL.Query().Get(key)
	if got != expected {
		t.Errorf("query param %q = %q, want %q (path: %s)", key, got, expected, r.URL.Path)
	}
}

// NewTestClient creates a GitLab client pointed at a test HTTP server.
func NewTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		GitLabURL:     srv.URL,
		GitLabToken:   "test-token",
		SkipTLSVerify: false,
	}

	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test gitlab client: %v", err)
	}

	return client
}

// RespondJSON writes a JSON response with the given status code and body.
func RespondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// PaginationHeaders holds GitLab pagination header values for test mocks.
type PaginationHeaders struct {
	Page       string
	PerPage    string
	Total      string
	TotalPages string
	NextPage   string
	PrevPage   string
}

// RespondJSONWithPagination writes a JSON response with GitLab pagination headers.
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
