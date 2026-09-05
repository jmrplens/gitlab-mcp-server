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

// NewMock returns a GitLab client backed by a local version endpoint, and the
// function that shuts that endpoint down.
//
// It does not return an error, and panics instead if the client cannot be
// built. The only input is a URL httptest allocated a line earlier and a fixed
// token, so a failure here is a programming error in this package rather than
// a condition a caller could handle: every caller could only print it and
// stop, which is what a panic already does, and nine of them were carrying an
// unreachable branch to say so. This is the standard-library Must convention
// (regexp.MustCompile, template.Must) applied to a fixture constructor used
// only by the audit commands.
func NewMock() (client *gitlabclient.Client, closeServer func()) {
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
		panic(fmt.Sprintf("auditclient: create mock GitLab client: %v", err))
	}

	return client, server.Close
}
