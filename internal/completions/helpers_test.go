// helpers_test.go provides shared test utilities for the completions package,
// including a mock GitLab client constructor and a JSON response writer.

package completions

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// Shared test assertion message for expected value counts.
const fmtExpected1Value = "expected 1 value, got %d"

// newTestClient creates a GitLab client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
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

// respondJSON writes a JSON response with the given status code and body.
func respondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
