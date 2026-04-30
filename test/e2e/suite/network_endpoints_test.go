//go:build e2e

// network_endpoints_test.go centralizes deterministic URLs used by Docker E2E
// tests for webhook, custom emoji, and remote mirror operations.
package suite

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

// Default Docker network endpoints used when setup-gitlab.sh has not written
// explicit E2E fixture settings into the environment.
const (
	defaultE2EFixtureURL        = "http://e2e-fixture:8080"
	defaultE2EGitLabInternalURL = "http://gitlab-e2e"
)

// e2eFixtureServiceURL returns a URL on the Docker-local fixture service for
// resourcePath, falling back to the default service name when unset.
func e2eFixtureServiceURL(resourcePath string) string {
	baseURL := os.Getenv("E2E_FIXTURE_URL")
	if baseURL == "" {
		baseURL = defaultE2EFixtureURL
	}
	return joinE2EURL(baseURL, resourcePath)
}

// remoteMirrorTargetURL returns an authenticated Git URL for target that GitLab
// can reach from inside the Docker network when configuring push mirrors.
func remoteMirrorTargetURL(t *testing.T, target ProjectFixture) string {
	t.Helper()

	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, token != "", "GITLAB_TOKEN is required to configure a remote mirror target")

	baseURL := os.Getenv("E2E_GITLAB_INTERNAL_URL")
	if baseURL == "" {
		baseURL = os.Getenv("GITLAB_URL")
	}
	if baseURL == "" {
		baseURL = defaultE2EGitLabInternalURL
	}

	parsedURL, err := url.Parse(baseURL)
	requireNoError(t, err, "parse internal GitLab URL")
	parsedURL.User = url.UserPassword("oauth2", token)
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + "/" + strings.TrimLeft(target.Path, "/") + ".git"
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""

	return parsedURL.String()
}

// joinE2EURL joins baseURL and resourcePath without preserving duplicate slash
// boundaries from either side.
func joinE2EURL(baseURL, resourcePath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resourcePath, "/")
}
