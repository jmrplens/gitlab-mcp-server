//go:build e2e

package suite

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

const (
	defaultE2EFixtureURL        = "http://e2e-fixture:8080"
	defaultE2EGitLabInternalURL = "http://gitlab-e2e"
)

func e2eFixtureServiceURL(resourcePath string) string {
	baseURL := os.Getenv("E2E_FIXTURE_URL")
	if baseURL == "" {
		baseURL = defaultE2EFixtureURL
	}
	return joinE2EURL(baseURL, resourcePath)
}

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

func joinE2EURL(baseURL, resourcePath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resourcePath, "/")
}
