// Package auditclient creates GitLab clients for command-line audit tools.
package auditclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

const (
	mockGitLabVersionResponse = `{"version":"17.0.0"}`
	mockGitLabToken           = "audit-token" // #nosec G101 -- audit-only dummy token.
)

var newGitLabClient = gitlabclient.NewClient

// NewMock returns a GitLab client backed by a local version endpoint.
func NewMock() (*gitlabclient.Client, func(), error) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockGitLabVersionResponse))
	}))

	client, err := newGitLabClient(&config.Config{
		GitLabURL:      server.URL,
		GitLabToken:    mockGitLabToken,
		DisableRetries: true,
	})
	if err != nil {
		server.Close()
		return nil, nil, fmt.Errorf("create mock GitLab client: %w", err)
	}

	return client, server.Close, nil
}
